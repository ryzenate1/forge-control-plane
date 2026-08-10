package dns

import (
	"encoding/json"
	"os"
	"testing"

	"gamepanel/forge/internal/store"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListSupportedProviders(t *testing.T) {
	svc, _ := New(nil)
	providers := svc.ListSupportedProviders()

	assert.Greater(t, len(providers), 30, "should have 30+ providers")
	assert.True(t, len(providers) >= 34, "should have at least 34 providers")

	seen := make(map[string]bool)
	for _, p := range providers {
		assert.NotEmpty(t, p.Type, "provider type should not be empty")
		assert.NotEmpty(t, p.Name, "provider name should not be empty")
		assert.False(t, seen[p.Type], "duplicate provider type: %s", p.Type)
		seen[p.Type] = true

		for _, f := range p.CredentialFields {
			assert.NotEmpty(t, f.Key, "credential field key should not be empty")
			assert.NotEmpty(t, f.Label, "credential field label should not be empty")
		}
	}

	// Verify sorted by name
	for i := 1; i < len(providers); i++ {
		assert.True(t, providers[i-1].Name <= providers[i].Name, "providers should be sorted by name")
	}

	// Spot check common providers
	types := make(map[string]string)
	for _, p := range providers {
		types[p.Type] = p.Name
	}

	assert.Equal(t, "Cloudflare", types["cloudflare"])
	assert.Equal(t, "Amazon Route 53", types["route53"])
	assert.Equal(t, "DigitalOcean", types["digitalocean"])
	assert.Equal(t, "Vultr", types["vultr"])
	assert.Equal(t, "OVH", types["ovh"])
	assert.Equal(t, "GoDaddy", types["godaddy"])
	assert.Equal(t, "Azure DNS", types["azure"])
	assert.Equal(t, "Google Cloud DNS", types["gcloud"])
	assert.Equal(t, "Porkbun", types["porkbun"])
	assert.Equal(t, "Namecheap", types["namecheap"])
}

func TestGetProviderDefinition(t *testing.T) {
	svc, _ := New(nil)

	t.Run("valid provider", func(t *testing.T) {
		def, err := svc.GetProviderDefinition("cloudflare")
		require.NoError(t, err)
		assert.Equal(t, "cloudflare", def.Type)
		assert.Equal(t, "Cloudflare", def.Name)
		assert.NotEmpty(t, def.CredentialFields)
		assert.Equal(t, "CF_DNS_API_TOKEN", def.CredentialFields[0].Key)
	})

	t.Run("invalid provider", func(t *testing.T) {
		_, err := svc.GetProviderDefinition("nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported provider type")
	})
}

func TestCredentialsFromProvider(t *testing.T) {
	creds := map[string]string{
		"CF_DNS_API_TOKEN": "test-token-123",
	}
	raw, err := json.Marshal(creds)
	require.NoError(t, err)

	result := credentialsFromProvider(store.DNSProvider{
		Credentials: json.RawMessage(raw),
	})
	assert.Equal(t, "test-token-123", result["CF_DNS_API_TOKEN"])
}

func TestCredentialsFromProviderEmpty(t *testing.T) {
	result := credentialsFromProvider(store.DNSProvider{})
	assert.Empty(t, result)
}

func TestSetEnvRestore(t *testing.T) {
	env := map[string]string{
		"TEST_DNS_VAR_1": "value1",
		"TEST_DNS_VAR_2": "value2",
	}

	restore := setEnvRestore(env)
	v1, ok := os.LookupEnv("TEST_DNS_VAR_1")
	assert.True(t, ok)
	assert.Equal(t, "value1", v1)

	restore()

	_, ok = os.LookupEnv("TEST_DNS_VAR_1")
	assert.False(t, ok, "env var should be restored")
}

func TestSetEnvRestoreOverwritesExisting(t *testing.T) {
	os.Setenv("TEST_DNS_PREEXIST", "original")
	defer os.Unsetenv("TEST_DNS_PREEXIST")

	env := map[string]string{
		"TEST_DNS_PREEXIST": "overwritten",
	}

	restore := setEnvRestore(env)
	assert.Equal(t, "overwritten", os.Getenv("TEST_DNS_PREEXIST"))

	restore()

	assert.Equal(t, "original", os.Getenv("TEST_DNS_PREEXIST"))
}

func TestCreateDNSProvider(t *testing.T) {
	env := map[string]string{
		"TEST_FLAG": "set",
	}

	called := false
	newProv := func() (challenge.Provider, error) {
		called = true
		assert.Equal(t, "set", os.Getenv("TEST_FLAG"))
		return &mockProvider{}, nil
	}

	cp, err := createDNSProvider(env, newProv)
	require.NoError(t, err)
	assert.True(t, called)
	assert.NotNil(t, cp)

	// Credential env vars are restored immediately after construction so
	// secrets never linger in the process environment.
	_, present := os.LookupEnv("TEST_FLAG")
	assert.False(t, present, "TEST_FLAG should be restored after construction")
}

type mockProvider struct {
	presentCalls int
	cleanUpCalls int
	cleanUpHook  func()
}

func (m *mockProvider) Present(domain, token, keyAuth string) error {
	m.presentCalls++
	return nil
}

func (m *mockProvider) CleanUp(domain, token, keyAuth string) error {
	m.cleanUpCalls++
	if m.cleanUpHook != nil {
		m.cleanUpHook()
	}
	return nil
}

func TestProviderRegistryHasAllDefinitions(t *testing.T) {
	svc, _ := New(nil)
	for _, def := range svc.ListSupportedProviders() {
		_, ok := providerRegistry[def.Type]
		assert.True(t, ok, "provider %s should have a factory registered", def.Type)
	}
}

func TestConfigureProviderValidation(t *testing.T) {
	svc, _ := New(nil)

	_, err := svc.GetProviderDefinition("cloudflare")
	require.NoError(t, err)

	_, err = svc.GetProviderDefinition("nonexistent-type")
	assert.Error(t, err)
}

func TestVerifyProviderInit(t *testing.T) {
	_, ok := providerRegistry["cloudflare"]
	assert.True(t, ok, "cloudflare should be registered")

	_, ok = providerRegistry["route53"]
	assert.True(t, ok, "route53 should be registered")
}
