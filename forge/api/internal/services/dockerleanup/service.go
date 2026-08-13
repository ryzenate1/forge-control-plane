package dockerleanup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/robfig/cron/v3"
)

// Sentinel errors mapped to HTTP statuses by the handler layer.
var (
	// ErrNotFound is returned (wrapped) when a policy does not exist.
	ErrNotFound = errors.New("docker cleanup policy not found")
	// ErrValidation is returned (wrapped) for user-correctable input errors
	// such as an invalid cron expression or a nonsensical retention limit.
	ErrValidation = errors.New("invalid docker cleanup policy")
	// ErrDispatch is returned (wrapped) when Beacon could not be reached or
	// rejected the query/prune command.
	ErrDispatch = errors.New("dispatch to beacon failed")
)

// maxResponseBytes bounds how much of a Beacon disk-usage report we will read
// into memory. DiskUsage payloads are small, but a compromised or misbehaving
// node must never be able to OOM the control plane with an endless body.
const maxResponseBytes = 8 << 20

// dispatchTimeout bounds one Beacon round-trip. DiskUsage over a large image
// set can be slow, so this is more generous than the scheduled-task command
// timeout, but still finite so a wedged node cannot pin a scheduler goroutine.
const dispatchTimeout = 60 * time.Second

// Service owns disk-usage reporting, manual prune actions, cleanup-policy CRUD
// and the background scheduler loop (see scheduler.go).
type Service struct {
	store      *store.Store
	beacon     Beacon
	nodes      NodeResolver
	logger     *slog.Logger
	parser     cron.Parser
	instanceID string
}

// New builds the service. store must not be nil; beacon and nodes may be nil,
// in which case every dispatch fails loudly instead of pretending to succeed.
func New(st *store.Store, beacon Beacon, nodes NodeResolver, logger *slog.Logger, instanceID string) (*Service, error) {
	if st == nil {
		return nil, errors.New("dockerleanup: store required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if instanceID == "" {
		instanceID = fmt.Sprintf("api-%d", time.Now().UnixNano())
	}
	return &Service{
		store:      st,
		beacon:     beacon,
		nodes:      nodes,
		logger:     logger,
		parser:     cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
		instanceID: instanceID,
	}, nil
}

// ValidateSchedule reports whether expr is a legal 5-field cron expression.
func (s *Service) ValidateSchedule(expr string) error {
	if strings.TrimSpace(expr) == "" {
		return fmt.Errorf("%w: schedule is required", ErrValidation)
	}
	if len(expr) > 64 {
		return fmt.Errorf("%w: schedule too long (max 64 characters)", ErrValidation)
	}
	if _, err := s.parser.Parse(expr); err != nil {
		return fmt.Errorf("%w: invalid cron expression %q: %v", ErrValidation, expr, err)
	}
	return nil
}

// nextRun computes the occurrence strictly after from.
func (s *Service) nextRun(schedule string, from time.Time) (time.Time, error) {
	sched, err := s.parser.Parse(schedule)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: invalid cron expression %q: %v", ErrValidation, schedule, err)
	}
	return sched.Next(from), nil
}

// ---- disk usage & prune operations ----

// GetDiskUsage queries one node's Docker engine and returns its aggregate disk
// accounting plus the per-image breakdown used for unused-image analysis.
func (s *Service) GetDiskUsage(ctx context.Context, nodeID string) (*DiskUsage, error) {
	target, err := s.resolve(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	usage, err := s.beaconDiskUsage(ctx, target)
	if err != nil {
		return nil, err
	}
	usage.NodeID = target.ID
	usage.NodeName = target.Name
	usage.TotalBytes = usage.ImagesBytes + usage.ContainersBytes + usage.VolumesBytes + usage.BuildCacheBytes
	return usage, nil
}

// ListUnusedImages returns images that are safe to remove on a node: those not
// referenced by any container, excluding the most recent MostRecentLimit that
// are always preserved so the currently-deployed version is never stranded.
func (s *Service) ListUnusedImages(ctx context.Context, nodeID string, mostRecentLimit int) ([]ImageInfo, error) {
	if mostRecentLimit < 0 {
		mostRecentLimit = 0
	}
	usage, err := s.GetDiskUsage(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	return selectUnusedImages(usage.Images, mostRecentLimit), nil
}

// selectUnusedImages is the retention rule, isolated for testability. Of the
// images not in use, the N most recently created are kept and the rest become
// prune candidates; in-use images are never candidates regardless of age.
func selectUnusedImages(images []ImageInfo, mostRecentLimit int) []ImageInfo {
	var free []ImageInfo
	for _, img := range images {
		if !img.InUse {
			free = append(free, img)
		}
	}
	sort.SliceStable(free, func(i, j int) bool { return free[i].CreatedAt.After(free[j].CreatedAt) })
	if mostRecentLimit > 0 {
		if mostRecentLimit >= len(free) {
			return []ImageInfo{}
		}
		free = free[mostRecentLimit:]
	}
	return free
}

// PruneImages removes the given image IDs from a node. An empty imageIDs list
// is rejected: callers must resolve the concrete deletable set (via
// ListUnusedImages) so the destructive scope is always explicit.
func (s *Service) PruneImages(ctx context.Context, nodeID string, imageIDs []string) (*PruneResult, error) {
	target, err := s.resolve(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(imageIDs))
	for _, id := range imageIDs {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: at least one image id is required", ErrValidation)
	}
	res, err := s.beaconPrune(ctx, http.MethodPost, "/api/admin/docker-cleanup/prune-images", target, map[string]any{"imageIds": ids})
	if err != nil {
		return nil, err
	}
	res.NodeID = target.ID
	return res, nil
}

// PruneBuildCache reclaims the node's build (BuildKit) cache.
func (s *Service) PruneBuildCache(ctx context.Context, nodeID string) (*PruneResult, error) {
	target, err := s.resolve(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	res, err := s.beaconPrune(ctx, http.MethodPost, "/api/admin/docker-cleanup/prune-build-cache", target, nil)
	if err != nil {
		return nil, err
	}
	res.NodeID = target.ID
	return res, nil
}

// PruneVolumes removes dangling (unused) volumes on the node.
func (s *Service) PruneVolumes(ctx context.Context, nodeID string) (*PruneResult, error) {
	target, err := s.resolve(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	res, err := s.beaconPrune(ctx, http.MethodPost, "/api/admin/docker-cleanup/prune-volumes", target, nil)
	if err != nil {
		return nil, err
	}
	res.NodeID = target.ID
	return res, nil
}

// resolve maps a node id to its control target. A single-node policy path always
// carries a concrete id; empty is rejected here (global fan-out is a scheduler
// concern and resolves via nodes.All()).
func (s *Service) resolve(ctx context.Context, nodeID string) (NodeTarget, error) {
	if s.nodes == nil {
		return NodeTarget{}, fmt.Errorf("%w: node resolver unavailable", ErrDispatch)
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return NodeTarget{}, fmt.Errorf("%w: node id is required", ErrValidation)
	}
	target, err := s.nodes.Resolve(ctx, nodeID)
	if err != nil {
		return NodeTarget{}, fmt.Errorf("resolve node %s: %w", nodeID, err)
	}
	if strings.TrimSpace(target.URL) == "" {
		return NodeTarget{}, fmt.Errorf("%w: node %s has no base URL", ErrDispatch, nodeID)
	}
	return target, nil
}

// ---- Beacon transport ----

// beaconDiskUsage performs GET /api/admin/docker-cleanup/disk-usage and decodes
// the normalized report. Beacon populates everything except the node identity
// (added by the caller), keeping the wire contract stable across SDK upgrades.
func (s *Service) beaconDiskUsage(ctx context.Context, target NodeTarget) (*DiskUsage, error) {
	var out DiskUsage
	if err := s.beaconJSON(ctx, http.MethodGet, "/api/admin/docker-cleanup/disk-usage", target, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// beaconPrune POSTs a prune command and returns the reclaimed-space report.
func (s *Service) beaconPrune(ctx context.Context, method, path string, target NodeTarget, payload any) (*PruneResult, error) {
	var out PruneResult
	if err := s.beaconJSON(ctx, method, path, target, payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// beaconJSON signs and issues one admin request against Beacon, decodes the
// JSON response into dest, and translates transport/HTTP failures into the
// ErrDispatch family so the handler layer can render honest 502s.
func (s *Service) beaconJSON(ctx context.Context, method, path string, target NodeTarget, payload any, dest any) error {
	if s.beacon == nil {
		return fmt.Errorf("%w: daemon client unavailable", ErrDispatch)
	}

	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode beacon request: %w", err)
		}
	}

	endpoint := strings.TrimRight(target.URL, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, endpoint, newRequestBody(body))
	if err != nil {
		return fmt.Errorf("build beacon request: %w", err)
	}
	headers, err := s.beacon.SignedHeaders(target.Token, method, req.URL.RequestURI(), body)
	if err != nil {
		return fmt.Errorf("%w: sign request: %v", ErrDispatch, err)
	}
	for key, values := range headers {
		req.Header[key] = values
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := s.beacon.HTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDispatch, err)
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		details, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		msg := strings.TrimSpace(string(details))
		if msg == "" {
			msg = http.StatusText(res.StatusCode)
		}
		return fmt.Errorf("%w: beacon returned %d: %s", ErrDispatch, res.StatusCode, msg)
	}
	if dest == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxResponseBytes)).Decode(dest); err != nil {
		return fmt.Errorf("%w: decode beacon response: %v", ErrDispatch, err)
	}
	return nil
}

// newRequestBody returns a nil *bytes.Reader for empty bodies so the request
// carries no body (matching the daemon client), otherwise a reader over body.
func newRequestBody(body []byte) io.Reader {
	if len(body) == 0 {
		return nil
	}
	return bytes.NewReader(body)
}

// ---- policy CRUD ----

const policyColumns = `id, COALESCE(node_id::text, ''), schedule, most_recent_limit, enabled,
	prune_build_cache, prune_volumes, last_run_at, next_run_at,
	COALESCE(last_status, ''), COALESCE(last_error, ''), created_at, updated_at, COALESCE(created_by::text, '')`

func scanPolicy(row interface{ Scan(...any) error }) (CleanupPolicy, error) {
	var p CleanupPolicy
	if err := row.Scan(&p.ID, &p.NodeID, &p.Schedule, &p.MostRecentLimit, &p.Enabled,
		&p.PruneBuildCache, &p.PruneVolumes, &p.LastRunAt, &p.NextRunAt,
		&p.LastStatus, &p.LastError, &p.CreatedAt, &p.UpdatedAt, &p.CreatedBy); err != nil {
		return CleanupPolicy{}, err
	}
	return p, nil
}

// CreatePolicy validates input, seeds next_run_at and persists the policy.
func (s *Service) CreatePolicy(ctx context.Context, in CreatePolicyInput) (*CleanupPolicy, error) {
	if err := s.ValidateSchedule(strings.TrimSpace(in.Schedule)); err != nil {
		return nil, err
	}
	limit := in.MostRecentLimit
	if limit == 0 {
		limit = 1
	}
	if limit < 0 {
		return nil, fmt.Errorf("%w: most_recent_limit must be zero or greater", ErrValidation)
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	pruneBuildCache := true
	if in.PruneBuildCache != nil {
		pruneBuildCache = *in.PruneBuildCache
	}
	pruneVolumes := false
	if in.PruneVolumes != nil {
		pruneVolumes = *in.PruneVolumes
	}

	var nextRun *time.Time
	if enabled {
		next, err := s.nextRun(strings.TrimSpace(in.Schedule), time.Now().UTC())
		if err != nil {
			return nil, err
		}
		nextRun = &next
	}

	var nodeID *string
	if id := strings.TrimSpace(in.NodeID); id != "" {
		nodeID = &id
	}
	var createdBy *string
	if id := strings.TrimSpace(in.CreatedBy); id != "" {
		createdBy = &id
	}

	row := s.store.DB().QueryRow(ctx, `
		INSERT INTO docker_cleanup_policies
			(node_id, schedule, most_recent_limit, enabled, prune_build_cache, prune_volumes, next_run_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+policyColumns,
		nodeID, strings.TrimSpace(in.Schedule), limit, enabled, pruneBuildCache, pruneVolumes, nextRun, createdBy)

	p, err := scanPolicy(row)
	if err != nil {
		return nil, fmt.Errorf("create docker cleanup policy: %w", err)
	}
	return &p, nil
}

// GetPolicy fetches a policy by id.
func (s *Service) GetPolicy(ctx context.Context, id string) (*CleanupPolicy, error) {
	row := s.store.DB().QueryRow(ctx, `SELECT `+policyColumns+` FROM docker_cleanup_policies WHERE id = $1`, strings.TrimSpace(id))
	p, err := scanPolicy(row)
	if err != nil {
		return nil, notFoundOr(err)
	}
	return &p, nil
}

// ListPolicies returns every policy, newest first.
func (s *Service) ListPolicies(ctx context.Context) ([]CleanupPolicy, error) {
	rows, err := s.store.DB().Query(ctx, `SELECT `+policyColumns+` FROM docker_cleanup_policies ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list docker cleanup policies: %w", err)
	}
	defer rows.Close()
	policies := []CleanupPolicy{}
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, fmt.Errorf("scan docker cleanup policy: %w", err)
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

// UpdatePolicy applies a patch; schedule/enabled changes recompute next_run_at.
func (s *Service) UpdatePolicy(ctx context.Context, id string, in UpdatePolicyInput) (*CleanupPolicy, error) {
	existing, err := s.GetPolicy(ctx, id)
	if err != nil {
		return nil, err
	}

	schedule := existing.Schedule
	if in.Schedule != nil {
		if err := s.ValidateSchedule(strings.TrimSpace(*in.Schedule)); err != nil {
			return nil, err
		}
		schedule = strings.TrimSpace(*in.Schedule)
	}
	limit := existing.MostRecentLimit
	if in.MostRecentLimit != nil {
		if *in.MostRecentLimit < 0 {
			return nil, fmt.Errorf("%w: most_recent_limit must be zero or greater", ErrValidation)
		}
		limit = *in.MostRecentLimit
	}
	enabled := existing.Enabled
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	pruneBuildCache := existing.PruneBuildCache
	if in.PruneBuildCache != nil {
		pruneBuildCache = *in.PruneBuildCache
	}
	pruneVolumes := existing.PruneVolumes
	if in.PruneVolumes != nil {
		pruneVolumes = *in.PruneVolumes
	}
	nodeID := existing.NodeID
	if in.NodeID != nil {
		nodeID = strings.TrimSpace(*in.NodeID)
	}

	scheduleChanged := in.Schedule != nil || in.Enabled != nil
	var nextRun *time.Time
	if enabled {
		if scheduleChanged {
			next, err := s.nextRun(schedule, time.Now().UTC())
			if err != nil {
				return nil, err
			}
			nextRun = &next
		} else {
			nextRun = existing.NextRunAt
		}
	}

	var nodeArg *string
	if nodeID != "" {
		nodeArg = &nodeID
	}

	row := s.store.DB().QueryRow(ctx, `
		UPDATE docker_cleanup_policies
		SET node_id = $2, schedule = $3, most_recent_limit = $4, enabled = $5,
		    prune_build_cache = $6, prune_volumes = $7, next_run_at = $8, updated_at = now()
		WHERE id = $1
		RETURNING `+policyColumns,
		existing.ID, nodeArg, schedule, limit, enabled, pruneBuildCache, pruneVolumes, nextRun)

	updated, err := scanPolicy(row)
	if err != nil {
		return nil, notFoundOr(fmt.Errorf("update docker cleanup policy: %w", err))
	}
	return &updated, nil
}

// DeletePolicy removes a policy.
func (s *Service) DeletePolicy(ctx context.Context, id string) error {
	if _, err := s.GetPolicy(ctx, id); err != nil {
		return err
	}
	tag, err := s.store.DB().Exec(ctx, `DELETE FROM docker_cleanup_policies WHERE id = $1`, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("delete docker cleanup policy: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return nil
}

// notFoundOr keeps store lookup misses in the ErrNotFound family while
// surfacing genuine database errors unchanged.
func notFoundOr(err error) error {
	if err == nil || errors.Is(err, ErrNotFound) {
		return err
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	return err
}
