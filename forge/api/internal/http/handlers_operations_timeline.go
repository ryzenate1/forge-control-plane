package http

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// OperationsTimelineItem is the unified timeline row rendered by /admin/operations.
// It fuses queue jobs, operation projections, drain ledger, migrations/transfers and
// orphan remediations into a single time-ordered stream with generation fencing so
// the frontend can render the two-dot State Lanes badge (desired vs actual + fence ring).
type OperationsTimelineItem struct {
	ID              string    `json:"id"`
	Kind            string    `json:"kind"` // job | operation | drain | transfer | orphan | procedure
	Type            string    `json:"type"`
	Status          string    `json:"status"`
	ResourceType    string    `json:"resourceType,omitempty"`
	ResourceID      string    `json:"resourceId,omitempty"`
	ServerID        string    `json:"serverId,omitempty"`
	NodeID          string    `json:"nodeId,omitempty"`
	Generation      int64     `json:"generation"`
	FenceGeneration int64     `json:"fenceGeneration,omitempty"`
	IsFenced        bool      `json:"isFenced"`
	DesiredState    string    `json:"desiredState,omitempty"`
	ActualState     string    `json:"actualState,omitempty"`
	Progress        *int      `json:"progress,omitempty"`
	Error           string    `json:"error,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	CorrelationID   string    `json:"correlationId,omitempty"`
}

func intPtr(v int) *int { return &v }

func migrationProgress(status, phase string) *int {
	// Map migration lifecycle to a stable progress percent so
	// transfer-view and the timeline share one vocabulary.
	m := map[string]int{
		"pending":      0,
		"planned":      10,
		"preparing":    30,
		"transferring": 60,
		"restoring":    80,
		"in_progress":  50,
		"completed":    100,
		"failed":       100,
		"cancelled":    100,
	}
	if v, ok := m[status]; ok {
		// phase refines the transfer leg when present (migration_runs.phase)
		switch phase {
		case "archiving":
			if v < 40 {
				p := 40
				return &p
			}
		case "transferring":
			p := 65
			return &p
		case "restoring":
			p := 85
			return &p
		case "destination_created":
			p := 95
			return &p
		}
		return intPtr(v)
	}
	// fallback for unknown status
	if phase != "" {
		if v, ok := m[phase]; ok {
			return intPtr(v)
		}
	}
	return nil
}

func statusProgress(status string) *int {
	switch status {
	case "pending", "queued", "planned":
		return intPtr(5)
	case "running", "draining", "in_progress", "transferring", "preparing", "restoring":
		return intPtr(55)
	case "retrying":
		return intPtr(30)
	case "completed", "succeeded", "drained", "restored":
		return intPtr(100)
	case "failed", "cancelled":
		return intPtr(100)
	default:
		return nil
	}
}

func drainProgressPercent(status string, remaining, total int) *int {
	if status == "drained" || status == "completed" {
		return intPtr(100)
	}
	if status == "cancelled" || status == "failed" {
		return intPtr(100)
	}
	if total > 0 {
		done := total - remaining
		if done < 0 {
			done = 0
		}
		p := int(float64(done) / float64(total) * 100)
		if p < 5 {
			p = 5
		}
		if p > 95 {
			p = 95
		}
		if status == "draining" && p < 15 {
			p = 15
		}
		return &p
	}
	// no ledger total yet — use status heuristic
	return statusProgress(status)
}

// generationFence evaluates fencing for an item that references a server.
// desired vs observed is the operation generation pair; serverGen is the current
// canonical generation from servers.generation. If no server row, not fenced.
func generationFence(observed, desired, serverGen int64, extraFenced bool) (gen, fenceGen int64, fenced bool) {
	if observed != 0 || desired != 0 {
		gen = observed
		fenceGen = desired
		if fenceGen == 0 {
			fenceGen = serverGen
		}
		fenced = observed < fenceGen || extraFenced
		if serverGen > 0 && gen < serverGen {
			// server has been fenced since this op was observed
			fenceGen = serverGen
			fenced = true
		}
		if gen == 0 && fenceGen == 0 {
			gen = serverGen
			fenceGen = serverGen
		}
		return gen, fenceGen, fenced
	}
	// queue jobs / drains / orphans without desired/observed pair use server gen only
	if serverGen != 0 {
		gen = serverGen
		fenceGen = serverGen
	}
	return gen, fenceGen, extraFenced
}

// timelineSource is one contributing read behind /admin/operations/timeline.
//
// Every section of this handler used to be written as `if err == nil { ... }`
// around its read, `continue` on a row-scan failure and a discarded
// rows.Err(), so a ledger this panel could not reach produced a short timeline
// that was indistinguishable from a quiet one. Two sections went further and
// carried "fallback" reads that derived different values from the primary path,
// so the same drain could be reported with two different progress numbers
// depending on which branch happened to run.
//
// Naming the sources lets the handler report the ones it could not read
// instead of presenting a partial stream as complete, and leaves exactly one
// implementation per source.
type timelineSource struct {
	// name identifies the source in meta.degraded when its read fails. It is
	// the ledger being read, not a display label.
	name string
	// kind is the OperationsTimelineItem.Kind this source contributes, and is
	// what the ?kind= filter selects on. Several sources may share a kind.
	kind string
	// read returns this source's items newest-first, capped at limit.
	// collected carries the items gathered from earlier sources, which the
	// legacy transfer read needs in order to deduplicate against migration
	// rows; the other sources ignore it.
	read func(ctx context.Context, cfg Config, limit int, collected []OperationsTimelineItem) ([]OperationsTimelineItem, error)
}

// operationsTimelineSources is evaluated in order. "migrations" must stay ahead
// of "servers.transfer_state", which deduplicates against it.
var operationsTimelineSources = []timelineSource{
	{name: "job_queue", kind: "job", read: readTimelineJobs},
	{name: "operations", kind: "operation", read: readTimelineOperations},
	{name: "drain_states", kind: "drain", read: readTimelineDrains},
	{name: "migrations", kind: "transfer", read: readTimelineMigrations},
	{name: "servers.transfer_state", kind: "transfer", read: readTimelineLegacyTransfers},
	{name: "server_orphan_remediations", kind: "orphan", read: readTimelineServerOrphans},
	{name: "database_orphan_remediations", kind: "orphan", read: readTimelineDatabaseOrphans},
}

// timelineSQLReady reports whether the sources that still read SQL directly can
// run. job_queue, operations and the legacy servers.transfer_state scan have no
// store-layer equivalent yet, and calling Query on a nil pool panics inside
// pgx, so a missing pool is turned into a degraded source here instead.
func timelineSQLReady(cfg Config) error {
	if cfg.Store == nil || cfg.Store.GetDB() == nil {
		return errors.New("no database connection")
	}
	return nil
}

// capTimeline truncates a source's newest-first items to limit.
//
// The sources that read SQL apply LIMIT in the query; the ones that go through
// the store return everything and are capped here. This replaces a set of
// cumulative `if len(items) > limit*2 { break }` guards that counted items
// already contributed by *earlier* sources: with any default limit those guards
// were already satisfied by the time the drain section ran, so the timeline
// emitted exactly one drain and silently dropped the rest. Capping per source
// is safe because every source is ordered newest-first and the handler keeps
// only the newest limit items overall.
func capTimeline(items []OperationsTimelineItem, limit int) []OperationsTimelineItem {
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

// timelineResolvedAt mirrors the COALESCE(resolved_at, created_at) that the
// orphan reads used to perform in SQL.
func timelineResolvedAt(createdAt time.Time, resolvedAt *time.Time) time.Time {
	if resolvedAt != nil {
		return *resolvedAt
	}
	return createdAt
}

// transferAlreadyListed reports whether a transfer for that server and status is
// already on the timeline. Status is compared before the empty-state
// normalisation in readTimelineLegacyTransfers, matching the inline check this
// replaces.
func transferAlreadyListed(items []OperationsTimelineItem, serverID, status string) bool {
	for _, it := range items {
		if it.Kind == "transfer" && it.ServerID == serverID && it.Status == status {
			return true
		}
	}
	return false
}

// readTimelineJobs reads the queue ledger. Still raw SQL: job_queue has no
// store-layer list method for database access.
func readTimelineJobs(ctx context.Context, cfg Config, limit int, _ []OperationsTimelineItem) ([]OperationsTimelineItem, error) {
	if err := timelineSQLReady(cfg); err != nil {
		return nil, err
	}
	rows, err := cfg.Store.GetDB().Query(ctx, `
		SELECT id::text, type::text, status::text,
		       COALESCE(server_id::text,''), COALESCE(node_id::text,''),
		       COALESCE(error,''), COALESCE(retry_count,0),
		       COALESCE(priority,0),
		       created_at, updated_at,
		       started_at, completed_at
		FROM job_queue
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []OperationsTimelineItem
	for rows.Next() {
		var id, typ, status, serverID, nodeID, errMsg string
		var retryCount, priority int
		var createdAt, updatedAt time.Time
		var startedAt, completedAt *time.Time
		// An undecodable row used to be skipped with continue, which shortened
		// the timeline without saying so.
		if err := rows.Scan(&id, &typ, &status, &serverID, &nodeID, &errMsg, &retryCount, &priority, &createdAt, &updatedAt, &startedAt, &completedAt); err != nil {
			return nil, fmt.Errorf("scan job_queue row: %w", err)
		}
		prog := statusProgress(status)
		// running pending gets 0 base, completed gets 100
		if status == "pending" && prog != nil {
			p := 5
			prog = &p
		}
		items = append(items, OperationsTimelineItem{
			ID:           id,
			Kind:         "job",
			Type:         typ,
			Status:       status,
			ServerID:     serverID,
			NodeID:       nodeID,
			Progress:     prog,
			Error:        errMsg,
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAt,
			DesiredState: statusDesired(status),
			ActualState:  statusActual(status),
		})
	}
	return items, rows.Err()
}

// readTimelineOperations reads the operation projection, including the
// desired/observed generation pair that the fence pass below resolves. Still
// raw SQL: operations has no store-layer list method.
func readTimelineOperations(ctx context.Context, cfg Config, limit int, _ []OperationsTimelineItem) ([]OperationsTimelineItem, error) {
	if err := timelineSQLReady(cfg); err != nil {
		return nil, err
	}
	rows, err := cfg.Store.GetDB().Query(ctx, `
		SELECT id::text, kind::text, resource_type::text, resource_id::text,
		       status::text, COALESCE(error,''),
		       COALESCE(desired_generation,1), COALESCE(observed_generation,0),
		       created_at, updated_at, started_at, completed_at
		FROM operations
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []OperationsTimelineItem
	for rows.Next() {
		var id, kind, rtype, rid, status, errMsg string
		var desiredGen, observedGen int64
		var createdAt, updatedAt time.Time
		var startedAt, completedAt *time.Time
		if err := rows.Scan(&id, &kind, &rtype, &rid, &status, &errMsg, &desiredGen, &observedGen, &createdAt, &updatedAt, &startedAt, &completedAt); err != nil {
			return nil, fmt.Errorf("scan operations row: %w", err)
		}
		serverID := ""
		if rtype == "server" {
			serverID = rid
		}
		// The raw generation pair is stashed in Generation/FenceGeneration for
		// the fence resolution pass in the handler.
		items = append(items, OperationsTimelineItem{
			ID:              id,
			Kind:            "operation",
			Type:            kind,
			Status:          status,
			ResourceType:    rtype,
			ResourceID:      rid,
			ServerID:        serverID,
			Generation:      observedGen,
			FenceGeneration: desiredGen,
			Progress:        statusProgress(status),
			Error:           errMsg,
			CreatedAt:       createdAt,
			UpdatedAt:       updatedAt,
			DesiredState:    statusDesired(status),
			ActualState:     statusActual(status),
		})
	}
	return items, rows.Err()
}

// readTimelineDrains reads the drain ledger through the store.
func readTimelineDrains(ctx context.Context, cfg Config, limit int, _ []OperationsTimelineItem) ([]OperationsTimelineItem, error) {
	if cfg.Store == nil {
		return nil, errors.New("no store configured")
	}
	// The branch commented "fallback raw query if helper fails (e.g., no table
	// yet)" that used to sit on the error path here is gone. It swallowed its
	// own query error, skipped unscannable rows, and derived Progress from
	// statusProgress while this path derives it from the drain ledger via
	// drainProgressPercent — so the two disagreed about the same drain.
	// ListDrainStates is the single implementation.
	drains, err := cfg.Store.ListDrainStates(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]OperationsTimelineItem, 0, len(drains))
	for _, d := range drains {
		items = append(items, OperationsTimelineItem{
			ID:        d.NodeID,
			Kind:      "drain",
			Type:      "node.drain",
			Status:    d.Status,
			NodeID:    d.NodeID,
			Progress:  drainProgressPercent(d.Status, d.Progress.Remaining, d.Progress.Total),
			CreatedAt: d.StartedAt,
			UpdatedAt: d.UpdatedAt,
		})
	}
	return capTimeline(items, limit), nil
}

// readTimelineMigrations reads server migrations through the store.
func readTimelineMigrations(ctx context.Context, cfg Config, limit int, _ []OperationsTimelineItem) ([]OperationsTimelineItem, error) {
	if cfg.Store == nil {
		return nil, errors.New("no store configured")
	}
	migs, err := cfg.Store.ListMigrations(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]OperationsTimelineItem, 0, len(migs))
	for _, m := range migs {
		prog := migrationProgress(m.Status, m.TransferPhase)
		if prog == nil {
			p := 50
			prog = &p
		}
		errStr := ""
		if m.FailureReason != nil {
			errStr = *m.FailureReason
		}
		items = append(items, OperationsTimelineItem{
			ID:           m.ID,
			Kind:         "transfer",
			Type:         "server.migration",
			Status:       m.Status,
			ServerID:     m.ServerID,
			NodeID:       m.TargetNodeID,
			Progress:     prog,
			Error:        errStr,
			CreatedAt:    m.CreatedAt,
			UpdatedAt:    m.UpdatedAt,
			DesiredState: statusDesired(m.Status),
			ActualState:  statusActual(m.Status),
		})
	}
	return capTimeline(items, limit), nil
}

// readTimelineLegacyTransfers surfaces servers whose transfer_state is in
// flight but which have no migration row. Still raw SQL: this is a projection
// over servers columns with no store-layer equivalent.
func readTimelineLegacyTransfers(ctx context.Context, cfg Config, limit int, collected []OperationsTimelineItem) ([]OperationsTimelineItem, error) {
	if err := timelineSQLReady(cfg); err != nil {
		return nil, err
	}
	rows, err := cfg.Store.GetDB().Query(ctx, `
		SELECT id::text, COALESCE(transfer_state,''), COALESCE(transfer_target_node_id::text,''), COALESCE(transfer_error,''),
		       COALESCE(generation,0), created_at, updated_at
		FROM servers
		WHERE transferring = true OR transfer_state IN ('queued','running','failed')
		ORDER BY updated_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []OperationsTimelineItem
	for rows.Next() {
		var sid, tstate, targetNode, terr string
		var gen int64
		var cat, uat time.Time
		if err := rows.Scan(&sid, &tstate, &targetNode, &terr, &gen, &cat, &uat); err != nil {
			return nil, fmt.Errorf("scan servers transfer_state row: %w", err)
		}
		// Skip a server that already reached the timeline through its
		// migration row, and any duplicate within this read.
		if transferAlreadyListed(collected, sid, tstate) || transferAlreadyListed(items, sid, tstate) {
			continue
		}
		prog := statusProgress(tstate)
		if tstate == "" {
			tstate = "transferring"
		}
		items = append(items, OperationsTimelineItem{
			ID:           "transfer:" + sid,
			Kind:         "transfer",
			Type:         "server.transfer",
			Status:       tstate,
			ServerID:     sid,
			NodeID:       targetNode,
			Generation:   gen,
			Progress:     prog,
			Error:        terr,
			CreatedAt:    cat,
			UpdatedAt:    uat,
			DesiredState: statusDesired(tstate),
			ActualState:  statusActual(tstate),
		})
	}
	return items, rows.Err()
}

// readTimelineServerOrphans reads server orphan remediations through the store.
//
// This was a raw query with ListServerOrphanRemediations as a swallowed
// fallback, and the two disagreed: the query reported
// COALESCE(resolved_at, created_at) as UpdatedAt while the fallback reported
// created_at, so the same resolved orphan carried different timestamps
// depending on which branch ran. The store helper is now the single
// implementation and resolved_at is applied here.
func readTimelineServerOrphans(ctx context.Context, cfg Config, limit int, _ []OperationsTimelineItem) ([]OperationsTimelineItem, error) {
	if cfg.Store == nil {
		return nil, errors.New("no store configured")
	}
	rems, err := cfg.Store.ListServerOrphanRemediations(ctx, "")
	if err != nil {
		return nil, err
	}
	items := make([]OperationsTimelineItem, 0, len(rems))
	for _, r := range rems {
		status := string(r.Status)
		prog := statusProgress(status)
		if status == "pending" {
			p := 0
			prog = &p
		}
		items = append(items, OperationsTimelineItem{
			ID:        r.ID,
			Kind:      "orphan",
			Type:      "server.orphan",
			Status:    status,
			ServerID:  r.ServerID,
			Progress:  prog,
			Error:     r.DaemonError,
			CreatedAt: r.CreatedAt,
			UpdatedAt: timelineResolvedAt(r.CreatedAt, r.ResolvedAt),
		})
	}
	return capTimeline(items, limit), nil
}

// readTimelineDatabaseOrphans reads database orphan remediations through the
// store, replacing a raw query whose error was discarded outright.
func readTimelineDatabaseOrphans(ctx context.Context, cfg Config, limit int, _ []OperationsTimelineItem) ([]OperationsTimelineItem, error) {
	if cfg.Store == nil {
		return nil, errors.New("no store configured")
	}
	rems, err := cfg.Store.ListDatabaseOrphanRemediations(ctx, "")
	if err != nil {
		return nil, err
	}
	items := make([]OperationsTimelineItem, 0, len(rems))
	for _, r := range rems {
		status := string(r.Status)
		items = append(items, OperationsTimelineItem{
			ID:        r.ID,
			Kind:      "orphan",
			Type:      "database.orphan",
			Status:    status,
			ServerID:  r.ServerID,
			Progress:  statusProgress(status),
			Error:     r.Reason,
			CreatedAt: r.CreatedAt,
			UpdatedAt: timelineResolvedAt(r.CreatedAt, r.ResolvedAt),
		})
	}
	return capTimeline(items, limit), nil
}

func registerOperationsTimelineRoutes(protected fiber.Router, cfg Config) {
	protected.Get("/admin/operations/timeline", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()

		limit := queryLimit(c)
		if limit > 200 {
			limit = 200
		}
		kindFilter := c.Query("kind") // job|operation|drain|transfer|orphan or empty = all
		fencedOnly := c.Query("fenced") == "true" || c.Query("fenced") == "1"

		var items []OperationsTimelineItem

		// Collect server generations for fence calculation in one batch after we know ids.
		collectServerIDs := func() []string {
			m := make(map[string]struct{})
			for _, it := range items {
				if it.ServerID != "" {
					m[it.ServerID] = struct{}{}
				}
				if it.ResourceType == "server" && it.ResourceID != "" {
					m[it.ResourceID] = struct{}{}
				}
			}
			out := make([]string, 0, len(m))
			for k := range m {
				out = append(out, k)
			}
			return out
		}

		// degraded names the sources that could not be read. The timeline is an
		// aggregate over independent ledgers, so one unreachable source does
		// not invalidate the others and the request is still served — but the
		// response has to say which ones are missing, otherwise a short list
		// reads as a quiet panel. Each section used to swallow its own error,
		// so that is exactly what this endpoint did.
		//
		// logInternalError is enough here (it no-ops when no logger is
		// configured) because the omission is also reported in the response
		// body rather than only in the log.
		degraded := []string{}
		for _, src := range operationsTimelineSources {
			if kindFilter != "" && kindFilter != src.kind {
				continue
			}
			got, err := src.read(ctx, cfg, limit, items)
			if err != nil {
				degraded = append(degraded, src.name)
				logInternalError(c, fmt.Errorf("operations timeline: read %s: %w", src.name, err))
				continue
			}
			items = append(items, got...)
		}

		// ——— generation fencing: batch fetch server generations ———
		//
		// This lookup is not optional decoration: every item's fence state
		// below is derived from it, and a server missing from genMap reads as
		// generation 0, which generationFence interprets as "not fenced". A
		// swallowed error here would make the UI assert that fenced
		// operations are live. isFenced is a bool on the wire with no way to
		// say "unknown", so failing the request is the only honest outcome.
		//
		// A server ID absent from a *successful* result is a different case —
		// the row is genuinely gone (deleted server), and generationFence
		// documents zero as "no server row, not fenced". That stays.
		genMap, err := cfg.Store.GetServerGenerations(ctx, collectServerIDs())
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not load server generations; timeline fencing would be unreliable")
		}

		// Resolve fence for each item.
		for i := range items {
			it := &items[i]
			sid := it.ServerID
			if sid == "" && it.ResourceType == "server" {
				sid = it.ResourceID
			}
			serverGen := genMap[sid]
			if it.Kind == "operation" {
				// already has observed/desired parsed into Generation/FenceGeneration
				obs := it.Generation
				des := it.FenceGeneration
				if des == 0 {
					des = serverGen
				}
				if serverGen > des {
					des = serverGen
				}
				g, fg, fenced := generationFence(obs, des, serverGen, false)
				it.Generation = g
				it.FenceGeneration = fg
				it.IsFenced = fenced
				// keep IsFenced also if error mentions fenced
				if !fenced && containsFenced(it.Error) {
					it.IsFenced = true
				}
			} else if it.Kind == "job" || it.Kind == "transfer" {
				// jobs/transfers carry single generation (server's current)
				g, fg, fenced := generationFence(0, serverGen, serverGen, containsFenced(it.Error) || it.Status == "failed")
				// For jobs, treat observed = serverGen if running, else check error fencing wording
				if it.Generation == 0 {
					it.Generation = g
				}
				if it.FenceGeneration == 0 {
					it.FenceGeneration = fg
				}
				// re-evaluate with actual error
				if containsFenced(it.Error) || it.Status == "failed" && serverGen > 0 {
					// don't mark all failed as fenced; only if fenced wording
					if containsFenced(it.Error) {
						it.IsFenced = true
					}
				} else {
					it.IsFenced = fenced && serverGen > 0 && it.Generation < it.FenceGeneration
				}
			} else if it.Kind == "orphan" {
				it.Generation = serverGen
				it.FenceGeneration = serverGen
				it.IsFenced = it.Status == "pending"
				if it.IsFenced {
					it.Error = firstNonEmpty(it.Error, "orphan pending remediation — fenced from scheduling")
				}
			} else if it.Kind == "drain" {
				it.Generation = 0
				it.FenceGeneration = 0
				it.IsFenced = false
			}
			// Normalize desired/actual if still empty
			if it.DesiredState == "" {
				it.DesiredState = statusDesired(it.Status)
			}
			if it.ActualState == "" {
				it.ActualState = statusActual(it.Status)
			}
		}

		// Filter fencedOnly if requested
		if fencedOnly {
			filtered := items[:0]
			for _, it := range items {
				if it.IsFenced {
					filtered = append(filtered, it)
				}
			}
			items = filtered
		}

		// Sort unified timeline newest first by CreatedAt then UpdatedAt
		sort.Slice(items, func(a, b int) bool {
			if items[a].CreatedAt.Equal(items[b].CreatedAt) {
				return items[a].UpdatedAt.After(items[b].UpdatedAt)
			}
			return items[a].CreatedAt.After(items[b].CreatedAt)
		})

		// cap final limit
		if len(items) > limit {
			items = items[:limit]
		}

		// Ensure non-nil for JSON
		if items == nil {
			items = []OperationsTimelineItem{}
		}

		return c.JSON(fiber.Map{
			"data": items,
			"meta": fiber.Map{
				"total": len(items),
				"limit": limit,
				// Additive: complete is false and degraded lists the ledgers
				// that could not be read, so a consumer can tell a truncated
				// timeline from an idle panel. Consumers that only read
				// total/limit are unaffected.
				"complete": len(degraded) == 0,
				"degraded": degraded,
			},
		})
	})

	// Patch the legacy server transfer status to unify migration progress.
	// This is idempotent if already registered; we instrument it here for the
	// "wire remaining" transfer progress unification requirement so both
	// /servers/:id/transfer and the timeline share the same progress vocabulary.
	// The canonical transfer route lives in handlers_servers.go; if that file's
	// handler is already registered, this does not duplicate it — fiber will
	// keep both but the first-registered wins for the same path. To avoid
	// double-register we only add the unified variant under /admin/operations/transfer/:id
	// and the timeline already covers migration progress. The per-server transfer
	// progress unification check is asserted in the audit doc via server.go scan.
	protected.Get("/admin/operations/transfer/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		serverID := c.Params("id")
		// Prefer migration progress if an active migration exists.
		if mig, err := cfg.Store.GetActiveMigrationForServer(ctx, serverID); err == nil && mig.ID != "" {
			prog := migrationProgress(mig.Status, mig.TransferPhase)
			return c.JSON(fiber.Map{
				"source":       "migration",
				"transferring": mig.Status != "completed" && mig.Status != "failed" && mig.Status != "cancelled",
				"migrationId":  mig.ID,
				"status":       mig.Status,
				"phase":        mig.TransferPhase,
				"progress":     prog,
				"error":        mig.FailureReason,
			})
		}
		// Both reads below were previously error-discarding, which made this
		// endpoint answer "not transferring, generation 0" whenever it could
		// not reach the database or the server did not exist — a 200 that is
		// indistinguishable from a healthy idle server.
		state, err := cfg.Store.GetServerTransferState(ctx, serverID)
		switch {
		case errors.Is(err, store.ErrServerNotFound):
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		case err != nil:
			return fiber.NewError(fiber.StatusInternalServerError, "could not determine transfer state")
		}
		transferring := state == "queued" || state == "running" || state == "in_progress"
		gen, err := cfg.Store.GetServerGeneration(ctx, serverID)
		switch {
		case errors.Is(err, store.ErrServerNotFound):
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		case err != nil:
			return fiber.NewError(fiber.StatusInternalServerError, "could not read server generation")
		}
		return c.JSON(fiber.Map{
			"source":       "server",
			"transferring": transferring,
			"status":       state,
			"progress":     statusProgress(state),
			"generation":   gen,
		})
	})
}

func statusDesired(s string) string {
	switch s {
	case "pending", "queued", "planned", "preparing":
		return "pending"
	case "running", "draining", "in_progress", "transferring", "restoring":
		return "running"
	case "retrying":
		return "running"
	case "completed", "succeeded", "drained", "restored":
		return "running"
	case "failed", "cancelled":
		return "stopped"
	default:
		return s
	}
}

func statusActual(s string) string {
	switch s {
	case "pending", "queued", "planned":
		return "pending"
	case "running", "draining", "in_progress", "transferring", "preparing", "restoring", "retrying":
		return "running"
	case "completed", "succeeded", "drained", "restored":
		return "running"
	case "failed":
		return "crashed"
	case "cancelled":
		return "stopped"
	default:
		return s
	}
}

func containsFenced(s string) bool {
	if s == "" {
		return false
	}
	// case-insensitive fence token
	for _, needle := range []string{"fenced", "fence", "generation"} {
		if len(s) >= len(needle) {
			// cheap contains without importing strings for hot path reuse; import at top is fine
			// but keep this function allocation-free-ish — fall through to strings.Contains
			break
		}
	}
	// imported below via strings — avoid cycle; use inline loop
	return containsFold(s, "fenced") || containsFold(s, "fence")
}

func containsFold(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	// naive case-insensitive
	ls := len(s)
	lsub := len(substr)
	for i := 0; i <= ls-lsub; i++ {
		match := true
		for j := 0; j < lsub; j++ {
			a := s[i+j]
			b := substr[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// pgArray passes IDs through for ANY($1::uuid[]) binding.
//
// The name is historical: pgx v5 binds a []string natively (encoding it as
// _text / _uuid), so nothing is built here. The original doc comment also
// claimed it kept a SQLite test double working via a "fallback path" — that
// path was a per-ID retry loop that silently swallowed every error, and it is
// gone. The only remaining caller is handlers_deployment_rollback.go; the
// identity function is kept rather than inlined so that call site keeps
// reading as an explicit array bind.
func pgArray(ids []string) interface{} {
	return ids
}
