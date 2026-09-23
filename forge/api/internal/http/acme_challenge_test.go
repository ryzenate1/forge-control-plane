package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	acmesvc "gamepanel/forge/internal/services/acme"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// registerAcmeChallengeRoute was removed: internal/http no longer mounts the
// ACME HTTP-01 challenge route onto the Fiber app (challenge serving is
// handled by the gateway/traffic-manager layer). The acme service still
// exposes its HTTP-01 solver via Service.HTTPSolver(); these tests pin that
// handler contract directly.

func TestAcmeHTTPSolverServesChallenges(t *testing.T) {
	svc := acmesvc.New(nil, nil)
	h := svc.HTTPSolver()
	require.NotNil(t, h, "acme service must expose HTTPSolver handler (F-NET-09)")

	token := "test-token-123"
	keyAuth := "test-token-123.keyAuthData"

	presenter, ok := h.(interface {
		Present(domain, token, keyAuth string) error
	})
	require.True(t, ok, "HTTPSolver does not implement Present")
	require.NoError(t, presenter.Present("example.com", token, keyAuth))

	// Correct host + token serves the key authorization
	req := httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/"+token, nil)
	req.Host = "example.com"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, keyAuth, w.Body.String())

	// Unknown token 404s
	req2 := httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/notfound", nil)
	req2.Host = "example.com"
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusNotFound, w2.Code)

	// Wrong host 404s (challenge is keyed per domain)
	req3 := httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/"+token, nil)
	req3.Host = "other.example.com"
	w3 := httptest.NewRecorder()
	h.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusNotFound, w3.Code)

	// CleanUp removes the token
	cleaner, ok := h.(interface {
		CleanUp(domain, token, keyAuth string) error
	})
	require.True(t, ok, "HTTPSolver does not implement CleanUp")
	require.NoError(t, cleaner.CleanUp("example.com", token, keyAuth))
	w4 := httptest.NewRecorder()
	h.ServeHTTP(w4, req)
	assert.Equal(t, http.StatusNotFound, w4.Code)
}
