package vaultprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Sentinel / wrapper errors surfaced to callers. They are intentionally plain
// (not wrapped secrets) so an operator sees *why* a resolve failed without the
// error ever carrying a secret value.
var (
	ErrUnknownAuthMethod = errors.New("vault auth method must be 'token' or 'approle'")
	ErrInvalidBaseURL    = errors.New("vault base URL must be an absolute http(s) URL without userinfo")
	ErrUnreachable       = errors.New("vault instance is unreachable")
	ErrSecretNotFound    = errors.New("vault secret path or field not found")
)

const requestTimeout = 15 * time.Second

// Client talks to the Vault HTTP API v1 (KV secrets engine) using only the
// standard library, mirroring how the DNS providers call their upstream APIs.
// It performs no caching: every resolve is a live read so a rotated or revoked
// secret is reflected immediately.
type Client struct {
	http *http.Client
}

// NewClient builds a Client with a bounded, redirect-respecting HTTP client.
func NewClient() *Client {
	c := &http.Client{Timeout: requestTimeout}
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("too many Vault redirects")
		}
		return validateBaseURL(req.URL.String())
	}
	return &Client{http: c}
}

// validateBaseURL accepts http/https endpoints with a host and no embedded
// credentials. Vault is frequently served over plain HTTP inside a private
// network, so — unlike the HTTPS-only webhook path — http is permitted here.
func validateBaseURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ErrInvalidBaseURL
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return ErrInvalidBaseURL
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, endpoint, token, namespace string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode vault request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("X-Vault-Token", token)
	}
	if namespace != "" {
		req.Header.Set("X-Vault-Namespace", namespace)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrUnreachable, err.Error())
	}
	return resp, nil
}

// login exchanges an AppRole role/secret id for a client token. For token auth
// it returns the configured token unchanged.
func (c *Client) login(ctx context.Context, conn VaultConnection) (string, error) {
	switch conn.Auth {
	case AuthToken:
		if strings.TrimSpace(conn.Token) == "" {
			return "", errors.New("token auth is configured but no token is present")
		}
		return conn.Token, nil
	case AuthAppRole:
		endpoint := strings.TrimRight(conn.BaseURL, "/") + "/v1/auth/approle/login"
		payload := map[string]string{"role_id": conn.RoleID, "secret_id": conn.SecretID}
		resp, err := c.do(ctx, http.MethodPost, endpoint, "", conn.Namespace, payload)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", vaultStatusError("AppRole login", resp)
		}
		var out struct {
			Auth struct {
				ClientToken string `json:"client_token"`
			} `json:"auth"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return "", fmt.Errorf("decode AppRole login response: %w", err)
		}
		if strings.TrimSpace(out.Auth.ClientToken) == "" {
			return "", errors.New("Vault returned no client token for AppRole login")
		}
		return out.Auth.ClientToken, nil
	default:
		return "", ErrUnknownAuthMethod
	}
}

// readSecret fetches the KV secret at path and returns its field map, handling
// the KV-v1 and KV-v2 response envelopes.
func (c *Client) readSecret(ctx context.Context, conn VaultConnection, token, path string) (map[string]any, error) {
	base := strings.TrimRight(conn.BaseURL, "/")
	mount := url.PathEscape(strings.Trim(conn.MountPath, "/"))
	escaped := escapePath(path)
	var endpoint string
	if conn.EngineVersion == 1 {
		endpoint = fmt.Sprintf("%s/v1/%s/%s", base, mount, escaped)
	} else {
		endpoint = fmt.Sprintf("%s/v1/%s/data/%s", base, mount, escaped)
	}

	resp, err := c.do(ctx, http.MethodGet, endpoint, token, conn.Namespace, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", ErrSecretNotFound, path)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, vaultStatusError("read secret", resp)
	}

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode vault secret response: %w", err)
	}
	if len(envelope.Data) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrSecretNotFound, path)
	}

	if conn.EngineVersion == 1 {
		var fields map[string]any
		if err := json.Unmarshal(envelope.Data, &fields); err != nil {
			return nil, fmt.Errorf("decode vault v1 data: %w", err)
		}
		return fields, nil
	}

	// KV v2 wraps the actual key/value map under data.data.
	var v2 struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(envelope.Data, &v2); err != nil {
		return nil, fmt.Errorf("decode vault v2 data: %w", err)
	}
	return v2.Data, nil
}

// ReadField resolves a single field from the secret at path. A missing path or
// missing field is an error (fail-closed); it never returns an empty success.
func (c *Client) ReadField(ctx context.Context, conn VaultConnection, path, field string) (string, error) {
	if strings.TrimSpace(field) == "" {
		return "", errors.New("vault reference field is empty")
	}
	token, err := c.login(ctx, conn)
	if err != nil {
		return "", err
	}
	fields, err := c.readSecret(ctx, conn, token, path)
	if err != nil {
		return "", err
	}
	value, ok := fields[field]
	if !ok || value == nil {
		return "", fmt.Errorf("%w: field %q in %s", ErrSecretNotFound, field, path)
	}
	switch typed := value.(type) {
	case string:
		return typed, nil
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return "", fmt.Errorf("vault field %q is not representable as a string", field)
		}
		return string(encoded), nil
	}
}

// TestConnection verifies the connection end-to-end: it authenticates (AppRole
// login exercises a real token exchange) and then validates the resulting
// token via token/lookup-self. Any failure returns a descriptive error — it
// never reports success for work it could not perform.
func (c *Client) TestConnection(ctx context.Context, conn VaultConnection) error {
	if err := validateBaseURL(conn.BaseURL); err != nil {
		return err
	}
	token, err := c.login(ctx, conn)
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(conn.BaseURL, "/") + "/v1/auth/token/lookup-self"
	resp, err := c.do(ctx, http.MethodGet, endpoint, token, conn.Namespace, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return vaultStatusError("token validation", resp)
	}
	return nil
}

// escapePath percent-encodes each segment of a Vault secret path while keeping
// the "/" separators, mirroring Vault's own URL expectations.
func escapePath(path string) string {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
}

// vaultStatusError renders a Vault error status plus a short, sanitized body
// snippet. Vault error bodies do not contain secret values, so this is safe to
// surface; the snippet is bounded to keep responses tidy.
func vaultStatusError(op string, resp *http.Response) error {
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	detail := strings.TrimSpace(string(snippet))
	if detail == "" {
		detail = http.StatusText(resp.StatusCode)
	}
	return fmt.Errorf("vault %s failed (status %d): %s", op, resp.StatusCode, detail)
}
