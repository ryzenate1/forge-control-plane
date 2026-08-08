package acme

import (
	"errors"
	"testing"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProvider struct{}

func (m *mockProvider) Present(domain, token, keyAuth string) error { return nil }
func (m *mockProvider) CleanUp(domain, token, keyAuth string) error { return nil }

func mockFactory(name string, credentials map[string]string) (challenge.Provider, error) {
	if credentials != nil && credentials["fail"] == "true" {
		return nil, errors.New("factory error")
	}
	return &mockProvider{}, nil
}

func TestRegisterAndGetDNSProvider(t *testing.T) {
	providersMu.Lock()
	providers = make(map[string]DNSProviderFactory)
	providersMu.Unlock()

	RegisterDNSProvider("mock", mockFactory)

	t.Run("registered provider found", func(t *testing.T) {
		p, err := GetDNSProvider("mock", nil)
		require.NoError(t, err)
		require.NotNil(t, p)
		assert.NoError(t, p.Present("example.com", "tok", "key"))
		assert.NoError(t, p.CleanUp("example.com", "tok", "key"))
	})

	t.Run("unregistered provider returns error", func(t *testing.T) {
		_, err := GetDNSProvider("nonexistent", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `dns provider "nonexistent" not registered`)
	})

	t.Run("factory error propagates", func(t *testing.T) {
		_, err := GetDNSProvider("mock", map[string]string{"fail": "true"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "factory error")
	})
}

func TestGetDNSProviderThreadSafe(t *testing.T) {
	providersMu.Lock()
	providers = make(map[string]DNSProviderFactory)
	providersMu.Unlock()

	RegisterDNSProvider("safe", mockFactory)

	done := make(chan struct{})
	go func() {
		_, _ = GetDNSProvider("safe", nil)
		close(done)
	}()

	_, err := GetDNSProvider("safe", nil)
	require.NoError(t, err)
	<-done
}
