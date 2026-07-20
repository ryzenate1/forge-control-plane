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

	ChallengeTypeHTTP01 = "http-01"
	ChallengeTypeDNS01  = "dns-01"

	defaultDirectoryURL = "https://acme-v02.api.letsencrypt.org/directory"
	stagingDirectoryURL = "https://acme-staging-v02.api.letsencrypt.org/directory"
	zeroSSLURL          = "https://acme.zerossl.com/v2/DV90"
	buyPassURL          = "https://api.buypass.com/acme/directory"
	googleTrustURL      = "https://dv.acme-v02.api.pki.goog/directory"
)

type DNSProviderFactory func(providerName string, credentials map[string]string) (challenge.Provider, error)

type Service struct {
	store          certificateStore
	logger         *slog.Logger
	httpChallenge  *httpChallenger
	dnsProviders   map[string]DNSProviderFactory
	mu             sync.RWMutex
	cancel         context.CancelFunc
	httpSolverAddr string
}

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
		Domains:       req.Domains,
		Issuer:        string(certRes.IssuerCertificate),
		Certificate:   certPEM,
		PrivateKey:    keyPEM,
		ExpiresAt:     expiresAt,
		AutoRenew:     req.AutoRenew,
		Provider:      req.Provider,
		ChallengeType: req.ChallengeType,
		Wildcard:      wildcard,
	})
	if err != nil {
		return store.Certificate{}, err
	}

	return cert, nil
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
	}
	return nil
}

func (s *Service) RenewCertificate(ctx context.Context, certID string) (store.Certificate, error) {
	cert, err := s.store.GetCertificate(ctx, certID)
	if err != nil {
		return store.Certificate{}, err
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

	now := time.Now().UTC()
	updated, err := s.store.UpdateCertificate(ctx, certID, store.UpdateCertificateRequest{
		Certificate: &certPEM,
		ExpiresAt:   &expiresAt,
		UpdatedAt:   &now,
	})
	if err != nil {
		return store.Certificate{}, err
	}

	return updated, nil
}

func (s *Service) RevokeCertificate(ctx context.Context, certID string) error {
	return s.store.DeleteCertificate(ctx, certID)
}

func (s *Service) GetCertificate(ctx context.Context, certID string) (store.Certificate, error) {
	return s.store.GetCertificate(ctx, certID)
}

func (s *Service) ListCertificates(ctx context.Context, filter store.CertificateFilter) ([]store.Certificate, error) {
	return s.store.ListCertificates(ctx, filter)
}

func (s *Service) StartAutoRenewal(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()

	go func() {
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
	certs := parseCertificateChain([]byte(cert.Certificate))
	if len(certs) == 0 {
		return nil, errors.New("no certificates found in stored cert")
	}

	x509Cert := certs[0]
	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}

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

	if cert.ChallengeType == ChallengeTypeDNS01 {
		client.Challenge.SetDNS01Provider(s.httpChallenge)
	} else {
		client.Challenge.SetHTTP01Provider(s.httpChallenge)
	}

	reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return nil, fmt.Errorf("register: %w", err)
	}
	myUser.registration = reg

	res, err := client.Certificate.Renew(certificate.Resource{
		Domain:      cert.Domains[0],
		Certificate: x509Cert.Raw,
		PrivateKey:  keyDER,
	}, true, false, "")
	if err != nil {
		return nil, fmt.Errorf("acme renew: %w", err)
	}

	return res, nil
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
