package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrGitPushAppNotFound is returned by the git-push getters when no row matches
// the requested id or slug. Handlers map it to HTTP 404.
var ErrGitPushAppNotFound = errors.New("git-push app not found")

// GitPushApp is the persistence shape for a Dokku-style "git push to deploy"
// application: a bare repository living on one Beacon node plus the shared
// secret its post-receive hook signs callbacks with. EnvVars holds a JSON
// object of key -> value; the gitpush service owns the encoding so the store
// stays free of the service's rich types.
type GitPushApp struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Slug         string     `json:"slug"`
	NodeID       string     `json:"nodeId"`
	ServerID     *string    `json:"serverId"`
	Builder      string     `json:"builder"`
	Branch       string     `json:"branch"`
	RepoPath     string     `json:"repoPath"`
	SharedSecret string     `json:"sharedSecret"`
	DeployedSHA  *string    `json:"deployedSha"`
	LastDeployAt *time.Time `json:"lastDeployAt"`
	Status       string     `json:"status"`
	AutoDeploy   bool       `json:"autoDeploy"`
	EnvVars      []byte     `json:"envVars"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

// GitPushEvent is one ref update reported by a repository's post-receive hook,
// together with how far the panel got acting on it.
type GitPushEvent struct {
	ID        string    `json:"id"`
	AppID     string    `json:"appId"`
	Ref       string    `json:"ref"`
	BeforeSHA string    `json:"beforeSha"`
	AfterSHA  string    `json:"afterSha"`
	Actor     string    `json:"actor"`
	Status    string    `json:"status"`
	Error     *string   `json:"error"`
	CreatedAt time.Time `json:"createdAt"`
}

const gitPushAppColumns = `id, name, slug, node_id, server_id, builder, branch, repo_path, shared_secret, deployed_sha, last_deploy_at, status, auto_deploy, env_vars, created_at, updated_at`

func scanGitPushApp(row interface{ Scan(dest ...any) error }) (*GitPushApp, error) {
	var a GitPushApp
	var envVars []byte
	if err := row.Scan(&a.ID, &a.Name, &a.Slug, &a.NodeID, &a.ServerID, &a.Builder, &a.Branch, &a.RepoPath,
		&a.SharedSecret, &a.DeployedSHA, &a.LastDeployAt, &a.Status, &a.AutoDeploy, &envVars, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}
	if len(envVars) == 0 {
		envVars = []byte(`{}`)
	}
	a.EnvVars = envVars
	return &a, nil
}

// maxGitPushApps bounds the admin listing. The table is small by design (one
// row per repository an operator has registered) but an unbounded read of it
// turns a compromised or slow database into an unbounded response.
const maxGitPushApps = 500

var errGitPushNoDB = errors.New("no database connection")

func (s *Store) CreateGitPushApp(ctx context.Context, a *GitPushApp) error {
	if s.db == nil {
		return errGitPushNoDB
	}
	if a.ID == "" {
		return errors.New("git-push app id is required")
	}
	if a.Slug == "" {
		return errors.New("git-push app slug is required")
	}
	if a.SharedSecret == "" {
		return errors.New("git-push app shared secret is required")
	}
	if a.Builder == "" {
		a.Builder = "dockerfile"
	}
	if a.Branch == "" {
		a.Branch = "main"
	}
	if a.Status == "" {
		a.Status = "provisioning"
	}
	env := a.EnvVars
	if len(env) == 0 {
		env = []byte(`{}`)
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = a.CreatedAt
	}
	// A plain INSERT, not an upsert: rewriting an existing row through the
	// create path would silently replace a live app's name, node and branch
	// while leaving its slug and secret untouched.
	_, err := s.db.Exec(ctx, `
		INSERT INTO git_push_apps (id, name, slug, node_id, server_id, builder, branch, repo_path, shared_secret, deployed_sha, last_deploy_at, status, auto_deploy, env_vars, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`, a.ID, a.Name, a.Slug, a.NodeID, a.ServerID, a.Builder, a.Branch, a.RepoPath, a.SharedSecret,
		a.DeployedSHA, a.LastDeployAt, a.Status, a.AutoDeploy, env, a.CreatedAt, a.UpdatedAt)
	return err
}

func (s *Store) GetGitPushApp(ctx context.Context, id string) (*GitPushApp, error) {
	if s.db == nil {
		return nil, errGitPushNoDB
	}
	row := s.db.QueryRow(ctx, `SELECT `+gitPushAppColumns+` FROM git_push_apps WHERE id = $1`, id)
	a, err := scanGitPushApp(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrGitPushAppNotFound
		}
		return nil, err
	}
	return a, nil
}

// GetGitPushAppBySlug resolves the hook's addressable identifier. Slugs are
// unique, so this is the receive endpoint's only lookup.
func (s *Store) GetGitPushAppBySlug(ctx context.Context, slug string) (*GitPushApp, error) {
	if s.db == nil {
		return nil, errGitPushNoDB
	}
	row := s.db.QueryRow(ctx, `SELECT `+gitPushAppColumns+` FROM git_push_apps WHERE slug = $1`, slug)
	a, err := scanGitPushApp(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrGitPushAppNotFound
		}
		return nil, err
	}
	return a, nil
}

func (s *Store) ListGitPushApps(ctx context.Context) ([]GitPushApp, error) {
	if s.db == nil {
		return nil, errGitPushNoDB
	}
	rows, err := s.db.Query(ctx, `SELECT `+gitPushAppColumns+` FROM git_push_apps ORDER BY created_at DESC LIMIT $1`, maxGitPushApps)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	apps := []GitPushApp{}
	for rows.Next() {
		a, err := scanGitPushApp(rows)
		if err != nil {
			return nil, err
		}
		apps = append(apps, *a)
	}
	return apps, rows.Err()
}

// UpdateGitPushApp writes the mutable fields of an existing row. It is not the
// create path: updating an app that no longer exists reports that instead of
// re-inserting a row nobody asked for.
func (s *Store) UpdateGitPushApp(ctx context.Context, a *GitPushApp) error {
	if s.db == nil {
		return errGitPushNoDB
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE git_push_apps
		SET name = $2, node_id = $3, server_id = $4, builder = $5, branch = $6,
		    repo_path = $7, status = $8, auto_deploy = $9, env_vars = $10, updated_at = now()
		WHERE id = $1
	`, a.ID, a.Name, a.NodeID, a.ServerID, a.Builder, a.Branch, a.RepoPath,
		a.Status, a.AutoDeploy, gitPushEnvJSON(a.EnvVars))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrGitPushAppNotFound
	}
	return nil
}

// gitPushEnvJSON keeps a NULL/empty JSONB column from being written back as an
// empty byte slice, which Postgres rejects.
func gitPushEnvJSON(env []byte) []byte {
	if len(env) == 0 {
		return []byte(`{}`)
	}
	return env
}

func (s *Store) DeleteGitPushApp(ctx context.Context, id string) error {
	if s.db == nil {
		return errGitPushNoDB
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM git_push_apps WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrGitPushAppNotFound
	}
	return nil
}

// SetGitPushAppStatus moves an app through its provisioning lifecycle without
// touching anything else - the beacon handshake needs a narrow, idempotent
// write so a concurrent env-var edit cannot be clobbered.
func (s *Store) SetGitPushAppStatus(ctx context.Context, id string, status string) error {
	if s.db == nil {
		return errGitPushNoDB
	}
	tag, err := s.db.Exec(ctx, `UPDATE git_push_apps SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrGitPushAppNotFound
	}
	return nil
}

func (s *Store) SetGitPushAppEnvVars(ctx context.Context, id string, envVars []byte) error {
	if s.db == nil {
		return errGitPushNoDB
	}
	tag, err := s.db.Exec(ctx, `UPDATE git_push_apps SET env_vars = $2::jsonb, updated_at = now() WHERE id = $1`, id, gitPushEnvJSON(envVars))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrGitPushAppNotFound
	}
	return nil
}

// RotateGitPushSecret replaces the HMAC key for an app's receive hook. The
// repository's post-receive hook must be re-issued afterwards; until it is,
// callbacks keep failing signature verification rather than deploying under a
// secret the caller no longer holds.
func (s *Store) RotateGitPushSecret(ctx context.Context, id string, secret string) error {
	if s.db == nil {
		return errGitPushNoDB
	}
	if secret == "" {
		return errors.New("git-push shared secret is required")
	}
	tag, err := s.db.Exec(ctx, `UPDATE git_push_apps SET shared_secret = $2, updated_at = now() WHERE id = $1`, id, secret)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrGitPushAppNotFound
	}
	return nil
}

// MarkGitPushAppDeployed records the SHA a deploy *completed* on. Nothing calls
// it yet: the git-push receive path only hands a push to the deployer, and a
// handoff is not a completed deploy. The deploy-completion callback is the
// caller this is waiting for; until it exists, deployed_sha stays NULL.
func (s *Store) MarkGitPushAppDeployed(ctx context.Context, id string, sha string) error {
	if s.db == nil {
		return errGitPushNoDB
	}
	if sha == "" {
		return errors.New("git-push deployed sha is required")
	}
	now := time.Now().UTC()
	tag, err := s.db.Exec(ctx, `
		UPDATE git_push_apps
		SET deployed_sha = $2, last_deploy_at = $3, status = 'ready', updated_at = $3
		WHERE id = $1
	`, id, sha, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrGitPushAppNotFound
	}
	return nil
}

func (s *Store) RecordGitPushEvent(ctx context.Context, e *GitPushEvent) error {
	if s.db == nil {
		return errGitPushNoDB
	}
	if e.ID == "" {
		return errors.New("git-push event id is required")
	}
	if e.Status == "" {
		e.Status = "received"
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO git_push_events (id, app_id, ref, before_sha, after_sha, actor, status, error, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, e.ID, e.AppID, e.Ref, e.BeforeSHA, e.AfterSHA, e.Actor, e.Status, e.Error, e.CreatedAt)
	return err
}

// UpdatePushEventStatus advances a recorded push. An empty errorMessage stores
// NULL so the events table only carries a message when there genuinely is one.
// Updating an event that is not there is an error: the receive path reports the
// state it wrote, and a no-op write would report a state that was never stored.
func (s *Store) UpdatePushEventStatus(ctx context.Context, eventID string, status string, errorMessage string) error {
	if s.db == nil {
		return errGitPushNoDB
	}
	var msg *string
	if errorMessage != "" {
		msg = &errorMessage
	}
	tag, err := s.db.Exec(ctx, `UPDATE git_push_events SET status = $2, error = $3 WHERE id = $1`, eventID, status, msg)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("git-push event not found")
	}
	return nil
}

func (s *Store) ListGitPushEvents(ctx context.Context, appID string, limit int) ([]GitPushEvent, error) {
	if s.db == nil {
		return nil, errGitPushNoDB
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, app_id, ref, before_sha, after_sha, actor, status, error, created_at
		FROM git_push_events
		WHERE app_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, appID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []GitPushEvent{}
	for rows.Next() {
		var e GitPushEvent
		if err := rows.Scan(&e.ID, &e.AppID, &e.Ref, &e.BeforeSHA, &e.AfterSHA, &e.Actor, &e.Status, &e.Error, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
