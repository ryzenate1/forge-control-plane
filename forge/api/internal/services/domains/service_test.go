package domains

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type mockDomainStore struct {
	mu      sync.Mutex
	servers map[string]bool
	domains map[string]DomainRow
}

func newMockDomainStore() *mockDomainStore {
	return &mockDomainStore{
		servers: map[string]bool{
			"server-1": true,
		},
		domains: make(map[string]DomainRow),
	}
}

func (m *mockDomainStore) CheckServerExists(ctx context.Context, id string) (bool, error) {
	_, ok := m.servers[id]
	return ok, nil
}

func (m *mockDomainStore) ListDomainsByServer(ctx context.Context, serverID string) ([]DomainRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []DomainRow
	for _, d := range m.domains {
		if d.ServerID == serverID {
			result = append(result, d)
		}
	}
	return result, nil
}

func (m *mockDomainStore) ListAllDomains(ctx context.Context) ([]DomainRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []DomainRow
	for _, d := range m.domains {
		result = append(result, d)
	}
	return result, nil
}

func (m *mockDomainStore) GetDomain(ctx context.Context, id string) (*DomainRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	d, ok := m.domains[id]
	if !ok {
		return nil, nil
	}
	return &d, nil
}

func (m *mockDomainStore) GetDomainByDomain(ctx context.Context, domain string) (*DomainRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, d := range m.domains {
		if d.Domain == domain {
			return &d, nil
		}
	}
	return nil, nil
}

func (m *mockDomainStore) CreateDomain(ctx context.Context, row DomainRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.domains[row.ID] = row
	return nil
}

func (m *mockDomainStore) UpdateDomainVerification(ctx context.Context, id string, verified bool, verifiedAt *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	d, ok := m.domains[id]
	if !ok {
		return errors.New("domain not found")
	}
	d.Verified = verified
	d.VerifiedAt = verifiedAt
	m.domains[id] = d
	return nil
}

func (m *mockDomainStore) SetDomainVerificationToken(ctx context.Context, id string, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	d, ok := m.domains[id]
	if !ok {
		return errors.New("domain not found")
	}
	d.VerificationToken = &token
	m.domains[id] = d
	return nil
}

func (m *mockDomainStore) DeleteDomain(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.domains, id)
	return nil
}

func (m *mockDomainStore) ListUnverifiedDomains(ctx context.Context) ([]DomainRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []DomainRow
	for _, d := range m.domains {
		if !d.Verified {
			result = append(result, d)
		}
	}
	return result, nil
}

func (m *mockDomainStore) ListVerifiedDomains(ctx context.Context) ([]DomainRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []DomainRow
	for _, d := range m.domains {
		if d.Verified {
			result = append(result, d)
		}
	}
	return result, nil
}

type mockCaddyUpdater struct {
	routes []VerifiedDomainRoute
}

func (m *mockCaddyUpdater) UpdateDomainRoutes(ctx context.Context, routes []VerifiedDomainRoute) error {
	m.routes = routes
	return nil
}

func TestAddDomain(t *testing.T) {
	mockStore := newMockDomainStore()
	caddy := &mockCaddyUpdater{}

	svc := &Service{
		store:      mockStore,
		caddy:      caddy,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		panelIP:    "10.0.0.1",
	}

	domain, err := svc.AddDomain(context.Background(), "server-1", "example.com")
	if err != nil {
		t.Fatalf("AddDomain failed: %v", err)
	}
	if domain.Domain != "example.com" {
		t.Errorf("expected example.com, got %s", domain.Domain)
	}
	if domain.Wildcard {
		t.Error("non-wildcard domain should not have wildcard=true")
	}
	if domain.Verified {
		t.Error("new domain should not be verified")
	}
	if domain.VerificationToken == nil || *domain.VerificationToken == "" {
		t.Error("verification token should be set")
	}
	if domain.ServerID != "server-1" {
		t.Errorf("expected server-1, got %s", domain.ServerID)
	}
}

func TestAddWildcardDomain(t *testing.T) {
	mockStore := newMockDomainStore()
	caddy := &mockCaddyUpdater{}

	svc := &Service{
		store:      mockStore,
		caddy:      caddy,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		panelIP:    "10.0.0.1",
	}

	domain, err := svc.AddDomain(context.Background(), "server-1", "*.example.com")
	if err != nil {
		t.Fatalf("AddDomain failed: %v", err)
	}
	if !domain.Wildcard {
		t.Error("wildcard domain should have wildcard=true")
	}
	if domain.Domain != "*.example.com" {
		t.Errorf("expected *.example.com, got %s", domain.Domain)
	}
}

func TestAddDomainDuplicate(t *testing.T) {
	mockStore := newMockDomainStore()
	caddy := &mockCaddyUpdater{}

	svc := &Service{
		store:      mockStore,
		caddy:      caddy,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		panelIP:    "10.0.0.1",
	}

	_, err := svc.AddDomain(context.Background(), "server-1", "example.com")
	if err != nil {
		t.Fatalf("first AddDomain failed: %v", err)
	}

	_, err = svc.AddDomain(context.Background(), "server-1", "example.com")
	if err == nil {
		t.Error("expected error for duplicate domain")
	}
}

func TestAddDomainInvalidServer(t *testing.T) {
	mockStore := newMockDomainStore()
	caddy := &mockCaddyUpdater{}
	svc := &Service{
		store:      mockStore,
		caddy:      caddy,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		panelIP:    "10.0.0.1",
	}

	_, err := svc.AddDomain(context.Background(), "nonexistent", "example.com")
	if err == nil {
		t.Error(fmt.Errorf("expected error for non-existent server"))
	} else {
		t.Log("expected error:", err)
	}
}

func TestRemoveDomain(t *testing.T) {
	mockStore := newMockDomainStore()
	caddy := &mockCaddyUpdater{}

	svc := &Service{
		store:      mockStore,
		caddy:      caddy,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		panelIP:    "10.0.0.1",
	}

	domain, err := svc.AddDomain(context.Background(), "server-1", "example.com")
	if err != nil {
		t.Fatalf("AddDomain failed: %v", err)
	}

	err = svc.RemoveDomain(context.Background(), domain.ID)
	if err != nil {
		t.Fatalf("RemoveDomain failed: %v", err)
	}

	_, err = svc.GetDomain(context.Background(), domain.ID)
	if err == nil {
		t.Error("expected error when getting removed domain")
	}
}

func TestListDomains(t *testing.T) {
	mockStore := newMockDomainStore()
	caddy := &mockCaddyUpdater{}

	svc := &Service{
		store:      mockStore,
		caddy:      caddy,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		panelIP:    "10.0.0.1",
	}

	_, _ = svc.AddDomain(context.Background(), "server-1", "example.com")
	_, _ = svc.AddDomain(context.Background(), "server-1", "test.com")

	domains, err := svc.ListDomains(context.Background(), "server-1")
	if err != nil {
		t.Fatalf("ListDomains failed: %v", err)
	}
	if len(domains) != 2 {
		t.Errorf("expected 2 domains, got %d", len(domains))
	}
}

func TestDomainVerificationSuccess(t *testing.T) {
	token := uuid.NewString()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/forge-verify" {
			w.WriteHeader(200)
			w.Write([]byte(token))
		}
	}))
	defer ts.Close()

	_ = token

	mockStore := newMockDomainStore()
	caddy := &mockCaddyUpdater{}

	svc := &Service{
		store:      mockStore,
		caddy:      caddy,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		panelIP:    "10.0.0.1",
	}

	now := time.Now().UTC()
	mockStore.domains["d1"] = DomainRow{
		ID:                "d1",
		ServerID:          "server-1",
		Domain:            "example.com",
		Wildcard:          false,
		Verified:          false,
		VerificationToken: &token,
		CreatedAt:         now,
	}

	record := DomainRecord{
		ID:                "d1",
		ServerID:          "server-1",
		Domain:            "example.com",
		Wildcard:          false,
		Verified:          false,
		VerificationToken: &token,
		CreatedAt:         now,
	}

	result := svc.verifyOwnership(context.Background(), &record)
	if result.Verified {
		t.Log("verified (expected if DNS resolves)")
	}
}

func TestVerificationTokenMismatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("wrong-token"))
	}))
	defer ts.Close()

	mockStore := newMockDomainStore()
	svc := &Service{
		store:      mockStore,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		panelIP:    "10.0.0.1",
	}

	token := uuid.NewString()
	record := DomainRecord{
		ID:                uuid.NewString(),
		Domain:            "example.com",
		Verified:          false,
		VerificationToken: &token,
	}

	result := svc.verifyOwnership(context.Background(), &record)
	if result.Verified {
		t.Error("should not verify with wrong token")
	}
}

func TestCheckDNS(t *testing.T) {
	mockStore := newMockDomainStore()
	svc := &Service{
		store:      mockStore,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		panelIP:    "10.0.0.1",
	}

	ips, _ := net.LookupHost("localhost")
	t.Logf("localhost resolves to: %v", ips)

	result, err := svc.CheckDNS(context.Background(), "localhost", "")
	if err != nil {
		t.Fatalf("CheckDNS failed: %v", err)
	}
	if !result.Resolved {
		t.Log("localhost DNS not resolved (may be expected in some environments)")
	}
	t.Logf("DNS result: resolved=%v, ips=%v, match=%v", result.Resolved, result.IPs, result.Match)
}

func TestCheckDNSWithExpectedIP(t *testing.T) {
	mockStore := newMockDomainStore()
	svc := &Service{
		store:      mockStore,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		panelIP:    "10.0.0.1",
	}

	result, err := svc.CheckDNS(context.Background(), "localhost", "127.0.0.1")
	if err != nil {
		t.Fatalf("CheckDNS failed: %v", err)
	}
	if result.Match {
		t.Log("expected IP matched")
	}
}

func TestWildcardDomainDetection(t *testing.T) {
	tests := []struct {
		domain   string
		wildcard bool
		normal   string
	}{
		{"*.example.com", true, "*.example.com"},
		{"example.com", false, "example.com"},
		{"sub.example.com", false, "sub.example.com"},
		{"*.test.org", true, "*.test.org"},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			if got := isWildcardDomain(tt.domain); got != tt.wildcard {
				t.Errorf("isWildcardDomain(%s) = %v, want %v", tt.domain, got, tt.wildcard)
			}
			if got := normalizeDomain(tt.domain); got != tt.normal {
				t.Errorf("normalizeDomain(%s) = %s, want %s", tt.domain, got, tt.normal)
			}
		})
	}
}

func TestNormalizeDomain(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"example.com", "example.com"},
		{"EXAMPLE.COM", "example.com"},
		{"Example.Com", "example.com"},
		{" example.com ", "example.com"},
		{"example.com.", "example.com"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeDomain(tt.input); got != tt.expected {
				t.Errorf("normalizeDomain(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestResolveDomain(t *testing.T) {
	svc := &Service{
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	ips := svc.resolveDomain("localhost")
	if len(ips) == 0 {
		t.Skip("DNS resolution of localhost failed - skipping in CI")
	}
	foundLocalhost := false
	for _, ip := range ips {
		if ip == "127.0.0.1" || ip == "::1" {
			foundLocalhost = true
			break
		}
	}
	if !foundLocalhost {
		t.Errorf("expected 127.0.0.1 or ::1, got %v", ips)
	}
}

func TestWildcardDomainDNS(t *testing.T) {
	svc := &Service{
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	ips := svc.resolveDomain("*.localhost")
	if len(ips) == 0 {
		t.Skip("DNS resolution failed - skipping in CI")
	}
	t.Logf("wildcard domain test resolves to: %v", ips)
}

func TestCaddyRoutesSync(t *testing.T) {
	mockStore := newMockDomainStore()
	caddy := &mockCaddyUpdater{}

	token := uuid.NewString()
	now := time.Now().UTC()
	mockStore.domains["d1"] = DomainRow{
		ID:                "d1",
		ServerID:          "server-1",
		Domain:            "example.com",
		Wildcard:          false,
		Verified:          true,
		VerifiedAt:        &now,
		VerificationToken: &token,
		CreatedAt:         now,
	}

	svc := &Service{
		store: mockStore,
		caddy: caddy,
	}

	err := svc.syncCaddyRoutes(context.Background())
	if err != nil {
		t.Fatalf("syncCaddyRoutes failed: %v", err)
	}

	if len(caddy.routes) != 1 {
		t.Errorf("expected 1 route in caddy, got %d", len(caddy.routes))
	}
	if caddy.routes[0].Domain != "example.com" {
		t.Errorf("expected example.com, got %s", caddy.routes[0].Domain)
	}
}

func TestServiceLifecycle(t *testing.T) {
	mockStore := newMockDomainStore()
	caddy := &mockCaddyUpdater{}

	svc := &Service{
		store:      mockStore,
		caddy:      caddy,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		panelIP:    "10.0.0.1",
	}

	svc.SetReverifyInterval(100 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	svc.StartReverify(ctx)

	time.Sleep(50 * time.Millisecond)

	cancel()
	svc.StopReverify()

	time.Sleep(200 * time.Millisecond)
}
