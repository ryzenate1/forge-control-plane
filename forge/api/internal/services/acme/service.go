package acme

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
)

type CertificateProvider = string

const (
	ProviderLetsEncrypt        CertificateProvider = "letsencrypt"
	ProviderLetsEncryptStaging CertificateProvider = "letsencrypt-staging"
	ProviderZeroSSL            CertificateProvider = "zerossl"
	ProviderBuyPass            CertificateProvider = "buypass"
	ProviderGoogleTrust        CertificateProvider = "google-trust"

	// Certificates the operator supplied themselves. Forge stores and serves
	// them but has no way to reissue them, so they are never ACME-renewable.
	// ProviderManual comes from POST /certificates/upload, ProviderCustom from
	// POST /certificates (the proxy-domain-bound import).
	ProviderManual CertificateProvider = "manual"
	ProviderCustom CertificateProvider = "custom"

	ChallengeTypeHTTP01 = "http-01"
	ChallengeTypeDNS01  = "dns-01"

	defaultDirectoryURL = "https://acme-v02.api.letsencrypt.org/directory"
	stagingDirectoryURL = "https://acme-staging-v02.api.letsencrypt.org/directory"
	zeroSSLURL          = "https://acme.zerossl.com/v2/DV90"
	buyPassURL          = "https://api.buypass.com/acme/directory"
	googleTrustURL      = "https://dv.acme-v02.api.pki.goog/directory"
)

// IsACMEProvider reports whether a stored certificate's provider identifies an
// ACME CA that Forge can order a replacement from.
//
// directoryURL falls back to Let's Encrypt for any string it does not
// recognise, so this cannot be a "not manual" check: an unknown provider must
// be treated as non-renewable rather than silently pointed at Let's Encrypt.
// An empty provider is renewable because IssueCertificate defaults it to
// ProviderLetsEncrypt, so rows written before that default was applied are
// genuinely Let's Encrypt certificates.
func IsACMEProvider(provider CertificateProvider) bool {
	switch provider {
	case "", ProviderLetsEncrypt, ProviderLetsEncryptStaging, ProviderZeroSSL, ProviderBuyPass, ProviderGoogleTrust:
		return true
	default:
		return false
	}
}

type DNSProviderFactory func(providerName string, credentials map[string]string) (challenge.Provider, error)

type Service struct {
	store          certificateStore
	logger         *slog.Logger
	httpChallenge  *httpChallenger
	dnsProviders   map[string]DNSProviderFactory
	mu             sync.RWMutex
	cancel         context.CancelFunc
	httpSolverAddr string
	gateway        GatewayCertInstaller
}

// GatewayCertInstaller installs an issued certificate into the live reverse
// proxy so HTTPS actually serves it. Without it, issuance only persists a DB row
// and the gateway never receives the material (the historical cert→gateway gap).
type GatewayCertInstaller interface {
	InstallCertificate(ctx context.Context, certPEM, keyPEM string, domains []string) error
}

// SetGateway wires the reverse proxy that receives issued/renewed certificates.
func (s *Service) SetGateway(g GatewayCertInstaller) { s.gateway = g }

type httpChallenger struct {
	mu    sync.RWMutex
	token map[string]string
}

func newHTTPChallenger() *httpChallenger {
	return &httpChallenger{token: make(map[string]string)}
}

func (h *httpChallenger) Present(domain, token, keyAuth string) error {
	h.mu.Lock()
	h.token[token+"/"+domain] = keyAuth
	h.mu.Unlock()
	return nil
}

func (h *httpChallenger) CleanUp(domain, token, keyAuth string) error {
	h.mu.Lock()
	delete(h.token, token+"/"+domain)
	h.mu.Unlock()
	return nil
}

func (h *httpChallenger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/.well-known/acme-challenge/")
	if token == "" || strings.Contains(token, "/") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	for _, domain := range []string{r.Host} {
		h.mu.RLock()
		keyAuth := h.token[token+"/"+domain]
		h.mu.RUnlock()
		if keyAuth != "" {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write([]byte(keyAuth))
			return
		}
	}
	http.Error(w, "not found", http.StatusNotFound)
}

type userReg struct {
	email        string
	registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *userReg) GetEmail() string                        { return u.email }
func (u *userReg) GetRegistration() *registration.Resource { return u.registration }
func (u *userReg) GetPrivateKey() crypto.PrivateKey        { return u.key }

type certificateStore interface {
	CreateCertificate(ctx context.Context, req store.CreateCertificateRequest) (store.Certificate, error)
	GetCertificate(ctx context.Context, id string) (store.Certificate, error)
	ListCertificates(ctx context.Context, filter store.CertificateFilter) ([]store.Certificate, error)
	UpdateCertificate(ctx context.Context, id string, req store.UpdateCertificateRequest) (store.Certificate, error)
	DeleteCertificate(ctx context.Context, id string) error
	FindExpiringCertificates(ctx context.Context) ([]store.Certificate, error)
	CreateCertificateAttempt(ctx context.Context, req store.CreateCertificateAttemptRequest) (store.CertificateAttempt, error)
	UpdateCertificateAttempt(ctx context.Context, id, status, errorMessage string) error
}

func New(s certificateStore, logger *slog.Logger) *Service {
	httpChallenger := newHTTPChallenger()
	return &Service{
		store:          s,
		logger:         logger,
		httpChallenge:  httpChallenger,
		dnsProviders:   make(map[string]DNSProviderFactory),
		httpSolverAddr: ":80",
	}
}

func (s *Service) SetHTTPSolverAddr(addr string) {
	s.httpSolverAddr = addr
}

func (s *Service) HTTPSolver() http.Handler {
	return s.httpChallenge
}

func (s *Service) RegisterDNSProvider(name string, factory DNSProviderFactory) {
	s.mu.Lock()
	s.dnsProviders[name] = factory
	s.mu.Unlock()
	RegisterDNSProvider(name, factory)
}

func (s *Service) directoryURL(provider CertificateProvider) string {
	switch provider {
	case ProviderLetsEncryptStaging:
		return stagingDirectoryURL
	case ProviderZeroSSL:
		return zeroSSLURL
	case ProviderBuyPass:
		return buyPassURL
	case ProviderGoogleTrust:
		return googleTrustURL
	default:
		return defaultDirectoryURL
	}
}

type IssueCertificateRequest struct {
	Domains        []string            `json:"domains"`
	Provider       CertificateProvider `json:"provider"`
	Email          string              `json:"email"`
	ChallengeType  string              `json:"challengeType"`
	DNSProvider    string              `json:"dnsProvider,omitempty"`
	DNSCredentials map[string]string   `json:"dnsCredentials,omitempty"`
	AutoRenew      bool                `json:"autoRenew"`
}

func (s *Service) IssueCertificate(ctx context.Context, req IssueCertificateRequest) (store.Certificate, error) {
	if len(req.Domains) == 0 {
		return store.Certificate{}, errors.New("at least one domain is required")
	}
	if req.Email == "" {
		req.Email = "admin@localhost"
	}
	if req.Provider == "" {
		req.Provider = ProviderLetsEncrypt
	}
	if req.ChallengeType == "" {
		req.ChallengeType = ChallengeTypeHTTP01
	}
	if req.ChallengeType != ChallengeTypeHTTP01 && req.ChallengeType != ChallengeTypeDNS01 {
		return store.Certificate{}, fmt.Errorf("unsupported challenge type: %s", req.ChallengeType)
	}

	wildcard := false
	for _, d := range req.Domains {
		if strings.HasPrefix(d, "*.") {
			wildcard = true
			break
		}
	}
	if wildcard && req.ChallengeType != ChallengeTypeDNS01 {
		return store.Certificate{}, errors.New("wildcard certificates require dns-01 challenge")
	}

	// Validate DNS-01 challenge requirements
	if req.ChallengeType == ChallengeTypeDNS01 {
		if req.DNSProvider == "" {
			return store.Certificate{}, errors.New("dns provider is required for dns-01 challenge")
		}
		if len(req.DNSCredentials) == 0 {
			return store.Certificate{}, errors.New("dns credentials are required for dns-01 challenge")
		}
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return store.Certificate{}, fmt.Errorf("generate private key: %w", err)
	}

	myUser := &userReg{email: req.Email, key: privateKey}

	config := lego.NewConfig(myUser)
	config.CADirURL = s.directoryURL(req.Provider)
	config.Certificate.KeyType = certcrypto.RSA2048

	client, err := lego.NewClient(config)
	if err != nil {
		return store.Certificate{}, fmt.Errorf("create acme client: %w", err)
	}

	challengeErr := s.configureChallenge(client, req.ChallengeType, req.DNSProvider, req.DNSCredentials)
	if challengeErr != nil {
		return store.Certificate{}, challengeErr
	}

	reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return store.Certificate{}, fmt.Errorf("register acme account: %w", err)
	}
	myUser.registration = reg

	obtainReq := certificate.ObtainRequest{
		Domains: req.Domains,
		Bundle:  true,
	}

	certRes, err := s.obtainWithRetry(ctx, client, obtainReq)
	if err != nil {
		return store.Certificate{}, fmt.Errorf("obtain certificate: %w", err)
	}

	certPEM := string(certRes.Certificate)
	keyPEM := string(certRes.PrivateKey)
	certs := parseCertificateChain(certRes.Certificate)
	expiresAt := time.Now().Add(90 * 24 * time.Hour)
	if len(certs) > 0 && !certs[0].NotAfter.IsZero() {
		expiresAt = certs[0].NotAfter
	}

	cert, err := s.store.CreateCertificate(ctx, store.CreateCertificateRequest{
		Domains:        req.Domains,
		Issuer:         string(certRes.IssuerCertificate),
		Certificate:    certPEM,
		PrivateKey:     keyPEM,
		ExpiresAt:      expiresAt,
		AutoRenew:      req.AutoRenew,
		Provider:       req.Provider,
		ChallengeType:  req.ChallengeType,
		DNSProvider:    req.DNSProvider,
		DNSCredentials: req.DNSCredentials,
		Wildcard:       wildcard,
	})
	if err != nil {
		return store.Certificate{}, err
	}

	// Install into the live gateway so HTTPS actually serves the issued cert.
	// This is not best-effort: a certificate the proxy never received leaves
	// HTTPS serving the previous (possibly expiring) material, and reporting a
	// plain success for it would be reporting work that was not performed. The
	// row is already persisted, so the error names the certificate ID to retry
	// delivery against instead of placing a second ACME order.
	if err := s.deliverCertificate(ctx, certPEM, keyPEM, req.Domains, cert.ID, "issue"); err != nil {
		return store.Certificate{}, fmt.Errorf("%w (certificate %s is stored; renew or re-install it rather than issuing again)", err, cert.ID)
	}

	return redactCertificate(cert), nil
}

// deliverCertificate installs issued/renewed material into the live reverse
// proxy and records the outcome in certificate_attempts so a delivery failure
// outlives the log line. A nil gateway means delivery is not wired, which is a
// configuration state, not a failure.
func (s *Service) deliverCertificate(ctx context.Context, certPEM, keyPEM string, domains []string, certID, attemptType string) error {
	if s.gateway == nil {
		return nil
	}
	attempt, aerr := s.store.CreateCertificateAttempt(ctx, store.CreateCertificateAttemptRequest{
		CertificateID: certID,
		AttemptType:   attemptType,
		Domains:       domains,
	})
	err := s.gateway.InstallCertificate(ctx, certPEM, keyPEM, domains)
	if err != nil {
		s.recordAttemptResult(ctx, attempt, aerr, "failed", err.Error())
		return err
	}
	s.recordAttemptResult(ctx, attempt, aerr, "completed", "")
	return nil
}

// recordAttemptResult persists an attempt outcome. A bookkeeping write failure is
// logged but never replaces the caller's result — the issuance/delivery outcome
// is the primary fact here.
func (s *Service) recordAttemptResult(ctx context.Context, attempt store.CertificateAttempt, createErr error, status, msg string) {
	if createErr != nil {
		s.logger.Warn("acme: could not record certificate attempt", "status", status, "error", createErr)
		return
	}
	if uerr := s.store.UpdateCertificateAttempt(ctx, attempt.ID, status, msg); uerr != nil {
		s.logger.Warn("acme: could not finalise certificate attempt", "attemptId", attempt.ID, "status", status, "error", uerr)
	}
}

func (s *Service) configureChallenge(client *lego.Client, challengeType, dnsProvider string, dnsCredentials map[string]string) error {
	switch challengeType {
	case ChallengeTypeHTTP01:
		client.Challenge.SetHTTP01Provider(s.httpChallenge)
	case ChallengeTypeDNS01:
		if dnsProvider == "" {
			return fmt.Errorf("dns provider name is required for dns-01 challenge")
		}
		s.mu.RLock()
		factory, ok := s.dnsProviders[dnsProvider]
		s.mu.RUnlock()
		if !ok {
			return fmt.Errorf("unknown dns provider: %s", dnsProvider)
		}
		provider, err := factory(dnsProvider, dnsCredentials)
		if err != nil {
			return fmt.Errorf("create dns provider: %w", err)
		}
		client.Challenge.SetDNS01Provider(provider,
			dns01.AddRecursiveNameservers([]string{"1.1.1.1:53", "8.8.8.8:53"}),
		)
	default:
		// Falling through here would leave the client with no solver at all, so
		// the CA rejects the order for an obscure reason. A stored row with an
		// empty or unrecognised challenge type is a data problem, not something
		// to silently paper over with a default.
		return fmt.Errorf("unsupported challenge type %q: expected %s or %s", challengeType, ChallengeTypeHTTP01, ChallengeTypeDNS01)
	}
	return nil
}

func (s *Service) RenewCertificate(ctx context.Context, certID string) (store.Certificate, error) {
	cert, err := s.store.GetCertificate(ctx, certID)
	if err != nil {
		return store.Certificate{}, err
	}
	// An operator-supplied certificate has no ACME order behind it. Renewing it
	// here would place a fresh order against whatever CA directoryURL falls back
	// to and then overwrite the operator's own certificate and key with the
	// result — replacing, for example, a corporate-CA certificate with a Let's
	// Encrypt one without anyone asking. Refuse instead; the operator uploads a
	// replacement through POST /certificates/upload.
	if !IsACMEProvider(cert.Provider) {
		return store.Certificate{}, fmt.Errorf("certificate provider %q is not ACME-issued and cannot be renewed automatically; upload a replacement certificate instead", cert.Provider)
	}
	if cert.PrivateKey == "" {
		return store.Certificate{}, errors.New("private key not available for renewal")
	}

	privateKey, err := certcrypto.ParsePEMPrivateKey([]byte(cert.PrivateKey))
	if err != nil {
		return store.Certificate{}, fmt.Errorf("parse private key: %w", err)
	}

	res, err := s.renewWithRetry(ctx, cert, privateKey)
	if err != nil {
		return store.Certificate{}, fmt.Errorf("renew certificate: %w", err)
	}

	certPEM := string(res.Certificate)
	certs := parseCertificateChain(res.Certificate)
	expiresAt := time.Now().Add(90 * 24 * time.Hour)
	if len(certs) > 0 && !certs[0].NotAfter.IsZero() {
		expiresAt = certs[0].NotAfter
	}

	keyPEM := string(res.PrivateKey)
	now := time.Now().UTC()
	updated, err := s.store.UpdateCertificate(ctx, certID, store.UpdateCertificateRequest{
		Certificate: &certPEM,
		PrivateKey:  &keyPEM,
		ExpiresAt:   &expiresAt,
		UpdatedAt:   &now,
	})
	if err != nil {
		return store.Certificate{}, err
	}

	// The renewed material has to reach the proxy or the rotation is invisible:
	// the gateway keeps serving the certificate it already has, which is the one
	// that was about to expire. Report a failed renewal rather than a success,
	// exactly as issuance does.
	if err := s.deliverCertificate(ctx, certPEM, keyPEM, cert.Domains, certID, "renew"); err != nil {
		return store.Certificate{}, fmt.Errorf("certificate renewed but not installed into the gateway: %w", err)
	}

	return redactCertificate(updated), nil
}

func (s *Service) RevokeCertificate(ctx context.Context, certID string) error {
	return s.store.DeleteCertificate(ctx, certID)
}

// ImportManualCertificate validates an operator-supplied PEM bundle and
// persists it as a non-renewable manual certificate. Layering: handlers ->
// acme.Service -> store; handlers never touch the certificates table here.
func (s *Service) ImportManualCertificate(ctx context.Context, certPEM, keyPEM, chain string) (store.Certificate, error) {
	if s == nil || s.store == nil {
		return store.Certificate{}, errors.New("certificate store not initialized")
	}
	certData, err := parseManualCertificatePEM(certPEM)
	if err != nil {
		return store.Certificate{}, fmt.Errorf("invalid certificate: %w", err)
	}
	if err := checkManualKeyPair(certPEM, keyPEM); err != nil {
		return store.Certificate{}, fmt.Errorf("key pair mismatch: %w", err)
	}
	domains := append([]string{}, certData.DNSNames...)
	if len(domains) == 0 && certData.Subject.CommonName != "" {
		domains = append(domains, certData.Subject.CommonName)
	}
	fullPEM := certPEM
	if strings.TrimSpace(chain) != "" {
		fullPEM = certPEM + "\n" + chain
	}
	return s.store.CreateCertificate(ctx, store.CreateCertificateRequest{
		Domains:     domains,
		Issuer:      certData.Issuer.String(),
		Certificate: fullPEM,
		PrivateKey:  keyPEM,
		ExpiresAt:   certData.NotAfter,
		AutoRenew:   false,
		Provider:    ProviderManual,
	})
}

// ExportCertificate returns the stored material for download/export after
// verifying a private key is present. GetCertificate stays the read path.
func (s *Service) ExportCertificate(ctx context.Context, certID string) (store.Certificate, error) {
	if s == nil || s.store == nil {
		return store.Certificate{}, errors.New("certificate store not initialized")
	}
	cert, err := s.store.GetCertificate(ctx, certID)
	if err != nil {
		return store.Certificate{}, err
	}
	if strings.TrimSpace(cert.PrivateKey) == "" {
		return store.Certificate{}, errors.New("private key not available for export")
	}
	return cert, nil
}

func parseManualCertificatePEM(certPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("no valid PEM certificate found")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}
	if time.Now().After(cert.NotAfter) {
		return nil, errors.New("certificate has expired")
	}
	return cert, nil
}

func checkManualKeyPair(certPEM, keyPEM string) error {
	certBlock, _ := pem.Decode([]byte(certPEM))
	if certBlock == nil {
		return errors.New("no certificate PEM data")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return err
	}
	keyBlock, _ := pem.Decode([]byte(keyPEM))
	if keyBlock == nil {
		return errors.New("no private key PEM data")
	}
	privKey, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		privKey, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
		if err != nil {
			privKey, err = x509.ParseECPrivateKey(keyBlock.Bytes)
			if err != nil {
				return fmt.Errorf("parse private key: %w", err)
			}
		}
	}
	certPubKey, ok := cert.PublicKey.(crypto.PublicKey)
	if !ok {
		return errors.New("invalid certificate public key type")
	}
	privPubKey, ok := privKey.(interface{ Public() crypto.PublicKey })
	if !ok {
		return errors.New("invalid private key type")
	}
	certPubKeyBytes, err := x509.MarshalPKIXPublicKey(certPubKey)
	if err != nil {
		return err
	}
	privPubKeyBytes, err := x509.MarshalPKIXPublicKey(privPubKey.Public())
	if err != nil {
		return err
	}
	if string(certPubKeyBytes) != string(privPubKeyBytes) {
		return errors.New("certificate and private key do not match")
	}
	return nil
}

func (s *Service) GetCertificate(ctx context.Context, certID string) (store.Certificate, error) {
	return s.store.GetCertificate(ctx, certID)
}

func (s *Service) ListCertificates(ctx context.Context, filter store.CertificateFilter) ([]store.Certificate, error) {
	return s.store.ListCertificates(ctx, filter)
}

func (s *Service) StartAutoRenewal(ctx context.Context) {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 4096)
				n := runtime.Stack(buf, false)
				s.logger.Error("acme auto-renewal panic recovered", "panic", r, "stack", string(buf[:n]))
			}
		}()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runAutoRenewal(ctx)
			}
		}
	}()
}

func (s *Service) StopAutoRenewal() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

func (s *Service) runAutoRenewal(ctx context.Context) {
	certs, err := s.store.FindExpiringCertificates(ctx)
	if err != nil {
		s.logger.Error("acme: failed to find expiring certificates", "error", err)
		return
	}

	for _, cert := range certs {
		// FindExpiringCertificates selects on auto_renew and expires_at only, so
		// an operator-supplied certificate that was stored with auto_renew set
		// lands in this set on every cycle. Skip it here rather than letting
		// RenewCertificate reject it and log a failure every pass.
		if !IsACMEProvider(cert.Provider) {
			s.logger.Warn("acme: skipping auto-renewal for operator-supplied certificate", "certId", cert.ID, "provider", cert.Provider)
			continue
		}
		if cert.PrivateKey == "" {
			s.logger.Warn("acme: skipping renewal for cert without private key", "certId", cert.ID)
			continue
		}
		if _, err := s.RenewCertificate(ctx, cert.ID); err != nil {
			s.logger.Error("acme: failed to renew certificate", "certId", cert.ID, "error", err)
		} else {
			s.logger.Info("acme: renewed certificate", "certId", cert.ID, "domains", cert.Domains)
		}
	}
}

func (s *Service) obtainWithRetry(ctx context.Context, client *lego.Client, req certificate.ObtainRequest) (*certificate.Resource, error) {
	const maxRetries = 3
	baseDelay := 5 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		res, err := client.Certificate.Obtain(req)
		if err == nil {
			return res, nil
		}

		if attempt == maxRetries {
			return nil, err
		}

		delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
		s.logger.Warn("acme: obtain failed, retrying", "attempt", attempt+1, "delay", delay, "error", err)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, errors.New("unreachable")
}

func (s *Service) renewWithRetry(ctx context.Context, cert store.Certificate, privateKey crypto.PrivateKey) (*certificate.Resource, error) {
	const maxRetries = 3
	baseDelay := 5 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		res, err := s.renewOnce(cert, privateKey)
		if err == nil {
			return res, nil
		}

		if attempt == maxRetries {
			return nil, err
		}

		delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
		s.logger.Warn("acme: renew failed, retrying", "attempt", attempt+1, "delay", delay, "error", err)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, errors.New("unreachable")
}

func (s *Service) renewOnce(cert store.Certificate, privateKey crypto.PrivateKey) (*certificate.Resource, error) {
	if len(cert.Domains) == 0 {
		return nil, errors.New("certificate has no domains recorded")
	}
	certs := parseCertificateChain([]byte(cert.Certificate))
	if len(certs) == 0 {
		return nil, errors.New("no certificates found in stored cert")
	}

	x509Cert := certs[0]
	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}
	// lego's Renew re-parses Resource.Certificate with certcrypto.ParsePEMBundle
	// and Resource.PrivateKey with certcrypto.ParsePEMPrivateKey. Both require
	// PEM: ParsePEMBundle fails with "no certificates were found while parsing
	// the bundle" on DER, and ParsePEMPrivateKey fails with "invalid PEM block".
	// Passing x509Cert.Raw and the PKCS#8 DER here therefore made *every* renewal
	// fail before a single ACME request was made, so no certificate ever rotated.
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	dirURL := defaultDirectoryURL
	if cert.Provider == ProviderLetsEncryptStaging {
		dirURL = stagingDirectoryURL
	} else if cert.Provider == ProviderZeroSSL {
		dirURL = zeroSSLURL
	} else if cert.Provider == ProviderBuyPass {
		dirURL = buyPassURL
	} else if cert.Provider == ProviderGoogleTrust {
		dirURL = googleTrustURL
	}

	myUser := &userReg{
		email: "admin@localhost",
		key:   privateKey,
	}

	config := lego.NewConfig(myUser)
	config.CADirURL = dirURL
	config.Certificate.KeyType = certcrypto.RSA2048

	client, err := lego.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("create acme client: %w", err)
	}

	if err := s.configureChallenge(client, cert.ChallengeType, cert.DNSProvider, cert.DNSCredentials); err != nil {
		return nil, fmt.Errorf("configure challenge for renewal: %w", err)
	}

	reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return nil, fmt.Errorf("register: %w", err)
	}
	myUser.registration = reg

	// lego derives the SAN set from the PEM in Resource.Certificate (it re-parses
	// the bundle and calls ExtractDomains on the leaf), so the stored PEM bundle
	// is passed through verbatim and Resource.Domain only labels the order.
	res, err := client.Certificate.Renew(certificate.Resource{
		Domain:      primaryDomain(cert, x509Cert),
		Certificate: []byte(cert.Certificate),
		PrivateKey:  keyPEM,
	}, true, false, "")
	if err != nil {
		return nil, fmt.Errorf("acme renew: %w", err)
	}

	return res, nil
}

// primaryDomain returns the name to label a renewal order with: the first SAN of
// the issued leaf when it has one, otherwise the CN, otherwise the first domain
// recorded for the certificate.
func primaryDomain(cert store.Certificate, leaf *x509.Certificate) string {
	if len(leaf.DNSNames) > 0 {
		return leaf.DNSNames[0]
	}
	if leaf.Subject.CommonName != "" {
		return leaf.Subject.CommonName
	}
	return cert.Domains[0]
}

func parseCertificateChain(certPEM []byte) []*x509.Certificate {
	var certs []*x509.Certificate
	for {
		block, rest := pem.Decode(certPEM)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err == nil {
				certs = append(certs, cert)
			}
		}
		certPEM = rest
	}
	return certs
}

func (s *Service) RecordAttempt(ctx context.Context, certID, attemptType string, domains []string) (store.CertificateAttempt, error) {
	return s.store.CreateCertificateAttempt(ctx, store.CreateCertificateAttemptRequest{
		CertificateID: certID,
		AttemptType:   attemptType,
		Domains:       domains,
	})
}

func (s *Service) CompleteAttempt(ctx context.Context, attemptID string, status, errorMessage string) error {
	return s.store.UpdateCertificateAttempt(ctx, attemptID, status, errorMessage)
}
