package phase1git

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"gamepanel/forge/internal/store"
)

// DeployKeyPayload is sent to providers when auto-provisioning a deploy key.
type deployKeyPayload struct {
	Title    string `json:"title,omitempty"`
	Key      string `json:"key"`
	ReadOnly bool   `json:"read_only,omitempty"`
	Readonly bool   `json:"readonly,omitempty"` // Gitea uses the British spelling
	CanPush  bool   `json:"can_push,omitempty"`
	Label    string `json:"label,omitempty"`
}

// DeployKeyResult mirrors the provider's created key id.
type DeployKeyResult struct {
	ID          string `json:"id"`
	URL         string `json:"url,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// AutoprovisionDeployKey pushes the generated public key onto the provider
// repository so a deploy key credential can actually clone. The credential
// itself is created by the caller via the existing git service.
func (b *Bridge) AutoprovisionDeployKey(ctx context.Context, pt store.GitProviderType, token, baseURL, repo, title, publicKey string) (*DeployKeyResult, error) {
	var apiURL string
	payload := deployKeyPayload{Title: title, Key: publicKey}
	if pt == store.GitProviderBitbucket {
		payload = deployKeyPayload{Label: title, Key: publicKey}
	}

	switch pt {
	case store.GitProviderGitHub:
		apiURL = "https://api.github.com/repos/" + repo + "/keys"
		payload.ReadOnly = true
	case store.GitProviderGitLab:
		apiURL = gitLabBase(baseURL) + "/projects/" + url.PathEscape(repo) + "/deploy_keys"
		payload.CanPush = true
	case store.GitProviderBitbucket:
		apiURL = bitbucketBase(baseURL) + "/repositories/" + repo + "/deploy-keys"
	case store.GitProviderGitea:
		apiURL = b.ghStyleBaseFor(pt, baseURL) + "/repos/" + repo + "/keys"
		payload.Readonly = true
	default:
		return nil, fmt.Errorf("unsupported provider: %s", pt)
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Forge-Git/1.0")
	switch pt {
	case store.GitProviderGitHub:
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
	case store.GitProviderGitLab:
		req.Header.Set("PRIVATE-TOKEN", token)
	case store.GitProviderBitbucket:
		req.SetBasicAuth("x-token-auth", token)
	case store.GitProviderGitea:
		req.Header.Set("Authorization", "token "+token)
	}

	resp, err := b.httpC.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s api returned %d: %s", pt, resp.StatusCode, truncate(string(raw), 400))
	}

	result := &DeployKeyResult{}
	var parsed struct {
		ID          *int    `json:"id"`
		KeyID       *string `json:"key_id"`
		UUID        *string `json:"uuid"`
		URL         string  `json:"url"`
		Fingerprint string  `json:"fingerprint"`
	}
	_ = json.Unmarshal(raw, &parsed)
	if parsed.ID != nil {
		result.ID = fmt.Sprintf("%d", *parsed.ID)
	}
	if parsed.KeyID != nil {
		result.ID = *parsed.KeyID
	}
	if parsed.UUID != nil {
		result.ID = *parsed.UUID
	}
	result.URL = parsed.URL
	result.Fingerprint = parsed.Fingerprint
	return result, nil
}
