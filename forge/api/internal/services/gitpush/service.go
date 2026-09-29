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
	// EventDeploying is the state after the ref update has been handed to the
	// deployer: the deploy is in flight, not finished. Nothing in this package
	// observes a completion, so it never claims "deployed".
	EventDeploying = "deploying"
	// EventDeployed is reserved for the deploy-completion callback, which does
	// not exist yet; nothing here advances an event past EventDeploying.
	EventDeployed = "deployed"
	EventFailed   = "failed"
	EventSkipped  = "skipped"
)

// repoPathTemplate is where a node's bare repository lives. It is a fixed
// layout, not caller input: letting an admin choose the path would let the
// panel mkdir anywhere the beacon user can write.
const repoPathTemplate = "/srv/git-push/%s.git"

const zeroSHA = "0000000000000000000000000000000000000000"

// maxNameLen, maxEnvValueLen and MaxReceiveBodyBytes bound the
// caller-controlled inputs: a display name, a config value and the post-receive
// body arriving on the public receive endpoint. MaxReceiveBodyBytes is exported
// so the handler can reject an oversized push before Fiber has buffered it.
const (
	maxNameLen      = 64
	maxEnvValueLen  = 8192
	MaxBodyBytes    = 1 << 20
	maxSignatureLen = 256
	// maxRefUpdates bounds one push's ref updates: a single push moves a handful
	// of refs, and every line becomes a row.
	maxRefUpdates = 128
)

var (
	ErrAppNotFound    = errors.New("git-push app not found")
	ErrInvalidName    = errors.New("app name must contain at least one alphanumeric character")
	ErrInvalidBuilder = errors.New("builder must be one of herokuish, dockerfile, nixpacks, null")
	ErrInvalidBranch  = errors.New("invalid deploy branch")
	ErrInvalidRef     = errors.New("invalid git ref in push body")
	ErrInvalidEnvKey  = errors.New("environment variable name is invalid")
	ErrSignature      = errors.New("invalid or missing git-push signature")
	ErrNotProvisioned = errors.New("git-push repository is not provisioned on the node yet")
	// ErrAppArchived is returned when a still-installed hook pushes to an
	// archived app: archiving is a teardown that has not finished, and deploying
	// onto it would report work nobody asked for.
	ErrAppArchived = errors.New("git-push app is archived")

	slugDisallowed = regexp.MustCompile(`[^a-z0-9]+`)
	slugPattern    = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,46}[a-z0-9])?$`)

	// refPattern accepts a namespaced git ref whose every segment starts with an
	// alphanumeric character. That single rule rejects a leading '-' (which a
	// git argv would read as an option), a ".." segment (path traversal once the
	// ref is used in a filesystem path), and every control character.
	refPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*(?:/[A-Za-z0-9][A-Za-z0-9._-]*)*$`)
)

// GitPushDeployer is the seam into the build/deploy pipeline. The existing
// gitsvc.DeployService takes a GitSourceID rather than a (node, repo, sha)
// triple, so it does not satisfy this interface as-is; wiring the two together
// is an adapter's job. A nil deployer is legal: the push is still recorded, it
// just is not acted on, and every such event is stored as skipped with the
// reason rather than reported as deployed.
type GitPushDeployer interface {
	DeployFromGit(ctx context.Context, nodeID, repoURL, branch, sha string) (operationID string, err error)
}

// App is the service-level view of a git-push application. EnvVars is the
// decoded form of the JSONB column; the wire shape keeps it camelCased.
// SharedSecret is the HMAC key the node's post-receive hook signs with, so it
// is populated only by the calls that hand an operator something to install:
// CreateApp and RotateSecret. Everything else returns a redacted copy.
type App struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Slug         string            `json:"slug"`
	NodeID       string            `json:"nodeId"`
	ServerID     *string           `json:"serverId,omitempty"`
	Builder      string            `json:"builder"`
	Branch       string            `json:"branch"`
	RepoPath     string            `json:"repoPath"`
	SharedSecret string            `json:"sharedSecret,omitempty"`
	DeployedSHA  *string           `json:"deployedSha,omitempty"`
	LastDeployAt *string           `json:"lastDeployAt,omitempty"`
	Status       string            `json:"status"`
	AutoDeploy   bool              `json:"autoDeploy"`
	EnvVars      map[string]string `json:"envVars"`
	CreatedAt    string            `json:"createdAt"`
	UpdatedAt    string            `json:"updatedAt"`
}

// withoutSecret returns a copy safe to serve from a read endpoint: the hook
// secret cannot be re-issued from a read, so it is not sent.
func (a *App) withoutSecret() *App {
	if a == nil {
		return nil
	}
	copied := *a
	copied.SharedSecret = ""
	return &copied
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
	// It means accepted, not finished: the events carry the real state.
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
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxNameLen {
		return nil, fmt.Errorf("app name must be between 1 and %d characters", maxNameLen)
	}
	// A missing node is not defaulted to any node: guessing one would reserve a
	// repository path on a machine nobody asked for.
	if strings.TrimSpace(nodeID) == "" {
		return nil, errors.New("node id is required")
	}
	slug, err := s.uniqueSlug(ctx, name)
	if err != nil {
		return nil, err
	}
	builder, err = normalizeBuilder(builder)
	if err != nil {
		return nil, err
	}
	branch, err = sanitizeBranch(branch)
	if err != nil {
		return nil, err
	}

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
		Name:         name,
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
		return nil, errors.New("store not configured")
	}
	rows, err := s.db.ListGitPushApps(ctx)
	if err != nil {
		return nil, err
	}
	apps := make([]App, 0, len(rows))
	for i := range rows {
		apps = append(apps, *appToService(&rows[i]).withoutSecret())
	}
	return apps, nil
}

func (s *Service) GetApp(ctx context.Context, id string) (*App, error) {
	if s.db == nil {
		return nil, errors.New("store not configured")
	}
	row, err := s.db.GetGitPushApp(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrGitPushAppNotFound) {
			return nil, ErrAppNotFound
		}
		return nil, err
	}
	return appToService(row).withoutSecret(), nil
}

// GetAppWithSecret resolves an app including the hook secret. Only the create
// and rotate responses use it; every other read goes through GetApp.
func (s *Service) GetAppWithSecret(ctx context.Context, id string) (*App, error) {
	if s.db == nil {
		return nil, errors.New("store not configured")
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
		return nil, errors.New("store not configured")
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
		// A node that cannot be read is not a node that has no address: falling
		// back to the panel host here would hand out a remote for the wrong
		// machine, which is exactly the ambiguous target this repo forbids.
		node, err := s.db.GetNode(ctx, app.NodeID)
		if err != nil {
			return "", fmt.Errorf("resolve node %q: %w", app.NodeID, err)
		}
		host = firstNonEmpty(node.FQDN, hostFromURL(node.BaseURL), node.PublicHostname)
		if node.DaemonSFTP > 0 {
			port = node.DaemonSFTP
		}
	}
	if host == "" {
		host = s.panelHost
	}
	if host == "" {
		return "", ErrNotProvisioned
	}
	if !validRemoteHost(host) {
		return "", fmt.Errorf("node host %q is not a usable remote address", host)
	}
	return fmt.Sprintf("ssh://git@%s:%d/%s.git", host, port, app.Slug), nil
}

// validRemoteHost guards the string interpolation that builds the remote: the
// host is stored on a node row, and a value containing a space, a scheme or a
// path separator would produce a remote that is not the one it looks like.
func validRemoteHost(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	if strings.ContainsAny(host, "@/:\\ \t\r\n") {
		return false
	}
	return true
}

// --------------------------------------------------------------- mutations ---

// UpdateApp changes the builder, deploy branch and auto-deploy toggle. A nil
// argument means "leave as stored".
func (s *Service) UpdateApp(ctx context.Context, id string, builder *string, branch *string, autoDeploy *bool) (*App, error) {
	if s.db == nil {
		return nil, errors.New("store not configured")
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
		b, err := sanitizeBranch(*branch)
		if err != nil {
			return nil, err
		}
		row.Branch = b
	}
	if autoDeploy != nil {
		row.AutoDeploy = *autoDeploy
	}
	if err := s.db.UpdateGitPushApp(ctx, row); err != nil {
		return nil, err
	}
	return appToService(row).withoutSecret(), nil
}

// DeleteApp removes the app and the bare repository it owns on the node. The
// repository is removed first and a failed removal aborts the delete: an app
// row that is gone but a live repository that is not leaves a push target
// whose hook secret nobody remembers.
func (s *Service) DeleteApp(ctx context.Context, id string) error {
	if s.db == nil {
		return errors.New("store not configured")
	}
	row, err := s.db.GetGitPushApp(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrGitPushAppNotFound) {
			return ErrAppNotFound
		}
		return err
	}
	if err := s.removeRepoFromNode(ctx, row); err != nil {
		return err
	}
	if err := s.db.DeleteGitPushApp(ctx, id); err != nil {
		if errors.Is(err, store.ErrGitPushAppNotFound) {
			return ErrAppNotFound
		}
		return err
	}
	return nil
}

// removeRepoFromNode deletes the repository directory when the node reports it
// present. A node that cannot be reached, or whose credential is missing, is
// an error: the caller has to know the teardown did not happen rather than
// receive a successful delete of an app whose repo is still accepting pushes.
func (s *Service) removeRepoFromNode(ctx context.Context, row *store.GitPushApp) error {
	if row.RepoPath == "" {
		return nil
	}
	if s.daemon == nil {
		return fmt.Errorf("git-push repository %s was not removed: no node client is configured", row.RepoPath)
	}
	node, err := s.db.GetNode(ctx, row.NodeID)
	if err != nil {
		return fmt.Errorf("git-push repository %s was not removed: resolve node: %w", row.RepoPath, err)
	}
	token, err := s.db.GetNodeDaemonCredential(ctx, node.ID)
	if err != nil {
		return fmt.Errorf("git-push repository %s was not removed: node credential: %w", row.RepoPath, err)
	}
	present, err := s.repoPresent(ctx, node.BaseURL, token, row.RepoPath)
	if err != nil {
		return fmt.Errorf("git-push repository %s removal unverified: %w", row.RepoPath, err)
	}
	if !present {
		return nil
	}
	if err := s.daemon.HostFilesRemove(ctx, node.BaseURL, token, row.RepoPath); err != nil {
		return fmt.Errorf("git-push repository %s could not be removed: %w", row.RepoPath, err)
	}
	return nil
}

// repoPresent asks the node whether path exists as a directory, by listing its
// parent. Listing the parent rather than the path itself keeps "absent" and
// "unreadable" distinguishable: only a failed parent read is an error.
func (s *Service) repoPresent(ctx context.Context, baseURL, token, path string) (bool, error) {
	if s.daemon == nil {
		return false, errors.New("daemon client not configured")
	}
	if baseURL == "" {
		return false, errors.New("node has no base url")
	}
	parent := strings.TrimSuffix(path, "/"+slugFromPath(path))
	if parent == path {
		return false, fmt.Errorf("repository path %q has no parent directory", path)
	}
	raw, err := s.daemon.HostFilesList(ctx, baseURL, token, parent)
	if err != nil {
		return false, err
	}
	var entries []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		return false, fmt.Errorf("decode node listing of %q: %w", parent, err)
	}
	want := slugFromPath(path)
	for _, e := range entries {
		if e.Name == want {
			return true, nil
		}
	}
	return false, nil
}

// VerifyProvisioned is the operator's answer to "is the repository actually
// there yet?". CreateApp can only reserve the path, so an app stays in
// "provisioning" until this check, which asks the node, moves it to "ready" or
// leaves it where it is. It never reports ready without the live check.
func (s *Service) VerifyProvisioned(ctx context.Context, id string) (*App, bool, error) {
	if s.db == nil {
		return nil, false, errors.New("store not configured")
	}
	row, err := s.db.GetGitPushApp(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrGitPushAppNotFound) {
			return nil, false, ErrAppNotFound
		}
		return nil, false, err
	}
	if s.daemon == nil {
		return nil, false, errors.New("no node client is configured, cannot verify the repository")
	}
	node, err := s.db.GetNode(ctx, row.NodeID)
	if err != nil {
		return nil, false, fmt.Errorf("resolve node %q: %w", row.NodeID, err)
	}
	token, err := s.db.GetNodeDaemonCredential(ctx, node.ID)
	if err != nil {
		return nil, false, fmt.Errorf("node credential: %w", err)
	}
	present, err := s.repoPresent(ctx, node.BaseURL, token, row.RepoPath)
	if err != nil {
		return nil, false, err
	}
	if present && row.Status != StatusReady {
		if err := s.db.SetGitPushAppStatus(ctx, row.ID, StatusReady); err != nil && !errors.Is(err, store.ErrGitPushAppNotFound) {
			return nil, false, err
		}
		row.Status = StatusReady
	}
	if !present && row.Status == StatusReady {
		// The repository went away under a row that still claims it exists.
		if err := s.db.SetGitPushAppStatus(ctx, row.ID, StatusProvisioning); err != nil && !errors.Is(err, store.ErrGitPushAppNotFound) {
			return nil, false, err
		}
		row.Status = StatusProvisioning
	}
	return appToService(row).withoutSecret(), present, nil
}

func (s *Service) SetGitPushAppStatus(ctx context.Context, id string, status string) error {
	if s.db == nil {
		return errors.New("store not configured")
	}
	if !validAppStatus(status) {
		return fmt.Errorf("unknown git-push app status %q", status)
	}
	if err := s.db.SetGitPushAppStatus(ctx, id, status); err != nil {
		if errors.Is(err, store.ErrGitPushAppNotFound) {
			return ErrAppNotFound
		}
		return err
	}
	return nil
}

func validAppStatus(status string) bool {
	switch status {
	case StatusProvisioning, StatusReady, StatusFailed, StatusArchived:
		return true
	default:
		return false
	}
}

// SetEnv writes one environment variable that the build/deploy handoff passes
// through to the app (Dokku's `config:set` equivalent).
func (s *Service) SetEnv(ctx context.Context, id string, vars map[string]string) (*App, error) {
	if s.db == nil {
		return nil, errors.New("store not configured")
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
		if len(v) > maxEnvValueLen {
			return nil, fmt.Errorf("environment value for %q exceeds %d characters", k, maxEnvValueLen)
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
	return appToService(row).withoutSecret(), nil
}

func (s *Service) DeleteEnv(ctx context.Context, id string, keys []string) (*App, error) {
	if s.db == nil {
		return nil, errors.New("store not configured")
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
	return appToService(row).withoutSecret(), nil
}

// RotateSecret replaces the HMAC key the post-receive hook signs with. Callers
// must re-issue the hook on the node; until they do, callbacks fail signature
// verification, which is the safe direction for a mistake. This is one of the
// two calls that return the secret, because the operator has to install it.
func (s *Service) RotateSecret(ctx context.Context, id string) (*App, error) {
	if s.db == nil {
		return nil, errors.New("store not configured")
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
	return s.GetAppWithSecret(ctx, id)
}

// ------------------------------------------------------------ receive path ---

// HandleReceive is the panel half of Dokku's `receive` workflow. The body is
// exactly what git wrote to the hook's stdin: one "<old> <new> <ref>" line per
// updated ref. The raw bytes are HMAC-SHA256'd with the app's shared secret;
// nothing else authenticates this endpoint, so the comparison is constant-time
// and the body is only parsed, and only refs accepted, once it succeeds.
func (s *Service) HandleReceive(ctx context.Context, slug string, signature string, body []byte) (*ReceiveResult, error) {
	if s.db == nil {
		return nil, errors.New("store not configured")
	}
	slug = strings.TrimSpace(slug)
	if !slugPattern.MatchString(slug) {
		return nil, ErrAppNotFound
	}
	// Bound the payload before it is copied into strings, maps and event rows.
	if len(body) > MaxBodyBytes || len(signature) > maxSignatureLen || signature == "" {
		return nil, ErrSignature
	}
	row, err := s.db.GetGitPushAppBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, store.ErrGitPushAppNotFound) {
			return nil, ErrAppNotFound
		}
		return nil, err
	}
	// Archiving keeps the row (and therefore the hook's secret) alive, so a push
	// that arrives afterwards has to be refused rather than deployed.
	if row.Status == StatusArchived {
		return nil, ErrAppArchived
	}
	// An unset secret would make verifySignature accept an all-zero key; the
	// hook can never be signed with nothing, so refuse it outright.
	if row.SharedSecret == "" || !verifySignature(row.SharedSecret, signature, body) {
		return nil, ErrSignature
	}

	// Parse the whole body before writing anything, so a malformed or hostile
	// line cannot leave a half-applied push behind it.
	updates, err := parseReceiveBody(string(body))
	if err != nil {
		return nil, err
	}

	result := &ReceiveResult{Events: make([]PushEvent, 0, len(updates))}
	const actor = "git-hook"
	for _, u := range updates {
		before, after, ref := u.before, u.after, u.ref

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
		// The deployer accepted the handoff; nothing here observes it finish, so
		// the event stops at "deploying". Marking it deployed - or the app as
		// running this SHA - would be reporting success for work that may still
		// fail on the node.
		result.Deployed = true
		if err := s.db.UpdatePushEventStatus(ctx, event.ID, EventDeploying, ""); err != nil {
			s.logger.Error("git-push event update failed", "event", event.ID, "error", err)
		}
	}

	return result, nil
}

// refUpdate is one parsed line of a post-receive body.
type refUpdate struct {
	before string
	after  string
	ref    string
}

// parseReceiveBody turns the hook's stdin into ref updates, refusing the whole
// body on the first line that is not a well-formed update. The ref is the only
// caller-controlled string that leaves this package as a name rather than data,
// so it is validated here: a leading '-' would read as an option in any git
// command built from it, and a '..' segment would escape a filesystem path.
func parseReceiveBody(body string) ([]refUpdate, error) {
	var updates []refUpdate
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		before, after, ref, ok := parseRefUpdate(line)
		if !ok {
			return nil, fmt.Errorf("git-push hook sent an unparsable ref line: %q", truncateLine(line))
		}
		if !validGitRef(ref) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidRef, truncateLine(ref))
		}
		updates = append(updates, refUpdate{before: before, after: after, ref: ref})
	}
	if len(updates) == 0 {
		return nil, errors.New("git-push hook body contained no ref updates")
	}
	if len(updates) > maxRefUpdates {
		return nil, fmt.Errorf("git-push hook body contained more than %d ref updates", maxRefUpdates)
	}
	return updates, nil
}

// truncateLine keeps caller-controlled text out of error strings at unbounded
// length; the body is already size-capped but an error is echoed to the client.
func truncateLine(s string) string {
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}

func (s *Service) failEvent(ctx context.Context, eventID string, msg string) {
	s.logger.Warn("git-push deploy failed", "event", eventID, "reason", msg)
	if err := s.db.UpdatePushEventStatus(ctx, eventID, EventFailed, msg); err != nil {
		s.logger.Error("git-push event update failed", "event", eventID, "error", err)
	}
}

// ------------------------------------------------------------------- helpers ---

// verifySignature accepts either "sha256=<hex>" (GitHub's X-Hub-Signature-256
// and Dokku's own plugins) or "sha256:<hex>", and compares in constant time so
// a wrong signature cannot be found one byte at a time. An empty secret or an
// empty signature never verifies.
func verifySignature(secret, signature string, body []byte) bool {
	signature = strings.TrimSpace(signature)
	if signature == "" || secret == "" {
		return false
	}
	if hexPart, ok := stripSignaturePrefix(signature); ok {
		signature = hexPart
	}
	provided, err := hex.DecodeString(signature)
	if err != nil || len(provided) == 0 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	// Equal lengths first: ConstantTimeCompare returns 0 for differing lengths
	// anyway, but the explicit check keeps the intent readable.
	if len(provided) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare(expected, provided) == 1
}

// stripSignaturePrefix removes the algorithm label from a keyed-digest header.
// Both '=' and ':' separators are in the wild; the hex payload is what hashes.
func stripSignaturePrefix(signature string) (string, bool) {
	for _, label := range []string{"sha256=", "sha256:"} {
		if len(signature) > len(label) && strings.EqualFold(signature[:len(label)], label) {
			return strings.TrimSpace(signature[len(label):]), true
		}
	}
	return "", false
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

// validGitRef accepts a namespaced ref ("refs/heads/main", "refs/tags/v1") and
// nothing that could be mistaken for an option or a path escape. Every segment
// must start alphanumeric, which rules out a leading '-' on the ref, a leading
// '-' on any segment, and a ".." or "." segment.
func validGitRef(ref string) bool {
	if len(ref) == 0 || len(ref) > 255 {
		return false
	}
	if strings.HasSuffix(ref, ".lock") || strings.Contains(ref, "//") || strings.HasSuffix(ref, "/") {
		return false
	}
	if !refPattern.MatchString(ref) {
		return false
	}
	for _, segment := range strings.Split(ref, "/") {
		if segment == ".." || segment == "." {
			return false
		}
	}
	return true
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

// isZeroOID reports the "this ref was deleted" marker. Git writes one for every
// object format, so a SHA-256 repository's 64-zero deletion must not be read as
// a commit to deploy.
func isZeroOID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	return strings.Trim(s, "0") == ""
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

// sanitizeBranch normalises the deploy branch and validates it as a ref name.
// The branch is compared against hook refs and handed to the deployer as a
// git argument, so a value like "-D" or "../../etc" must never be stored.
func sanitizeBranch(branch string) (string, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "main", nil
	}
	branch = strings.TrimPrefix(branch, "refs/heads/")
	if branch == "" || !validGitRef("refs/heads/" + branch) {
		return "", fmt.Errorf("%w: %q", ErrInvalidBranch, truncateLine(branch))
	}
	return branch, nil
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
	if len(slug) > maxSlugBaseLen {
		slug = strings.Trim(slug[:maxSlugBaseLen], "-")
	}
	return slug
}

// maxSlugBaseLen leaves room for the collision suffix inside slugPattern's
// 48-character limit.
const maxSlugBaseLen = 40

// uniqueSlug keeps a generated suffix on collisions: two admins naming an app
// "web" must not end up fighting over /srv/git-push/web.git.
func (s *Service) uniqueSlug(ctx context.Context, name string) (string, error) {
	base := slugify(name)
	if base == "" {
		return "", ErrInvalidName
	}
	taken := map[string]bool{}
	apps, err := s.db.ListGitPushApps(ctx)
	if err != nil {
		return "", err
	}
	for _, a := range apps {
		taken[a.Slug] = true
	}
	if !taken[base] {
		return base, nil
	}
	for i := 2; i < 1000; i++ {
		candidate := base + "-" + strconv.Itoa(i)
		if !taken[candidate] && slugPattern.MatchString(candidate) {
			return candidate, nil
		}
	}
	suffix, err := randomHex(3)
	if err != nil {
		return "", err
	}
	candidate := base + "-" + suffix
	if !slugPattern.MatchString(candidate) {
		return "", ErrInvalidName
	}
	return candidate, nil
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
