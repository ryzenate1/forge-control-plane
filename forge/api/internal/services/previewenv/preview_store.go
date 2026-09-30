package previewenv

// Project-scoped preview persistence.
//
// The legacy previewenv surface (service.go) talks to preview_deployments
// through dedicated *store.Store methods. preview_environments is a different
// model (project-scoped, branch-addressable, compose-backed) and lives beside
// it, so its SQL is owned here. Persistence is expressed as an interface so
// the service can be unit-tested with a fake and so the package keeps working
// when the pool is unavailable (the registrar then skips the routes).

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PreviewEnvStore is the storage contract the preview lifecycle needs.
type PreviewEnvStore interface {
	CreatePreviewEnv(ctx context.Context, p *PreviewDeployment) error
	UpdatePreviewEnv(ctx context.Context, p *PreviewDeployment) error
	GetPreviewEnv(ctx context.Context, id string) (*PreviewDeployment, error)
	ListPreviewEnvsByProject(ctx context.Context, projectID string) ([]PreviewDeployment, error)
	FindLivePreviewEnvByBranch(ctx context.Context, projectID, branch string) (*PreviewDeployment, error)
	FindLivePreviewEnvByPR(ctx context.Context, projectID string, prNumber int) (*PreviewDeployment, error)
	ListExpiredPreviewEnvs(ctx context.Context, now time.Time) ([]PreviewDeployment, error)
	DeletePreviewEnv(ctx context.Context, id string) error
	CountLivePreviewEnvs(ctx context.Context, projectID string) (int, error)

	GetPreviewProject(ctx context.Context, projectID string) (PreviewProject, error)
	SetProjectPreviewSecret(ctx context.Context, projectID, secret string) error
	ResolveComposeSource(ctx context.Context, projectID, baseStackID string) (PreviewComposeSource, error)
	NodeIDForServer(ctx context.Context, serverID string) (string, error)
}

// PreviewProject is the tenancy context a preview needs from its project.
type PreviewProject struct {
	ID            string
	OrgID         string
	Name          string
	Slug          string
	WebhookSecret string
}

// PreviewComposeSource is the compose document a preview is cloned from plus
// the placement hints (node, owning user, environment) that made the original
// stack deployable.
type PreviewComposeSource struct {
	StackID       string
	ComposeYAML   string
	NodeID        string
	UserID        string
	EnvironmentID string
}

// previewColumns is the single read projection for preview_environments; keep
// it in sync with scanPreviewEnv.
const previewColumns = `id::text, project_id::text, COALESCE(environment_id::text, ''), COALESCE(server_id::text, ''),
	branch, COALESCE(pr_number, 0), COALESCE(commit_sha, ''), slug, status, COALESCE(url, ''),
	COALESCE(compose_content, ''), COALESCE(stack_id, ''), COALESCE(source, ''), COALESCE(title, ''),
	COALESCE(pr_url, ''), COALESCE(close_reason, ''), COALESCE(error, ''),
	expires_at, created_at, updated_at, COALESCE(created_by::text, '')`

// livePreviewStatuses are the statuses that still occupy a branch slot and a
// concurrency permit.
const livePreviewStatuses = "'pending', 'deploying', 'active'"

// pgPreviewStore is the Postgres implementation of PreviewEnvStore.
type pgPreviewStore struct {
	pool *pgxpool.Pool
}

// newPreviewEnvStore builds the pool-backed store. store.Store.GetDB() is the
// sanctioned escape hatch for surfaces that own their own schema; it returns
// nil when the API is not running against Postgres, which callers treat as
// "previews are unavailable", never as "previews are empty".
func newPreviewEnvStore(s *store.Store) PreviewEnvStore {
	if s == nil {
		return nil
	}
	pool := s.GetDB()
	if pool == nil {
		return nil
	}
	return &pgPreviewStore{pool: pool}
}

func (st *pgPreviewStore) CreatePreviewEnv(ctx context.Context, p *PreviewDeployment) error {
	_, err := st.pool.Exec(ctx, `
		INSERT INTO preview_environments (
			id, project_id, environment_id, server_id, branch, pr_number, commit_sha, slug,
			status, url, compose_content, stack_id, source, title, pr_url, close_reason, error,
			expires_at, created_at, updated_at, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14, $15, $16, $17,
			$18, $19, $20, $21
		)`,
		p.ID, p.ProjectID, nullString(p.EnvironmentID), nullString(p.ServerID), p.Branch, p.PRNumber, p.CommitSHA, p.Slug,
		p.Status, p.URL, p.ComposeContent, p.StackID, p.Source, p.Title, p.PRURL, p.CloseReason, p.Error,
		p.ExpiresAt, p.CreatedAt, p.UpdatedAt, nullString(p.CreatedBy),
	)
	return err
}

func (st *pgPreviewStore) UpdatePreviewEnv(ctx context.Context, p *PreviewDeployment) error {
	_, err := st.pool.Exec(ctx, `
		UPDATE preview_environments SET
			environment_id = $2, server_id = $3, branch = $4, pr_number = $5, commit_sha = $6,
			status = $7, url = $8, compose_content = $9, stack_id = $10, title = $11, pr_url = $12,
			close_reason = $13, error = $14, expires_at = $15, updated_at = $16
		WHERE id = $1`,
		p.ID, nullString(p.EnvironmentID), nullString(p.ServerID), p.Branch, p.PRNumber, p.CommitSHA,
		p.Status, p.URL, p.ComposeContent, p.StackID, p.Title, p.PRURL,
		p.CloseReason, p.Error, p.ExpiresAt, time.Now().UTC(),
	)
	return err
}

func (st *pgPreviewStore) GetPreviewEnv(ctx context.Context, id string) (*PreviewDeployment, error) {
	return st.scanPreviewRow(st.pool.QueryRow(ctx, `SELECT `+previewColumns+` FROM preview_environments WHERE id = $1`, id))
}

func (st *pgPreviewStore) ListPreviewEnvsByProject(ctx context.Context, projectID string) ([]PreviewDeployment, error) {
	rows, err := st.pool.Query(ctx, `SELECT `+previewColumns+`
		FROM preview_environments WHERE project_id = $1 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectPreviews(rows)
}

func (st *pgPreviewStore) FindLivePreviewEnvByBranch(ctx context.Context, projectID, branch string) (*PreviewDeployment, error) {
	return st.scanPreviewRow(st.pool.QueryRow(ctx, `SELECT `+previewColumns+`
		FROM preview_environments
		WHERE project_id = $1 AND lower(branch) = lower($2) AND status IN (`+livePreviewStatuses+`)
		ORDER BY created_at DESC LIMIT 1`, projectID, branch))
}

func (st *pgPreviewStore) FindLivePreviewEnvByPR(ctx context.Context, projectID string, prNumber int) (*PreviewDeployment, error) {
	return st.scanPreviewRow(st.pool.QueryRow(ctx, `SELECT `+previewColumns+`
		FROM preview_environments
		WHERE project_id = $1 AND pr_number = $2 AND status IN (`+livePreviewStatuses+`)
		ORDER BY created_at DESC LIMIT 1`, projectID, prNumber))
}

func (st *pgPreviewStore) ListExpiredPreviewEnvs(ctx context.Context, now time.Time) ([]PreviewDeployment, error) {
	rows, err := st.pool.Query(ctx, `SELECT `+previewColumns+`
		FROM preview_environments
		WHERE status IN (`+livePreviewStatuses+`) AND expires_at IS NOT NULL AND expires_at < $1
		ORDER BY expires_at ASC`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectPreviews(rows)
}

func (st *pgPreviewStore) DeletePreviewEnv(ctx context.Context, id string) error {
	tag, err := st.pool.Exec(ctx, `DELETE FROM preview_environments WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrPreviewNotFound
	}
	return nil
}

func (st *pgPreviewStore) CountLivePreviewEnvs(ctx context.Context, projectID string) (int, error) {
	var count int
	err := st.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM preview_environments
		WHERE project_id = $1 AND status IN (`+livePreviewStatuses+`)`, projectID).Scan(&count)
	return count, err
}

func (st *pgPreviewStore) GetPreviewProject(ctx context.Context, projectID string) (PreviewProject, error) {
	var (
		proj   PreviewProject
		secret *string
	)
	err := st.pool.QueryRow(ctx, `
		SELECT id::text, org_id::text, name, slug, preview_webhook_secret
		FROM projects WHERE id = $1`, projectID).Scan(&proj.ID, &proj.OrgID, &proj.Name, &proj.Slug, &secret)
	if errors.Is(err, pgx.ErrNoRows) {
		return PreviewProject{}, ErrProjectNotFound
	}
	if err != nil {
		return PreviewProject{}, err
	}
	if secret != nil {
		proj.WebhookSecret = *secret
	}
	return proj, nil
}

func (st *pgPreviewStore) SetProjectPreviewSecret(ctx context.Context, projectID, secret string) error {
	tag, err := st.pool.Exec(ctx,
		`UPDATE projects SET preview_webhook_secret = $2 WHERE id = $1`, projectID, secret)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrProjectNotFound
	}
	return nil
}

// ResolveComposeSource finds the compose document a preview should run.
//
// When baseStackID is set the caller named the stack explicitly and that is
// the answer (or an error). Without one, the only safe resolution is a
// project with exactly one live stack: zero means there is nothing to preview,
// more than one means the caller has to choose. Guessing here would deploy an
// arbitrary workload under a preview hostname, so both cases are errors.
func (st *pgPreviewStore) ResolveComposeSource(ctx context.Context, projectID, baseStackID string) (PreviewComposeSource, error) {
	query := func(where string, args ...any) (PreviewComposeSource, error) {
		var src PreviewComposeSource
		var envID *string
		err := st.pool.QueryRow(ctx, `
			SELECT id::text, COALESCE(compose_yaml, ''), node_id::text, user_id::text, environment_id
			FROM compose_stacks WHERE `+where, args...).
			Scan(&src.StackID, &src.ComposeYAML, &src.NodeID, &src.UserID, &envID)
		if errors.Is(err, pgx.ErrNoRows) {
			return src, ErrNoComposeSource
		}
		if err != nil {
			return src, err
		}
		if envID != nil {
			src.EnvironmentID = *envID
		}
		return src, nil
	}

	if baseStackID != "" {
		src, err := query(`id = $1 AND status <> 'deleted'`, baseStackID)
		if err != nil {
			return src, err
		}
		// A stack from another project must not be previewable: the caller can
		// only reach this method for a project they are a member of.
		if src.EnvironmentID != "" {
			owned, err := st.environmentBelongsToProject(ctx, src.EnvironmentID, projectID)
			if err != nil {
				return PreviewComposeSource{}, err
			}
			if !owned {
				return PreviewComposeSource{}, ErrComposeSourceOutsideProject
			}
		}
		return src, nil
	}

	rows, err := st.pool.Query(ctx, `
		SELECT cs.id::text, COALESCE(cs.compose_yaml, ''), cs.node_id::text, cs.user_id::text, cs.environment_id
		FROM compose_stacks cs
		WHERE cs.status <> 'deleted'
		  AND cs.environment_id IN (SELECT id::text FROM environments WHERE project_id = $1)
		LIMIT 2`, projectID)
	if err != nil {
		return PreviewComposeSource{}, err
	}
	defer rows.Close()

	var candidates []PreviewComposeSource
	for rows.Next() {
		var src PreviewComposeSource
		var envID *string
		if err := rows.Scan(&src.StackID, &src.ComposeYAML, &src.NodeID, &src.UserID, &envID); err != nil {
			return PreviewComposeSource{}, err
		}
		if envID != nil {
			src.EnvironmentID = *envID
		}
		candidates = append(candidates, src)
	}
	if err := rows.Err(); err != nil {
		return PreviewComposeSource{}, err
	}
	switch len(candidates) {
	case 0:
		return PreviewComposeSource{}, ErrNoComposeSource
	case 1:
		return candidates[0], nil
	default:
		return PreviewComposeSource{}, ErrAmbiguousComposeSource
	}
}

func (st *pgPreviewStore) environmentBelongsToProject(ctx context.Context, envID, projectID string) (bool, error) {
	var count int
	err := st.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM environments WHERE id = $1 AND project_id = $2`, envID, projectID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check environment ownership: %w", err)
	}
	return count > 0, nil
}

// NodeIDForServer maps a Forge server to the node that hosts it, so a caller
// that pinned a preview to a server gets it placed on that server's node.
func (st *pgPreviewStore) NodeIDForServer(ctx context.Context, serverID string) (string, error) {
	var nodeID string
	err := st.pool.QueryRow(ctx,
		`SELECT node_id::text FROM servers WHERE id = $1`, serverID).Scan(&nodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrServerNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolve node for server %s: %w", serverID, err)
	}
	return nodeID, nil
}

func (st *pgPreviewStore) scanPreviewRow(row interface{ Scan(dest ...any) error }) (*PreviewDeployment, error) {
	var p PreviewDeployment
	if err := scanPreviewEnv(row, &p); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPreviewNotFound
		}
		return nil, err
	}
	return &p, nil
}

func collectPreviews(rows pgx.Rows) ([]PreviewDeployment, error) {
	out := make([]PreviewDeployment, 0, 8)
	for rows.Next() {
		var p PreviewDeployment
		if err := scanPreviewEnv(rows, &p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func scanPreviewEnv(scanner interface{ Scan(dest ...any) error }, p *PreviewDeployment) error {
	var environmentID, serverID, createdBy *string
	var expiresAt *time.Time
	if err := scanner.Scan(
		&p.ID, &p.ProjectID, &environmentID, &serverID,
		&p.Branch, &p.PRNumber, &p.CommitSHA, &p.Slug, &p.Status, &p.URL,
		&p.ComposeContent, &p.StackID, &p.Source, &p.Title, &p.PRURL, &p.CloseReason, &p.Error,
		&expiresAt, &p.CreatedAt, &p.UpdatedAt, &createdBy,
	); err != nil {
		return err
	}
	p.EnvironmentID = derefString(environmentID)
	p.ServerID = derefString(serverID)
	p.CreatedBy = derefString(createdBy)
	p.ExpiresAt = expiresAt
	return nil
}

func nullString(v *string) any {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil
	}
	return *v
}

func derefString(v *string) *string {
	if v == nil || *v == "" {
		return nil
	}
	return v
}

// isUniqueViolation reports whether err is a Postgres (or SQLite-driven test
// double) unique/index violation, which the slug allocator and the per-branch
// partial index both surface as expected contention.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if uniqueConstraint(err) != "" {
		return true
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "duplicate key") ||
		strings.Contains(low, "unique constraint failed") ||
		strings.Contains(low, "preview_environments_slug_key") ||
		strings.Contains(low, "preview_environments_branch_unique")
}

// uniqueConstraint returns the violated constraint/index name, or "" when the
// driver did not report one. The slug allocator uses it to tell "someone else
// already owns this hostname" (retry with a suffix) from "this branch already
// has a live preview" (a real conflict the caller must see).
func uniqueConstraint(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pgErr.ConstraintName
	}
	return ""
}
