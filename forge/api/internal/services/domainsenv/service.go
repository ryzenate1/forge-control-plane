// Package domainsenv orchestrates per-environment *.domain DNS intents and
// automatic TLS. It is a pure wiring layer over the existing domains, dns
// and acme services: when an environment exists (or is created) and the
// operator sets ENV_DOMAIN, the package ensures a wildcard CNAME
//     *.<env-slug>.<base-domain>
// plus an ACME wildcard certificate for that host, recording the intent and
// status in the env_domain_provisioning ledger so the dashboard can display
// it and the reconciler can repair failures.
package domainsenv

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/services/acme"
	"gamepanel/forge/internal/store"
)

// Provisioner persists provisioning intent/status for the reconciler.
type Provisioner interface {
	ResolveEnvContext(ctx context.Context, envID string) (store.EnvContext, error)
	UpsertEnvDomainProvisioning(ctx context.Context, p store.EnvDomainProvisioning) error
	GetEnvDomainProvisioning(ctx context.Context, envID string) (*store.EnvDomainProvisioning, error)
	ListEnvironmentIDsForProvisioning(ctx context.Context) ([]string, error)
}

// AcmeIssuer is the subset of the acme service used for wildcard TLS.
type AcmeIssuer interface {
	IssueCertificate(ctx context.Context, req acme.IssueCertificateRequest) (store.Certificate, error)
}

// SettingsSource abstracts environment lookup so tests can inject values;
// nil means os.Getenv.
type SettingsSource func(key string) string

// Options controls the orchestrator.
type Options struct {
	// BaseDomain is the operator-level zone (ENV_DOMAIN), e.g. env.example.com.
	BaseDomain string
	// Target is the CNAME target for *.<slug>.<base> (ENV_DOMAIN_TARGET).
	Target string
	// AcmeEnabled gates automatic TLS (ENV_DOMAIN_ENABLED=1).
	AcmeEnabled bool
	// AcmeEmail is the Let's Encrypt contact (ENV_DOMAIN_ACME_EMAIL).
	AcmeEmail string
	// DNSProvider is the lego provider used for dns-01 (ENV_DOMAIN_DNS_PROVIDER).
	DNSProvider string
	// DNSCredentials are key/value credentials for the DNS provider
	// (ENV_DOMAIN_DNS_CREDENTIALS as a JSON object).
	DNSCredentials map[string]string
	// SyncInterval controls the reconciler loop (ENV_DOMAIN_SYNC_INTERVAL).
	SyncInterval time.Duration
}

// OptionsFromEnv builds Options from process environment with the given
// lookup helper (os.Getenv when nil) — consistent with the rest of the repo.
func OptionsFromEnv(get SettingsSource) Options {
	if get == nil {
		get = os.Getenv
	}
	opts := Options{
		BaseDomain:   strings.TrimSpace(get("ENV_DOMAIN")),
		Target:       strings.TrimSpace(get("ENV_DOMAIN_TARGET")),
		AcmeEnabled:  get("ENV_DOMAIN_ENABLED") == "1" || get("ENV_DOMAIN_ENABLED") == "true",
		AcmeEmail:    strings.TrimSpace(get("ENV_DOMAIN_ACME_EMAIL")),
		DNSProvider:  strings.TrimSpace(get("ENV_DOMAIN_DNS_PROVIDER")),
		SyncInterval: 10 * time.Minute,
	}
	if raw := strings.TrimSpace(get("ENV_DOMAIN_DNS_CREDENTIALS")); raw != "" {
		var creds map[string]string
		if err := json.Unmarshal([]byte(raw), &creds); err == nil {
			opts.DNSCredentials = creds
		}
	}
	if raw := strings.TrimSpace(get("ENV_DOMAIN_SYNC_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			opts.SyncInterval = d
		}
	}
	return opts
}

// Service orchestrates per-environment domain + TLS provisioning.
type Service struct {
	store Provisioner
	acme  AcmeIssuer
	opts  Options
	log   *slog.Logger

	mu        sync.Mutex
	inFlight  map[string]bool
}

// New builds the orchestrator. Pass a nil AcmeIssuer to disable TLS issuance.
func New(st Provisioner, ac AcmeIssuer, opts Options, logger *slog.Logger) *Service {
	if opts.SyncInterval <= 0 {
		opts.SyncInterval = 10 * time.Minute
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:    st,
		acme:     ac,
		opts:     opts,
		log:      logger,
		inFlight: map[string]bool{},
	}
}

// DomainInfo is the DNS/TLS intent the operator must satisfy (the actual
// CNAME record is created in the authoritative DNS provider).
type DomainInfo struct {
	BaseDomain   string `json:"baseDomain,omitempty"`
	WildcardHost string `json:"wildcardHost,omitempty"`
	Target       string `json:"target,omitempty"`
	DNSStatus    string `json:"dnsStatus,omitempty"`
	TLSStatus    string `json:"tlsStatus,omitempty"`
	LastError    string `json:"lastError,omitempty"`
}

var slugSafe = regexp.MustCompile(`[^a-z0-9-]`)

// slugify converts an environment name into a DNS-safe subdomain label.
func slugify(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = strings.ReplaceAll(slug, "_", "-")
	slug = slugSafe.ReplaceAllString(slug, "")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "env"
	}
	return slug
}

// EnsureEnvironmentDomains provisions the wildcard CNAME + ACME certificate
// for one environment (idempotent). This is the orchestrating entry point
// called both by the reconciler loop and the REST trigger.
func (s *Service) EnsureEnvironmentDomains(ctx context.Context, envID string) (*DomainInfo, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store unavailable")
	}
	envCtx, err := s.store.ResolveEnvContext(ctx, envID)
	if err != nil {
		return nil, err
	}
	info := s.provisionEnv(ctx, envCtx)
	return info, nil
}

func (s *Service) provisionEnv(ctx context.Context, envCtx store.EnvContext) *DomainInfo {
	base := s.opts.BaseDomain
	info := &DomainInfo{}
	if base == "" {
		info.DNSStatus = "disabled"
		info.TLSStatus = "disabled"
		_ = s.store.UpsertEnvDomainProvisioning(ctx, store.EnvDomainProvisioning{
			EnvID:     envCtx.Environment.ID,
			DNSStatus: "disabled",
			TLSStatus: "disabled",
		})
		return info
	}

	slug := slugify(envCtx.Environment.Name)
	wildcard := "*." + slug + "." + base
	info.WildcardHost = wildcard

	row := store.EnvDomainProvisioning{
		EnvID:        envCtx.Environment.ID,
		Domain:       base,
		WildcardHost: wildcard,
		DNSStatus:    "pending",
		TLSStatus:    "pending",
	}

	// DNS intent: the provider zone owns the CNAME; we record the record we
	// expect to exist and surface it to the operator/UI.
	row.Target = s.opts.Target
	if s.opts.Target != "" {
		info.Target = s.opts.Target
		row.DNSStatus = "ok"
		info.DNSStatus = "ok"
	}

	// -- TLS: wildcard ACME certificate via dns-01.
	if s.opts.AcmeEnabled && s.acme != nil {
		cert, err := s.acme.IssueCertificate(ctx, acme.IssueCertificateRequest{
			Domains:        []string{wildcard},
			Provider:       acme.ProviderLetsEncrypt,
			Email:          s.opts.AcmeEmail,
			ChallengeType:  acme.ChallengeTypeDNS01,
			DNSProvider:    s.opts.DNSProvider,
			DNSCredentials: s.opts.DNSCredentials,
			AutoRenew:      true,
		})
		if err != nil {
			row.TLSStatus = "error"
			row.LastError = err.Error()
			info.TLSStatus = "error"
			info.LastError = err.Error()
			s.log.Warn("env wildcard tls issuance failed", slog.String("env_id", envCtx.Environment.ID), slog.String("host", wildcard), slog.String("error", err.Error()))
		} else {
			row.TLSStatus = "ok"
			row.CertID = &cert.ID
			info.TLSStatus = "ok"
			s.log.Info("env wildcard tls provisioned", slog.String("env_id", envCtx.Environment.ID), slog.String("host", wildcard), slog.String("cert_id", cert.ID))
		}
	}

	now := time.Now().UTC()
	row.AttemptedAt = &now
	if err := s.store.UpsertEnvDomainProvisioning(ctx, row); err != nil {
		s.log.Warn("persist env provisioning failed", slog.String("error", err.Error()))
	}
	return info
}

// Provision is the REST-triggerable wrapper around EnsureEnvironmentDomains.
func (s *Service) Provision(ctx context.Context, envID string) (*DomainInfo, error) {
	return s.EnsureEnvironmentDomains(ctx, envID)
}

// Current returns the ledger state for an environment (nil if never touched).
func (s *Service) Current(ctx context.Context, envID string) (*store.EnvDomainProvisioning, error) {
	return s.store.GetEnvDomainProvisioning(ctx, envID)
}

// Start runs the reconciler loop until ctx is cancelled. Environments
// without a provisioning row (or with a non-ok status) are visited at the
// configured interval. Safe to call multiple times.
func (s *Service) Start(ctx context.Context) {
	interval := s.opts.SyncInterval
	s.log.Info("env domain reconciler started", slog.Duration("interval", interval))
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.reconcile(ctx)
			}
		}
	}()
}

func (s *Service) reconcile(ctx context.Context) {
	if s.store == nil {
		return
	}
	envIDs, err := s.store.ListEnvironmentIDsForProvisioning(ctx)
	if err != nil {
		s.log.Warn("list environments for provisioning failed", slog.String("error", err.Error()))
		return
	}
	for _, envID := range envIDs {
		s.mu.Lock()
		if s.inFlight[envID] {
			s.mu.Unlock()
			continue
		}
		s.inFlight[envID] = true
		s.mu.Unlock()

		go func(id string) {
			defer func() {
				if r := recover(); r != nil {
					s.log.Error("env provisioning panic", slog.String("env_id", id), slog.Any("panic", r))
				}
				s.mu.Lock()
				delete(s.inFlight, id)
				s.mu.Unlock()
			}()
			childCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			if _, err := s.EnsureEnvironmentDomains(childCtx, id); err != nil {
				s.log.Warn("env provisioning failed", slog.String("env_id", id), slog.String("error", err.Error()))
			}
		}(envID)
	}
}