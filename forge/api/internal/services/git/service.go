package git

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

type Service struct {
	store  *store.Store
	logger *slog.Logger
	client *http.Client
}

func NewService(s *store.Store, logger *slog.Logger) *Service {
	return &Service{
		store:  s,
		logger: logger,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

type GeneratedKeyPair struct {
	PublicKey  string `json:"publicKey"`
	PrivateKey string `json:"privateKey,omitempty"`
	Type       string `json:"type"`
	Bits       int    `json:"bits,omitempty"`
}

func (s *Service) GenerateDeployKeyPair(keyType string, bits int) (*GeneratedKeyPair, error) {
	switch strings.ToLower(keyType) {
	case "ed25519", "":
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate ed25519 key: %w", err)
		}
		sshPub, err := ssh.NewPublicKey(pub)
		if err != nil {
			return nil, fmt.Errorf("marshal ssh public key: %w", err)
		}
		privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
		if err != nil {
			return nil, fmt.Errorf("marshal private key: %w", err)
		}
		return &GeneratedKeyPair{
			PublicKey:  strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))),
			PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})),
			Type:       "ed25519",
		}, nil
	case "rsa", "rsa4096":
		if bits == 0 {
			bits = 4096
		}
		priv, err := rsa.GenerateKey(rand.Reader, bits)
		if err != nil {
			return nil, fmt.Errorf("generate RSA key: %w", err)
		}
		sshPub, err := ssh.NewPublicKey(&priv.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("marshal ssh public key: %w", err)
		}
		return &GeneratedKeyPair{
			PublicKey:  strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))),
			PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})),
			Type:       "rsa",
			Bits:       bits,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported key type: %s (supported: ed25519, rsa)", keyType)
	}
}

func (s *Service) GenerateDeployKey(ctx context.Context, credentialID string) (*GeneratedKeyPair, error) {
	internal, err := s.store.GetGitCredential(ctx, credentialID)
	if err != nil {
		return nil, err
	}
	if internal.CredentialType != store.GitCredentialSSHKey {
		return nil, errors.New("credential type must be ssh_key for key generation")
	}

	kp, err := s.GenerateDeployKeyPair("ed25519", 0)
	if err != nil {
		return nil, err
	}

	_ = s.store.DeleteGitCredential(ctx, credentialID)
	_, err = s.store.CreateGitCredential(ctx, store.CreateGitCredentialRequest{
		UserID:         internal.UserID,
		Name:           internal.Name,
		CredentialType: internal.CredentialType,
		Credential:     kp.PrivateKey,
		PublicKey:      kp.PublicKey,
		Description:    internal.Description,
	})
	if err != nil {
		return nil, fmt.Errorf("store key pair: %w", err)
	}

	kp.PrivateKey = ""
	return kp, nil
}

func (s *Service) ListProviderRepos(ctx context.Context, providerTokenID string) ([]store.GitProviderRepo, error) {
	pt, err := s.store.GetGitProviderTokenUnmasked(ctx, providerTokenID)
	if err != nil {
		return nil, err
	}

	switch pt.Provider {
	case store.GitProviderGitHub:
		return s.callGitHubRepos(ctx, pt.AccessToken, pt.BaseURL)
	case store.GitProviderGitLab:
		return s.callGitLabRepos(ctx, pt.AccessToken, pt.BaseURL)
	case store.GitProviderBitbucket:
		return s.callBitbucketRepos(ctx, pt.AccessToken, pt.BaseURL)
	case store.GitProviderGitea:
		return s.callGiteaRepos(ctx, pt.AccessToken, pt.BaseURL)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", pt.Provider)
	}
}

func (s *Service) ListProviderBranches(ctx context.Context, providerTokenID, repoFullName string) ([]store.GitProviderBranch, error) {
	pt, err := s.store.GetGitProviderTokenUnmasked(ctx, providerTokenID)
	if err != nil {
		return nil, err
	}

	encoded := url.PathEscape(repoFullName)
	switch pt.Provider {
	case store.GitProviderGitHub:
		return s.callGitHubBranches(ctx, pt.AccessToken, encoded, pt.BaseURL)
	case store.GitProviderGitLab:
		return s.callGitLabBranches(ctx, pt.AccessToken, encoded, pt.BaseURL)
	case store.GitProviderBitbucket:
		return s.callBitbucketBranches(ctx, pt.AccessToken, encoded, pt.BaseURL)
	case store.GitProviderGitea:
		return s.callGiteaBranches(ctx, pt.AccessToken, encoded, pt.BaseURL)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", pt.Provider)
	}
}

func (s *Service) SetupProviderWebhook(ctx context.Context, providerTokenID, repoFullName, webhookURL, webhookSecret string) (string, error) {
	pt, err := s.store.GetGitProviderTokenUnmasked(ctx, providerTokenID)
	if err != nil {
		return "", err
	}

	encoded := url.PathEscape(repoFullName)
	switch pt.Provider {
	case store.GitProviderGitHub:
		return s.createGitHubWebhook(ctx, pt.AccessToken, encoded, webhookURL, webhookSecret, pt.BaseURL)
	case store.GitProviderGitLab:
		return s.createGitLabWebhook(ctx, pt.AccessToken, encoded, webhookURL, webhookSecret, pt.BaseURL)
	case store.GitProviderBitbucket:
		return s.createBitbucketWebhook(ctx, pt.AccessToken, encoded, webhookURL, webhookSecret, pt.BaseURL)
	case store.GitProviderGitea:
		return s.createGiteaWebhook(ctx, pt.AccessToken, encoded, webhookURL, webhookSecret, pt.BaseURL)
	default:
		return "", fmt.Errorf("unsupported provider: %s", pt.Provider)
	}
}

const (
	maxBuildContextSize int64 = 1024 * 1024 * 1024
	projectTypeUnknown       = "unknown"
)

var (
	ErrWebhookSignatureMissing = errors.New("webhook signature header missing")
	ErrWebhookSignatureInvalid = errors.New("webhook signature verification failed")
	ErrInvalidRepoURL          = errors.New("repository URL is invalid")
	ErrHostNotAllowed          = errors.New("git host is not in the allow-list")
	ErrCloneFailed             = errors.New("repository clone failed")
	ErrNoProjectType           = errors.New("no recognizable project type found (Dockerfile, compose.yml, etc.)")
	ErrBuildContextTooBig      = errors.New("build context exceeds maximum size")
	ErrInvalidBranch           = errors.New("branch name is invalid")
	ErrCmdUnavailable          = errors.New("git executable not found")
)

func VerifyGitHubSignature(payload []byte, signatureHeader, secret string) error {
	if signatureHeader == "" {
		return ErrWebhookSignatureMissing
	}
	if !strings.HasPrefix(signatureHeader, "sha256=") {
		return ErrWebhookSignatureInvalid
	}
	mac := computeHMAC(secret, payload)
	expected := "sha256=" + mac
	if !hmac.Equal([]byte(signatureHeader), []byte(expected)) {
		return ErrWebhookSignatureInvalid
	}
	return nil
}

func VerifyGitLabSignature(payload []byte, tokenHeader, secret string) error {
	if tokenHeader == "" {
		return ErrWebhookSignatureMissing
	}
	if tokenHeader != secret {
		return ErrWebhookSignatureInvalid
	}
	return nil
}

func VerifyBitbucketSignature(payload []byte, eventHeader, secret string) error {
	if secret == "" {
		return nil
	}
	if eventHeader == "" {
		return ErrWebhookSignatureMissing
	}
	prefix := "sha256="
	if !strings.HasPrefix(eventHeader, prefix) {
		return ErrWebhookSignatureInvalid
	}
	sigHex := strings.TrimPrefix(eventHeader, prefix)
	mac := computeHMAC(secret, payload)
	if !hmac.Equal([]byte(sigHex), []byte(mac)) {
		return ErrWebhookSignatureInvalid
	}
	return nil
}

func VerifyGiteaSignature(payload []byte, signatureHeader, secret string) error {
	if signatureHeader == "" {
		return ErrWebhookSignatureMissing
	}
	mac := computeHMAC(secret, payload)
	if !hmac.Equal([]byte(signatureHeader), []byte(mac)) {
		return ErrWebhookSignatureInvalid
	}
	return nil
}

// ---------- GitHub API ----------

func githubAPIBase(baseURL string) string {
	if baseURL != "" && !strings.Contains(baseURL, "github.com") {
		return strings.TrimRight(baseURL, "/") + "/api/v3"
	}
	return "https://api.github.com"
}

func (s *Service) callGitHubRepos(ctx context.Context, token, baseURL string) ([]store.GitProviderRepo, error) {
	apiBase := githubAPIBase(baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", apiBase+"/user/repos?per_page=100&affiliation=owner,collaborator,organization_member", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Forge-Git/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github api returned %d: %s", resp.StatusCode, string(body))
	}

	var repos []struct {
		Name        string `json:"name"`
		FullName    string `json:"full_name"`
		CloneURL    string `json:"clone_url"`
		SSHURL      string `json:"ssh_url"`
		DefaultBr   string `json:"default_branch"`
		Private     bool   `json:"private"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, err
	}

	result := make([]store.GitProviderRepo, len(repos))
	for i, r := range repos {
		result[i] = store.GitProviderRepo{
			Name:          r.Name,
			FullName:      r.FullName,
			CloneURL:      r.CloneURL,
			SSHURL:        r.SSHURL,
			DefaultBranch: r.DefaultBr,
			Private:       r.Private,
			Description:   r.Description,
		}
	}
	return result, nil
}

func (s *Service) callGitHubBranches(ctx context.Context, token, repoFullName, baseURL string) ([]store.GitProviderBranch, error) {
	apiBase := githubAPIBase(baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", apiBase+"/repos/"+repoFullName+"/branches?per_page=100", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Forge-Git/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("github branches api returned %d", resp.StatusCode)
	}

	var branches []struct {
		Name   string `json:"name"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&branches); err != nil {
		return nil, err
	}

	result := make([]store.GitProviderBranch, len(branches))
	for i, b := range branches {
		result[i] = store.GitProviderBranch{Name: b.Name, SHA: b.Commit.SHA}
	}
	return result, nil
}

func (s *Service) createGitHubWebhook(ctx context.Context, token, repoFullName, webhookURL, secret, baseURL string) (string, error) {
	apiBase := githubAPIBase(baseURL)

	body := map[string]any{
		"name":   "web",
		"active": true,
		"events": []string{"push"},
		"config": map[string]any{
			"url":          webhookURL,
			"content_type": "json",
			"secret":       secret,
			"insecure_ssl": "0",
		},
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", apiBase+"/repos/"+repoFullName+"/hooks", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Forge-Git/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", result.ID), nil
}

// ---------- GitLab API ----------

func gitLabAPIBase(baseURL string) string {
	if baseURL != "" {
		return strings.TrimRight(baseURL, "/") + "/api/v4"
	}
	return "https://gitlab.com/api/v4"
}

func (s *Service) callGitLabRepos(ctx context.Context, token, baseURL string) ([]store.GitProviderRepo, error) {
	apiBase := gitLabAPIBase(baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", apiBase+"/projects?membership=true&per_page=100", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("User-Agent", "Forge-Git/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("gitlab api returned %d", resp.StatusCode)
	}

	var repos []struct {
		Name          string `json:"name"`
		PathWithNS    string `json:"path_with_namespace"`
		HTTPURLToRepo string `json:"http_url_to_repo"`
		SSHURLToRepo  string `json:"ssh_url_to_repo"`
		DefaultBranch string `json:"default_branch"`
		Visibility    string `json:"visibility"`
		Description   string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, err
	}

	result := make([]store.GitProviderRepo, len(repos))
	for i, r := range repos {
		result[i] = store.GitProviderRepo{
			Name:          r.Name,
			FullName:      r.PathWithNS,
			CloneURL:      r.HTTPURLToRepo,
			SSHURL:        r.SSHURLToRepo,
			DefaultBranch: r.DefaultBranch,
			Private:       r.Visibility != "public",
			Description:   r.Description,
		}
	}
	return result, nil
}

func (s *Service) callGitLabBranches(ctx context.Context, token, repoFullName, baseURL string) ([]store.GitProviderBranch, error) {
	apiBase := gitLabAPIBase(baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", apiBase+"/projects/"+repoFullName+"/repository/branches?per_page=100", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("User-Agent", "Forge-Git/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("gitlab branches api returned %d", resp.StatusCode)
	}

	var branches []struct {
		Name   string `json:"name"`
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
		Default bool `json:"default"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&branches); err != nil {
		return nil, err
	}

	result := make([]store.GitProviderBranch, len(branches))
	for i, b := range branches {
		result[i] = store.GitProviderBranch{Name: b.Name, SHA: b.Commit.ID, IsMain: b.Default}
	}
	return result, nil
}

func (s *Service) createGitLabWebhook(ctx context.Context, token, repoFullName, webhookURL, secret, baseURL string) (string, error) {
	apiBase := gitLabAPIBase(baseURL)

	body := map[string]any{
		"url":                     webhookURL,
		"push_events":             true,
		"enable_ssl_verification": true,
		"token":                   secret,
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", apiBase+"/projects/"+repoFullName+"/hooks", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", err
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Forge-Git/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", result.ID), nil
}

// ---------- Bitbucket API ----------

func bitbucketAPIBase(baseURL string) string {
	if baseURL != "" {
		return strings.TrimRight(baseURL, "/") + "/2.0"
	}
	return "https://api.bitbucket.org/2.0"
}

func (s *Service) callBitbucketRepos(ctx context.Context, token, baseURL string) ([]store.GitProviderRepo, error) {
	apiBase := bitbucketAPIBase(baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", apiBase+"/repositories?role=contributor", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("x-token-auth", token)
	req.Header.Set("User-Agent", "Forge-Git/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bitbucket api returned %d", resp.StatusCode)
	}

	var result struct {
		Values []struct {
			Name        string `json:"name"`
			FullName    string `json:"full_name"`
			Links       struct {
				Clone []struct {
					Name string `json:"name"`
					Href string `json:"href"`
				} `json:"clone"`
			} `json:"links"`
			MainBranch struct {
				Name string `json:"name"`
			} `json:"mainbranch"`
			IsPrivate   bool   `json:"is_private"`
			Description string `json:"description"`
		} `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	repos := make([]store.GitProviderRepo, len(result.Values))
	for i, r := range result.Values {
		cloneURL := ""
		sshURL := ""
		for _, link := range r.Links.Clone {
			if link.Name == "https" {
				cloneURL = link.Href
			} else if link.Name == "ssh" {
				sshURL = link.Href
			}
		}
		repos[i] = store.GitProviderRepo{
			Name:          r.Name,
			FullName:      r.FullName,
			CloneURL:      cloneURL,
			SSHURL:        sshURL,
			DefaultBranch: r.MainBranch.Name,
			Private:       r.IsPrivate,
			Description:   r.Description,
		}
	}
	return repos, nil
}

func (s *Service) callBitbucketBranches(ctx context.Context, token, repoFullName, baseURL string) ([]store.GitProviderBranch, error) {
	apiBase := bitbucketAPIBase(baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", apiBase+"/repositories/"+repoFullName+"/refs/branches", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("x-token-auth", token)
	req.Header.Set("User-Agent", "Forge-Git/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Values []struct {
			Name   string `json:"name"`
			Target struct {
				Hash string `json:"hash"`
			} `json:"target"`
		} `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	branches := make([]store.GitProviderBranch, len(result.Values))
	for i, b := range result.Values {
		branches[i] = store.GitProviderBranch{Name: b.Name, SHA: b.Target.Hash}
	}
	return branches, nil
}

func (s *Service) createBitbucketWebhook(ctx context.Context, token, repoFullName, webhookURL, secret, baseURL string) (string, error) {
	apiBase := bitbucketAPIBase(baseURL)

	body := map[string]any{
		"description": "Forge auto-deploy webhook",
		"url":         webhookURL,
		"active":      true,
		"events":      []string{"repo:push"},
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", apiBase+"/repositories/"+repoFullName+"/hooks", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth("x-token-auth", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Forge-Git/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var result struct {
			UUID string `json:"uuid"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return uuid.NewString(), nil
		}
		if result.UUID != "" {
			return result.UUID, nil
		}
	}
	return uuid.NewString(), nil
}

// ---------- Gitea API ----------

func (s *Service) callGiteaRepos(ctx context.Context, token, baseURL string) ([]store.GitProviderRepo, error) {
	apiBase := strings.TrimRight(baseURL, "/") + "/api/v1"
	req, err := http.NewRequestWithContext(ctx, "GET", apiBase+"/user/repos?limit=100", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("User-Agent", "Forge-Git/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("gitea api returned %d", resp.StatusCode)
	}

	var repos []struct {
		Name        string `json:"name"`
		FullName    string `json:"full_name"`
		CloneURL    string `json:"clone_url"`
		SSHURL      string `json:"ssh_url"`
		DefaultBr   string `json:"default_branch"`
		Private     bool   `json:"private"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, err
	}

	result := make([]store.GitProviderRepo, len(repos))
	for i, r := range repos {
		result[i] = store.GitProviderRepo{
			Name:          r.Name,
			FullName:      r.FullName,
			CloneURL:      r.CloneURL,
			SSHURL:        r.SSHURL,
			DefaultBranch: r.DefaultBr,
			Private:       r.Private,
			Description:   r.Description,
		}
	}
	return result, nil
}

func (s *Service) callGiteaBranches(ctx context.Context, token, repoFullName, baseURL string) ([]store.GitProviderBranch, error) {
	apiBase := strings.TrimRight(baseURL, "/") + "/api/v1"
	req, err := http.NewRequestWithContext(ctx, "GET", apiBase+"/repos/"+repoFullName+"/branches?limit=100", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("User-Agent", "Forge-Git/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var branches []struct {
		Name   string `json:"name"`
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&branches); err != nil {
		return nil, err
	}

	result := make([]store.GitProviderBranch, len(branches))
	for i, b := range branches {
		result[i] = store.GitProviderBranch{Name: b.Name, SHA: b.Commit.ID}
	}
	return result, nil
}

func (s *Service) createGiteaWebhook(ctx context.Context, token, repoFullName, webhookURL, secret, baseURL string) (string, error) {
	apiBase := strings.TrimRight(baseURL, "/") + "/api/v1"

	body := map[string]any{
		"type":   "gitea",
		"active": true,
		"events": []string{"push"},
		"config": map[string]any{
			"url":          webhookURL,
			"content_type": "json",
			"secret":       secret,
		},
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", apiBase+"/repos/"+repoFullName+"/hooks", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Forge-Git/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", result.ID), nil
}

func computeHMAC(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) HandleWebhookTrigger(ctx context.Context, source *store.GitSource, commitSHA, commitMsg, commitAuthor string) error {
	if !source.AutoDeploy {
		return nil
	}
	return s.store.UpdateGitSourceDeploy(ctx, source.ID, commitSHA, commitMsg, commitAuthor)
}
