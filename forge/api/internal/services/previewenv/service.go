// Package previewenv implements the Phase 4 preview environment lifecycle on
// top of the shared store/preview_deployments table. It parallels the older
// services/preview package without being wired into main: the HTTP registrar
// (internal/http/phase4_registrar.go) constructs it from cfg and owns its
// lifecycle (webhooks, reaper goroutine, commit-status worker).
package previewenv

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/services/acme"
	composesvc "gamepanel/forge/internal/services/compose"
	"gamepanel/forge/internal/services/domains"
	"gamepanel/forge/internal/services/git"
	"gamepanel/forge/internal/services/trafficmanager"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

var (
	ErrNotFound                = errors.New("preview deployment not found")
	ErrNotDeploying            = errors.New("preview deployment is not in deploying status")
	ErrAlreadyExists           = errors.New("active preview deployment already exists for this PR")
	ErrOrgLimitReached         = errors.New("preview limit reached for this organization")
	ErrSourceRequired          = errors.New("server_id and pr_number are required")
	ErrWebhookSignatureMissing = git.ErrWebhookSignatureMissing
	ErrWebhookSignatureInvalid = git.ErrWebhookSignatureInvalid
)

// Options configures lifecycle behavior. Every field has a default applied in
// New; callers are expected to pass what the environment already knows (the
// binary reads config itself in cmd/api/main.go), so this package never reads
// process environment directly.
type Options struct {
	// BaseDomain is the root domain previews are issued under; per-PR
	// hostnames look like pr<Number>-<owner>-<repo>.<BaseDomain>.
	BaseDomain string
	// TTL is how long a preview stays alive before the reaper destroys it.
	TTL time.Duration
	// MaxPerOrg caps concurrent live previews sharing repo_owner.
	MaxPerOrg int
	// RetainCleaned is how long cleaned_up rows are kept before deletion.
	RetainCleaned time.Duration

	// ---- project-scoped previews (preview_environments) ----

	// PreviewDomain is the wildcard root project previews are issued under;
	// hostnames look like <branch>-<project-slug>.<PreviewDomain>. Empty falls
	// back to defaultPreviewDomain.
	PreviewDomain string
	// PreviewTTL bounds how long a project preview lives when it was created
	// without an explicit expiry. Non-positive falls back to defaultPreviewTTL.
	PreviewTTL time.Duration
	// MaxPreviewsPerProject caps concurrent live previews per project. Negative
	// is normalized to defaultMaxPreviewsPerProject; an explicit zero means
	// "unlimited" so an operator can lift the cap without removing it.
	MaxPreviewsPerProject int
	// Compose is the compose lifecycle that actually runs previews. When it is
	// set (and Deployer is not) previews are provisioned for real; when neither
	// is set the preview surface reports ErrPreviewRuntimeUnavailable instead
	// of recording rows that can never run.
	Compose *composesvc.Service
	// Deployer runs the real workload behind a preview. Injected directly by
	// callers that do not use the compose lifecycle (tests, alternative
	// runtimes); it takes precedence over Compose.
	Deployer PreviewDeployer
	// PreviewStore overrides preview_environments persistence. Nil uses the
	// pool-backed store of the shared *store.Store.
	PreviewStore PreviewEnvStore
	// BackgroundContext anchors provisioning goroutines so process shutdown
	// cancels them. Nil falls back to context.Background().
	BackgroundContext context.Context
	// Preview limits are handed to the deployer verbatim; zero means the compose
	// service applies its own default rather than this package inventing one.
	PreviewMemoryMB  int64
	PreviewCPUShares int64
	PreviewDiskMB    int64

	Logger      *slog.Logger
	Publisher   events.Publisher
	GitService  *git.Service
	AcmeService *acme.Service
	TrafficMgr  *trafficmanager.Service
	DomainSvc   *domains.Service
	// PanelURL links commit-status target_url to the web UI environments page.
	PanelURL string
}

const (
	defaultBaseDomain            = "env.example.com"
	defaultTTL                   = 24 * time.Hour
	defaultMaxPerOrg             = 10
	defaultRetain                = 24 * time.Hour
	defaultPreviewTTL            = 7 * 24 * time.Hour
	defaultMaxPreviewsPerProject = 10
)

type Service struct {
	store *store.Store
	opts  Options

	// previewsStore and deployer back the project-scoped preview surface
	// (previews.go). Both are resolved in New and either may be nil, in which
	// case that surface reports ErrPreviewRuntimeUnavailable rather than
	// pretending a preview exists.
	previewsStore PreviewEnvStore
	deployer      PreviewDeployer
}

func New(s *store.Store, opts Options) *Service {
	if s == nil {
		return nil
	}
	if opts.BaseDomain == "" {
		opts.BaseDomain = defaultBaseDomain
	}
	opts.BaseDomain = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(opts.BaseDomain)), "*.")
	if opts.TTL <= 0 {
		opts.TTL = defaultTTL
	}
	if opts.MaxPerOrg <= 0 {
		opts.MaxPerOrg = defaultMaxPerOrg
	}
	if opts.RetainCleaned <= 0 {
		opts.RetainCleaned = defaultRetain
	}

	opts.PreviewDomain = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(opts.PreviewDomain)), "*.")
	if opts.PreviewTTL <= 0 {
		opts.PreviewTTL = defaultPreviewTTL
	}
	if opts.MaxPreviewsPerProject < 0 {
		opts.MaxPreviewsPerProject = defaultMaxPreviewsPerProject
	}

	svc := &Service{store: s, opts: opts}
	// Defaults are resolved here rather than in the constructing caller, so the
	// existing wiring (cmd/api/main.go) keeps compiling and still gains the
	// project-scoped surface as soon as a compose service is handed over.
	if svc.previewsStore = opts.PreviewStore; svc.previewsStore == nil {
		svc.previewsStore = newPreviewEnvStore(s)
	}
	switch {
	case opts.Deployer != nil:
		svc.deployer = opts.Deployer
	case opts.Compose != nil:
		svc.deployer = composePreviewDeployer{lifecycle: opts.Compose}
	}
	return svc
}

// PreviewURL builds the per-PR canonical hostname.
func (o Options) PreviewURL(prNumber int, repoOwner, repoName string) string {
	host := strings.ToLower(fmt.Sprintf("pr%d-%s-%s.%s",
		prNumber, sanitizeHostPart(repoOwner), sanitizeHostPart(repoName), o.BaseDomain))
	return "https://" + host
}

func sanitizeHostPart(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	part := strings.Trim(b.String(), "-")
	if part == "" {
		part = "env"
	}
	return part
}

// LoadTestsPolicy is unused; kept out of the public API.
// Create persists a deploying preview row enforcing the per-PR uniqueness
// (ErrAlreadyExists) and per-org limits (ErrOrgLimitReached). serverID must
// reference an existing server (FK constraint). Source values: "github",
// "gitlab", "bitbucket", "gitea", "manual".
func (s *Service) Create(ctx context.Context, serverID string, req *store.PreviewDeployment) (*store.PreviewDeployment, error) {
	if serverID == "" {
		return nil, fmt.Errorf("serverId is required")
	}
	if req.PRNumber <= 0 {
		return nil, fmt.Errorf("prNumber is required")
	}
	if req.RepoOwner == "" {
		return nil, fmt.Errorf("repoOwner is required")
	}
	if req.Source == "" {
		req.Source = "github"
	}

	// Lifecycle limit: max live previews per organization (repo_owner).
	count, err := s.store.CountActivePreviewDeploymentsForOrg(ctx, req.RepoOwner)
	if err != nil {
		return nil, fmt.Errorf("count active previews: %w", err)
	}
	if count >= s.opts.MaxPerOrg {
		return nil, fmt.Errorf("%w: %s has %d active previews", ErrOrgLimitReached, req.RepoOwner, count)
	}

	// Uniqueness: exactly one active preview per PR.
	actives, err := s.store.ListActivePreviewDeployments(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active previews: %w", err)
	}
	for _, p := range actives {
		if p.PRNumber == req.PRNumber &&
			strings.EqualFold(p.RepoOwner, req.RepoOwner) &&
			strings.EqualFold(p.RepoName, req.RepoName) {
			return nil, ErrAlreadyExists
		}
	}

	now := time.Now().UTC()
	suffix := uuid.NewString()[:8]
	expires := now.Add(s.opts.TTL)

	preview := &store.PreviewDeployment{
		ID:           uuid.NewString(),
		ServerID:     serverID,
		ServiceID:    req.ServiceID,
		PRNumber:     req.PRNumber,
		PRTitle:      req.PRTitle,
		PRURL:        req.PRURL,
		Branch:       req.Branch,
		RepoOwner:    req.RepoOwner,
		RepoName:     req.RepoName,
		CommitSHA:    req.CommitSHA,
		Status:       "deploying",
		Source:       req.Source,
		UniqueSuffix: suffix,
		IsIsolated:   true,
		CreatedBy:    req.CreatedBy,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.store.CreatePreviewDeployment(ctx, preview); err != nil {
		return nil, fmt.Errorf("create preview deployment: %w", err)
	}
	if err := s.store.SetPreviewDeploymentExpiresAt(ctx, preview.ID, &expires); err != nil {
		return nil, fmt.Errorf("set preview ttl: %w", err)
	}

	s.publish(ctx, "preview_deployment_created", preview.ID, map[string]any{
		"previewId": preview.ID,
		"prNumber":  preview.PRNumber,
		"branch":    preview.Branch,
		"serverId":  serverID,
	})

	// A "deploy" is issued for pushes and PR open/synchronize; the status
	// check shows pending until Deploy flips it to running.
	s.reportStatus(ctx, preview, "pending", "preview environment is being provisioned")
	return preview, nil
}

func (s *Service) Get(ctx context.Context, id string) (*store.PreviewDeployment, error) {
	p, err := s.store.GetPreviewDeployment(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	return &p, nil
}

// List returns all preview deployments for a server.
func (s *Service) List(ctx context.Context, serverID string) ([]store.PreviewDeployment, error) {
	return s.store.ListPreviewDeployments(ctx, serverID)
}

// ListAll returns every preview deployment across all servers (admin list view).
func (s *Service) ListAll(ctx context.Context) ([]store.PreviewDeployment, error) {
	return s.store.ListAllPreviewDeployments(ctx)
}

// UpdateStatus sets the lifecycle status and publishes the change so the UI and
// subscribers observe it. It does not itself provision or tear down any work.
func (s *Service) UpdateStatus(ctx context.Context, id string, status string) error {
	if err := s.store.UpdatePreviewDeploymentStatus(ctx, id, status); err != nil {
		return fmt.Errorf("update preview status: %w", err)
	}
	s.publish(ctx, "preview_deployment_status_changed", id, map[string]any{"status": status})
	return nil
}

// ListWithExpiry returns all rows including the computed expires_at, newest
// first — the data source for the environments UI.
func (s *Service) ListWithExpiry(ctx context.Context) ([]store.PreviewDeployment, error) {
	return s.store.ListPreviewDeploymentsWithExpiry(ctx)
}

func (s *Service) Active(ctx context.Context) ([]store.PreviewDeployment, error) {
	return s.store.ListActivePreviewDeployments(ctx)
}

// Deploy transitions a fresh row to running, derives the real per-PR URL,
// ensures TLS (ACME, when configured) and the gateway route (traffic
// manager, when configured), then reports commit status. ACME and traffic
// steps are fail-open: a missing or misconfigured dependency must never
// block the preview itself.
func (s *Service) Deploy(ctx context.Context, id string) error {
	p, err := s.store.GetPreviewDeployment(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	if p.Status == "running" {
		return nil
	}
	if p.Status != "deploying" {
		return ErrNotDeploying
	}

	previewURL := s.opts.PreviewURL(p.PRNumber, p.RepoOwner, p.RepoName)

	// TLS: guard so ACME only runs when the service is configured.
	if s.opts.AcmeService != nil {
		if err := s.ensureBaseCertificate(ctx); err != nil && s.opts.Logger != nil {
			s.opts.Logger.Warn("previewenv: acme certificate step skipped",
				"preview", id, "baseDomain", s.opts.BaseDomain, "err", err)
		}
	}

	// Routing: register the per-PR host with the traffic manager so the
	// gateway can serve it before the next reconcile tick.
	if s.opts.TrafficMgr != nil {
		if err := s.ensureRoutingRule(ctx, &p); err != nil && s.opts.Logger != nil {
			s.opts.Logger.Warn("previewenv: traffic route step skipped",
				"preview", p.ID, "host", previewURL, "err", err)
		}
	}

	// Domains: track the wildcard base so domain reverify picks previews up.
	if s.opts.DomainSvc != nil {
		if err := s.ensureWildcardDomain(ctx, &p); err != nil && s.opts.Logger != nil {
			s.opts.Logger.Warn("previewenv: wildcard domain step skipped",
				"preview", p.ID, "wildcard", "*."+s.opts.BaseDomain, "err", err)
		}
	}

	if err := s.store.UpdatePreviewDeployment(ctx, &store.PreviewDeployment{
		ID:            id,
		Status:        "running",
		PreviewURL:    previewURL,
		DeploymentURL: previewURL,
	}); err != nil {
		return fmt.Errorf("update preview deployment: %w", err)
	}

	s.publish(ctx, "preview_deployment_running", id, map[string]any{
		"previewUrl": previewURL,
		"prNumber":   p.PRNumber,
	})

	s.reportStatus(ctx, &p, "success", "preview environment is running at "+previewURL)
	return nil
}

// Cleanup marks a preview destroyed, publishes the event and withdraws the
// traffic route. Idempotent for already-cleaned rows.
func (s *Service) Cleanup(ctx context.Context, id string) error {
	p, err := s.store.GetPreviewDeployment(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	if p.Status == "cleaned_up" {
		return nil
	}

	if err := s.store.UpdatePreviewDeploymentStatus(ctx, id, "cleaned_up"); err != nil {
		return fmt.Errorf("cleanup preview: %w", err)
	}

	if s.opts.TrafficMgr != nil {
		if err := s.withdrawRoutingRule(ctx, id); err != nil && s.opts.Logger != nil {
			s.opts.Logger.Warn("previewenv: traffic route not withdrawn",
				"preview", id, "err", err)
		}
	}

	s.publish(ctx, "preview_deployment_cleaned_up", id, map[string]any{
		"previewId": id,
		"prNumber":  p.PRNumber,
		"cleanedAt": time.Now().UTC(),
	})

	s.reportStatus(ctx, &p, "failure", "preview environment was cleaned up")
	return nil
}

// Destroy cleans up the environment and removes the row outright (destroy
// button in the UI and reaper row expiry).
func (s *Service) Destroy(ctx context.Context, id string) error {
	p, err := s.store.GetPreviewDeployment(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	if p.Status != "cleaned_up" {
		if err := s.Cleanup(ctx, id); err != nil {
			return err
		}
	}
	if err := s.store.DeletePreviewDeployment(ctx, id); err != nil {
		return fmt.Errorf("delete preview: %w", err)
	}
	return nil
}

// ---- TLS / routing / DNS wiring ----

// ensureBaseCertificate issues (or reuses) a certificate covering the legacy
// preview base domain. ACME is service-gated at the caller; unknown errors are
// returned so Deploy can log and continue.
func (s *Service) ensureBaseCertificate(ctx context.Context) error {
	return s.ensureCertificateForDomain(ctx, s.opts.BaseDomain)
}

// ensureCertificateForDomain issues (or reuses) a certificate for domain. ACME
// is service-gated at the caller; unknown errors are returned so the caller can
// log and continue. Uses http-01 for the apex host which works for preview
// wildcards once the operator points DNS at the panel.
func (s *Service) ensureCertificateForDomain(ctx context.Context, domain string) error {
	if s.opts.AcmeService == nil || domain == "" {
		return nil
	}
	existing, err := s.opts.AcmeService.ListCertificates(ctx, store.CertificateFilter{})
	if err == nil {
		for _, c := range existing {
			for _, d := range c.Domains {
				if strings.EqualFold(strings.TrimPrefix(d, "*."), domain) {
					return nil
				}
			}
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	_, err = s.opts.AcmeService.IssueCertificate(ctx, acme.IssueCertificateRequest{
		Domains:       []string{domain},
		Provider:      acme.ProviderLetsEncrypt,
		Email:         "admin@localhost",
		ChallengeType: acme.ChallengeTypeHTTP01,
		AutoRenew:     true,
	})
	return err
}

func (s *Service) ensureRoutingRule(ctx context.Context, p *store.PreviewDeployment) error {
	if s.opts.TrafficMgr == nil {
		return nil
	}
	host := strings.TrimPrefix(s.opts.PreviewURL(p.PRNumber, p.RepoOwner, p.RepoName), "https://")
	err := s.opts.TrafficMgr.CreateRoutingRule(ctx, &trafficmanager.RoutingRule{
		ID:         "preview-" + p.ID,
		Name:       "preview-" + p.ID,
		ServerID:   p.ServerID,
		Domain:     host,
		Path:       "",
		TargetHost: "localhost",
		TargetPort: 8080,
		Protocol:   "https",
		Strategy:   "round_robin",
		Weight:     1,
		Enabled:    true,
		WebSocket:  true,
	})
	return err
}

func (s *Service) withdrawRoutingRule(ctx context.Context, id string) error {
	return s.opts.TrafficMgr.DeleteRoutingRule(ctx, "preview-"+id)
}

func (s *Service) ensureWildcardDomain(ctx context.Context, p *store.PreviewDeployment) error {
	wildcard := "*." + s.opts.BaseDomain
	existing, err := s.opts.DomainSvc.FindDomainByHost(ctx, wildcard)
	if err == nil && existing != nil {
		return nil
	}
	_, err = s.opts.DomainSvc.AddDomain(ctx, p.ServerID, wildcard)
	return err
}

// ---- commit status worker ----

func (s *Service) reportStatus(ctx context.Context, p *store.PreviewDeployment, state, description string) {
	if s.opts.GitService == nil || p.CreatedBy == nil || p.CommitSHA == "" {
		return
	}
	if p.CreatedBy != nil && *p.CreatedBy == "" {
		return
	}
	st := git.CommitStatus{
		State:       state,
		Description: description,
		Context:     "forge/preview",
	}
	if s.opts.PanelURL != "" {
		st.TargetURL = strings.TrimRight(s.opts.PanelURL, "/") + "/environments/previews?preview=" + p.ID
	} else if p.PreviewURL != "" {
		st.TargetURL = p.PreviewURL
	}
	stCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := s.opts.GitService.ReportCommitStatusForUser(
		stCtx, *p.CreatedBy, providerType(p.Source),
		p.RepoOwner, p.RepoName, p.CommitSHA, st,
	); err != nil && s.opts.Logger != nil {
		s.opts.Logger.Warn("previewenv: commit status not reported",
			"preview", p.ID, "repo", p.RepoOwner+"/"+p.RepoName, "state", state, "err", err)
	}
}

func providerType(source string) store.GitProviderType {
	switch strings.ToLower(source) {
	case "gitlab":
		return store.GitProviderGitLab
	case "bitbucket":
		return store.GitProviderBitbucket
	case "gitea":
		return store.GitProviderGitea
	default:
		return store.GitProviderGitHub
	}
}

// ---- outbox publishing ----

func (s *Service) publish(ctx context.Context, eventType, resourceID string, payload map[string]any) {
	if s.opts.Publisher == nil {
		return
	}
	_ = s.opts.Publisher.Publish(ctx, events.NewEnvelope(events.EventType(eventType), "previewenv", "preview", resourceID, payload))
}

// Options accessor used by the registrar for the UI config endpoint.
func (s *Service) Config() (baseDomain string, ttl, retain time.Duration, maxPerOrg int) {
	return s.opts.BaseDomain, s.opts.TTL, s.opts.RetainCleaned, s.opts.MaxPerOrg
}

// PreviewConfig exposes the project-scoped preview settings for the UI so the
// panel shows the domain and lifetime previews actually use.
func (s *Service) PreviewConfig() (domain string, ttl time.Duration, maxPerProject int) {
	return s.PreviewBaseDomain(), s.opts.PreviewTTL, s.opts.MaxPreviewsPerProject
}

// resolveServerID finds the server a git source is linked to (via
// servers.docker_labels["git_source_id"]). Webhook flows that cannot resolve
// a server skip preview creation (fail-open).
func (s *Service) resolveServerID(ctx context.Context, source *store.GitSource) (string, error) {
	if source == nil || source.ID == "" {
		return "", fmt.Errorf("git source is required")
	}
	serverID, err := s.store.FindServerIDByGitSource(ctx, source.ID)
	if err != nil {
		return "", err
	}
	if serverID == "" {
		return "", fmt.Errorf("no server linked to git source %s", source.ID)
	}
	return serverID, nil
}
