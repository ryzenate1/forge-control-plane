package daemon

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// NOTE: The retry round tripper gates retries on request idempotency.
// GET and HEAD are always safe to replay. A mutating request is replayed only
// when it carries a command id on the single route Beacon dedupes on —
// POST /servers/{id}/power, whose handler reads X-Forge-Command-ID (falling
// back to Idempotency-Key) and enqueues by it. Every other mutating route
// ignores those headers, so a replay there would execute the command twice:
// create, transfer push and file writes are never replayed even when they
// carry a key. The HMAC signature is re-generated per attempt via the resign
// callback. Likewise, validateNodeURL keys plain-HTTP acceptance off the
// client's loopback flag (set at construction) rather than inspecting each
// request target, and command IDs ride on the context
// (ContextWithCommandID) instead of CreateServer setting headers. Tests below
// match that behavior.

// countingTransport counts requests and can fail the first N attempts.
type countingTransport struct {
	failures   int
	statusCode int
	requests   int
	lastReq    *http.Request
}

func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.requests++
	c.lastReq = req
	if c.requests <= c.failures {
		return nil, errors.New("transport error")
	}
	return &http.Response{
		StatusCode: c.statusCode,
		Body:       io.NopCloser(strings.NewReader("{}")),
		Header:     http.Header{},
		Request:    req,
	}, nil
}

func newTestRetryClient(base *countingTransport) *retryRoundTripper {
	return &retryRoundTripper{base: base}
}

func fastRequest(t *testing.T, method, target string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, "http://127.0.0.1:1"+target, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return req
}

func TestRetryRoundTripper_RetriesIdempotentMethodsOnTransportError(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			base := &countingTransport{failures: 1, statusCode: http.StatusOK}
			rt := newTestRetryClient(base)
			resp, err := rt.RoundTrip(fastRequest(t, method, "/ping"))
			if err != nil {
				t.Fatalf("expected retry to succeed, got %v", err)
			}
			resp.Body.Close()
			if base.requests != 2 {
				t.Fatalf("requests = %d, want 2 (initial + 1 retry)", base.requests)
			}
		})
	}
}

func TestRetryRoundTripper_DoesNotRetryKeylessMutations(t *testing.T) {
	for _, method := range []string{http.MethodPut, http.MethodDelete, http.MethodPost, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			base := &countingTransport{failures: 1, statusCode: http.StatusOK}
			rt := newTestRetryClient(base)
			_, err := rt.RoundTrip(fastRequest(t, method, "/ping"))
			if err == nil {
				t.Fatal("expected transport error to surface without retry")
			}
			if base.requests != 1 {
				t.Fatalf("requests = %d, want 1 (no replay of a keyless mutation)", base.requests)
			}
		})
	}
}

func TestRetryRoundTripper_RetriesMutationsWithIdempotencyKey(t *testing.T) {
	// Only the two header names Beacon's power handler actually reads license a
	// replay; X-Idempotency-Key is ignored by the daemon, so honouring it would
	// replay a command nothing deduped.
	for _, header := range []string{"Idempotency-Key", "X-Forge-Command-ID"} {
		t.Run(header, func(t *testing.T) {
			base := &countingTransport{failures: 1, statusCode: http.StatusOK}
			rt := newTestRetryClient(base)
			req := fastRequest(t, http.MethodPost, "/servers/srv-1/power")
			req.Header.Set(header, "test-key-1")
			resp, err := rt.RoundTrip(req)
			if err != nil {
				t.Fatalf("expected retry to succeed, got %v", err)
			}
			resp.Body.Close()
			if base.requests != 2 {
				t.Fatalf("requests = %d, want 2 (initial + 1 retry)", base.requests)
			}
		})
	}
	t.Run("X-Idempotency-Key is not a replay licence", func(t *testing.T) {
		base := &countingTransport{failures: 1, statusCode: http.StatusOK}
		rt := newTestRetryClient(base)
		req := fastRequest(t, http.MethodPost, "/servers/srv-1/power")
		req.Header.Set("X-Idempotency-Key", "test-key-1")
		_, err := rt.RoundTrip(req)
		if err == nil {
			t.Fatal("expected the transport error to surface without replay")
		}
		if base.requests != 1 {
			t.Fatalf("requests = %d, want 1 (header the daemon does not read)", base.requests)
		}
	})
}

func TestRetryRoundTripper_DoesNotReplayKeyedMutationsOnUndedupedRoutes(t *testing.T) {
	// Beacon dedupes command ids only on POST /servers/{id}/power. A create, a
	// compose deploy or a file write that carries a key is still not replayed:
	// the key is bookkeeping, not a dedupe guarantee, and a lost response to
	// these routes may mean the command already landed.
	for _, target := range []string{"/servers", "/compose/stack-1/deploy", "/api/v1/transfers/m-1/source/push", "/v1/files/write"} {
		t.Run(target, func(t *testing.T) {
			base := &countingTransport{failures: 1, statusCode: http.StatusOK}
			rt := newTestRetryClient(base)
			req := fastRequest(t, http.MethodPost, target)
			req.Header.Set("Idempotency-Key", "cmd-1")
			req.Header.Set("X-Forge-Command-ID", "cmd-1")
			if _, err := rt.RoundTrip(req); err == nil {
				t.Fatal("expected the transport error to surface without replay")
			}
			if base.requests != 1 {
				t.Fatalf("requests = %d, want 1 (route the daemon does not dedupe)", base.requests)
			}
		})
	}
}

func TestRetryRoundTripper_KeylessMutationStatusReturnedWithoutRetry(t *testing.T) {
	base := &countingTransport{failures: 0, statusCode: http.StatusServiceUnavailable}
	rt := newTestRetryClient(base)
	resp, err := rt.RoundTrip(fastRequest(t, http.MethodPost, "/command"))
	if err != nil {
		t.Fatalf("expected the daemon response, got error %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	if base.requests != 1 {
		t.Fatalf("requests = %d, want 1 (no replay of a keyless mutation)", base.requests)
	}
}

func TestRetryRoundTripper_ResignsRetriedRequests(t *testing.T) {
	var resignations int
	base := &countingTransport{failures: 1, statusCode: http.StatusOK}
	rt := &retryRoundTripper{base: base, resign: func(*http.Request, []byte) error {
		resignations++
		return nil
	}}
	req := fastRequest(t, http.MethodPost, "/servers/srv-1/power")
	req.Header.Set("X-Forge-Command-ID", "power:srv-1:stop")
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("expected retry to succeed, got %v", err)
	}
	resp.Body.Close()
	if resignations != 1 {
		t.Fatalf("resignations = %d, want 1 (fresh signature for the retried attempt)", resignations)
	}
}

func TestRetryRoundTripper_ResignErrorAbortsRetry(t *testing.T) {
	base := &countingTransport{failures: 1, statusCode: http.StatusOK}
	rt := &retryRoundTripper{base: base, resign: func(*http.Request, []byte) error {
		return errors.New("signing failed")
	}}
	_, err := rt.RoundTrip(func() *http.Request {
		req := fastRequest(t, http.MethodPost, "/power")
		req.Header.Set("Idempotency-Key", "test-key-1")
		return req
	}())
	if err == nil {
		t.Fatal("expected resign error to surface")
	}
	if base.requests != 1 {
		t.Fatalf("requests = %d, want 1 (retry aborted before dispatch)", base.requests)
	}
}

func TestRetryRoundTripper_RetriesRetryableStatus(t *testing.T) {
	t.Run("GET retried on 503 then gives up after budget", func(t *testing.T) {
		base := &countingTransport{failures: 0, statusCode: http.StatusServiceUnavailable}
		rt := newTestRetryClient(base)
		resp, err := rt.RoundTrip(fastRequest(t, http.MethodGet, "/stats"))
		if err == nil {
			resp.Body.Close()
			t.Fatal("expected exhaustion error after retry budget")
		}
		if base.requests != maxRetries+1 {
			t.Fatalf("requests = %d, want %d", base.requests, maxRetries+1)
		}
	})
	t.Run("power POST retried on 503 then gives up after budget", func(t *testing.T) {
		base := &countingTransport{failures: 0, statusCode: http.StatusServiceUnavailable}
		rt := newTestRetryClient(base)
		req := fastRequest(t, http.MethodPost, "/servers/srv-1/power")
		req.Header.Set("X-Forge-Command-ID", "power:srv-1:stop")
		_, err := rt.RoundTrip(req)
		if err == nil {
			t.Fatal("expected exhaustion error after retry budget")
		}
		if base.requests != maxRetries+1 {
			t.Fatalf("requests = %d, want %d", base.requests, maxRetries+1)
		}
	})
	t.Run("non-retryable status returned immediately", func(t *testing.T) {
		base := &countingTransport{failures: 0, statusCode: http.StatusBadRequest}
		rt := newTestRetryClient(base)
		resp, err := rt.RoundTrip(fastRequest(t, http.MethodGet, "/x"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()
		if base.requests != 1 {
			t.Fatalf("requests = %d, want 1", base.requests)
		}
	})
}

func TestRetryRoundTripper_ExhaustionKeepsTypedDaemonError(t *testing.T) {
	// The standing complaint about this client was that an operator sees only a
	// generic failure when the node gave a real reason. Retry exhaustion must
	// keep the status and the daemon's body, or callers that map
	// *daemon.ResponseError to an HTTP code lose the verdict entirely.
	base := &countingTransport{failures: 0, statusCode: http.StatusServiceUnavailable}
	rt := newTestRetryClient(base)
	_, err := rt.RoundTrip(fastRequest(t, http.MethodGet, "/stats"))
	if err == nil {
		t.Fatal("expected exhaustion error")
	}
	var re *ResponseError
	if !errors.As(err, &re) {
		t.Fatalf("exhaustion error = %T (%v), want *ResponseError", err, err)
	}
	if re.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", re.StatusCode, http.StatusServiceUnavailable)
	}
	if !strings.Contains(re.Operation, "/stats") {
		t.Fatalf("operation = %q, want the request it belongs to", re.Operation)
	}
}

func TestValidateNodeURL_HTTPAllowedOnlyWhenClientIsLoopback(t *testing.T) {
	// Acceptance of plain HTTP is decided by the client's loopback flag (set
	// in NewClient from the default base URL), not per request target.
	remote := &Client{}
	loopback := &Client{loopback: true}
	cases := []struct {
		name    string
		client  *Client
		raw     string
		wantErr bool
	}{
		{"loopback client allows http loopback", loopback, "http://127.0.0.1:9090/servers", false},
		{"loopback client allows http localhost", loopback, "http://localhost:9090/servers", false},
		{"loopback client allows https", loopback, "https://beacon.example.com/servers", false},
		{"default client rejects http", remote, "http://127.0.0.1:9090/servers", true},
		{"default client rejects http remote", remote, "http://beacon.example.com/servers", true},
		{"default client allows https", remote, "https://beacon.example.com/servers", false},
		{"non-http scheme rejected", remote, "ftp://beacon.example.com", true},
		{"credentials rejected", remote, "https://user:pass@beacon.example.com", true},
	}
	for _, tc := range cases {
		parsed, err := url.Parse(tc.raw)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.raw, err)
		}
		err = tc.client.validateNodeURL(parsed)
		if tc.wantErr && err == nil {
			t.Errorf("%s: validateNodeURL(%q) = nil, want error", tc.name, tc.raw)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("%s: validateNodeURL(%q) = %v, want nil", tc.name, tc.raw, err)
		}
	}
}

func TestSendPowerAttachesCommandIDFromContext(t *testing.T) {
	base := &countingTransport{failures: 0, statusCode: http.StatusOK}
	client := &Client{loopback: true, httpClient: &http.Client{Transport: newRetryRoundTripper(base, nil)}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = ContextWithCommandID(ctx, "power:srv-1:stop")
	_, _ = client.SendPower(ctx, "http://127.0.0.1:1", "token", "srv-1", "stop")
	if base.lastReq == nil {
		t.Fatal("no request observed")
	}
	if got := base.lastReq.Header.Get("X-Forge-Command-ID"); got != "power:srv-1:stop" {
		t.Fatalf("X-Forge-Command-ID = %q, want %q", got, "power:srv-1:stop")
	}
}
