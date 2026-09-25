package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gamepanel/forge/internal/services/deployment"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// User-facing deployment history and one-click rollback to any past version.
//
// # Why these routes are not at /servers/:id/deployments
//
// That prefix is already taken by the zero-downtime *release* surface:
// handlers_zerodowntime.go registers GET/POST /servers/:id/deployments and
// /servers/:id/deployments/:releaseId{,/rollback,/health,/promote,/events}.
// Fiber matches in registration order, so a second
// /deployments/:deploymentId/rollback under the same prefix would silently
// shadow theirs (or be shadowed by theirs) depending on which registrar ran
// first. /deployment-versions names the same idea without colliding: one row
// per version the workload has actually been asked to run, which is exactly
// what a rollback targets. Both scopes are offered, /apps/:appId and
// /servers/:serverId.
//
// # Layering
//
// handler -> deployment.Service -> store. Nothing here re-implements a deploy.
// "Put this older image back on the node" is deployment.Service.StartRollout;
// reading history is ListDeployments / GetDeployment / ListRevisions /
// ListSteps / CompareRevisions. The handler only writes SQL against
// deployment_rollbacks (the audit table this feature owns, migration 227) and
// the batched revision enrichment for one page of history; neither has a
// service method, and adding one would mean editing files this feature does
// not own.
//
// # What a rollback record means
//
// deployment_rollbacks keeps the facts a deployments row cannot: which version
// was chosen, which version was live when it was chosen, who asked, why, and
// the new deployment row created to carry it out. The mirror row is created by
// StartRollout with strategy 'recreate' and then relabelled 'rollback' — see
// the comment at that call site for why that relabel is race-free.

const (
	deploymentRollbackRegistrarName = "deployment-rollback-history"
	deploymentRollbackRegistrarPrio = 230
	deploymentVersionSegment        = "deployment-versions"
	deploymentRollbackSegment       = "deployment-rollbacks"

	// deploymentRollbackAuditLimit caps how many rollback records one page of
	// history is enriched with. Rollbacks are rare next to deployments, so this
	// is a ceiling, not an expected working set.
	deploymentRollbackAuditLimit  = 500
	deploymentRollbackMaxPageSize = 100
)

func init() {
	RegisterPhaseRegistrar(deploymentRollbackRegistrarName, deploymentRollbackRegistrarPrio, registerDeploymentRollbackRoutes)
}

func registerDeploymentRollbackRoutes(_ fiber.Router, protected fiber.Router, cfg *Config) error {
	if cfg.Store == nil || cfg.Store.DB() == nil || cfg.DeploymentSvc == nil {
		if cfg.Logger != nil {
			cfg.Logger.Warn(deploymentRollbackRegistrarName + ": store or deployment service not configured, rollback history routes skipped")
		}
		return nil
	}
	limiter := RateLimiter(GetRateLimitForEndpoint("mutation", cfg.Redis,
		cfg.RedisEnabled && strings.EqualFold(strings.TrimSpace(cfg.AppEnv), "production")))
	registerDeploymentRollbackRoutesOn(protected, *cfg, limiter)
	return nil
}

func registerDeploymentRollbackRoutesOn(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	// Registered per concrete scope so the handler never mounts a root wildcard
	// that could shadow an existing static route.
	for _, prefix := range []string{"/apps/:appId", "/servers/:serverId"} {
		versions := prefix + "/" + deploymentVersionSegment
		rollbacks := prefix + "/" + deploymentRollbackSegment

		protected.Get(versions, listDeploymentVersionsHandler(cfg))
		protected.Get(versions+"/:deploymentId", getDeploymentVersionHandler(cfg))
		protected.Post(versions+"/:deploymentId/rollback", mutationLimiter, rollbackDeploymentVersionHandler(cfg))
		protected.Get(versions+"/:deploymentId/diff", diffDeploymentVersionHandler(cfg))

		protected.Get(rollbacks, listDeploymentRollbacksHandler(cfg))
		protected.Get(rollbacks+"/:rollbackId", getDeploymentRollbackHandler(cfg))
	}
}

// ---------------------------------------------------------------------------
// authorization / scope resolution
// ---------------------------------------------------------------------------

// deploymentRollbackScope is the workload a history request is about. App
// scoped requests carry the application as well so the audit trail records
// which app the operator was looking at.
type deploymentRollbackScope struct {
	serverID      string
	applicationID *string
}

// resolveDeploymentRollbackScope authenticates the caller against the workload
// named in the path. Reads need server access (or org membership plus server
// access for the app scope); rollbacks additionally need the permission that
// lets the caller change what is installed, which is the same gate the web UI
// uses for its deploy controls.
//
// Non-members get 404 rather than 403 on the app scope: an application the
// caller cannot see must not be distinguishable from one that does not exist.
func resolveDeploymentRollbackScope(c *fiber.Ctx, cfg Config, write bool) (deploymentRollbackScope, error) {
	claims, ok := c.Locals("user").(tokenClaims)
	if !ok {
		return deploymentRollbackScope{}, fiber.NewError(fiber.StatusUnauthorized, "missing session")
	}
	permission := ""
	if write {
		permission = store.PermSettingsReinstall
	}

	ctx, cancel := requestContext()
	defer cancel()

	if appID := strings.TrimSpace(c.Params("appId")); appID != "" {
		app, err := cfg.Store.GetApplication(ctx, appID)
		if err != nil || app == nil {
			return deploymentRollbackScope{}, fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, merr := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if merr != nil {
				return deploymentRollbackScope{}, respondInternalError(c, fmt.Errorf("check organization membership: %w", merr))
			}
			if !isMember {
				return deploymentRollbackScope{}, fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		if app.ServerID == nil || strings.TrimSpace(*app.ServerID) == "" {
			return deploymentRollbackScope{}, fiber.NewError(fiber.StatusConflict, "application has no server bound to it")
		}
		serverID := strings.TrimSpace(*app.ServerID)
		allowed, aerr := cfg.Store.UserCanAccessServer(ctx, serverID, claims.Sub, claims.Role, permission)
		if aerr != nil {
			return deploymentRollbackScope{}, respondInternalError(c, fmt.Errorf("check server access: %w", aerr))
		}
		if !allowed {
			return deploymentRollbackScope{}, deploymentRollbackForbidden(permission)
		}
		applicationID := app.ID
		return deploymentRollbackScope{serverID: serverID, applicationID: &applicationID}, nil
	}

	if serverID := strings.TrimSpace(c.Params("serverId")); serverID != "" {
		allowed, aerr := cfg.Store.UserCanAccessServer(ctx, serverID, claims.Sub, claims.Role, permission)
		if aerr != nil {
			return deploymentRollbackScope{}, respondInternalError(c, fmt.Errorf("check server access: %w", aerr))
		}
		if !allowed {
			return deploymentRollbackScope{}, deploymentRollbackForbidden(permission)
		}
		return deploymentRollbackScope{serverID: serverID}, nil
	}

	return deploymentRollbackScope{}, fiber.NewError(fiber.StatusNotFound, "deployment history scope not found")
}

func deploymentRollbackForbidden(permission string) error {
	if permission == "" {
		return fiber.NewError(fiber.StatusForbidden, "server access is not assigned to this user")
	}
	return fiber.NewError(fiber.StatusForbidden, "missing server permission: "+permission)
}

// ---------------------------------------------------------------------------
// response shapes
// ---------------------------------------------------------------------------

// deploymentVersionView is one entry of the browsable history. The embedded
// deployment carries the fields the executor owns (status, image, strategy,
// timestamps, progress); the rest is what an operator needs to decide whether
// to roll back to it.
type deploymentVersionView struct {
	deployment.Deployment

	// ApplicationID is set when the history was read through the /apps scope,
	// so the caller can tell which application the rows belonged to.
	ApplicationID      *string `json:"applicationId,omitempty"`
	RevisionNumber     int     `json:"revisionNumber,omitempty"`
	CommitSha          string  `json:"commitSha,omitempty"`
	ComposeManifestRef string  `json:"composeManifestRef,omitempty"`
	ConfigHash         string  `json:"configHash,omitempty"`
	Description        string  `json:"description,omitempty"`
	// CanRollbackTo is computed server-side so the UI does not have to
	// re-derive the rules (and cannot offer a button the API would refuse).
	CanRollbackTo bool `json:"canRollbackTo"`
	IsLive        bool `json:"isLive"`
	// Rollback records why this version exists: set when it was created to
	// carry out an explicit rollback.
	Rollback *deploymentRollbackRecord `json:"rollback,omitempty"`
	// RolledBackTo lists the rollbacks that chose this version, so history can
	// show "this version was restored on <date>" without a second request.
	RolledBackTo []deploymentRollbackSummary `json:"rolledBackTo,omitempty"`
}

type deploymentRollbackSummary struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type deploymentRollbackRecord struct {
	ID                      string         `json:"id"`
	DeploymentID            string         `json:"deploymentId"`
	TriggeredByDeploymentID *string        `json:"triggeredByDeploymentId,omitempty"`
	RollbackDeploymentID    *string        `json:"rollbackDeploymentId,omitempty"`
	ApplicationID           *string        `json:"applicationId,omitempty"`
	ServerID                string         `json:"serverId"`
	InitiatedBy             *string        `json:"initiatedBy,omitempty"`
	Status                  string         `json:"status"`
	Reason                  string         `json:"reason,omitempty"`
	Snapshot                map[string]any `json:"snapshot,omitempty"`
	CreatedAt               time.Time      `json:"createdAt"`
	CompletedAt             *time.Time     `json:"completedAt,omitempty"`
}

// deploymentVersionDetail adds the compose/env snapshot and step list a
// drill-down needs. Snapshots come from the version's own revision row: that
// is the only place compose refs and commit SHAs are recorded per version.
type deploymentVersionDetail struct {
	deploymentVersionView

	Steps           []*deployment.DeploymentStep `json:"steps"`
	Revisions       []*deployment.Revision       `json:"revisions"`
	ComposeSnapshot map[string]any               `json:"composeSnapshot,omitempty"`
	Environment     map[string]any               `json:"environment,omitempty"`
	Rollbacks       []deploymentRollbackRecord   `json:"rollbacks"`
}

type deploymentVersionDiff struct {
	From    deploymentVersionSide       `json:"from"`
	To      deploymentVersionSide       `json:"to"`
	Changes []deployment.RevisionChange `json:"changes"`
	EnvDiff []deployment.RevisionChange `json:"envDiff"`
	// Notes keeps the diff honest: it says so when a side had no revision row
	// and the comparison therefore covers less than it appears to.
	Notes []string `json:"notes,omitempty"`
}

type deploymentVersionSide struct {
	DeploymentID       string         `json:"deploymentId"`
	Status             string         `json:"status"`
	Image              string         `json:"image"`
	Strategy           string         `json:"strategy"`
	CommitSha          string         `json:"commitSha,omitempty"`
	ComposeManifestRef string         `json:"composeManifestRef,omitempty"`
	RevisionNumber     int            `json:"revisionNumber,omitempty"`
	CreatedAt          time.Time      `json:"createdAt"`
	CompletedAt        *time.Time     `json:"completedAt,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
}

// ---------------------------------------------------------------------------
// history list
// ---------------------------------------------------------------------------

func listDeploymentVersionsHandler(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		scope, err := resolveDeploymentRollbackScope(c, cfg, false)
		if err != nil {
			return err
		}
		page, perPage, perr := parseDeploymentRollbackPaging(c)
		if perr != nil {
			return perr
		}
		statusFilter := strings.ToLower(strings.TrimSpace(c.Query("status")))

		ctx, cancel := requestContext()
		defer cancel()

		// The audit rows are reconciled before being read: a rollback whose
		// mirror deployment has finished still says "in_progress" until
		// something folds that outcome back in, and deriving it here means the
		// answer survives an API restart instead of depending on a goroutine
		// that may no longer be running.
		if rerr := reconcileDeploymentRollbacks(ctx, cfg, scope.serverID); rerr != nil {
			return deploymentRollbackError(c, rerr)
		}

		rows, lerr := cfg.DeploymentSvc.ListDeployments(ctx, scope.serverID)
		if lerr != nil {
			return deploymentRollbackError(c, lerr)
		}

		filtered := make([]*deployment.Deployment, 0, len(rows))
		for _, row := range rows {
			if statusFilter != "" && strings.ToLower(string(row.Status)) != statusFilter {
				continue
			}
			filtered = append(filtered, row)
		}

		total := len(filtered)
		start := (page - 1) * perPage
		if start > total {
			start = total
		}
		end := start + perPage
		if end > total {
			end = total
		}
		window := filtered[start:end]

		ids := make([]string, 0, len(window))
		for _, row := range window {
			ids = append(ids, row.ID)
		}
		revisions, rerr := loadDeploymentVersionRevisions(ctx, cfg, ids)
		if rerr != nil {
			return deploymentRollbackError(c, rerr)
		}
		byRollbackRow, byTarget, auerr := loadDeploymentRollbackIndex(ctx, cfg, scope.serverID)
		if auerr != nil {
			return deploymentRollbackError(c, auerr)
		}

		liveID := liveDeploymentVersionID(rows)

		data := make([]deploymentVersionView, 0, len(window))
		for _, row := range window {
			view := deploymentVersionView{Deployment: *row}
			if rev, ok := revisions[row.ID]; ok {
				view.RevisionNumber = rev.RevisionNumber
				view.CommitSha = rev.GitCommitSha
				view.ComposeManifestRef = rev.ComposeManifestRef
				view.ConfigHash = rev.ConfigHash
				view.Description = rev.Description
			}
			view.IsLive = row.ID == liveID
			view.CanRollbackTo = deploymentVersionRollbackable(row) && !view.IsLive
			if rec, ok := byRollbackRow[row.ID]; ok {
				mirror := *rec
				view.Rollback = &mirror
			}
			for _, rec := range byTarget[row.ID] {
				view.RolledBackTo = append(view.RolledBackTo, deploymentRollbackSummary{
					ID:        rec.ID,
					Status:    rec.Status,
					Reason:    rec.Reason,
					CreatedAt: rec.CreatedAt,
				})
			}
			data = append(data, view)
		}

		pageCount := total / perPage
		if total%perPage != 0 {
			pageCount++
		}
		return c.JSON(fiber.Map{
			"data":             data,
			"page":             page,
			"perPage":          perPage,
			"total":            total,
			"pageCount":        pageCount,
			"serverId":         scope.serverID,
			"liveDeploymentId": liveID,
		})
	}
}

// liveDeploymentVersionID returns the version presumed to be running: the most
// recent deployment that reached a completed state. A never-completed server
// has no live version, which is reported as "" rather than as the newest row
// that failed.
func liveDeploymentVersionID(rows []*deployment.Deployment) string {
	for _, row := range rows {
		if row == nil {
			continue
		}
		if row.Status == deployment.StatusCompleted {
			return row.ID
		}
	}
	return ""
}

// deploymentVersionRollbackable mirrors the API's own precondition: only a
// version that finished (successfully or by being rolled back) and carries an
// image can be chosen as a rollback target.
func deploymentVersionRollbackable(row *deployment.Deployment) bool {
	if row == nil {
		return false
	}
	if strings.TrimSpace(row.Image) == "" {
		return false
	}
	return row.Status == deployment.StatusCompleted || row.Status == deployment.StatusRolledBack
}

func parseDeploymentRollbackPaging(c *fiber.Ctx) (int, int, error) {
	page := 1
	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return 0, 0, fiber.NewError(fiber.StatusBadRequest, "page must be a positive integer")
		}
		page = n
	}
	perPage := 25
	if raw := strings.TrimSpace(c.Query("perPage")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return 0, 0, fiber.NewError(fiber.StatusBadRequest, "perPage must be a positive integer")
		}
		if n > deploymentRollbackMaxPageSize {
			n = deploymentRollbackMaxPageSize
		}
		perPage = n
	}
	return page, perPage, nil
}

// ---------------------------------------------------------------------------
// detail
// ---------------------------------------------------------------------------

func getDeploymentVersionHandler(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		scope, err := resolveDeploymentRollbackScope(c, cfg, false)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()

		row, derr := cfg.DeploymentSvc.GetDeployment(ctx, c.Params("deploymentId"))
		if derr != nil {
			return deploymentRollbackError(c, derr)
		}
		if row.ServerID != scope.serverID {
			// Wrong scope: refuse rather than leak another workload's history.
			return fiber.NewError(fiber.StatusNotFound, "deployment not found for this workload")
		}

		detail, verr := buildDeploymentVersionDetail(ctx, cfg, scope, *row)
		if verr != nil {
			return deploymentRollbackError(c, verr)
		}
		return c.JSON(fiber.Map{"data": detail})
	}
}

func buildDeploymentVersionDetail(ctx context.Context, cfg Config, scope deploymentRollbackScope, row deployment.Deployment) (*deploymentVersionDetail, error) {
	revisions, err := cfg.DeploymentSvc.ListRevisions(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("list revisions: %w", err)
	}
	steps, err := cfg.DeploymentSvc.ListSteps(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("list steps: %w", err)
	}
	records, err := readDeploymentRollbacksFor(ctx, cfg, row.ID)
	if err != nil {
		return nil, err
	}

	detail := &deploymentVersionDetail{
		deploymentVersionView: deploymentVersionView{
			Deployment:    row,
			CanRollbackTo: false,
			IsLive:        false,
		},
		Steps:     steps,
		Revisions: revisions,
		Rollbacks: records,
	}

	liveID := ""
	if rows, lerr := cfg.DeploymentSvc.ListDeployments(ctx, row.ServerID); lerr == nil {
		liveID = liveDeploymentVersionID(rows)
	}
	detail.IsLive = liveID == row.ID
	detail.CanRollbackTo = deploymentVersionRollbackable(&row) && !detail.IsLive

	// Newest revision is what this version last asked the node to run.
	var newest *deployment.Revision
	for _, rev := range revisions {
		if rev == nil {
			continue
		}
		if newest == nil || rev.RevisionNumber > newest.RevisionNumber {
			newest = rev
		}
	}
	if newest == nil && row.CurrentRevisionID != nil {
		if rev, rerr := cfg.DeploymentSvc.GetRevision(ctx, *row.CurrentRevisionID); rerr == nil {
			newest = rev
		}
	}
	if newest != nil {
		detail.RevisionNumber = newest.RevisionNumber
		detail.CommitSha = newest.GitCommitSHA
		detail.ComposeManifestRef = newest.ComposeManifestRef
		detail.ConfigHash = newest.ConfigHash
		detail.Description = newest.Description
		snapshot := map[string]any{
			"revisionNumber":     newest.RevisionNumber,
			"imageRef":           newest.ImageRef,
			"composeManifestRef": newest.ComposeManifestRef,
			"gitCommitSha":       newest.GitCommitSHA,
			"configHash":         newest.ConfigHash,
			"description":        newest.Description,
		}
		var meta map[string]any
		if len(newest.Metadata) > 0 && json.Unmarshal(newest.Metadata, &meta) == nil {
			snapshot["metadata"] = meta
			if env := extractEnvironmentSection(meta); env != nil {
				detail.Environment = env
			}
		}
		detail.ComposeSnapshot = snapshot
	}

	for _, rec := range records {
		if rec.RollbackDeploymentID != nil && *rec.RollbackDeploymentID == row.ID {
			mirror := rec
			detail.Rollback = &mirror
			break
		}
	}
	if scope.applicationID != nil {
		// Only report the application when the caller reached this version
		// through the app scope; a server-scoped read has not checked that the
		// caller may see the application at all.
		detail.ApplicationID = scope.applicationID
	}
	return detail, nil
}

// extractEnvironmentSection pulls the env map out of a revision's metadata.
// Rollout-created revisions record only an image, so this is often absent; the
// caller reports "no environment snapshot" instead of inventing one.
func extractEnvironmentSection(meta map[string]any) map[string]any {
	for _, key := range []string{"environment", "env", "envVars"} {
		if section, ok := meta[key].(map[string]any); ok {
			return section
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// rollback
// ---------------------------------------------------------------------------

func rollbackDeploymentVersionHandler(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		scope, err := resolveDeploymentRollbackScope(c, cfg, true)
		if err != nil {
			return err
		}

		var req struct {
			Reason string `json:"reason"`
		}
		if len(c.Body()) > 0 {
			if berr := c.BodyParser(&req); berr != nil {
				return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
			}
		}
		reason := strings.TrimSpace(req.Reason)
		if len(reason) > 2000 {
			return fiber.NewError(fiber.StatusBadRequest, "reason must be at most 2000 characters")
		}

		var claims tokenClaims
		if user, ok := c.Locals("user").(tokenClaims); ok {
			claims = user
		}

		ctx, cancel := longRequestContext()
		defer cancel()

		target, terr := cfg.DeploymentSvc.GetDeployment(ctx, c.Params("deploymentId"))
		if terr != nil {
			return deploymentRollbackError(c, terr)
		}
		if target.ServerID != scope.serverID {
			return fiber.NewError(fiber.StatusNotFound, "deployment not found for this workload")
		}
		if strings.TrimSpace(target.Image) == "" {
			return fiber.NewError(fiber.StatusConflict, "that version recorded no image, so there is nothing to restore")
		}
		if !strings.Contains(target.Image, "@sha256:") {
			// The executor pins by digest; a mutable tag could resolve to a
			// different image than the one this version actually ran, so
			// refusing is the honest answer rather than a rollback to something
			// the operator did not choose.
			return fiber.NewError(fiber.StatusConflict, "that version recorded an image without a digest (@sha256:), so it cannot be restored exactly: "+target.Image)
		}
		if !deploymentVersionRollbackable(target) {
			return fiber.NewError(fiber.StatusConflict, "only a version that finished deploying can be rolled back to (current status: "+string(target.Status)+")")
		}

		rows, lerr := cfg.DeploymentSvc.ListDeployments(ctx, scope.serverID)
		if lerr != nil {
			return deploymentRollbackError(c, lerr)
		}
		for _, row := range rows {
			if row != nil && deploymentVersionStatusActive(row.Status) {
				return fiber.NewError(fiber.StatusConflict, "another deployment is still running for this workload; wait for it to finish")
			}
		}
		liveID := liveDeploymentVersionID(rows)
		var live *deployment.Deployment
		for _, row := range rows {
			if row != nil && row.ID == liveID {
				live = row
				break
			}
		}
		if live != nil && live.ID == target.ID {
			return fiber.NewError(fiber.StatusConflict, "that version is already the live one")
		}
		if live != nil && strings.TrimSpace(live.Image) == strings.TrimSpace(target.Image) {
			return fiber.NewError(fiber.StatusConflict, "the live version already runs this image, so a rollback would change nothing")
		}

		snapshot := map[string]any{
			"targetDeploymentId": target.ID,
			"targetImage":        target.Image,
			"targetStrategy":     string(target.Strategy),
			"requestedAt":        time.Now().UTC().Format(time.RFC3339),
		}
		if live != nil {
			snapshot["liveDeploymentId"] = live.ID
			snapshot["liveImage"] = live.Image
		} else {
			snapshot["liveDeploymentId"] = nil
			snapshot["note"] = "no completed version was recorded as live before this rollback"
		}
		if latest, ok := latestImageOf(rows); ok && latest.ID != target.ID {
			snapshot["newestDeploymentId"] = latest.ID
			snapshot["newestImage"] = latest.Image
		}

		initiator := strings.TrimSpace(claims.Sub)
		var initiatedBy *string
		if initiator != "" {
			initiatedBy = &initiator
		}
		var triggeredBy *string
		if newest := newestDeployment(rows); newest != nil {
			triggeredBy = &newest.ID
		}

		rec := &deploymentRollbackRecord{
			ID:                      uuid.NewString(),
			DeploymentID:            target.ID,
			TriggeredByDeploymentID: triggeredBy,
			ApplicationID:           scope.applicationID,
			ServerID:                scope.serverID,
			InitiatedBy:             initiatedBy,
			Status:                  "pending",
			Reason:                  reason,
			Snapshot:                snapshot,
			CreatedAt:               time.Now().UTC(),
		}
		if ierr := insertDeploymentRollback(ctx, cfg, rec); ierr != nil {
			return deploymentRollbackError(c, ierr)
		}

		// Delegate the actual work: StartRollout creates a new deployment row,
		// snapshots a revision, drives the runtime executor and reports real
		// progress. Health gating and auto-rollback are deliberately off — a
		// rollback must not trigger another rollback.
		mirror, rerr := cfg.DeploymentSvc.StartRollout(ctx, &deployment.RolloutRequest{
			ServerID:        scope.serverID,
			Strategy:        deployment.StrategyRecreate,
			Image:           target.Image,
			HealthCheckPath: target.HealthCheckPath,
			HealthCheckPort: target.HealthCheckPort,
			HealthCheckHost: target.HealthCheckHost,
			TimeoutSeconds:  target.TimeoutSeconds,
			TargetReplicas:  target.TargetReplicas,
			// CleanupOnFailure stays false so a failed rollback leaves the
			// previous containers in place for inspection instead of tearing
			// down what is currently serving traffic.
		})
		if rerr != nil {
			if ferr := failDeploymentRollback(ctx, cfg, rec.ID); ferr != nil {
				return deploymentRollbackError(c, errors.New(rerr.Error()+"; additionally: "+ferr.Error()))
			}
			return deploymentRollbackError(c, rerr)
		}

		// Relabel the mirror row. StartRollout has already handed the row to its
		// executor goroutine, which reads `strategy` to pick its step list — so
		// this write is only race-free because both readings produce the same
		// plan: recreate with health gating off is {init, provision, complete},
		// and an unknown strategy (which is what 'rollback' is to the executor)
		// falls through to exactly that same list.
		if uerr := markDeploymentRollbackMirror(ctx, cfg, mirror.ID, rec.ID); uerr != nil {
			return deploymentRollbackError(c, uerr)
		}

		recordAudit(cfg, c, "deployment:rollback", "server", &scope.serverID, map[string]string{
			"rollbackId":    rec.ID,
			"targetVersion": target.ID,
			"newDeployment": mirror.ID,
			"image":         target.Image,
		})

		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"data": fiber.Map{
			"rollbackId":              rec.ID,
			"deploymentId":            mirror.ID,
			"targetDeploymentId":      target.ID,
			"triggeredByDeploymentId": triggeredBy,
			"serverId":                scope.serverID,
			"image":                   target.Image,
			"status":                  string(mirror.Status),
			"reason":                  reason,
			"createdAt":               rec.CreatedAt,
		}})
	}
}

func newestDeployment(rows []*deployment.Deployment) *deployment.Deployment {
	for _, row := range rows {
		if row != nil {
			return row
		}
	}
	return nil
}

// latestImageOf returns the newest row that recorded an image, which is what
// the operator is moving away from even when it never completed.
func latestImageOf(rows []*deployment.Deployment) (*deployment.Deployment, bool) {
	for _, row := range rows {
		if row != nil && strings.TrimSpace(row.Image) != "" {
			return row, true
		}
	}
	return nil, false
}

func deploymentVersionStatusActive(status deployment.Status) bool {
	switch status {
	case deployment.StatusPending, deployment.StatusProvisioning, deployment.StatusInProgress,
		deployment.StatusAwaitingHealth, deployment.StatusPromoting,
		deployment.StatusRollbackPending, deployment.StatusRollingBack:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// diff
// ---------------------------------------------------------------------------

func diffDeploymentVersionHandler(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		scope, err := resolveDeploymentRollbackScope(c, cfg, false)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()

		to, terr := cfg.DeploymentSvc.GetDeployment(ctx, c.Params("deploymentId"))
		if terr != nil {
			return deploymentRollbackError(c, terr)
		}
		if to.ServerID != scope.serverID {
			return fiber.NewError(fiber.StatusNotFound, "deployment not found for this workload")
		}

		fromID := strings.TrimSpace(c.Query("with"))
		if fromID == "" {
			fromID = strings.TrimSpace(c.Query("from"))
		}
		rows, lerr := cfg.DeploymentSvc.ListDeployments(ctx, scope.serverID)
		if lerr != nil {
			return deploymentRollbackError(c, lerr)
		}
		if fromID == "" {
			// Default to what is live now: "what would change if I rolled back
			// to this version?" is the question this endpoint exists for.
			fromID = liveDeploymentVersionID(rows)
		}
		if fromID == "" {
			for _, row := range rows {
				if row != nil && row.ID != to.ID {
					fromID = row.ID
					break
				}
			}
		}
		if fromID == "" {
			return fiber.NewError(fiber.StatusNotFound, "there is no second version to compare against")
		}
		if fromID == to.ID {
			return fiber.NewError(fiber.StatusBadRequest, "cannot diff a version against itself")
		}
		from, ferr := cfg.DeploymentSvc.GetDeployment(ctx, fromID)
		if ferr != nil {
			return deploymentRollbackError(c, ferr)
		}
		if from.ServerID != scope.serverID {
			return fiber.NewError(fiber.StatusNotFound, "comparison deployment not found for this workload")
		}

		fromRev, fromErr := newestVersionRevision(ctx, cfg, from)
		toRev, toErr := newestVersionRevision(ctx, cfg, to)

		diff := &deploymentVersionDiff{
			From:    deploymentVersionSideFrom(from, fromRev),
			To:      deploymentVersionSideFrom(to, toRev),
			Changes: []deployment.RevisionChange{},
			EnvDiff: []deployment.RevisionChange{},
		}
		if fromErr != nil {
			diff.Notes = append(diff.Notes, "the "+diff.From.DeploymentID+" version has no revision snapshot; its image is compared directly")
		}
		if toErr != nil {
			diff.Notes = append(diff.Notes, "the "+diff.To.DeploymentID+" version has no revision snapshot; its image is compared directly")
		}

		switch {
		case fromErr == nil && toErr == nil:
			changes, cerr := cfg.DeploymentSvc.CompareRevisions(ctx, fromRev.ID, toRev.ID)
			if cerr != nil {
				if !errors.Is(cerr, deployment.ErrRevisionNotFound) {
					return deploymentRollbackError(c, cerr)
				}
				diff.Notes = append(diff.Notes, "revision comparison was unavailable: "+cerr.Error())
			} else if changes != nil {
				diff.Changes = append(diff.Changes, changes.Changes...)
			}
		default:
			if strings.TrimSpace(from.Image) != strings.TrimSpace(to.Image) {
				diff.Changes = append(diff.Changes, deployment.RevisionChange{
					Field: "imageRef", OldValue: from.Image, NewValue: to.Image,
				})
			}
		}
		if from.Strategy != to.Strategy {
			diff.Changes = append(diff.Changes, deployment.RevisionChange{
				Field: "strategy", OldValue: string(from.Strategy), NewValue: string(to.Strategy),
			})
		}
		if fromRev != nil && toRev != nil {
			diff.EnvDiff = diffEnvironmentSections(
				extractEnvironmentSection(revisionMetadata(fromRev)),
				extractEnvironmentSection(revisionMetadata(toRev)),
			)
		}

		return c.JSON(fiber.Map{"data": diff})
	}
}

// newestVersionRevision returns the revision describing what this version last
// asked the node to run.
func newestVersionRevision(ctx context.Context, cfg Config, row *deployment.Deployment) (*deployment.Revision, error) {
	revisions, err := cfg.DeploymentSvc.ListRevisions(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	var newest *deployment.Revision
	for _, rev := range revisions {
		if rev == nil {
			continue
		}
		if newest == nil || rev.RevisionNumber > newest.RevisionNumber {
			newest = rev
		}
	}
	if newest == nil && row.CurrentRevisionID != nil {
		if rev, rerr := cfg.DeploymentSvc.GetRevision(ctx, *row.CurrentRevisionID); rerr == nil {
			return rev, nil
		}
	}
	if newest == nil {
		return nil, deployment.ErrRevisionNotFound
	}
	return newest, nil
}

func deploymentVersionSideFrom(row *deployment.Deployment, rev *deployment.Revision) deploymentVersionSide {
	side := deploymentVersionSide{
		DeploymentID: row.ID,
		Status:       string(row.Status),
		Image:        row.Image,
		Strategy:     string(row.Strategy),
		CreatedAt:    row.CreatedAt,
		CompletedAt:  row.CompletedAt,
	}
	if rev == nil {
		return side
	}
	side.RevisionNumber = rev.RevisionNumber
	side.CommitSha = rev.GitCommitSHA
	side.ComposeManifestRef = rev.ComposeManifestRef
	side.Metadata = revisionMetadata(rev)
	return side
}

// revisionMetadata decodes a revision's raw metadata column. A revision with
// no metadata, or with metadata that is not a JSON object, decodes to nil, so
// callers can tell "nothing was recorded" apart from "nothing was recorded as
// an object" without inventing a value.
func revisionMetadata(rev *deployment.Revision) map[string]any {
	if rev == nil || len(rev.Metadata) == 0 {
		return nil
	}
	var meta map[string]any
	if err := json.Unmarshal(rev.Metadata, &meta); err != nil {
		return nil
	}
	return meta
}

// diffEnvironmentSections reports added, removed and changed keys between two
// env snapshots. Values are compared as text; the snapshot stores what was
// configured, and secret material never lands in a revision's metadata.
func diffEnvironmentSections(from, to map[string]any) []deployment.RevisionChange {
	if len(from) == 0 && len(to) == 0 {
		return nil
	}
	keys := make(map[string]struct{}, len(from)+len(to))
	for key := range from {
		keys[key] = struct{}{}
	}
	for key := range to {
		keys[key] = struct{}{}
	}
	names := make([]string, 0, len(keys))
	for key := range keys {
		names = append(names, key)
	}
	sort.Strings(names)

	changes := make([]deployment.RevisionChange, 0, len(names))
	for _, key := range names {
		oldValue, inFrom := from[key]
		newValue, inTo := to[key]
		if inFrom && inTo && scalarEqual(oldValue, newValue) {
			continue
		}
		changes = append(changes, deployment.RevisionChange{
			Field:    "env." + key,
			OldValue: scalarText(oldValue, inFrom),
			NewValue: scalarText(newValue, inTo),
		})
	}
	return changes
}

func scalarEqual(a, b any) bool {
	return scalarText(a, true) == scalarText(b, true)
}

func scalarText(value any, present bool) string {
	if !present {
		return ""
	}
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprintf("%v", typed)
		}
		return string(raw)
	}
}

// ---------------------------------------------------------------------------
// rollback audit records
// ---------------------------------------------------------------------------

func listDeploymentRollbacksHandler(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		scope, err := resolveDeploymentRollbackScope(c, cfg, false)
		if err != nil {
			return err
		}
		limit := 50
		if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
			n, nerr := strconv.Atoi(raw)
			if nerr != nil || n < 1 {
				return fiber.NewError(fiber.StatusBadRequest, "limit must be a positive integer")
			}
			if n > deploymentRollbackAuditLimit {
				n = deploymentRollbackAuditLimit
			}
			limit = n
		}
		target := strings.TrimSpace(c.Query("deploymentId"))

		ctx, cancel := requestContext()
		defer cancel()
		if rerr := reconcileDeploymentRollbacks(ctx, cfg, scope.serverID); rerr != nil {
			return deploymentRollbackError(c, rerr)
		}

		records, err := readDeploymentRollbacks(ctx, cfg, scope.serverID, target, limit)
		if err != nil {
			return deploymentRollbackError(c, err)
		}
		return c.JSON(fiber.Map{"data": records, "total": len(records), "serverId": scope.serverID})
	}
}

func getDeploymentRollbackHandler(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		scope, err := resolveDeploymentRollbackScope(c, cfg, false)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		if rerr := reconcileDeploymentRollbacks(ctx, cfg, scope.serverID); rerr != nil {
			return deploymentRollbackError(c, rerr)
		}

		rollbackID := c.Params("rollbackId")
		query := `
			SELECT ` + deploymentRollbackColumns + `
			FROM deployment_rollbacks
			WHERE id = $1 AND server_id = $2
		`
		record, scanErr := scanDeploymentRollback(cfg.Store.DB().QueryRow(ctx, query, rollbackID, scope.serverID))
		if scanErr != nil {
			if errors.Is(scanErr, pgx.ErrNoRows) {
				return fiber.NewError(fiber.StatusNotFound, "rollback record not found")
			}
			return deploymentRollbackError(c, scanErr)
		}
		return c.JSON(fiber.Map{"data": record})
	}
}

const deploymentRollbackColumns = `id::text, deployment_id::text, triggered_by_deployment_id::text,
	rollback_deployment_id::text, application_id::text, server_id::text, initiated_by::text,
	status, reason, snapshot, created_at, completed_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDeploymentRollback(scanner rowScanner) (*deploymentRollbackRecord, error) {
	var rec deploymentRollbackRecord
	var snapshotRaw []byte
	err := scanner.Scan(
		&rec.ID, &rec.DeploymentID, &rec.TriggeredByDeploymentID,
		&rec.RollbackDeploymentID, &rec.ApplicationID, &rec.ServerID, &rec.InitiatedBy,
		&rec.Status, &rec.Reason, &snapshotRaw, &rec.CreatedAt, &rec.CompletedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(snapshotRaw) > 0 {
		var snapshot map[string]any
		if json.Unmarshal(snapshotRaw, &snapshot) == nil {
			rec.Snapshot = snapshot
		}
	}
	return &rec, nil
}

func insertDeploymentRollback(ctx context.Context, cfg Config, rec *deploymentRollbackRecord) error {
	snapshotRaw, err := json.Marshal(rec.Snapshot)
	if err != nil {
		return fmt.Errorf("encode rollback snapshot: %w", err)
	}
	if len(snapshotRaw) == 0 {
		snapshotRaw = []byte("{}")
	}
	_, err = cfg.Store.DB().Exec(ctx, `
		INSERT INTO deployment_rollbacks (
			id, deployment_id, triggered_by_deployment_id, rollback_deployment_id,
			application_id, server_id, initiated_by, status, reason, snapshot, created_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, rec.ID, rec.DeploymentID, rec.TriggeredByDeploymentID, rec.RollbackDeploymentID,
		rec.ApplicationID, rec.ServerID, rec.InitiatedBy, rec.Status, rec.Reason, snapshotRaw,
		rec.CreatedAt, rec.CompletedAt)
	if err != nil {
		return fmt.Errorf("record rollback: %w", err)
	}
	return nil
}

// markDeploymentRollbackMirror links the rollback record to the deployment row
// created to carry it out and relabels that row as a rollback.
//
// The strategy write looks racy — StartRollout has already handed the row to
// its executor goroutine, which reads `strategy` to choose its step list — but
// both readings produce the same plan: 'recreate' with health gating off is
// {init, provision, complete}, and an unrecognised strategy falls through to
// that identical default. So the row can be labelled honestly without changing
// what gets executed.
func markDeploymentRollbackMirror(ctx context.Context, cfg Config, mirrorID, rollbackID string) error {
	pool := cfg.Store.DB()
	if _, err := pool.Exec(ctx, `UPDATE deployments SET strategy = 'rollback', updated_at = now() WHERE id = $1`, mirrorID); err != nil {
		return fmt.Errorf("label rollback deployment: %w", err)
	}
	tag, err := pool.Exec(ctx, `
		UPDATE deployment_rollbacks
		SET status = 'in_progress', rollback_deployment_id = $2
		WHERE id = $1 AND status = 'pending'
	`, rollbackID, mirrorID)
	if err != nil {
		return fmt.Errorf("link rollback record: %w", err)
	}
	// The mirror row is live now. If the audit link did not land, say so instead
	// of leaving a rollback that nobody can find in the history.
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("link rollback record: rollback %s is no longer pending", rollbackID)
	}
	return nil
}

func failDeploymentRollback(ctx context.Context, cfg Config, rollbackID string) error {
	_, err := cfg.Store.DB().Exec(ctx, `
		UPDATE deployment_rollbacks
		SET status = 'failed', completed_at = now()
		WHERE id = $1 AND status = 'pending'
	`, rollbackID)
	if err != nil {
		return fmt.Errorf("record failed rollback: %w", err)
	}
	return nil
}

// reconcileDeploymentRollbacks folds the outcome of a rollback's mirror
// deployment back into the audit row. A rollback that is still running stays
// untouched.
func reconcileDeploymentRollbacks(ctx context.Context, cfg Config, serverID string) error {
	_, err := cfg.Store.DB().Exec(ctx, `
		UPDATE deployment_rollbacks dr
		SET status = CASE nd.status
				WHEN 'completed' THEN 'completed'
				WHEN 'failed' THEN 'failed'
				WHEN 'cancelled' THEN 'cancelled'
				ELSE 'failed'
			END,
			completed_at = COALESCE(dr.completed_at, nd.completed_at, now())
		FROM deployments nd
		WHERE nd.id = dr.rollback_deployment_id
		  AND dr.server_id = $1
		  AND dr.status IN ('pending', 'in_progress')
		  AND nd.status IN ('completed', 'failed', 'cancelled', 'rolled_back')
	`, serverID)
	if err != nil {
		return fmt.Errorf("reconcile rollback records: %w", err)
	}
	return nil
}

func readDeploymentRollbacks(ctx context.Context, cfg Config, serverID, targetDeploymentID string, limit int) ([]deploymentRollbackRecord, error) {
	sql := `
		SELECT ` + deploymentRollbackColumns + `
		FROM deployment_rollbacks
		WHERE server_id = $1`
	args := []any{serverID}
	if targetDeploymentID != "" {
		sql += ` AND deployment_id = $2`
		args = append(args, targetDeploymentID)
	}
	sql += ` ORDER BY created_at DESC LIMIT ` + strconv.Itoa(limit)

	rows, err := cfg.Store.DB().Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("list rollback records: %w", err)
	}
	defer rows.Close()

	records := make([]deploymentRollbackRecord, 0, 8)
	for rows.Next() {
		rec, serr := scanDeploymentRollback(rows)
		if serr != nil {
			return nil, fmt.Errorf("read rollback record: %w", serr)
		}
		records = append(records, *rec)
	}
	return records, rows.Err()
}

func readDeploymentRollbacksFor(ctx context.Context, cfg Config, deploymentID string) ([]deploymentRollbackRecord, error) {
	rows, err := cfg.Store.DB().Query(ctx, `
		SELECT `+deploymentRollbackColumns+`
		FROM deployment_rollbacks
		WHERE deployment_id = $1 OR rollback_deployment_id = $1 OR triggered_by_deployment_id = $1
		ORDER BY created_at DESC
		LIMIT 50
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("list rollbacks for version: %w", err)
	}
	defer rows.Close()

	records := make([]deploymentRollbackRecord, 0, 4)
	for rows.Next() {
		rec, serr := scanDeploymentRollback(rows)
		if serr != nil {
			return nil, fmt.Errorf("read rollback record: %w", serr)
		}
		records = append(records, *rec)
	}
	return records, rows.Err()
}

// loadDeploymentRollbackIndex returns rollback records for one server keyed by
// the mirror deployment that carried them out, and by the version they chose.
func loadDeploymentRollbackIndex(ctx context.Context, cfg Config, serverID string) (map[string]*deploymentRollbackRecord, map[string][]*deploymentRollbackRecord, error) {
	byMirror := make(map[string]*deploymentRollbackRecord)
	byTarget := make(map[string][]*deploymentRollbackRecord)

	rows, err := cfg.Store.DB().Query(ctx, `
		SELECT `+deploymentRollbackColumns+`
		FROM deployment_rollbacks
		WHERE server_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, serverID, deploymentRollbackAuditLimit)
	if err != nil {
		return byMirror, byTarget, fmt.Errorf("list rollback records: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		rec, serr := scanDeploymentRollback(rows)
		if serr != nil {
			return byMirror, byTarget, fmt.Errorf("read rollback record: %w", serr)
		}
		if rec.RollbackDeploymentID != nil && *rec.RollbackDeploymentID != "" {
			mirror := *rec
			byMirror[*rec.RollbackDeploymentID] = &mirror
		}
		summary := *rec
		byTarget[rec.DeploymentID] = append(byTarget[rec.DeploymentID], &summary)
	}
	return byMirror, byTarget, rows.Err()
}

// ---------------------------------------------------------------------------
// revision enrichment
// ---------------------------------------------------------------------------

type deploymentVersionRevisionInfo struct {
	RevisionNumber     int
	ImageRef           string
	GitCommitSha       string
	ComposeManifestRef string
	ConfigHash         string
	Description        string
}

// loadDeploymentVersionRevisions fetches one revision per version in a single
// query. Doing it per row would cost a query per history entry, and the page
// size is user-controlled.
func loadDeploymentVersionRevisions(ctx context.Context, cfg Config, ids []string) (map[string]deploymentVersionRevisionInfo, error) {
	result := make(map[string]deploymentVersionRevisionInfo, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := cfg.Store.DB().Query(ctx, `
		SELECT DISTINCT ON (deployment_id)
			deployment_id::text, revision_number, image_ref, compose_manifest_ref,
			git_commit_sha, config_hash, COALESCE(description, '')
		FROM deployment_revisions
		WHERE deployment_id = ANY($1::uuid[])
		ORDER BY deployment_id, revision_number DESC
	`, pgArray(ids))
	if err != nil {
		return result, fmt.Errorf("load version revisions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var info deploymentVersionRevisionInfo
		if serr := rows.Scan(&id, &info.RevisionNumber, &info.ImageRef, &info.ComposeManifestRef,
			&info.GitCommitSha, &info.ConfigHash, &info.Description); serr != nil {
			return result, fmt.Errorf("read version revision: %w", serr)
		}
		result[id] = info
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// error mapping
// ---------------------------------------------------------------------------

// deploymentRollbackError maps deployment and rollback failures onto honest
// HTTP statuses. Everything the caller can act on — a version that is not a
// valid target, a deployment already running, an image the executor refuses —
// is reported as 4xx with the service's own message rather than a 500.
func deploymentRollbackError(c *fiber.Ctx, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, deployment.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, deployment.ErrRevisionNotFound):
		return fiber.NewError(fiber.StatusNotFound, "that version has no recorded revision snapshot")
	case errors.Is(err, deployment.ErrInProgress),
		errors.Is(err, store.ErrDeploymentInProgress),
		errors.Is(err, deployment.ErrNotTerminal),
		errors.Is(err, deployment.ErrNoRollback):
		return fiber.NewError(fiber.StatusConflict, err.Error())
	case errors.Is(err, deployment.ErrNoRevisions):
		return fiber.NewError(fiber.StatusConflict, "that version has no revision history to roll back to")
	case errors.Is(err, deployment.ErrNoRuntimeExecutor):
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
	case errors.Is(err, deployment.ErrInvalidImage),
		errors.Is(err, deployment.ErrInvalidImageRef),
		errors.Is(err, deployment.ErrInvalidServer):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		return err
	}
	return respondInternalError(c, err)
}
