package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// BillingPlan is a purchasable tier. Entitlements is the JSON payload defined
// in migration 195 (max_servers, max_memory_gb, max_environments, max_nodes,
// price_per_gb, features[]).
type BillingPlan struct {
	ID            string          `json:"id"`
	Code          string          `json:"code"`
	Name          string          `json:"name"`
	CentsPerMonth int64           `json:"centsPerMonth"`
	Entitlements  json.RawMessage `json:"entitlements"`
	TrialDays     int             `json:"trialDays"`
	Active        bool            `json:"active"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

// OrgQuota is the per-organization consumption row. Counters are a reaper
// projection; the authoritative values always come from a plan + live tables.
type OrgQuota struct {
	OrgID             string    `json:"orgId"`
	PlanCode          string    `json:"planCode"`
	TrialUntil        *time.Time `json:"trialUntil,omitempty"`
	MemoryUsageBytes  int64     `json:"memoryUsageBytes"`
	ServersCount      int64     `json:"serversCount"`
	EnvironmentsCount int64     `json:"environmentsCount"`
	NodesCount        int64     `json:"nodesCount"`
	StorageBytes      int64     `json:"storageBytes"`
	PrevPlanCode      string    `json:"prevPlanCode"`
	QuotaUpdatedAt    time.Time `json:"quotaUpdatedAt"`
}

// UsageEvent is a single meter reading for an org resource.
type UsageEvent struct {
	ID         string    `json:"id"`
	OrgID      string    `json:"orgId"`
	Resource   string    `json:"resource"`
	Quantity   int64     `json:"quantity"`
	Kind       string    `json:"kind"`
	OccurredAt time.Time `json:"occurredAt"`
}

// BillingWebhookEvent is a received, signature-verified webhook payload.
type BillingWebhookEvent struct {
	ID         string    `json:"id"`
	Provider   string    `json:"provider"`
	EventID    string    `json:"eventId"`
	EventType  string    `json:"eventType"`
	OrgID      *string   `json:"orgId,omitempty"`
	Status     string    `json:"status"`
	RawPayload string    `json:"rawPayload,omitempty"`
	GotAt      time.Time `json:"gotAt"`
}

// BillingSettings stores the shared webhook HMAC secret and the name of the
// (optional) external processor. 'none' until a Stripe-style adapter lands.
type BillingSettings struct {
	ID                bool      `json:"id"`
	WebhookSecret     string    `json:"-"`
	HasWebhookSecret  bool      `json:"hasWebhookSecret"`
	ExternalProcessor string    `json:"externalProcessor"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

const defaultBillingPlan = "free"

// ---- Plans ----

func scanBillingPlan(row interface{ Scan(...any) error }) (BillingPlan, error) {
	var p BillingPlan
	var entitlements []byte
	err := row.Scan(&p.ID, &p.Code, &p.Name, &p.CentsPerMonth,
		&entitlements, &p.TrialDays, &p.Active, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return BillingPlan{}, err
	}
	p.Entitlements = json.RawMessage(entitlements)
	if len(p.Entitlements) == 0 {
		p.Entitlements = json.RawMessage(`{}`)
	}
	return p, nil
}

func (s *Store) ListBillingPlans(ctx context.Context) ([]BillingPlan, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, code, name, cents_per_month, entitlements, trial_days, active, created_at, updated_at
		FROM billing_plans ORDER BY cents_per_month ASC, code ASC`)
	if err != nil {
		return nil, fmt.Errorf("list billing plans: %w", err)
	}
	defer rows.Close()

	plans := make([]BillingPlan, 0, 8)
	for rows.Next() {
		p, err := scanBillingPlan(rows)
		if err != nil {
			return nil, fmt.Errorf("scan billing plan: %w", err)
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

func (s *Store) GetBillingPlanByCode(ctx context.Context, code string) (*BillingPlan, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id::text, code, name, cents_per_month, entitlements, trial_days, active, created_at, updated_at
		FROM billing_plans WHERE code = $1`, code)
	p, err := scanBillingPlan(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get billing plan %s: %w", code, err)
	}
	return &p, nil
}

func (s *Store) GetBillingPlanByID(ctx context.Context, id string) (*BillingPlan, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id::text, code, name, cents_per_month, entitlements, trial_days, active, created_at, updated_at
		FROM billing_plans WHERE id = $1`, id)
	p, err := scanBillingPlan(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get billing plan %s: %w", id, err)
	}
	return &p, nil
}

func (s *Store) CreateBillingPlan(ctx context.Context, p BillingPlan) (*BillingPlan, error) {
	if p.Code == "" || p.Name == "" {
		return nil, errors.New("plan code and name are required")
	}
	if len(p.Entitlements) == 0 {
		p.Entitlements = json.RawMessage(`{}`)
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO billing_plans (id, code, name, cents_per_month, entitlements, trial_days, active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)`,
		id, p.Code, p.Name, p.CentsPerMonth, []byte(p.Entitlements), p.TrialDays, p.Active, now)
	if err != nil {
		return nil, fmt.Errorf("create billing plan %s: %w", p.Code, err)
	}
	return s.GetBillingPlanByID(ctx, id)
}

func (s *Store) UpdateBillingPlan(ctx context.Context, id, name string, centsPerMonth *int64, entitlements json.RawMessage, trialDays *int, active *bool) (*BillingPlan, error) {
	cur, err := s.GetBillingPlanByID(ctx, id)
	if err != nil || cur == nil {
		return nil, errors.New("billing plan not found")
	}
	if name != "" {
		cur.Name = name
	}
	if centsPerMonth != nil {
		cur.CentsPerMonth = *centsPerMonth
	}
	if len(entitlements) > 0 {
		cur.Entitlements = entitlements
	}
	if trialDays != nil {
		cur.TrialDays = *trialDays
	}
	if active != nil {
		cur.Active = *active
	}
	_, err = s.db.Exec(ctx, `
		UPDATE billing_plans SET name = $2, cents_per_month = $3, entitlements = $4, trial_days = $5, active = $6, updated_at = now()
		WHERE id = $1`,
		id, cur.Name, cur.CentsPerMonth, []byte(cur.Entitlements), cur.TrialDays, cur.Active)
	if err != nil {
		return nil, fmt.Errorf("update billing plan %s: %w", id, err)
	}
	return s.GetBillingPlanByID(ctx, id)
}

func (s *Store) DeleteBillingPlan(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM billing_plans WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete billing plan %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("billing plan not found")
	}
	return nil
}

// ---- Org quotas ----

func (s *Store) GetOrgQuota(ctx context.Context, orgID string) (*OrgQuota, error) {
	row := s.db.QueryRow(ctx, `
		SELECT org_id::text, plan_code, trial_until, memory_usage_bytes, servers_count,
		       environments_count, nodes_count, storage_bytes, prev_plan_code, quota_updated_at
		FROM org_quotas WHERE org_id = $1`, orgID)

	var q OrgQuota
	err := row.Scan(&q.OrgID, &q.PlanCode, &q.TrialUntil, &q.MemoryUsageBytes,
		&q.ServersCount, &q.EnvironmentsCount, &q.NodesCount, &q.StorageBytes, &q.PrevPlanCode, &q.QuotaUpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get org quota %s: %w", orgID, err)
	}
	return &q, nil
}

// EnsureOrgQuota returns the org's quota row, creating a default (free plan)
// row on first touch.
func (s *Store) EnsureOrgQuota(ctx context.Context, orgID string) (*OrgQuota, error) {
	q, err := s.GetOrgQuota(ctx, orgID)
	if err != nil || q != nil {
		return q, err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO org_quotas (org_id, plan_code) VALUES ($1, $2)
		ON CONFLICT (org_id) DO NOTHING`, orgID, defaultBillingPlan)
	if err != nil {
		return nil, fmt.Errorf("ensure org quota %s: %w", orgID, err)
	}
	return s.GetOrgQuota(ctx, orgID)
}

// SetOrgPlan moves the org onto a new plan, preserving the previous code so
// downgrades can be audited and reverted by operators.
func (s *Store) SetOrgPlan(ctx context.Context, orgID, planCode string, trialUntil *time.Time) error {
	if _, err := s.EnsureOrgQuota(ctx, orgID); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `
		UPDATE org_quotas
		SET plan_code = $2, trial_until = $3,
		    prev_plan_code = plan_code,
		    quota_updated_at = now()
		WHERE org_id = $1`, orgID, planCode, trialUntil)
	if err != nil {
		return fmt.Errorf("set org plan %s -> %s: %w", orgID, planCode, err)
	}
	return nil
}

// RecomputeOrgCounters re-derives the live counters from the authoritative
// tables (servers, environments, projects) so quota checks never trust a
// stale write-projection.
func (s *Store) RecomputeOrgCounters(ctx context.Context, orgID string) error {
	if _, err := s.EnsureOrgQuota(ctx, orgID); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `
		UPDATE org_quotas
		SET servers_count = (SELECT count(*) FROM servers WHERE org_id = $1),
		    environments_count = (
		        SELECT count(*) FROM environments e
		        JOIN projects p ON p.id = e.project_id
		        WHERE p.org_id = $1
		    ),
		    memory_usage_bytes = (SELECT COALESCE(sum(memory_mb), 0) * 1024 * 1024 FROM servers WHERE org_id = $1),
		    storage_bytes = (SELECT COALESCE(sum(disk_mb), 0) * 1024 * 1024 FROM servers WHERE org_id = $1),
		    quota_updated_at = now()
		WHERE org_id = $1`, orgID)
	if err != nil {
		return fmt.Errorf("recompute org counters %s: %w", orgID, err)
	}
	return nil
}

// ---- Usage meter ----

func (s *Store) InsertUsageEvent(ctx context.Context, orgID, resource string, quantity int64, kind string) error {
	if quantity <= 0 {
		return nil
	}
	if _, err := s.EnsureOrgQuota(ctx, orgID); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO usage_events (id, org_id, resource, quantity, kind, occurred_at)
		VALUES ($1, $2, $3, $4, $5, now())`,
		uuid.NewString(), orgID, resource, quantity, kind)
	if err != nil {
		return fmt.Errorf("insert usage event org=%s resource=%s: %w", orgID, resource, err)
	}
	return nil
}

func (s *Store) ListUsageEvents(ctx context.Context, orgID string, since time.Time, limit int) ([]UsageEvent, error) {
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, org_id::text, resource, quantity, kind, occurred_at
		FROM usage_events WHERE org_id = $1 AND occurred_at >= $2
		ORDER BY occurred_at DESC LIMIT $3`, orgID, since, limit)
	if err != nil {
		return nil, fmt.Errorf("list usage events org=%s: %w", orgID, err)
	}
	defer rows.Close()

	events := make([]UsageEvent, 0, 16)
	for rows.Next() {
		var e UsageEvent
		if err := rows.Scan(&e.ID, &e.OrgID, &e.Resource, &e.Quantity, &e.Kind, &e.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan usage event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// SummarizeUsageEvents rolls usage_events up into org_quotas counters for the
// given window and then prunes events older than retention. It is called by
// the hourly reaper; data retention costs are bounded by design.
func (s *Store) SummarizeUsageEvents(ctx context.Context, since time.Time, retention time.Duration) error {
	rows, err := s.db.Query(ctx, `
		SELECT org_id::text, resource, COALESCE(sum(quantity), 0)
		FROM usage_events
		WHERE occurred_at >= $1
		GROUP BY org_id, resource`, since)
	if err != nil {
		return fmt.Errorf("summarize usage events: %w", err)
	}
	defer rows.Close()

	type aggregate struct {
		orgID    string
		resource string
		quantity int64
	}
	var aggregates []aggregate
	for rows.Next() {
		var a aggregate
		if err := rows.Scan(&a.orgID, &a.resource, &a.quantity); err != nil {
			return fmt.Errorf("scan usage aggregate: %w", err)
		}
		aggregates = append(aggregates, a)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, a := range aggregates {
		if _, err := s.EnsureOrgQuota(ctx, a.orgID); err != nil {
			return err
		}
		if err := s.RecordUsageCounter(ctx, a.orgID, a.resource, a.quantity); err != nil {
			return err
		}
	}

	if _, err := s.db.Exec(ctx, `DELETE FROM usage_events WHERE occurred_at < $1`,
		time.Now().UTC().Add(-retention)); err != nil {
		return fmt.Errorf("prune usage events: %w", err)
	}
	return nil
}

// RecordUsageCounter bumps the projected quota counter for a resource.
// Resource names: memory, storage, servers, environments, nodes.
func (s *Store) RecordUsageCounter(ctx context.Context, orgID, resource string, quantity int64) error {
	if _, err := s.EnsureOrgQuota(ctx, orgID); err != nil {
		return err
	}
	var query string
	switch resource {
	case "memory":
		query = `UPDATE org_quotas SET memory_usage_bytes = memory_usage_bytes + $2, quota_updated_at = now() WHERE org_id = $1`
	case "storage":
		query = `UPDATE org_quotas SET storage_bytes = storage_bytes + $2, quota_updated_at = now() WHERE org_id = $1`
	case "servers":
		query = `UPDATE org_quotas SET servers_count = servers_count + $2, quota_updated_at = now() WHERE org_id = $1`
	case "environments":
		query = `UPDATE org_quotas SET environments_count = environments_count + $2, quota_updated_at = now() WHERE org_id = $1`
	case "nodes":
		query = `UPDATE org_quotas SET nodes_count = nodes_count + $2, quota_updated_at = now() WHERE org_id = $1`
	default:
		return nil
	}
	if _, err := s.db.Exec(ctx, query, orgID, quantity); err != nil {
		return fmt.Errorf("record usage counter org=%s resource=%s: %w", orgID, resource, err)
	}
	return nil
}

// ---- Billing webhook events ----

func (s *Store) InsertBillingWebhookEvent(ctx context.Context, e BillingWebhookEvent) (*BillingWebhookEvent, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO billing_webhook_events (id, provider, event_id, event_type, org_id, status, raw_payload, got_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (provider, event_id) DO NOTHING`,
		id, e.Provider, e.EventID, e.EventType, e.OrgID, e.Status, e.RawPayload)
	if err != nil {
		return nil, fmt.Errorf("insert billing webhook event: %w", err)
	}
	stored := e
	stored.ID = id
	stored.GotAt = time.Now().UTC()
	return &stored, nil
}

func (s *Store) GetBillingWebhookEvent(ctx context.Context, provider, eventID string) (*BillingWebhookEvent, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id::text, provider, event_id, event_type, org_id::text, status, raw_payload, got_at
		FROM billing_webhook_events WHERE provider = $1 AND event_id = $2`, provider, eventID)
	var e BillingWebhookEvent
	var orgID *string
	var raw []byte
	if err := row.Scan(&e.ID, &e.Provider, &e.EventID, &e.EventType, &orgID, &e.Status, &raw, &e.GotAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get billing webhook event: %w", err)
	}
	e.OrgID = orgID
	e.RawPayload = string(raw)
	return &e, nil
}

// ---- Billing settings ----

func (s *Store) GetBillingSettings(ctx context.Context) (BillingSettings, error) {
	var settings BillingSettings
	var secret string
	err := s.db.QueryRow(ctx, `
		SELECT id, webhook_secret, external_processor, updated_at
		FROM billing_settings WHERE id = TRUE`).Scan(&settings.ID, &secret, &settings.ExternalProcessor, &settings.UpdatedAt)
	if err != nil {
		return BillingSettings{}, fmt.Errorf("get billing settings: %w", err)
	}
	settings.WebhookSecret = secret
	settings.HasWebhookSecret = secret != ""
	if !settings.HasWebhookSecret {
		settings.ExternalProcessor = "none"
	}
	return settings, nil
}

func (s *Store) UpdateBillingSettings(ctx context.Context, secret, processor string) (BillingSettings, error) {
	if processor == "" {
		processor = "none"
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO billing_settings (id, webhook_secret, external_processor, updated_at)
		VALUES (TRUE, $1, $2, now())
		ON CONFLICT (id) DO UPDATE SET webhook_secret = $1, external_processor = $2, updated_at = now()`,
		secret, processor)
	if err != nil {
		return BillingSettings{}, fmt.Errorf("update billing settings: %w", err)
	}
	return s.GetBillingSettings(ctx)
}