// Package billing implements the Phase 7 commerce v1: plans/tiers, per-org
// quotas, a usage meter, and a generic (processor-agnostic) webhook receiver.
//
// No Stripe SDK is linked. The integration seam for a real processor lives in
// HandleWebhookEvent: adapters map processor events ("payment.intent.succeeded",
// "customer.subscription.updated", ...) onto the same primitives the admin API
// already exposes (SetOrgPlan, RecordUsage). The receiver deliberately echoes
// "accepted but not charged" until an adapter is wired.
package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gamepanel/forge/internal/store"
)

// ErrQuotaExceeded mirrors store.ErrUserLimitExceeded: handlers map it to
// HTTP 422 so clients receive the same 422 semantics as per-user limits.
type ErrQuotaExceeded struct {
	OrgID    string
	Resource string
	Limit    int64
	Current  int64
	Request  int64
}

func (e *ErrQuotaExceeded) Error() string {
	return fmt.Sprintf("org %s quota exceeded for %s (%d + %d > %d)",
		e.OrgID, e.Resource, e.Current, e.Request, e.Limit)
}

// IsQuotaExceeded reports whether err is a billing quota rejection.
func IsQuotaExceeded(err error) bool {
	var e *ErrQuotaExceeded
	return errors.As(err, &e)
}

// Store is the persistence seam consumed by the service. *store.Store
// satisfies it; tests substitute an in-memory fake.
type Store interface {
	GetBillingPlanByCode(ctx context.Context, code string) (*store.BillingPlan, error)
	ListBillingPlans(ctx context.Context) ([]store.BillingPlan, error)
	GetBillingPlanByID(ctx context.Context, id string) (*store.BillingPlan, error)
	CreateBillingPlan(ctx context.Context, p store.BillingPlan) (*store.BillingPlan, error)
	UpdateBillingPlan(ctx context.Context, id, name string, cents *int64, entitlements json.RawMessage, trialDays *int, active *bool) (*store.BillingPlan, error)
	DeleteBillingPlan(ctx context.Context, id string) error

	GetOrgQuota(ctx context.Context, orgID string) (*store.OrgQuota, error)
	EnsureOrgQuota(ctx context.Context, orgID string) (*store.OrgQuota, error)
	SetOrgPlan(ctx context.Context, orgID, planCode string, trialUntil *time.Time) error
	RecomputeOrgCounters(ctx context.Context, orgID string) error
	RecordUsageCounter(ctx context.Context, orgID, resource string, quantity int64) error

	InsertUsageEvent(ctx context.Context, orgID, resource string, quantity int64, kind string) error
	ListUsageEvents(ctx context.Context, orgID string, since time.Time, limit int) ([]store.UsageEvent, error)
	SummarizeUsageEvents(ctx context.Context, since time.Time, retention time.Duration) error

	InsertBillingWebhookEvent(ctx context.Context, e store.BillingWebhookEvent) (*store.BillingWebhookEvent, error)
	GetBillingWebhookEvent(ctx context.Context, provider, eventID string) (*store.BillingWebhookEvent, error)
	GetBillingSettings(ctx context.Context) (store.BillingSettings, error)
	UpdateBillingSettings(ctx context.Context, secret, processor string) (store.BillingSettings, error)
}

// Service holds billing logic. It is safe for concurrent use.
type Service struct {
	store Store
}

func New(st Store) *Service {
	return &Service{store: st}
}

// Entitlements mirrors the JSON document stored on billing_plans.
type Entitlements struct {
	MaxServers     int64    `json:"max_servers"`
	MaxMemoryGB    int64    `json:"max_memory_gb"`
	MaxEnvironments int64   `json:"max_environments"`
	MaxNodes       int64    `json:"max_nodes"`
	PricePerGB     int64    `json:"price_per_gb"`
	Features       []string `json:"features"`
}

func parseEntitlements(raw json.RawMessage) (Entitlements, error) {
	var e Entitlements
	if len(raw) == 0 {
		return e, nil
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return e, fmt.Errorf("parse plan entitlements: %w", err)
	}
	return e, nil
}

func marshalEntitlements(e Entitlements) json.RawMessage {
	raw, _ := json.Marshal(e)
	return raw
}

// GetPlanForOrg resolves the org's plan, falling back to the free plan when
// the org has no quota row yet.
func (s *Service) GetPlanForOrg(ctx context.Context, orgID string) (*store.BillingPlan, error) {
	quota, err := s.store.GetOrgQuota(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("resolve org quota: %w", err)
	}
	code := "free"
	if quota != nil && quota.PlanCode != "" {
		code = quota.PlanCode
	}
	plan, err := s.store.GetBillingPlanByCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("resolve org plan: %w", err)
	}
	if plan == nil {
		plan, err = s.store.GetBillingPlanByCode(ctx, "free")
		if err != nil {
			return nil, fmt.Errorf("resolve fallback plan: %w", err)
		}
	}
	return plan, nil
}

// CheckQuotaBeforeCreate enforces a plan entitlement against the org's live
// counters BEFORE a resource is created. It is the hook create paths call
// {servers, environments, allocations}: pass the org ID, the resource name
// ("servers", "memory", "storage", "environments", "nodes") and the amount
// being added. Zero entitlements mean unlimited.
//
// value = 0 for count resources and memory/storage bytes (amount is bytes).
func (s *Service) CheckQuotaBeforeCreate(ctx context.Context, orgID, resource string, amount int64) error {
	plan, err := s.GetPlanForOrg(ctx, orgID)
	if err != nil {
		return fmt.Errorf("quota check prereq: %w", err)
	}
	ent, err := parseEntitlements(plan.Entitlements)
	if err != nil {
		return fmt.Errorf("quota check entitlements: %w", err)
	}

	// Recompute from authoritative tables so stale projections cannot be
	// exploited by concurrent create paths.
	if err := s.store.RecomputeOrgCounters(ctx, orgID); err != nil {
		return fmt.Errorf("quota check recompute: %w", err)
	}
	quota, err := s.store.EnsureOrgQuota(ctx, orgID)
	if err != nil {
		return fmt.Errorf("quota check ensure: %w", err)
	}

	switch resource {
	case "servers":
		if ent.MaxServers > 0 {
			if quota.ServersCount+amount > ent.MaxServers {
				return &ErrQuotaExceeded{OrgID: orgID, Resource: resource, Limit: ent.MaxServers, Current: quota.ServersCount, Request: amount}
			}
		}
	case "memory":
		if ent.MaxMemoryGB > 0 {
			limit := ent.MaxMemoryGB * 1024 * 1024
			if quota.MemoryUsageBytes+amount > limit {
				return &ErrQuotaExceeded{OrgID: orgID, Resource: resource, Limit: limit, Current: quota.MemoryUsageBytes, Request: amount}
			}
		}
	case "storage":
		limit := s.storageLimit(orgID, *plan, ent)
		if limit > 0 && quota.StorageBytes+amount > limit {
			return &ErrQuotaExceeded{OrgID: orgID, Resource: resource, Limit: limit, Current: quota.StorageBytes, Request: amount}
		}
	case "environments":
		if ent.MaxEnvironments > 0 {
			if quota.EnvironmentsCount+amount > ent.MaxEnvironments {
				return &ErrQuotaExceeded{OrgID: orgID, Resource: resource, Limit: ent.MaxEnvironments, Current: quota.EnvironmentsCount, Request: amount}
			}
		}
	case "nodes":
		if ent.MaxNodes > 0 {
			if quota.NodesCount+amount > ent.MaxNodes {
				return &ErrQuotaExceeded{OrgID: orgID, Resource: resource, Limit: ent.MaxNodes, Current: quota.NodesCount, Request: amount}
			}
		}
	default:
		return fmt.Errorf("unknown quotable resource %q", resource)
	}
	return nil
}

// storageLimit derives the storage cap from the memory cap (a standard
// oversell-free ratio) until entitlements carry an explicit storage field.
func (s *Service) storageLimit(_ string, _ store.BillingPlan, ent Entitlements) int64 {
	return ent.MaxMemoryGB * 4 * 1024 * 1024 // 4x memory, in bytes
}

// EnforceQuota is an alias over CheckQuotaBeforeCreate kept for callers that
// think in "enforce before write" terms.
func (s *Service) EnforceQuota(ctx context.Context, orgID, resource string, amount int64) error {
	return s.CheckQuotaBeforeCreate(ctx, orgID, resource, amount)
}

// RecordUsage appends a meter event. The reaper folds meter events into the
// projected counters each hour.
func (s *Service) RecordUsage(ctx context.Context, orgID, resource string, quantity int64) error {
	if quantity <= 0 {
		return nil
	}
	if err := s.store.InsertUsageEvent(ctx, orgID, resource, quantity, "meter"); err != nil {
		return fmt.Errorf("record usage: %w", err)
	}
	return s.store.RecordUsageCounter(ctx, orgID, resource, quantity)
}

// UsageSummary is the org-facing usage view.
type UsageSummary struct {
	OrgID   string `json:"orgId"`
	Plan    string `json:"plan"`
	Servers int64  `json:"servers"`
	Memory  int64  `json:"memoryBytes"`
	Storage int64  `json:"storageBytes"`
	Env     int64  `json:"environments"`
	Nodes   int64  `json:"nodes"`
	Events  int64  `json:"meterEvents"`
}

// GetOrgUsage returns live usage for the org's plan.
func (s *Service) GetOrgUsage(ctx context.Context, orgID string) (*UsageSummary, error) {
	if err := s.store.RecomputeOrgCounters(ctx, orgID); err != nil {
		return nil, fmt.Errorf("usage recompute: %w", err)
	}
	quota, err := s.store.GetOrgQuota(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("usage quota: %w", err)
	}
	events, err := s.store.ListUsageEvents(ctx, orgID, time.Now().Add(-24*time.Hour), 0)
	if err != nil {
		return nil, fmt.Errorf("usage events: %w", err)
	}
	summary := &UsageSummary{OrgID: orgID, Plan: "free"}
	if quota != nil {
		summary.Plan = quota.PlanCode
		summary.Servers = quota.ServersCount
		summary.Memory = quota.MemoryUsageBytes
		summary.Storage = quota.StorageBytes
		summary.Environments = quota.EnvironmentsCount
		summary.Nodes = quota.NodesCount
	}
	summary.Events = int64(len(events))
	return summary, nil
}

// SetOrgPlan moves an org onto a plan; isTrial gives trial_days of runway.
func (s *Service) SetOrgPlan(ctx context.Context, orgID, planCode string, isTrial bool) (*store.BillingPlan, error) {
	plan, err := s.store.GetBillingPlanByCode(ctx, planCode)
	if err != nil {
		return nil, fmt.Errorf("set org plan lookup: %w", err)
	}
	if plan == nil || !plan.Active {
		return nil, fmt.Errorf("billing plan %q does not exist or is inactive", planCode)
	}
	var trialUntil *time.Time
	if isTrial && plan.TrialDays > 0 {
		until := time.Now().UTC().Add(time.Duration(plan.TrialDays) * 24 * time.Hour)
		trialUntil = &until
	}
	if err := s.store.SetOrgPlan(ctx, orgID, planCode, trialUntil); err != nil {
		return nil, fmt.Errorf("assign org plan: %w", err)
	}
	return plan, nil
}

// WebhookReceipt is the HTTP response for a received webhook.
type WebhookReceipt struct {
	Accepted bool   `json:"accepted"`
	Status   string `json:"status"`
	EventID  string `json:"eventId,omitempty"`
	Mapped   bool   `json:"mapped"`
	Note     string `json:"note,omitempty"`
}

func noteFromStatus(status string) string {
	switch status {
	case "mapped":
		return "applied to plan/quota state"
	case "received":
		return "accepted; no processor adapter mapped this event yet (not charging externally)"
	default:
		return "accepted"
	}
}

// mapEvent is the generic processor-agnostic mapper: provider events collapse
// onto the same primitives the admin API uses (SetOrgPlan, RecordUsage).
//
// TODO(stripe-adapter): once a real processor ships, replace this switch with
// a `type WebhookAdapter interface { Provider() string; Apply(ctx context.Context,
// event WebhookEvent) (WebhookOutcome, error) }` and register Stripe there.
// The table below is deliberately small so the seam stays obvious.
func (s *Service) mapEvent(ctx context.Context, provider, eventType string, payload []byte) (*string, string, int64, bool) {
	var body struct {
		OrgID    string `json:"org_id"`
		PlanCode string `json:"plan_code"`
		Resource string `json:"resource"`
		Quantity int64  `json:"quantity"`
		Data     struct {
			Object struct {
				Metadata map[string]string `json:"metadata"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, "", 0, false
	}
	orgID := body.OrgID
	if orgID == "" && body.Data.Object.Metadata != nil {
		orgID = body.Data.Object.Metadata["org_id"]
	}
	if orgID == "" {
		return nil, "", 0, false
	}
	orgIDPtr := &orgID

	switch eventType {
	case "billing.plan.updated", "customer.subscription.updated", "subscription.updated":
		if body.PlanCode != "" {
			if _, err := s.SetOrgPlan(ctx, orgID, body.PlanCode, false); err == nil {
				return orgIDPtr, "plan", 1, true
			}
		}
	case "billing.meter.increment", "meter.record", "payment.intent.succeeded":
		resource := body.Resource
		quantity := body.Quantity
		if quantity <= 0 {
			quantity = 1
		}
		if resource != "" {
			if err := s.RecordUsage(ctx, orgID, resource, quantity); err == nil {
				return orgIDPtr, resource, quantity, true
			}
		}
	}
	return orgIDPtr, "", 0, false
}

// ProcessWebhook verifies-and-applies a webhook event with idempotency:
// a provider/event_id pair is applied exactly once.
func (s *Service) ProcessWebhook(ctx context.Context, provider, eventID, eventType string, payload []byte) (*WebhookReceipt, error) {
	if provider == "" {
		provider = "generic"
	}
	existing, err := s.store.GetBillingWebhookEvent(ctx, provider, eventID)
	if err != nil {
		return nil, fmt.Errorf("webhook idempotency check: %w", err)
	}
	if existing != nil {
		return &WebhookReceipt{Accepted: true, Status: existing.Status, EventID: existing.ID, Mapped: existing.Status == "mapped", Note: "already processed"}, nil
	}

	orgID, resource, quantity, mapped := s.mapEvent(ctx, provider, eventType, payload)

	status := "received"
	if mapped {
		status = "mapped"
	}

	event, err := s.store.InsertBillingWebhookEvent(ctx, store.BillingWebhookEvent{
		Provider:   provider,
		EventID:    eventID,
		EventType:  eventType,
		OrgID:      orgID,
		Status:     status,
		RawPayload: string(payload),
	})
	if err != nil {
		return nil, fmt.Errorf("persist webhook event: %w", err)
	}
	_ = resource
	_ = quantity

	return &WebhookReceipt{Accepted: true, Status: status, EventID: event.ID, Mapped: mapped, Note: noteFromStatus(status)}, nil
}

// VerifyWebhookSignature is a constant-time HMAC check over the raw body
// using the shared secret configured in billing_settings. Header formats
// accepted: "sha256=<hex>" (GitHub style) or bare hex.
func (s *Service) VerifyWebhookSignature(ctx context.Context, body []byte, signature string) (bool, error) {
	if signature == "" {
		return false, nil
	}
	settings, err := s.store.GetBillingSettings(ctx)
	if err != nil {
		return false, fmt.Errorf("webhook settings: %w", err)
	}
	if !settings.HasWebhookSecret || settings.WebhookSecret == "" {
		return false, fmt.Errorf("billing webhook secret is not configured")
	}
	expected := settings.WebhookSecret
	if len(signature) > 7 && signature[:7] == "sha256=" {
		signature = signature[7:]
	}
	actual := hmacSHA256Hex(body, expected)
	return hmacConstantTimeEqual(actual, signature), nil
}

// UsageReaper rolls usage_events into org_quotas every interval and prunes
// events older than retention. Run it from the package goroutine (StartReaper).
func (s *Service) UsageReaper(ctx context.Context, interval time.Duration, retention time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.store.SummarizeUsageEvents(ctx, time.Now().UTC().Add(-2*interval), retention)
		}
	}
}

// StartReaper launches the hourly usage summarizer in a background goroutine.
// The returned stop func cancels the loop.
func (s *Service) StartReaper(ctx context.Context) context.CancelFunc {
	reaperCtx, cancel := context.WithCancel(ctx)
	go s.UsageReaper(reaperCtx, time.Hour, 7*24*time.Hour)
	return cancel
}