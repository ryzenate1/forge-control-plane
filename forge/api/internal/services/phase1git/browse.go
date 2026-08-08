package phase1git

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gamepanel/forge/internal/store"
)

// RepoItem is one entry in a repository directory listing.
type RepoItem struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"` // dir | file | submodule | symlink
	Size int64  `json:"size,omitempty"`
	SHA  string `json:"sha,omitempty"`
}

// FileContent is a file or README fetched from the provider, with the raw
// payload base64-encoded (provider format) and the human-readable decoded
// form supplied for the web client.
type FileContent struct {
	Path     string `json:"path"`
	SHA      string `json:"sha,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Encoding string `json:"encoding,omitempty"` // base64 / plain
	Content  string `json:"content,omitempty"`  // base64 or raw depending on Encoding
	Decoded  string `json:"decoded,omitempty"`
}

// CommitInfo is one line of the repository commit list.
type CommitInfo struct {
	SHA     string     `json:"sha"`
	Message string     `json:"message"`
	Author  string     `json:"author"`
	Date    *time.Time `json:"date,omitempty"`
}

// BrowseDir lists a repository directory at the given provider.
func (b *Bridge) BrowseDir(ctx context.Context, pt store.GitProviderType, token, baseURL, repo, path, branch string) ([]RepoItem, error) {
	switch pt {
	case store.GitProviderGitHub, store.GitProviderGitea:
		return b.browseGithubStyle(ctx, pt, token, baseURL, repo, path, branch)
	case store.GitProviderGitLab:
		return b.browseGitLab(ctx, token, baseURL, repo, path, branch)
	case store.GitProviderBitbucket:
		return b.browseBitbucket(ctx, token, baseURL, repo, path, branch)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", pt)
	}
}

// GetFile fetches a single file at a provider.
func (b *Bridge) GetFile(ctx context.Context, pt store.GitProviderType, token, baseURL, repo, path, branch string) (*FileContent, error) {
	switch pt {
	case store.GitProviderGitHub, store.GitProviderGitea:
		return b.fileGithubStyle(ctx, pt, token, baseURL, repo, path, branch)
	case store.GitProviderGitLab:
		return b.fileGitLab(ctx, token, baseURL, repo, path, branch)
	case store.GitProviderBitbucket:
		return b.fileBitbucket(ctx, token, baseURL, repo, path, branch)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", pt)
	}
}

// GetReadme fetches and decodes the repository README (best effort).
func (b *Bridge) GetReadme(ctx context.Context, pt store.GitProviderType, token, baseURL, repo, branch string) (*FileContent, error) {
	return b.GetFile(ctx, pt, token, baseURL, repo, "README.md", branch)
}

// ListCommits lists the repository's recent commits, optionally filtered to a
// directory path.
func (b *Bridge) ListCommits(ctx context.Context, pt store.GitProviderType, token, baseURL, repo, branch, path string) ([]CommitInfo, error) {
	switch pt {
	case store.GitProviderGitHub, store.GitProviderGitea:
		return b.commitsGithubStyle(ctx, pt, token, baseURL, repo, branch, path)
	case store.GitProviderGitLab:
		return b.commitsGitLab(ctx, token, baseURL, repo, branch, path)
	case store.GitProviderBitbucket:
		return b.commitsBitbucket(ctx, token, baseURL, repo)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", pt)
	}
}

// ---------- shared plumbing ----------

func (b *Bridge) ghStyleBaseFor(pt store.GitProviderType, baseURL string) string {
	if pt == store.GitProviderGitea {
		if baseURL != "" {
			return strings.TrimRight(baseURL, "/") + "/api/v1"
		}
		return "https://gitea.example.com/api/v1"
	}
	if baseURL != "" && !strings.Contains(baseURL, "github.com") {
		return strings.TrimRight(baseURL, "/") + "/api/v3"
	}
	return "https://api.github.com"
}

func gitLabBase(baseURL string) string {
	if baseURL != "" {
		return strings.TrimRight(baseURL, "/") + "/api/v4"
	}
	return "https://gitlab.com/api/v4"
}

func bitbucketBase(baseURL string) string {
	if baseURL != "" {
		return strings.TrimRight(baseURL, "/") + "/2.0"
	}
	return "https://api.bitbucket.org/2.0"
}

func (b *Bridge) authedRequest(ctx context.Context, pt store.GitProviderType, token, rawURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Forge-Git/1.0")
	switch pt {
	case store.GitProviderGitHub:
		req.Header.Set("Authorization", "Bearer "+token)
	case store.GitProviderGitLab:
		req.Header.Set("PRIVATE-TOKEN", token)
	case store.GitProviderBitbucket:
		req.SetBasicAuth("x-token-auth", token)
	case store.GitProviderGitea:
		req.Header.Set("Authorization", "token "+token)
	}
	return req, nil
}

func (b *Bridge) doGet(ctx context.Context, pt store.GitProviderType, token, rawURL string) ([]byte, error) {
	req, err := b.authedRequest(ctx, pt, token, rawURL)
	if err != nil {
		return nil, err
	}
	resp, err := b.httpC.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s api returned %d: %s", pt, resp.StatusCode, truncate(string(body), 400))
	}
	return body, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func decodeBase64(encoded string) string {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return ""
	}
	return string(raw)
}

func firstLine(s string) string {
	lines := strings.SplitN(s, "\n", 2)
	return strings.TrimSpace(lines[0])
}

func baseName(p string) string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	return parts[len(parts)-1]
}

// ---------- GitHub / Gitea (shared JSON shapes) ----------

type ghStyleEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64  `json:"size"`
	SHA  string `json:"sha"`
}

func (b *Bridge) browseGithubStyle(ctx context.Context, pt store.GitProviderType, token, baseURL, repo, path, branch string) ([]RepoItem, error) {
	api := b.ghStyleBaseFor(pt, baseURL)
	q := url.Values{}
	if branch != "" {
		q.Set("ref", branch)
	}
	rawURL := api + "/repos/" + repo + "/contents/" + url.PathEscape(path)
	if branch != "" {
		rawURL += "?" + q.Encode()
	}
	body, err := b.doGet(ctx, pt, token, rawURL)
	if err != nil {
		return nil, err
	}

	var entries []ghStyleEntry
	if err := json.Unmarshal(body, &entries); err == nil && entries != nil {
		items := make([]RepoItem, 0, len(entries))
		for _, e := range entries {
			items = append(items, RepoItem{Name: e.Name, Path: e.Path, Type: e.Type, Size: e.Size, SHA: e.SHA})
		}
		return items, nil
	}

	var single ghStyleEntry
	if err := json.Unmarshal(body, &single); err == nil && single.Path != "" {
		return []RepoItem{{Name: single.Name, Path: single.Path, Type: single.Type, Size: single.Size, SHA: single.SHA}}, nil
	}
	return nil, fmt.Errorf("unexpected directory listing response")
}

func (b *Bridge) ghStyleBase(baseURL string) string {
	if baseURL != "" && !strings.Contains(baseURL, "github.com") {
		return strings.TrimRight(baseURL, "/") + "/api/v3"
	}
	return "https://api.github.com"
}

func (b *Bridge) fileGithubStyle(ctx context.Context, pt store.GitProviderType, token, baseURL, repo, path, branch string) (*FileContent, error) {
	api := b.ghStyleBase(baseURL)
	if pt == store.GitProviderGitea {
		api = b.ghStyleBaseFor(pt, baseURL)
	}
	q := url.Values{}
	if branch != "" {
		q.Set("ref", branch)
	}
	rawURL := api + "/repos/" + repo + "/contents/" + url.PathEscape(path)
	if branch != "" {
		rawURL += "?" + q.Encode()
	}
	body, err := b.doGet(ctx, pt, token, rawURL)
	if err != nil {
		return nil, err
	}
	var res struct {
		Name     string `json:"name"`
		Path     string `json:"path"`
		SHA      string `json:"sha"`
		Size     int64  `json:"size"`
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}
	fc := &FileContent{Path: res.Path, SHA: res.SHA, Size: res.Size, Encoding: res.Encoding, Content: res.Content}
	if fc.Encoding == "base64" {
		fc.Decoded = decodeBase64(fc.Content)
	}
	return fc, nil
}

func (b *Bridge) commitsGithubStyle(ctx context.Context, pt store.GitProviderType, token, baseURL, repo, branch, path string) ([]CommitInfo, error) {
	api := b.ghStyleBase(baseURL)
	if pt == store.GitProviderGitea {
		api = b.ghStyleBaseFor(pt, baseURL)
	}
	q := url.Values{}
	if branch != "" {
		q.Set("sha", branch)
	}
	if path != "" {
		q.Set("path", path)
	}
	q.Set("per_page", "100")
	body, err := b.doGet(ctx, pt, token, api+"/repos/"+repo+"/commits?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var raw []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Name string `json:"name"`
				Date string `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	commits := make([]CommitInfo, 0, len(raw))
	for _, c := range raw {
		ci := CommitInfo{SHA: c.SHA, Message: firstLine(c.Commit.Message), Author: c.Commit.Author.Name}
		if t, err := time.Parse(time.RFC3339, c.Commit.Author.Date); err == nil {
			ci.Date = &t
		}
		commits = append(commits, ci)
	}
	return commits, nil
}

// ---------- GitLab ----------

func (b *Bridge) browseGitLab(ctx context.Context, token, baseURL, repo, path, branch string) ([]RepoItem, error) {
	q := url.Values{}
	if path != "" {
		q.Set("path", path)
	}
	if branch != "" {
		q.Set("ref", branch)
	}
	q.Set("per_page", "100")
	body, err := b.doGet(ctx, store.GitProviderGitLab, token,
		gitLabBase(baseURL)+"/projects/"+url.PathEscape(repo)+"/repository/tree?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	items := make([]RepoItem, 0, len(raw))
	for _, e := range raw {
		typ := e.Type
		if typ == "" {
			typ = "file"
		}
		items = append(items, RepoItem{Name: e.Name, Path: e.Path, Type: typ, SHA: e.ID})
	}
	return items, nil
}

func (b *Bridge) fileGitLab(ctx context.Context, token, baseURL, repo, path, ref string) (*FileContent, error) {
	q := url.Values{}
	if ref != "" {
		q.Set("ref", ref)
	}
	rawURL := gitLabBase(baseURL) + "/projects/" + url.PathEscape(repo) + "/repository/files/" + url.PathEscape(path)
	if ref != "" {
		rawURL += "?" + q.Encode()
	}
	body, err := b.doGet(ctx, store.GitProviderGitLab, token, rawURL)
	if err != nil {
		return nil, err
	}
	var res struct {
		FilePath string `json:"file_path"`
		SHA      string `json:"blob_id"`
		Size     int64  `json:"size"`
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}
	fc := &FileContent{Path: res.FilePath, SHA: res.SHA, Size: res.Size, Encoding: res.Encoding, Content: res.Content}
	if fc.Encoding == "base64" {
		fc.Decoded = decodeBase64(fc.Content)
	}
	return fc, nil
}

func (b *Bridge) commitsGitLab(ctx context.Context, token, baseURL, repo, ref, path string) ([]CommitInfo, error) {
	q := url.Values{}
	if ref != "" {
		q.Set("ref_name", ref)
	}
	if path != "" {
		q.Set("path", path)
	}
	q.Set("per_page", "100")
	body, err := b.doGet(ctx, store.GitProviderGitLab, token,
		gitLabBase(baseURL)+"/projects/"+url.PathEscape(repo)+"/repository/commits?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID         string `json:"id"`
		Message    string `json:"message"`
		AuthorName string `json:"author_name"`
		CreatedAt  string `json:"created_at"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	commits := make([]CommitInfo, 0, len(raw))
	for _, c := range raw {
		ci := CommitInfo{SHA: c.ID, Message: firstLine(c.Message), Author: c.AuthorName}
		if t, err := time.Parse(time.RFC3339, c.CreatedAt); err == nil {
			ci.Date = &t
		}
		commits = append(commits, ci)
	}
	return commits, nil
}

// ---------- Bitbucket ----------

func (b *Bridge) browseBitbucket(ctx context.Context, token, baseURL, repo, path, branch string) ([]RepoItem, error) {
	rawURL := bitbucketBase(baseURL) + "/repositories/" + repo + "/src"
	if branch != "" {
		rawURL += "/" + branch
	}
	if path != "" {
		rawURL += "/" + strings.Trim(path, "/")
	}
	body, err := b.doGet(ctx, store.GitProviderBitbucket, token, rawURL)
	if err != nil {
		return nil, err
	}
	var res struct {
		Values []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			Size int64  `json:"size"`
		} `json:"values"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}
	items := make([]RepoItem, 0, len(res.Values))
	for _, e := range res.Values {
		items = append(items, RepoItem{Path: e.Path, Name: baseName(e.Path), Type: e.Type, Size: e.Size})
	}
	return items, nil
}

func (b *Bridge) fileBitbucket(ctx context.Context, token, baseURL, repo, path, branch string) (*FileContent, error) {
	rawURL := bitbucketBase(baseURL) + "/repositories/" + repo + "/src"
	if branch != "" {
		rawURL += "/" + branch
	}
	rawURL += "/" + strings.Trim(path, "/")
	body, err := b.doGet(ctx, store.GitProviderBitbucket, token, rawURL)
	if err != nil {
		return nil, err
	}
	return &FileContent{Path: path, Size: int64(len(body)), Encoding: "plain", Content: string(body), Decoded: string(body)}, nil
}

func (b *Bridge) commitsBitbucket(ctx context.Context, token, baseURL, repo string) ([]CommitInfo, error) {
	body, err := b.doGet(ctx, store.GitProviderBitbucket, token,
		bitbucketBase(baseURL)+"/repositories/"+repo+"/commits?pagelen=100")
	if err != nil {
		return nil, err
	}
	var res struct {
		Values []struct {
			Hash    string `json:"hash"`
			Message string `json:"message"`
			Author  struct {
				Raw string `json:"raw"`
			} `json:"author"`
			Date string `json:"date"`
		} `json:"values"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}
	commits := make([]CommitInfo, 0, len(res.Values))
	for _, c := range res.Values {
		ci := CommitInfo{SHA: c.Hash, Message: firstLine(c.Message), Author: c.Author.Raw}
		if t, err := time.Parse(time.RFC3339, c.Date); err == nil {
			ci.Date = &t
		}
		commits = append(commits, ci)
	}
	return commits, nil
}
