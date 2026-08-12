package git

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gamepanel/forge/internal/store"
)

// providerUserCheck describes the current-user request used to validate a
// token without persisting anything.
type providerUserCheck struct {
	endpoint string
	headers  map[string]string
}

// providerUserEndpoint builds the current-user request for a token check.
// It performs no network I/O, so it is safe to unit test exhaustively.
func providerUserEndpoint(provider store.GitProviderType, baseURL string) (providerUserCheck, error) {
	baseURL = strings.TrimSpace(baseURL)
	switch provider {
	case store.GitProviderGitHub:
		return providerUserCheck{
			endpoint: githubAPIBase(baseURL) + "/user",
			headers: map[string]string{
				"Authorization": "Bearer {{token}}",
				"Accept":        "application/vnd.github+json",
			},
		}, nil
	case store.GitProviderGitLab:
		return providerUserCheck{
			endpoint: gitLabAPIBase(baseURL) + "/user",
			headers: map[string]string{
				"PRIVATE-TOKEN": "{{token}}",
			},
		}, nil
	case store.GitProviderBitbucket:
		return providerUserCheck{
			endpoint: bitbucketAPIBase(baseURL) + "/user",
			headers: map[string]string{
				"Authorization": "Bearer {{token}}",
			},
		}, nil
	case store.GitProviderGitea:
		if baseURL == "" {
			return providerUserCheck{}, errors.New("gitea base URL is required")
		}
		return providerUserCheck{
			endpoint: strings.TrimRight(baseURL, "/") + "/api/v1/user",
			headers: map[string]string{
				"Authorization": "token {{token}}",
			},
		}, nil
	default:
		return providerUserCheck{}, fmt.Errorf("token validation is not supported for provider %q", string(provider))
	}
}

// ValidateProviderToken checks a provider token against the provider's
// current-user endpoint and returns the login the provider reports. A custom
// base URL is SSRF-guarded by validateProviderBaseURL before any request is
// made; the provider's own redirect guard in the shared client applies too.
func (s *Service) ValidateProviderToken(ctx context.Context, provider store.GitProviderType, token, baseURL string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("access token is required")
	}
	check, err := providerUserEndpoint(provider, baseURL)
	if err != nil {
		return "", err
	}
	if err := validateProviderBaseURL(provider, baseURL); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, check.endpoint, nil)
	if err != nil {
		return "", err
	}
	for key, value := range check.headers {
		req.Header.Set(key, strings.ReplaceAll(value, "{{token}}", token))
	}
	req.Header.Set("User-Agent", "Forge-Git/1.0")

	client := s.client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("provider check failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", fmt.Errorf("read provider response: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return "", fmt.Errorf("token rejected by provider (HTTP %d)", resp.StatusCode)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return "", fmt.Errorf("provider check returned HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Login    string `json:"login"`
		Username string `json:"username"`
		Nickname string `json:"nickname"`
		Name     string `json:"name"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("parse provider response: %w", err)
	}
	for _, candidate := range []string{payload.Login, payload.Username, payload.Nickname, payload.Name} {
		if strings.TrimSpace(candidate) != "" {
			return strings.TrimSpace(candidate), nil
		}
	}
	return "token accepted", nil
}
