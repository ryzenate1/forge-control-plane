// Package gitpush implements a Dokku-style "git push to deploy" flow.
//
// Dokku's receive workflow (reference/app-platforms/dokku/plugins/git) is three
// moving parts: a bare repository per app, a post-receive hook that turns the
// "<old> <new> <ref>" lines git wrote to stdin into a build, and a deploy
// branch that decides which ref updates actually ship. This package keeps those
// three ideas and drops the shell plumbing: the panel records the repository's
// location and hands the push to the existing build/deploy pipeline instead of
// re-implementing one.
//
// The node side is deliberately conservative. Beacon exposes host file
// operations (/v1/files/mkdir) but no host-level exec, so the panel can create
// the repository's parent directory but cannot run `git init --bare` or install
// the post-receive hook itself. Rather than reporting a repository that does not
// exist, CreateApp provisions what it can, stores the intended repo path, and
// leaves the app in "provisioning" until a Beacon capability (or an operator)
// completes the bare repository.
package gitpush

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

// Builder names mirror the Dokku builder plugins (builder-herokuish,
// builder-dockerfile, builder-nixpacks, builder-null). "null" means "the pushed
// image is already built; skip compilation".
const (
	BuilderHerokuish  = "herokuish"
	BuilderDockerfile = "dockerfile"
	BuilderNixpacks   = "nixpacks"
	BuilderNull       = "null"
)

// App and event statuses. Kept as constants because the receive path branches
// on them and the UI renders a pill per value.
const (
	StatusProvisioning = "provisioning"
	StatusReady        = "ready"
	StatusFailed       = "failed"
	StatusArchived     = "archived"

	EventReceived = "received"
	EventQueued   = "queued"
	EventDeployed = "deployed"
	EventFailed   = "failed"
	EventSkipped  = "skipped"
)

// repoPathTemplate is where a node's bare repository lives. It is a fixed
// layout, not caller input: letting an admin choose the path would let the
// panel mkdir anywhere the beacon user can write.
const repoPathTemplate = "/srv/git-push/%s.git"

const zeroSHA = "0000000000000000000000000000000000000000"

var (
	ErrAppNotFound    = errors.New("git-push app not found")
	ErrInvalidName    = errors.New("app name must contain at least one alphanumeric character")
	ErrInvalidBuilder = errors.New("builder must be one of herokuish, dockerfile, nixpacks, null")
	ErrInvalidBranch  = errors.New("invalid deploy branch")
	ErrInvalidEnvKey  = errors.New("environment variable name is invalid")
	ErrSignature      = errors.New("invalid or missing git-push signature")
	ErrNotProvisioned = errors.New("git-push repository is not provisioned on the node yet")

	slugDisallowed = regexp.MustCompile(`[^a-z0-9]+`)
)

// GitPushDeployer is the seam into the build/deploy pipeline. The existing
// gitsvc.DeployService takes a GitSourceID rather than a (node, repo, sha)
// triple, so it does not satisfy this interface as-is; wiring the two together
// is an adapter's job (see the note in main.go). A nil deployer is legal: the
// push is still recorded, it just is not acted on.
type GitPushDeployer interface {
	DeployFromGit(ctx context.Context, nodeID, repoURL, branch, sha string) (operationID string, err error)
}

// App is the service-level view of a git-push application. EnvVars is the
// decoded form of the JSONB column; the wire shape keeps it camelCased.
type App struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Slug         string            `json:"slug"`
	NodeID       string            `json:"nodeId"`
	ServerID     *string           `json:"serverId,omitempty"`
	Builder      string            `json:"builder"`
	Branch       string            `json:"branch"`
	RepoPath     string            `json:"repoPath"`
	SharedSecret string            `json:"sharedSecret"`
	DeployedSHA  *string           `json:"deployedSha,omitempty"`
	LastDeployAt *string           `json:"lastDeployAt,omitempty"`
	Status       string            `json:"status"`
	AutoDeploy   bool              `json:"autoDeploy"`
	EnvVars      map[string]string `json:"envVars"`
	CreatedAt    string            `json:"createdAt"`
	UpdatedAt    string            `json:"updatedAt"`
}

// PushEvent is one ref update reported by a repository's post-receive hook.
type PushEvent struct {
	ID        string  `json:"id"`
	AppID     string  `json:"appId"`
	Ref       string  `json:"ref"`
	BeforeSHA string  `json:"beforeSha"`
	AfterSHA  string  `json:"afterSha"`
	Actor     string  `json:"actor"`
	Status    string  `json:"status"`
	Error     *string `json:"error,omitempty"`
	CreatedAt string  `json:"createdAt"`
}

// ReceiveResult reports what the panel did with one hook callback.
type ReceiveResult struct {
	Events []PushEvent `json:"events"`
	// Deployed is true when at least one ref update was handed to the deployer.
	Deployed bool `json:"deployed"`
}

type Service struct {
	db       *store.Store
	daemon   *daemon.Client
	deployer GitPushDeployer
	logger   *slog.Logger

	// panelHost is the fallback SSH target when a node carries neither an FQDN
	// nor a base URL (a node reached only through the panel's reverse proxy).
	panelHost string
	sshPort   int
}

func New(db *store.Store, daemonClient *daemon.Client, deployer GitPushDeployer, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{db: db, daemon: daemonClient, deployer: deployer, logger: logger, sshPort: 22}
}

// WithPanelEndpoint sets the fallback host (and port) used to build remote URLs
// for nodes that do not advertise their own address. The host is a bare
// hostname, not a URL.
func (s *Service) WithPanelEndpoint(host string, port int) *Service {
	s.panelHost = strings.TrimSpace(host)
	if port > 0 && port <= 65535 {
		s.sshPort = port
	}
	return s
}

// ---------------------------------------------------------------- creation ---

// CreateApp registers a git-push application: it allocates the slug and hook
// secret, records the repository path the node will host, and creates what it
// actually can on the node. The app is left in "provisioning" because the bare
// repository and its post-receive hook need a host-level exec that Beacon does
// not expose; claiming otherwise would hand the operator a remote URL that
// cannot be pushed to.
func (s *Service) CreateApp(ctx context.Context, name, nodeID, builder, branch string) (*App, error) {
	if s.db == nil {
		return nil, errors.New("store not configured")
	}
	slug, err := s.uniqueSlug(ctx, name)
	if err != nil {
		return nil, err
	}
	builder, err = normalizeBuilder(builder)
	if err != nil {
		return nil, err
	}
	branch = normalizeBranch(branch)

	node, err := s.db.GetNode(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("node %q not available: %w", nodeID, err)
	}

	secret, err := randomHex(32)
	if err != nil {
		return nil, err
	}

	app := &store.GitPushApp{
		ID:           uuid.NewString(),
		Name:         strings.TrimSpace(name),
		Slug:         slug,
		NodeID:       node.ID,
		Builder:      builder,
		Branch:       branch,
		RepoPath:     fmt.Sprintf(repoPathTemplate, slug),
		SharedSecret: secret,
		Status:       StatusProvisioning,
		AutoDeploy:   true,
		EnvVars:      []byte(`{}`),
	}
	if err := s.db.CreateGitPushApp(ctx, app); err != nil {
		return nil, fmt.Errorf("create git-push app: %w", err)
	}

	// Best effort: create the directory the bare repository will live in. This
	// is the only provisioning step the current Beacon API supports.
	if err := s.ensureRepoParent(ctx, node, app.RepoPath); err != nil {
		s.logger.Warn("git-push repository parent could not be created on the node",
			"app", app.Slug, "node", node.ID, "path", app.RepoPath, "error", err)
	} else {
		s.logger.Info("git-push app registered; bare repository still needs `git init --bare` on the node",
			"app", app.Slug, "node", node.ID, "path", app.RepoPath, "status", StatusProvisioning)
	}

	return appToService(app), nil
}

func (s *Service) ensureRepoParent(ctx context.Context, node store.Node, repoPath string) error {
	if s.daemon == nil {
		return errors.New("daemon client not configured")
	}
	token, err := s.db.GetNodeDaemonCredential(ctx, node.ID)
	if err != nil {
		return fmt.Errorf("node credential: %w", err)
	}
	parent := strings.TrimSuffix(repoPath, "/"+slugFromPath(repoPath))
	return s.daemon.HostFilesMkdir(ctx, node.BaseURL, token, parent)
}

// ------------------------------------------------------------------- reads ---

func (s *Service) ListApps(ctx context.Context) ([]App, error) {
	if s.db == nil {
		return []App{}, nil
	}
	rows, err := s.db.ListGitPushApps(ctx)
	if err != nil {
		return nil, err
	}
	apps := make([]App, 0, len(rows))
	for i := range rows {
		apps = append(apps, *appToService(&rows[i]))
	}
	return apps, nil
}

func (s *Service) GetApp(ctx context.Context, id string) (*App, error) {
	if s.db == nil {
		return nil, ErrAppNotFound
	}
	row, err := s.db.GetGitPushApp(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrGitPushAppNotFound) {
			return nil, ErrAppNotFound
		}
		return nil, err
	}
	return appToService(row), nil
}

func (s *Service) ListEvents(ctx context.Context, appID string, limit int) ([]PushEvent, error) {
	if s.db == nil {
		return []PushEvent{}, nil
	}
	rows, err := s.db.ListGitPushEvents(ctx, appID, limit)
	if err != nil {
		return nil, err
	}
	events := make([]PushEvent, 0, len(rows))
	for i := range rows {
		events = append(events, *eventToService(&rows[i]))
	}
	return events, nil
}

// RemoteURL builds the SSH remote a developer adds to their local clone:
// `ssh://git@<host>:<port>/<slug>.git`. The host is the node's own FQDN when it
// has one, otherwise the host its Beacon is reachable at, otherwise the panel
// fallback (a deployment where one reverse proxy fronts every node's SSH).
func (s *Service) RemoteURL(ctx context.Context, app *App) (string, error) {
	if app == nil {
		return "", ErrAppNotFound
	}
	host := ""
	port := s.sshPort
	if s.db != nil {
		if node, err := s.db.GetNode(ctx, app.NodeID); err == nil {
			host = firstNonEmpty(node.FQDN, hostFromURL(node.BaseURL), node.PublicHostname)
			if node.DaemonSFTP > 0 {
				port = node.DaemonSFTP
			}
		}
	}
	if host == "" {
		host = s.panelHost
	}
	if host == "" {
		return "", ErrNotProvisioned
	}
	return fmt.Sprintf("ssh://git@%s:%d/%s.git", host, port, app.Slug), nil
}

// --------------------------------------------------------------- mutations ---

// UpdateApp changes the builder, deploy branch and auto-deploy toggle. A nil
// argument means "leave as stored".
func (s *Service) UpdateApp(ctx context.Context, id string, builder *string, branch *string, autoDeploy *bool) (*App, error) {
	if s.db == nil {
		return nil, ErrAppNotFound
	}
	row, err := s.db.GetGitPushApp(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrGitPushAppNotFound) {
			return nil, ErrAppNotFound
		}
		return nil, err
	}
	if builder != nil {
		b, err := normalizeBuilder(*builder)
		if err != nil {
			return nil, err
		}
		row.Builder = b
	}
	if branch != nil {
		b := normalizeBranch(*branch)
		if b == "" {
			return nil, ErrInvalidBranch
		}
		row.Branch = b
	}
	if autoDeploy != nil {
		row.AutoDeploy = *autoDeploy
	}
	if err := s.db.UpdateGitPushApp(ctx, row); err != nil {
		return nil, err
	}
	return appToService(row), nil
}

func (s *Service) DeleteApp(ctx context.Context, id string) error {
	if s.db == nil {
		return ErrAppNotFound
	}
	row, err := s.db.GetGitPushApp(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrGitPushAppNotFound) {
			return ErrAppNotFound
		}
		return err
	}
	// Best-effort teardown of the repository. A node that never finished
	// provisioning has nothing to remove, and a failure here must not strand
	// the database row, so it is logged rather than returned.
	if s.daemon != nil && row.RepoPath != "" {
		if node, nodeErr := s.db.GetNode(ctx, row.NodeID); nodeErr == nil && node.BaseURL != "" {
			if token, tokenErr := s.db.GetNodeDaemonCredential(ctx, node.ID); tokenErr == nil {
				if rmErr := s.daemon.HostFilesRemove(ctx, node.BaseURL, token, row.RepoPath); rmErr != nil {
					s.logger.Warn("git-push repository removal failed",
						"app", row.Slug, "path", row.RepoPath, "error", rmErr)
				}
			}
		}
	}
	if err := s.db.DeleteGitPushApp(ctx, id); err != nil {
		if errors.Is(err, store.ErrGitPushAppNotFound) {
			return ErrAppNotFound
		}
		return err
	}
	return nil
}

func (s *Service) SetGitPushAppStatus(ctx context.Context, id string, status string) error {
	if s.db == nil {
		return ErrAppNotFound
	}
	if err := s.db.SetGitPushAppStatus(ctx, id, status); err != nil {
		if errors.Is(err, store.ErrGitPushAppNotFound) {
			return ErrAppNotFound
		}
		return err
	}
	return nil
}

// SetEnv writes one environment variable that the build/deploy handoff passes
// through to the app (Dokku's `config:set` equivalent).
func (s *Service) SetEnv(ctx context.Context, id string, vars map[string]string) (*App, error) {
	if s.db == nil {
		return nil, ErrAppNotFound
	}
	row, err := s.db.GetGitPushApp(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrGitPushAppNotFound) {
			return nil, ErrAppNotFound
		}
		return nil, err
	}
	current := decodeEnv(row.EnvVars)
	for k, v := range vars {
		k = strings.TrimSpace(k)
		if !validEnvKey(k) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidEnvKey, k)
		}
		current[k] = v
	}
	encoded, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}
	if err := s.db.SetGitPushAppEnvVars(ctx, id, encoded); err != nil {
		if errors.Is(err, store.ErrGitPushAppNotFound) {
			return nil, ErrAppNotFound
		}
		return nil, err
	}
	row.EnvVars = encoded
	return appToService(row), nil
}

func (s *Service) DeleteEnv(ctx context.Context, id string, keys []string) (*App, error) {
	if s.db == nil {
		return nil, ErrAppNotFound
	}
	row, err := s.db.GetGitPushApp(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrGitPushAppNotFound) {
			return nil, ErrAppNotFound
		}
		return nil, err
	}
	current := decodeEnv(row.EnvVars)
	for _, k := range keys {
		delete(current, strings.TrimSpace(k))
	}
	encoded, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}
	if err := s.db.SetGitPushAppEnvVars(ctx, id, encoded); err != nil {
		if errors.Is(err, store.ErrGitPushAppNotFound) {
			return nil, ErrAppNotFound
		}
		return nil, err
	}
	row.EnvVars = encoded
	return appToService(row), nil
}

// RotateSecret replaces the HMAC key the post-receive hook signs with. Callers
// must re-issue the hook on the node; until they do, callbacks fail signature
// verification, which is the safe direction for a mistake.
func (s *Service) RotateSecret(ctx context.Context, id string) (*App, error) {
	if s.db == nil {
		return nil, ErrAppNotFound
	}
	secret, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	if err := s.db.RotateGitPushSecret(ctx, id, secret); err != nil {
		if errors.Is(err, store.ErrGitPushAppNotFound) {
			return nil, ErrAppNotFound
		}
		return nil, err
	}
	return s.GetApp(ctx, id)
}

// ------------------------------------------------------------ receive path ---

// HandleReceive is the panel half of Dokku's `receive` workflow. The body is
// exactly what git wrote to the hook's stdin: one "<old> <new> <ref>" line per
// updated ref. The raw bytes are HMAC-SHA256'd with the app's shared secret;
// nothing else authenticates this endpoint, so the comparison is constant-time
// and the parse happens only after it succeeds.
func (s *Service) HandleReceive(ctx context.Context, slug string, signature string, body []byte) (*ReceiveResult, error) {
	if s.db == nil {
		return nil, ErrAppNotFound
	}
	row, err := s.db.GetGitPushAppBySlug(ctx, strings.TrimSpace(slug))
	if err != nil {
		if errors.Is(err, store.ErrGitPushAppNotFound) {
			return nil, ErrAppNotFound
		}
		return nil, err
	}
	if row.SharedSecret == "" || !verifySignature(row.SharedSecret, signature, body) {
		return nil, ErrSignature
	}

	result := &ReceiveResult{Events: []PushEvent{}}
	actor := "git-hook"
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		before, after, ref, ok := parseRefUpdate(line)
		if !ok {
			s.logger.Warn("git-push hook sent an unparsable ref line", "app", row.Slug, "line", line)
			continue
		}

		status := EventReceived
		var failure string
		switch {
		case after == zeroSHA:
			// A branch deletion. Dokku ignores these too; there is no code to build.
			status = EventSkipped
		case ref != "refs/heads/"+row.Branch:
			// Not the deploy branch (Dokku's `git:set-deploy-branch`).
			status = EventSkipped
		case !row.AutoDeploy:
			status = EventSkipped
			failure = "auto-deploy is disabled for this app"
		default:
			status = EventQueued
		}

		event := &store.GitPushEvent{
			ID:        uuid.NewString(),
			AppID:     row.ID,
			Ref:       ref,
			BeforeSHA: before,
			AfterSHA:  after,
			Actor:     actor,
			Status:    status,
		}
		if failure != "" {
			msg := failure
			event.Error = &msg
		}
		if err := s.db.RecordGitPushEvent(ctx, event); err != nil {
			return nil, fmt.Errorf("record push event: %w", err)
		}
		result.Events = append(result.Events, *eventToService(event))

		if status != EventQueued {
			continue
		}
		if s.deployer == nil {
			// Honest, not fatal: the push is recorded and can be deployed by
			// hand, but no pipeline is wired to this service yet.
			msg := "no deployer is wired to the git-push service"
			s.logger.Warn("git-push deploy skipped", "app", row.Slug, "sha", after, "reason", msg)
			if err := s.db.UpdatePushEventStatus(ctx, event.ID, EventSkipped, msg); err != nil {
				s.logger.Error("git-push event update failed", "event", event.ID, "error", err)
			}
			continue
		}

		repoURL, urlErr := s.RemoteURL(ctx, appToService(row))
		if urlErr != nil {
			s.failEvent(ctx, event.ID, fmt.Sprintf("resolve remote: %v", urlErr))
			continue
		}
		if _, deployErr := s.deployer.DeployFromGit(ctx, row.NodeID, repoURL, row.Branch, after); deployErr != nil {
			s.failEvent(ctx, event.ID, fmt.Sprintf("deploy: %v", deployErr))
			continue
		}
		result.Deployed = true
		if err := s.db.UpdatePushEventStatus(ctx, event.ID, EventDeployed, ""); err != nil {
			s.logger.Error("git-push event update failed", "event", event.ID, "error", err)
		}
		if err := s.db.MarkGitPushAppDeployed(ctx, row.ID, after); err != nil {
			s.logger.Error("git-push app deploy marker failed", "app", row.ID, "error", err)
		}
	}

	if len(result.Events) == 0 {
		return nil, fmt.Errorf("git-push hook body contained no ref updates")
	}
	return result, nil
}

func (s *Service) failEvent(ctx context.Context, eventID string, msg string) {
	s.logger.Warn("git-push deploy failed", "event", eventID, "reason", msg)
	if err := s.db.UpdatePushEventStatus(ctx, eventID, EventFailed, msg); err != nil {
		s.logger.Error("git-push event update failed", "event", eventID, "error", err)
	}
}

// ------------------------------------------------------------------- helpers ---

// verifySignature accepts either "sha256=<hex>" (the header shape Dokku's own
// plugins and GitHub both use) or a bare hex digest, and compares in constant
// time so a wrong signature cannot be found one byte at a time.
func verifySignature(secret, signature string, body []byte) bool {
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return false
	}
	if idx := strings.IndexByte(signature, '='); strings.HasPrefix(signature, "sha256:") && idx > 0 {
		signature = signature[idx+1:]
	}
	provided, err := hex.DecodeString(strings.TrimSpace(signature))
	if err != nil || len(provided) == 0 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	if len(provided) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare(expected, provided) == 1
}

// parseRefUpdate splits one line of a post-receive stdin: "<old> <new> <ref>".
func parseRefUpdate(line string) (before, after, ref string, ok bool) {
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return "", "", "", false
	}
	before, after = parts[0], parts[1]
	ref = strings.Join(parts[2:], " ")
	if !isHexSHA(before) || !isHexSHA(after) {
		return "", "", "", false
	}
	return before, after, ref, true
}

func isHexSHA(s string) bool {
	if s == zeroSHA {
		return true
	}
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') && !(r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

func normalizeBuilder(builder string) (string, error) {
	switch b := strings.ToLower(strings.TrimSpace(builder)); b {
	case BuilderHerokuish, BuilderDockerfile, BuilderNixpacks, BuilderNull:
		return b, nil
	case "":
		return BuilderDockerfile, nil
	default:
		return "", ErrInvalidBuilder
	}
}

func normalizeBranch(branch string) string {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "main"
	}
	return strings.TrimPrefix(branch, "refs/heads/")
}

// slugify turns a display name into the path segment that becomes both the
// repository name and the hook's address. Anything that is not [a-z0-9]
// collapses to a single hyphen.
func slugify(name string) string {
	slug := slugDisallowed.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return ""
	}
	if len(slug) > 48 {
		slug = strings.Trim(slug[:48], "-")
	}
	return slug
}

// uniqueSlug keeps a generated suffix on collisions: two admins naming an app
// "web" must not end up fighting over /srv/git-push/web.git.
func (s *Service) uniqueSlug(ctx context.Context, name string) (string, error) {
	base := slugify(name)
	if base == "" {
		return "", ErrInvalidName
	}
	taken := map[string]bool{}
	if apps, err := s.db.ListGitPushApps(ctx); err == nil {
		for _, a := range apps {
			taken[a.Slug] = true
		}
	} else {
		return "", err
	}
	if !taken[base] {
		return base, nil
	}
	for i := 2; i < 1000; i++ {
		candidate := base + "-" + strconv.Itoa(i)
		if !taken[candidate] {
			return candidate, nil
		}
	}
	suffix, err := randomHex(3)
	if err != nil {
		return "", err
	}
	return base + "-" + suffix, nil
}

func slugFromPath(path string) string {
	path = strings.TrimSuffix(path, "/")
	if idx := strings.LastIndexByte(path, '/'); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

func hostFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		isValid := r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (i > 0 && r >= '0' && r <= '9')
		if !isValid {
			return false
		}
	}
	return true
}

func decodeEnv(raw []byte) map[string]string {
	out := map[string]string{}
	if len(raw) == 0 {
		return out
	}
	// Env vars are a flat string map by contract; a corrupt column degrades to
	// "no variables" rather than failing the whole read.
	var parsed map[string]string
	if err := json.Unmarshal(raw, &parsed); err == nil {
		for k, v := range parsed {
			out[k] = v
		}
	}
	return out
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate git-push secret: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func appToService(row *store.GitPushApp) *App {
	app := &App{
		ID:           row.ID,
		Name:         row.Name,
		Slug:         row.Slug,
		NodeID:       row.NodeID,
		ServerID:     row.ServerID,
		Builder:      row.Builder,
		Branch:       row.Branch,
		RepoPath:     row.RepoPath,
		SharedSecret: row.SharedSecret,
		DeployedSHA:  row.DeployedSHA,
		Status:       row.Status,
		AutoDeploy:   row.AutoDeploy,
		EnvVars:      decodeEnv(row.EnvVars),
		CreatedAt:    row.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:    row.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if row.LastDeployAt != nil {
		stamp := row.LastDeployAt.UTC().Format(time.RFC3339)
		app.LastDeployAt = &stamp
	}
	return app
}

func eventToService(row *store.GitPushEvent) *PushEvent {
	return &PushEvent{
		ID:        row.ID,
		AppID:     row.AppID,
		Ref:       row.Ref,
		BeforeSHA: row.BeforeSHA,
		AfterSHA:  row.AfterSHA,
		Actor:     row.Actor,
		Status:    row.Status,
		Error:     row.Error,
		CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339),
	}
}
