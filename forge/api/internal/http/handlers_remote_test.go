package http

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func remoteHMACTestApp(token string) *fiber.App {
	app := fiber.New()
	nonces := newRemoteNonceStore()
	app.Post("/api/remote/test", func(c *fiber.Ctx) error {
		if err := verifyRemoteHMAC(c, token, nonces); err != nil {
			return fiber.NewError(fiber.StatusForbidden, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	return app
}

func signedRemoteRequest(token, timestamp, nonce string, body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/remote/test", bytes.NewReader(body))
	req.Header.Set("X-Panel-Timestamp", timestamp)
	req.Header.Set("X-Panel-Nonce", nonce)
	req.Header.Set("X-Panel-Signature", signHMAC(token, http.MethodPost, "/api/remote/test", timestamp, nonce, body))
	return req
}

func TestRemoteHMACRejectsUnsignedStaleAndReplayedRequests(t *testing.T) {
	const token = "node-id.node-secret"
	app := remoteHMACTestApp(token)

	unsigned, err := app.Test(httptest.NewRequest(http.MethodPost, "/api/remote/test", nil))
	if err != nil || unsigned.StatusCode != http.StatusForbidden {
		t.Fatalf("unsigned status=%v err=%v, want 403", unsigned.StatusCode, err)
	}

	stale := signedRemoteRequest(token, time.Now().Add(-10*time.Minute).UTC().Format(time.RFC3339), "11111111111111111111111111111111", []byte(`{"ok":true}`))
	staleResponse, err := app.Test(stale)
	if err != nil || staleResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("stale status=%v err=%v, want 403", staleResponse.StatusCode, err)
	}

	future := signedRemoteRequest(token, time.Now().Add(10*time.Minute).UTC().Format(time.RFC3339), "44444444444444444444444444444444", []byte(`{"ok":true}`))
	futureResponse, err := app.Test(future)
	if err != nil || futureResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("future timestamp status=%v err=%v, want 403", futureResponse.StatusCode, err)
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	nonce := "22222222222222222222222222222222"
	first, err := app.Test(signedRemoteRequest(token, timestamp, nonce, []byte(`{"ok":true}`)))
	if err != nil || first.StatusCode != http.StatusNoContent {
		t.Fatalf("first status=%v err=%v, want 204", first.StatusCode, err)
	}
	replay, err := app.Test(signedRemoteRequest(token, timestamp, nonce, []byte(`{"ok":true}`)))
	if err != nil || replay.StatusCode != http.StatusForbidden {
		t.Fatalf("replay status=%v err=%v, want 403", replay.StatusCode, err)
	}
}

func TestRemoteHMACRejectsTamperedBodiesAndSignatures(t *testing.T) {
	const token = "node-id.node-secret"
	app := remoteHMACTestApp(token)
	timestamp := time.Now().UTC().Format(time.RFC3339)
	nonce := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	tamperedBody := signedRemoteRequest(token, timestamp, nonce, []byte(`{"ok":true}`))
	body := []byte(`{"ok":false}`)
	tamperedBody.Body = io.NopCloser(bytes.NewReader(body))
	tamperedBody.ContentLength = int64(len(body))
	tamperedBody.GetBody = nil
	resp, err := app.Test(tamperedBody)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("tampered body status=%v err=%v, want 403", resp.StatusCode, err)
	}

	tamperedSignature := signedRemoteRequest(token, time.Now().UTC().Format(time.RFC3339), "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", []byte(`{"ok":true}`))
	tamperedSignature.Header.Set("X-Panel-Signature", strings.Repeat("0", 64))
	resp, err = app.Test(tamperedSignature)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("tampered signature status=%v err=%v, want 403", resp.StatusCode, err)
	}
}

// TestBeaconRemoteContractRoutesRegistered pins the /api/remote and node v1
// routes beacon's remote client (beacon/internal/remote/client.go) calls, so a
// future route removal breaks the contract test instead of the daemon.
func TestBeaconRemoteContractRoutesRegistered(t *testing.T) {
	app := NewServer(Config{ReadTimeout: time.Second})
	registered := map[string]bool{}
	for _, route := range app.GetRoutes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"GET /api/remote/servers",
		"POST /api/remote/servers/reset",
		"POST /api/remote/activity",
		"GET /api/remote/servers/:id",
		"POST /api/remote/servers/:id/install",
		"POST /api/remote/servers/:id/crash",
		"POST /api/remote/servers/:id/backups/status",
		"POST /api/remote/servers/:id/backups/restore-status",
		"POST /api/remote/sftp/auth",
		"POST /api/v1/nodes/:id/heartbeat",
		"POST /api/v1/nodes/capabilities",
	} {
		if !registered[route] {
			t.Errorf("beacon contract route %s is not registered", route)
		}
	}
}

func TestRemoteHMACRejectsForgedAndMalformedNonces(t *testing.T) {
	const token = "node-id.node-secret"
	app := remoteHMACTestApp(token)
	timestamp := time.Now().UTC().Format(time.RFC3339)

	// Signature covers nonce A, but the request carries a different nonce B.
	forgedNonce := signedRemoteRequest(token, timestamp, "cccccccccccccccccccccccccccccccc", []byte(`{"ok":true}`))
	forgedNonce.Header.Set("X-Panel-Nonce", "dddddddddddddddddddddddddddddddd")
	resp, err := app.Test(forgedNonce)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("forged nonce status=%v err=%v, want 403", resp.StatusCode, err)
	}

	// Non-hex nonce, even with a matching signature over it, must be rejected.
	badNonce := "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"
	nonHexNonce := signedRemoteRequest(token, timestamp, badNonce, []byte(`{"ok":true}`))
	resp, err = app.Test(nonHexNonce)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-hex nonce status=%v err=%v, want 403", resp.StatusCode, err)
	}

	// Short nonce with a matching signature must be rejected.
	shortNonce := signedRemoteRequest(token, timestamp, "short", []byte(`{"ok":true}`))
	resp, err = app.Test(shortNonce)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("short nonce status=%v err=%v, want 403", resp.StatusCode, err)
	}

	// Missing nonce entirely must be rejected even with a signature.
	missingNonce := signedRemoteRequest(token, timestamp, "", []byte(`{"ok":true}`))
	resp, err = app.Test(missingNonce)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("missing nonce status=%v err=%v, want 403", resp.StatusCode, err)
	}
}
