package nodeprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/secrets"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	keyring, err := secrets.New("test", "0000000000000000000000000000000000000000000000000000000000000000", nil)
	if err != nil {
		t.Fatalf("create keyring: %v", err)
	}
	s, err := store.ConnectWithKeyring(context.Background(), databaseURL, keyring)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(s.Close)
	if err := s.RunMigrations(context.Background(), "../../migrations"); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	return s
}

// seedNode creates a location, region, and node via the store, returning the node ID and its credential.
func seedNode(t *testing.T, s *store.Store, ctx context.Context, fqdn, scheme string) (nodeID, credential string) {
	t.Helper()
	locationID := uuid.NewString()
	regionID := uuid.NewString()

	if _, err := s.DB().Exec(ctx, `INSERT INTO locations (id, short, long) VALUES ($1, 'test', 'Test')`, locationID); err != nil {
		t.Fatalf("insert location: %v", err)
	}
	if _, err := s.DB().Exec(ctx, `INSERT INTO regions (id, uuid, name, slug) VALUES ($1, $1, 'Test', 'test')`, regionID); err != nil {
		t.Fatalf("insert region: %v", err)
	}

	node, token, err := s.CreateNode(ctx, store.CreateNodeRequest{
		Name:        "Test Node",
		LocationID:  locationID,
		RegionID:    regionID,
		Description: "test node",
		BaseURL:     scheme + "://" + fqdn,
		FQDN:        fqdn,
		Scheme:      scheme,
		MemoryMB:    4096,
		DiskMB:      102400,
		UploadSizeMB: 100,
		DaemonBase:   "/srv/daemon",
		DaemonListen: 8080,
		DaemonSFTP:   2022,
	}, nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	return node.ID, token
}

func TestProbeNodeStoreIsNil(t *testing.T) {
	svc := &Service{client: &http.Client{Timeout: time.Second}, signer: daemon.NewClient()}
	info, err := svc.ProbeNode(context.Background(), "test-id")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if info.Online {
		t.Fatal("expected Online=false when store is nil")
	}
	if info.Error != "no database connection" {
		t.Fatalf("error = %q, want %q", info.Error, "no database connection")
	}
}

func TestProbeNodeNotFound(t *testing.T) {
	s := testStore(t)
	svc := NewService(s)
	info, err := svc.ProbeNode(context.Background(), uuid.NewString())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if info.Online {
		t.Fatal("expected Online=false when node not found")
	}
}

func TestProbeNodeNoFQDN(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	locationID := uuid.NewString()
	nodeID := uuid.NewString()

	if _, err := s.DB().Exec(ctx, `INSERT INTO locations (id, short, long) VALUES ($1, 'test', 'Test')`, locationID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(ctx, `INSERT INTO nodes (id, name, region, base_url, token_hash, location_id) VALUES ($1, 'NoFQDN', 'test', 'https://x.test', 'hash', $2)`, nodeID, locationID); err != nil {
		t.Fatal(err)
	}

	svc := NewService(s)
	info, err := svc.ProbeNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if info.Online {
		t.Fatal("expected Online=false when no FQDN")
	}
	if !strings.Contains(info.Error, "no FQDN") {
		t.Fatalf("error = %q, want substring %q", info.Error, "no FQDN")
	}
}

func TestProbeNodeDaemonSuccess(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	returned := daemonSystemResponse{
		NodeID:        "test-node",
		Version:       "1.2.3",
		OS:            "linux",
		Architecture:  "x86_64",
		CPUThreads:    8,
		MemoryMB:      16384,
		DockerStatus:  "ok",
		Capabilities:  []string{"docker", "backup"},
		UptimeSeconds: 3600,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/system" {
			http.Error(w, "wrong route", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(returned)
	}))
	defer server.Close()

	fqdn := strings.TrimPrefix(server.URL, "http://")
	nodeID, _ := seedNode(t, s, ctx, fqdn, "http")

	svc := NewService(s)
	info, err := svc.ProbeNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("ProbeNode: %v", err)
	}
	if !info.Online {
		t.Fatalf("expected Online=true, got error=%q", info.Error)
	}
	if info.Version != "1.2.3" {
		t.Fatalf("version = %q, want %q", info.Version, "1.2.3")
	}
	if info.OS != "linux" {
		t.Fatalf("os = %q, want %q", info.OS, "linux")
	}
	if info.CPUThreads != 8 {
		t.Fatalf("cpuThreads = %d, want %d", info.CPUThreads, 8)
	}
	if info.MemoryMB != 16384 {
		t.Fatalf("memoryMb = %d, want %d", info.MemoryMB, 16384)
	}
	if !info.DockerAvailable {
		t.Fatal("expected DockerAvailable=true")
	}
	if info.DockerStatus != "ok" {
		t.Fatalf("dockerStatus = %q, want %q", info.DockerStatus, "ok")
	}
	if len(info.Capabilities) != 2 || info.Capabilities[0] != "docker" {
		t.Fatalf("capabilities = %v, want [docker backup]", info.Capabilities)
	}
	if info.UptimeSeconds != 3600 {
		t.Fatalf("uptimeSeconds = %d, want %d", info.UptimeSeconds, 3600)
	}
}

func TestProbeNodeDaemonNonOKStatus(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	fqdn := strings.TrimPrefix(server.URL, "http://")
	nodeID, _ := seedNode(t, s, ctx, fqdn, "http")

	svc := NewService(s)
	info, err := svc.ProbeNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if info.Online {
		t.Fatal("expected Online=false for non-2xx response")
	}
	if !strings.Contains(info.Error, "503") {
		t.Fatalf("error = %q, want status 503", info.Error)
	}
}

func TestProbeNodeDaemonInvalidJSON(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `not json at all`)
	}))
	defer server.Close()

	fqdn := strings.TrimPrefix(server.URL, "http://")
	nodeID, _ := seedNode(t, s, ctx, fqdn, "http")

	svc := NewService(s)
	info, err := svc.ProbeNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if info.Online {
		t.Fatal("expected Online=false for invalid JSON")
	}
	if !strings.Contains(info.Error, "invalid JSON") {
		t.Fatalf("error = %q, want 'invalid JSON'", info.Error)
	}
}

func TestProbeNodeNetworkError(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	locationID := uuid.NewString()
	nodeID := uuid.NewString()

	if _, err := s.DB().Exec(ctx, `INSERT INTO locations (id, short, long) VALUES ($1, 'test', 'Test')`, locationID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(ctx, `INSERT INTO regions (id, uuid, name, slug) VALUES ($1, $1, 'Test', 'test')`, locationID); err != nil {
		t.Fatal(err)
	}

	// 192.0.2.0/24 is IANA TEST-NET; connection to port 1 will fail quickly.
	fqdn := "192.0.2.1:1"
	if _, err := s.DB().Exec(ctx, `
		INSERT INTO nodes (id, name, region, base_url, fqdn, scheme, token_hash,
		                   daemon_token_id, daemon_token, location_id, region_id)
		VALUES ($1, 'NetFail', 'test', 'http://`+fqdn+`', $2, 'http', 'hash',
		        'token-id', 'test-token-value', $3, $4)
	`, nodeID, fqdn, locationID, locationID); err != nil {
		t.Fatal(err)
	}

	svc := NewService(s)
	info, err := svc.ProbeNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if info.Online {
		t.Fatal("expected Online=false for network error")
	}
	if info.Error == "" {
		t.Fatal("expected non-empty error for network failure")
	}
}

func TestProbeNodeNoDaemonToken(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	locationID := uuid.NewString()
	nodeID := uuid.NewString()

	if _, err := s.DB().Exec(ctx, `INSERT INTO locations (id, short, long) VALUES ($1, 'test', 'Test')`, locationID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(ctx, `
		INSERT INTO nodes (id, name, region, base_url, fqdn, scheme, token_hash, location_id)
		VALUES ($1, 'NoTokenFQDN', 'test', 'http://example.test', 'example.test', 'http', 'hash', $2)
	`, nodeID, locationID); err != nil {
		t.Fatal(err)
	}

	svc := NewService(s)
	info, err := svc.ProbeNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if info.Online {
		t.Fatal("expected Online=false when no token")
	}
	if !strings.Contains(info.Error, "daemon token") {
		t.Fatalf("error = %q, want 'daemon token'", info.Error)
	}
}

func TestProbeNodeDaemonEmptyDockerStatus(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	returned := daemonSystemResponse{
		NodeID:       "test-node",
		Version:      "1.2.3",
		OS:           "linux",
		Architecture: "x86_64",
		CPUThreads:   4,
		MemoryMB:     8192,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(returned)
	}))
	defer server.Close()

	fqdn := strings.TrimPrefix(server.URL, "http://")
	nodeID, _ := seedNode(t, s, ctx, fqdn, "http")

	svc := NewService(s)
	info, err := svc.ProbeNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("ProbeNode: %v", err)
	}
	if !info.Online {
		t.Fatalf("expected Online=true, got error=%q", info.Error)
	}
	if info.DockerAvailable {
		t.Fatal("expected DockerAvailable=false when status is empty")
	}
}

type daemonSystemResponse struct {
	NodeID        string   `json:"nodeId"`
	Version       string   `json:"version,omitempty"`
	OS            string   `json:"os,omitempty"`
	Architecture  string   `json:"architecture,omitempty"`
	CPUThreads    int      `json:"cpuThreads,omitempty"`
	MemoryMB      uint64   `json:"memoryMb,omitempty"`
	DockerStatus  string   `json:"dockerStatus,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	UptimeSeconds int64    `json:"uptimeSeconds,omitempty"`
}
