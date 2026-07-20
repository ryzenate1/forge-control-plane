package acme

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"gamepanel/forge/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectoryURL(t *testing.T) {
	svc := New(nil, nil)

	assert.Equal(t, "https://acme-v02.api.letsencrypt.org/directory", svc.directoryURL(ProviderLetsEncrypt))
	assert.Equal(t, "https://acme-staging-v02.api.letsencrypt.org/directory", svc.directoryURL(ProviderLetsEncryptStaging))
	assert.Equal(t, "https://acme.zerossl.com/v2/DV90", svc.directoryURL(ProviderZeroSSL))
	assert.Equal(t, "https://api.buypass.com/acme/directory", svc.directoryURL(ProviderBuyPass))
	assert.Equal(t, "https://dv.acme-v02.api.pki.goog/directory", svc.directoryURL(ProviderGoogleTrust))
	assert.Equal(t, "https://acme-v02.api.letsencrypt.org/directory", svc.directoryURL("unknown"))
}

func TestIssueCertificateValidation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := New(nil, logger)

	t.Run("empty domains", func(t *testing.T) {
		_, err := svc.IssueCertificate(context.Background(), IssueCertificateRequest{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one domain is required")
	})

	t.Run("wildcard with http-01 fails", func(t *testing.T) {
		_, err := svc.IssueCertificate(context.Background(), IssueCertificateRequest{
			Domains:       []string{"*.example.com"},
			ChallengeType: ChallengeTypeHTTP01,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "wildcard certificates require dns-01")
	})

	t.Run("invalid challenge type", func(t *testing.T) {
		_, err := svc.IssueCertificate(context.Background(), IssueCertificateRequest{
			Domains:       []string{"example.com"},
			ChallengeType: "invalid-type",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported challenge type")
	})
}

func TestWildcardDetection(t *testing.T) {
	tests := []struct {
		domains  []string
		wildcard bool
	}{
		{[]string{"example.com"}, false},
		{[]string{"*.example.com"}, true},
		{[]string{"example.com", "*.example.com"}, true},
		{[]string{"sub.example.com"}, false},
	}

	for _, tt := range tests {
		wildcard := false
		for _, d := range tt.domains {
			if len(d) > 2 && d[0] == '*' && d[1] == '.' {
				wildcard = true
				break
			}
		}
		assert.Equal(t, tt.wildcard, wildcard, "domains: %v", tt.domains)
	}
}

func TestHTTPSolver(t *testing.T) {
	svc := New(nil, nil)
	solver := svc.HTTPSolver()
	require.NotNil(t, solver)
}

func TestGetCertificateNotFound(t *testing.T) {
	store := &stubCertificateStore{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := New(store, logger)

	_, err := svc.GetCertificate(context.Background(), "nonexistent")
	require.Error(t, err)
}

func TestListCertificates(t *testing.T) {
	stub := &stubCertificateStore{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := New(stub, logger)

	certs, err := svc.ListCertificates(context.Background(), store.CertificateFilter{})
	require.NoError(t, err)
	assert.Len(t, certs, 0)
}

type stubCertificateStore struct{}

func (s *stubCertificateStore) CreateCertificate(ctx context.Context, req store.CreateCertificateRequest) (store.Certificate, error) {
	return store.Certificate{}, nil
}

func (s *stubCertificateStore) GetCertificate(ctx context.Context, id string) (store.Certificate, error) {
	return store.Certificate{}, context.DeadlineExceeded // simulate not found
}

func (s *stubCertificateStore) ListCertificates(ctx context.Context, filter store.CertificateFilter) ([]store.Certificate, error) {
	return nil, nil
}

func (s *stubCertificateStore) UpdateCertificate(ctx context.Context, id string, req store.UpdateCertificateRequest) (store.Certificate, error) {
	return store.Certificate{}, nil
}

func (s *stubCertificateStore) DeleteCertificate(ctx context.Context, id string) error {
	return nil
}

func (s *stubCertificateStore) FindExpiringCertificates(ctx context.Context) ([]store.Certificate, error) {
	return nil, nil
}

func (s *stubCertificateStore) CreateCertificateAttempt(ctx context.Context, req store.CreateCertificateAttemptRequest) (store.CertificateAttempt, error) {
	return store.CertificateAttempt{}, nil
}

func (s *stubCertificateStore) UpdateCertificateAttempt(ctx context.Context, id, status, errorMessage string) error {
	return nil
}
