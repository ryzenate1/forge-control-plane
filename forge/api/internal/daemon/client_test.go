package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientSignsRequestsWithIndependentNodeCredentials(t *testing.T) {
	var tokenA atomic.Value
	tokenA.Store("node-a.token-a")
	serverA := newSigningTestServer(&tokenA)
	defer serverA.Close()

	var tokenB atomic.Value
	tokenB.Store("node-b.token-b")
	serverB := newSigningTestServer(&tokenB)
	defer serverB.Close()

	client := NewClient()
	ctx := context.Background()

	errCh := make(chan error, 2)
	go func() {
		_, err := client.Stats(ctx, serverA.URL, "node-a.token-a", "server-a")
		errCh <- err
	}()
	go func() {
		_, err := client.Stats(ctx, serverB.URL, "node-b.token-b", "server-b")
		errCh <- err
	}()
	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatalf("independently signed request failed: %v", err)
		}
	}

	// Simulate loading a fresh target after rotating node A's credential. The
	// same shared client must sign the next request with the new target value.
	tokenA.Store("node-a.token-rotated")
	if _, err := client.Stats(ctx, serverA.URL, "node-a.token-rotated", "server-a"); err != nil {
		t.Fatalf("request with rotated credential failed: %v", err)
	}
	if _, err := client.Stats(ctx, serverA.URL, "node-a.token-a", "server-a"); err == nil {
		t.Fatal("request with stale credential unexpectedly succeeded after rotation")
	}
}

func TestReinstallServerUsesBeaconReinstallContractAndNodeCredential(t *testing.T) {
	var method, path, timestamp, signature string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		timestamp = r.Header.Get("X-Panel-Timestamp")
		signature = r.Header.Get("X-Panel-Signature")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"serverId":"server-a","accepted":true,"mode":"docker","exitCode":0}`))
	}))
	defer server.Close()

	client := NewClientWithDevelopmentFallback("fallback.token")
	if _, err := client.ReinstallServer(context.Background(), server.URL, "node-id.node-secret", "server-a", InstallRequest{ServerID: "server-a"}); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || path != "/servers/server-a/reinstall" {
		t.Fatalf("request = %s %s, want POST /servers/server-a/reinstall", method, path)
	}
	if want := sign("node-id.node-secret", http.MethodPost, "/servers/server-a/reinstall", timestamp, []byte(`{"serverId":"server-a","image":"","entrypoint":"","script":"","env":null}`)); signature != want {
		t.Fatal("request was not signed with the per-node credential")
	}
}

func TestPullRemoteFileUsesBeaconHardenedPullContract(t *testing.T) {
	const token = "node-id.node-secret"
	var received bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		timestamp := r.Header.Get("X-Panel-Timestamp")
		if r.Method != http.MethodPost || r.URL.Path != "/servers/server-a/files/pull" {
			http.Error(w, "wrong route", http.StatusNotFound)
			return
		}
		if r.Header.Get("X-Panel-Signature") != sign(token, r.Method, r.URL.RequestURI(), timestamp, body) {
			http.Error(w, "bad signature", http.StatusUnauthorized)
			return
		}
		var payload map[string]string
		if err := json.Unmarshal(body, &payload); err != nil || payload["url"] != "https://downloads.example/game.bin" || payload["target"] != "mods" || payload["fileName"] != "game.bin" {
			http.Error(w, "bad payload", http.StatusBadRequest)
			return
		}
		received = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	if err := NewClient().PullRemoteFile(context.Background(), server.URL, token, "server-a", "https://downloads.example/game.bin", "mods", "game.bin"); err != nil {
		t.Fatal(err)
	}
	if !received {
		t.Fatal("Beacon pull endpoint did not receive the request")
	}
}

func TestPullRemoteFileReturnsBoundedDaemonErrorDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "private network destination is not allowed", http.StatusForbidden)
	}))
	defer server.Close()

	err := NewClient().PullRemoteFile(context.Background(), server.URL, "node.secret", "server-a", "http://127.0.0.1/secret", "", "secret")
	if err == nil || !strings.Contains(err.Error(), "status 403") || !strings.Contains(err.Error(), "private network") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClientFailsClosedWithoutNodeCredential(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := NewClient().Stats(context.Background(), server.URL, "", "server-a")
	if !errors.Is(err, ErrMissingNodeToken) {
		t.Fatalf("Stats() error = %v, want %v", err, ErrMissingNodeToken)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("daemon received %d requests without a credential, want 0", got)
	}
}

func TestTransferCredentialRegistrationUsesNodeAuthAndScopedCallsUseOnlyTransferBearer(t *testing.T) {
	const nodeToken = "node-id.node-secret"
	const scoped = "scoped-random-credential"
	var registered bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/transfers/credentials":
			body, _ := io.ReadAll(r.Body)
			timestamp := r.Header.Get("X-Panel-Timestamp")
			if r.Header.Get("Authorization") != "" || r.Header.Get("X-Panel-Signature") != sign(nodeToken, r.Method, r.URL.RequestURI(), timestamp, body) {
				http.Error(w, "bad node auth", http.StatusUnauthorized)
				return
			}
			var registration TransferCredentialRegistration
			if json.Unmarshal(body, &registration) != nil || registration.CredentialHash == scoped || registration.Claims.Direction != TransferDirectionSourceControl {
				http.Error(w, "bad registration", http.StatusBadRequest)
				return
			}
			registered = true
			w.WriteHeader(http.StatusCreated)
		case "/api/v1/transfers/migration-1/source/prepare":
			if !registered || r.Header.Get("Authorization") != "Bearer "+scoped || r.Header.Get("X-Panel-Signature") != "" || strings.Contains(r.RequestURI, scoped) {
				http.Error(w, "bad scoped auth", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"forge-beacon-transfer/v1","phase":"archived","archiveSize":10,"checksum":"abc"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClient()
	claims := TransferCredentialClaims{Version: TransferProtocolVersion, MigrationID: "migration-1", ServerID: "server-1", SourceNodeID: "source-1", TargetNodeID: "target-1", Direction: TransferDirectionSourceControl, ExpiresAt: time.Now().Add(time.Minute)}
	if err := client.RegisterTransferCredential(context.Background(), server.URL, nodeToken, TransferCredentialRegistration{Claims: claims, CredentialHash: strings.Repeat("a", 64)}); err != nil {
		t.Fatal(err)
	}
	metadata, err := client.PrepareTransferSource(context.Background(), server.URL, "migration-1", scoped)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Phase != "archived" || metadata.ArchiveSize != 10 {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
}

func newSigningTestServer(expectedToken *atomic.Value) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timestamp := r.Header.Get("X-Panel-Timestamp")
		expectedSignature := sign(expectedToken.Load().(string), r.Method, r.URL.RequestURI(), timestamp, nil)
		if timestamp == "" || r.Header.Get("X-Panel-Signature") != expectedSignature {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cpuPercent":1}`))
	}))
}
