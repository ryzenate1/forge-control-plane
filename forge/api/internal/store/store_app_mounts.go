package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gamepanel/forge/internal/services/mounts"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// This file implements mounts.Repository on *Store. It is distinct from
// store_mounts_ext.go, which persists the *admin*-scoped `mounts` catalogue
// (node/egg/server eligible paths). app_mounts are DECLARATIVE, per-application
// persistent storage that the deploy pipeline injects into a compose document.

func scanAppMount(row interface{ Scan(dest ...any) error }) (mounts.AppMount, error) {
	var m mounts.AppMount
	if err := row.Scan(&m.ID, &m.ApplicationID, &m.Name, &m.Type, &m.Source, &m.Target, &m.ReadOnly, &m.Content, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return mounts.AppMount{}, err
	}
	return m, nil
}

const appMountColumns = `id::text, application_id::text, name, type, source, target, read_only, content, created_at, updated_at`

func (s *Store) CreateAppMount(ctx context.Context, appID string, req mounts.CreateRequest) (mounts.AppMount, error) {
	id := uuid.NewString()
	source := strings.TrimSpace(req.Source)
	target := strings.TrimSpace(req.Target)
	content := req.Content
	if req.Type == string(mounts.MountTmpfs) {
		source = ""
		content = nil
	}
	if req.Type != string(mounts.MountSeedFile) {
		content = nil
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO app_mounts (id, application_id, name, type, source, target, read_only, content)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, id, appID, strings.TrimSpace(req.Name), req.Type, source, target, req.ReadOnly, content); err != nil {
		if isTagUniqueViolation(err) {
			return mounts.AppMount{}, fmt.Errorf("another mount already targets %s for this application", target)
		}
		return mounts.AppMount{}, err
	}
	return s.GetAppMount(ctx, id)
}

func (s *Store) GetAppMount(ctx context.Context, mountID string) (mounts.AppMount, error) {
	row := s.db.QueryRow(ctx, `SELECT `+appMountColumns+` FROM app_mounts WHERE id = $1`, mountID)
	m, err := scanAppMount(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return mounts.AppMount{}, mounts.ErrNotFound
	}
	return m, err
}

func (s *Store) ListAppMounts(ctx context.Context, appID string) ([]mounts.AppMount, error) {
	rows, err := s.db.Query(ctx, `SELECT `+appMountColumns+` FROM app_mounts WHERE application_id = $1 ORDER BY name`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []mounts.AppMount{}
	for rows.Next() {
		m, err := scanAppMount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) UpdateAppMount(ctx context.Context, mountID string, req mounts.UpdateRequest) (mounts.AppMount, error) {
	existing, err := s.GetAppMount(ctx, mountID)
	if err != nil {
		return mounts.AppMount{}, err
	}
	setClauses := []string{}
	args := []any{}
	argIdx := 1
	if req.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, strings.TrimSpace(*req.Name))
		argIdx++
	}
	if req.Type != nil {
		setClauses = append(setClauses, fmt.Sprintf("type = $%d", argIdx))
		args = append(args, strings.ToLower(strings.TrimSpace(*req.Type)))
		argIdx++
	}
	if req.Source != nil {
		setClauses = append(setClauses, fmt.Sprintf("source = $%d", argIdx))
		args = append(args, strings.TrimSpace(*req.Source))
		argIdx++
	}
	if req.Target != nil {
		setClauses = append(setClauses, fmt.Sprintf("target = $%d", argIdx))
		args = append(args, strings.TrimSpace(*req.Target))
		argIdx++
	}
	if req.ReadOnly != nil {
		setClauses = append(setClauses, fmt.Sprintf("read_only = $%d", argIdx))
		args = append(args, *req.ReadOnly)
		argIdx++
	}
	if req.Content != nil {
		setClauses = append(setClauses, fmt.Sprintf("content = $%d", argIdx))
		args = append(args, *req.Content)
		argIdx++
	}
	if len(setClauses) == 0 {
		return existing, nil
	}
	// Type transitions must not leave stale state behind (e.g. content on a
	// non-seed mount), so normalise exactly like the insert path does.
	effectiveType := existing.Type
	if req.Type != nil {
		effectiveType = strings.ToLower(strings.TrimSpace(*req.Type))
	}
	switch effectiveType {
	case string(mounts.MountTmpfs):
		setClauses = append(setClauses, "source = ''", "content = NULL")
		if req.Source != nil || req.Content != nil {
			return mounts.AppMount{}, errors.New("tmpfs mounts have no source or content")
		}
	case string(mounts.MountVolume):
		if req.Content != nil {
			return mounts.AppMount{}, errors.New("only seed-file mounts can store content")
		}
	case string(mounts.MountSeedFile):
		if req.Content == nil && existing.Content == nil {
			return mounts.AppMount{}, mounts.ErrSeedContent
		}
	default:
		if req.Content != nil {
			return mounts.AppMount{}, fmt.Errorf("only seed-file mounts can store content")
		}
	}
	args = append(args, mountID)
	tag, err := s.db.Exec(ctx, fmt.Sprintf("UPDATE app_mounts SET %s, updated_at = now() WHERE id = $%d", strings.Join(setClauses, ", "), argIdx), args...)
	if err != nil {
		if isTagUniqueViolation(err) {
			return mounts.AppMount{}, errors.New("another mount already targets that path for this application")
		}
		return mounts.AppMount{}, err
	}
	if tag.RowsAffected() == 0 {
		return mounts.AppMount{}, mounts.ErrNotFound
	}
	return s.GetAppMount(ctx, mountID)
}

func (s *Store) DeleteAppMount(ctx context.Context, mountID string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM app_mounts WHERE id = $1`, mountID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return mounts.ErrNotFound
	}
	return nil
}

// AppOrgID resolves the organization that owns an application (empty when the
// application does not exist), so mount routes can enforce org membership.
func (s *Store) AppOrgID(ctx context.Context, appID string) (string, error) {
	var orgID string
	err := s.db.QueryRow(ctx, `SELECT COALESCE(org_id::text, '') FROM applications WHERE id = $1`, appID).Scan(&orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return orgID, err
}

// UserInOrg reports organization membership for mount authorization.
func (s *Store) UserInOrg(ctx context.Context, orgID, userID string) (bool, error) {
	return s.UserIsOrgMember(ctx, orgID, userID)
}

var _ mounts.Repository = (*Store)(nil)
