package redirects

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Service owns the redirect-rule lifecycle: what a rule may say, whether adding
// it would create a loop, and what the gateway should do about it.
//
// It is transport-agnostic on purpose. The HTTP handlers translate Fiber
// requests into these calls, and the deploy/gateway path calls
// GenerateGatewayConfig — so a rule means the same thing whether it was created
// from the panel, the API or ApplyPresets.
type Service struct {
	store Store
	// now is a seam for the timestamp defaults; production uses time.Now.
	now func() time.Time
}

// New builds a Service over any Store implementation.
func New(store Store) *Service {
	if store == nil {
		panic("redirects: New requires a non-nil store")
	}
	return &Service{store: store, now: time.Now}
}

// NewFromPool builds the production Service over the shared Postgres pool.
func NewFromPool(db *pgxpool.Pool) *Service {
	return New(NewPostgresStore(db))
}

// Store exposes the underlying persistence for the few callers that need direct
// access (the preset planner).
func (s *Service) Store() Store {
	return s.store
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

// List returns every rule for an application, ordered so the output is stable
// (source host, then path, then target host) and a refresh never reshuffles the
// panel.
func (s *Service) List(ctx context.Context, applicationID string) ([]Redirect, error) {
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return nil, ErrEmptyAppID
	}
	rules, err := s.store.ListByApplication(ctx, applicationID)
	if err != nil {
		return nil, err
	}
	if rules == nil {
		rules = []Redirect{}
	}
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].SourceDomain != rules[j].SourceDomain {
			return rules[i].SourceDomain < rules[j].SourceDomain
		}
		if rules[i].SourcePath != rules[j].SourcePath {
			return rules[i].SourcePath < rules[j].SourcePath
		}
		return rules[i].TargetDomain < rules[j].TargetDomain
	})
	return rules, nil
}

// Get returns one rule scoped to its application. A rule that exists but
// belongs to a different application is reported as not found — the same
// non-enumerable answer the application routes give.
func (s *Service) Get(ctx context.Context, applicationID, redirectID string) (Redirect, error) {
	applicationID = strings.TrimSpace(applicationID)
	redirectID = strings.TrimSpace(redirectID)
	if applicationID == "" {
		return Redirect{}, ErrEmptyAppID
	}
	if redirectID == "" {
		return Redirect{}, invalidf("redirect id is required")
	}
	return s.store.Get(ctx, applicationID, redirectID)
}

// Create stores one new rule after normalising and validating it.
func (s *Service) Create(ctx context.Context, applicationID string, req CreateRequest) (Redirect, error) {
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return Redirect{}, ErrEmptyAppID
	}

	sourceDomain, err := NormalizeDomain(req.SourceDomain)
	if err != nil {
		return Redirect{}, err
	}
	targetDomain, err := NormalizeDomain(req.TargetDomain)
	if err != nil {
		return Redirect{}, err
	}
	sourcePath := "/"
	if req.SourcePath != nil {
		sourcePath, err = NormalizePath(*req.SourcePath)
		if err != nil {
			return Redirect{}, err
		}
	}
	targetPath, err := NormalizeTargetPath(req.TargetPath)
	if err != nil {
		return Redirect{}, err
	}
	statusCode := DefaultStatusCode
	if req.StatusCode != nil {
		statusCode = *req.StatusCode
	}
	if !ValidStatusCode(statusCode) {
		return Redirect{}, invalidf("status code %d is not supported: choose 301, 302, 307 or 308", statusCode)
	}
	preset, err := ParsePresetType(string(req.PresetType))
	if err != nil {
		return Redirect{}, err
	}
	if preset == "" {
		preset = PresetCustom
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	candidate := Redirect{
		ApplicationID: applicationID,
		SourceDomain:  sourceDomain,
		TargetDomain:  targetDomain,
		SourcePath:    sourcePath,
		TargetPath:    targetPath,
		StatusCode:    statusCode,
		PresetType:    preset,
		Enabled:       enabled,
	}
	if err := s.check(ctx, candidate, nil); err != nil {
		return Redirect{}, err
	}
	return s.store.Create(ctx, candidate)
}

// Update patches one rule. Fields the payload omits keep their stored values;
// the candidate is re-validated in full, including the loop check, so an edit
// cannot turn a sane rule set into a ping-pong.
func (s *Service) Update(ctx context.Context, applicationID, redirectID string, req UpdateRequest) (Redirect, error) {
	current, err := s.Get(ctx, applicationID, redirectID)
	if err != nil {
		return Redirect{}, err
	}

	updated := current
	if req.SourceDomain != nil {
		host, err := NormalizeDomain(*req.SourceDomain)
		if err != nil {
			return Redirect{}, err
		}
		updated.SourceDomain = host
	}
	if req.TargetDomain != nil {
		host, err := NormalizeDomain(*req.TargetDomain)
		if err != nil {
			return Redirect{}, err
		}
		updated.TargetDomain = host
	}
	if req.SourcePath != nil {
		cleaned, err := NormalizePath(*req.SourcePath)
		if err != nil {
			return Redirect{}, err
		}
		updated.SourcePath = cleaned
	}
	switch {
	case req.ClearTargetPath:
		updated.TargetPath = nil
	case req.TargetPath != nil:
		normalized, err := NormalizeTargetPath(req.TargetPath)
		if err != nil {
			return Redirect{}, err
		}
		updated.TargetPath = normalized
	}
	if req.StatusCode != nil {
		if !ValidStatusCode(*req.StatusCode) {
			return Redirect{}, invalidf("status code %d is not supported: choose 301, 302, 307 or 308", *req.StatusCode)
		}
		updated.StatusCode = *req.StatusCode
	}
	if req.Enabled != nil {
		updated.Enabled = *req.Enabled
	}
	if req.PresetType != nil {
		preset, err := ParsePresetType(*req.PresetType)
		if err != nil {
			return Redirect{}, err
		}
		if preset != "" {
			updated.PresetType = preset
		}
	}

	// Re-derive the scheme-upgrade invariant: an edit that changes either host
	// can leave a "http-to-https" label attached to a rule that now crosses two
	// different domains, and a rule can be edited into pointing at itself.
	if updated.PresetType == PresetHTTPToHTTPS && updated.SourceDomain != updated.TargetDomain {
		updated.PresetType = PresetCustom
	}

	existing, err := s.store.ListByApplication(ctx, updated.ApplicationID)
	if err != nil {
		return Redirect{}, err
	}
	if err := s.check(ctx, updated, existing); err != nil {
		return Redirect{}, err
	}
	return s.store.Update(ctx, updated)
}

// Delete removes one rule. Removing a rule that does not exist for this
// application is an error, not a no-op: the caller must know nothing was
// deleted.
func (s *Service) Delete(ctx context.Context, applicationID, redirectID string) error {
	if _, err := s.Get(ctx, applicationID, redirectID); err != nil {
		return err
	}
	deleted, err := s.store.Delete(ctx, strings.TrimSpace(applicationID), strings.TrimSpace(redirectID))
	if err != nil {
		return err
	}
	if !deleted {
		return ErrRedirectNotFound
	}
	return nil
}

// SetEnabled is the one-flag convenience the gateway sync needs, and the shape
// most panel toggles send. It goes through the same validation as a full patch
// because disabling a rule can un-block the loop its sibling was waiting for.
func (s *Service) SetEnabled(ctx context.Context, applicationID, redirectID string, enabled bool) (Redirect, error) {
	return s.Update(ctx, applicationID, redirectID, UpdateRequest{Enabled: &enabled})
}

// ---------------------------------------------------------------------------
// Validation gate
// ---------------------------------------------------------------------------

// check runs the rule's own validation plus the loop/duplicate test against
// whatever is already stored. `existing` may be nil, in which case the stored
// rules are loaded — Create passes nil, Update passes the list it already read.
func (s *Service) check(ctx context.Context, candidate Redirect, existing []Redirect) error {
	if existing == nil {
		loaded, err := s.store.ListByApplication(ctx, candidate.ApplicationID)
		if err != nil {
			return err
		}
		existing = loaded
	}
	return validateCandidate(candidate, existing)
}
