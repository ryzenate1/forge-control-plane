package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CatalogEntry is a single one-click service kind in the catalog table.
type CatalogEntry struct {
	ID             string   `json:"id"`
	Key            string   `json:"key"`
	DisplayName    string   `json:"displayName"`
	Description    string   `json:"description"`
	Category       string   `json:"category"`
	Versions       []string `json:"versions"`
	DefaultVersion string   `json:"defaultVersion"`
	Icon           string   `json:"icon"`
	Requires       []string `json:"requires"`
	Enabled        bool     `json:"enabled"`
	SortOrder      int      `json:"sortOrder"`
	CreatedAt      string   `json:"createdAt"`
	UpdatedAt      string   `json:"updatedAt"`
}

// CatalogInstance tracks a provisioned catalog service and the runtime row
// backing it (a db_container or a compose stack).
type CatalogInstance struct {
	ID            string `json:"id"`
	EntryKey      string `json:"entryKey"`
	Kind          string `json:"kind"`
	Version       string `json:"version"`
	EnvironmentID string `json:"environmentId,omitempty"`
	NodeID        string `json:"nodeId,omitempty"`
	RefType       string `json:"refType,omitempty"`
	InstanceRef   string `json:"instanceRef,omitempty"`
	Host          string `json:"host,omitempty"`
	Port          int    `json:"port"`
	ConnString    string `json:"connString,omitempty"`
	Status        string `json:"status"`
	ErrorMessage  string `json:"errorMessage,omitempty"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// CatalogAttachLink records that a catalog instance's connection variables
// were injected into a specific environment.
type CatalogAttachLink struct {
	ID                string `json:"id"`
	CatalogInstanceID string `json:"catalogInstanceId"`
	EnvironmentID     string `json:"environmentId"`
	VarPrefix         string `json:"varPrefix"`
	CreatedAt         string `json:"createdAt"`
}

// BackupRetention is a per-kind backup retention policy consumed by the
// hourly catalog retention worker.
type BackupRetention struct {
	Kind          string `json:"kind"`
	Enabled       bool   `json:"enabled"`
	RetentionDays int    `json:"retentionDays"`
	RetentionMax  int    `json:"retentionMax"`
	UpdatedAt     string `json:"updatedAt"`
}

const catalogEntryColumns = `id::text, key, display_name, description, category, versions, default_version,
	icon, requires, enabled, sort_order, created_at, updated_at`

func scanCatalogRow(row interface{ Scan(...any) error }) (CatalogEntry, error) {
	var e CatalogEntry
	var versionsRaw, requiresRaw []byte
	var createdAt, updatedAt time.Time
	if err := row.Scan(&e.ID, &e.Key, &e.DisplayName, &e.Description, &e.Category, &versionsRaw, &e.DefaultVersion, &e.Icon, &requiresRaw, &e.Enabled, &e.SortOrder, &createdAt, &updatedAt); err != nil {
		return e, err
	}
	_ = json.Unmarshal(versionsRaw, &e.Versions)
	_ = json.Unmarshal(requiresRaw, &e.Requires)
	e.CreatedAt = createdAt.Format(time.RFC3339)
	e.UpdatedAt = updatedAt.Format(time.RFC3339)
	return e, nil
}

// ListCatalogEntries returns catalog entries, optionally filtered to enabled
// rows only, ordered by sort_order then name.
func (s *Store) ListCatalogEntries(ctx context.Context, enabledOnly bool) ([]CatalogEntry, error) {
	query := `SELECT ` + catalogSelect
	if enabledOnly {
		query += ` WHERE enabled = TRUE`
	}
	query += ` ORDER BY sort_order ASC, display_name ASC`
	rows, err := s.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []CatalogEntry{}
	for rows.Next() {
		e, scanErr := scanCatalogRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetCatalogEntry returns one catalog entry by its key.
func (s *Store) GetCatalogEntry(ctx context.Context, key string) (*CatalogEntry, error) {
	row := s.db.QueryRow(ctx, `SELECT `+catalogSelect+` WHERE key = $1`, strings.TrimSpace(key))
	e, err := scanCatalogRow(row)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// CatalogInstanceRef identifies the backing runtime row for a catalog
// instance.
type CatalogInstanceRef struct {
	RefType     string
	InstanceRef string
	Host        string
	Port        int
	ConnString  string
	Status      string
	Error       string
}

// CreateCatalogInstance inserts a new catalog instance row.
func (s *Store) CreateCatalogInstance(ctx context.Context, inst CatalogInstance) (CatalogInstance, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	var ref *uuid.UUID
	if inst.InstanceRef != "" {
		parsed, parseErr := uuid.Parse(inst.InstanceRef)
		if parseErr == nil {
			ref = &parsed
		}
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO catalog_instances
		    (id, entry_key, kind, version, environment_id, node_id, ref_type, instance_ref, host, port, conn_string, status, error_message, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), $7, $8, $9, $10, $11, $12, $13, $14, $14)
	`, id, inst.EntryKey, inst.Kind, inst.Version, inst.EnvironmentID, inst.NodeID, inst.RefType, ref, inst.Host, inst.Port, inst.ConnString, inst.Status, inst.ErrorMessage, now)
	if err != nil {
		return CatalogInstance{}, err
	}
	inst.ID = id
	inst.CreatedAt = now.Format(time.RFC3339)
	inst.UpdatedAt = now.Format(time.RFC3339)
	return inst, nil
}

// GetCatalogInstance returns one catalog instance row.
func (s *Store) GetCatalogInstance(ctx context.Context, id string) (*CatalogInstance, error) {
	var inst CatalogInstance
	var ref *uuid.UUID
	var createdAt, updatedAt time.Time
	err := s.db.QueryRow(ctx, `
		SELECT id::text, entry_key, kind, version, environment_id::text, node_id::text, ref_type, instance_ref, host, port, conn_string, status, error_message, created_at, updated_at
		FROM catalog_instances WHERE id = $1
	`, id).Scan(&inst.ID, &inst.EntryKey, &inst.Kind, &inst.Version, &inst.EnvironmentID, &inst.NodeID, &inst.RefType, &ref, &inst.Host, &inst.Port, &inst.ConnString, &inst.Status, &inst.ErrorMessage, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if ref != nil {
		inst.InstanceRef = ref.String()
	}
	inst.CreatedAt = createdAt.Format(time.RFC3339)
	inst.UpdatedAt = updatedAt.Format(time.RFC3339)
	return &inst, nil
}

// ListCatalogInstances returns catalog instances for one entry key, optionally
// filtered to a specific environment.
func (s *Store) ListCatalogInstances(ctx context.Context, entryKey, envID string) ([]CatalogInstance, error) {
	query := `
		SELECT id::text, entry_key, kind, version, environment_id::text, node_id::text, ref_type, instance_ref::text, host, port, conn_string, status, error_message, created_at, updated_at
		FROM catalog_instances WHERE entry_key = $1`
	args := []any{strings.TrimSpace(entryKey)}
	if strings.TrimSpace(envID) != "" {
		query += ` AND environment_id = $2`
		args = append(args, envID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	instances := []CatalogInstance{}
	for rows.Next() {
		var inst CatalogInstance
		if err := rows.Scan(&inst.ID, &inst.EntryKey, &inst.Kind, &inst.Version, &inst.EnvironmentID, &inst.NodeID, &inst.RefType, &inst.InstanceRef, &inst.Host, &inst.Port, &inst.ConnString, &inst.Status, &inst.ErrorMessage, &inst.CreatedAt, &inst.UpdatedAt); err != nil {
			return nil, err
		}
		instances = append(instances, inst)
	}
	return instances, rows.Err()
}

// UpdateCatalogInstanceStatus persists the provisioning outcome of a catalog
// instance and its derived connection string.
func (s *Store) UpdateCatalogInstanceStatus(ctx context.Context, id string, ref CatalogInstanceRef) error {
	var instanceRef *uuid.UUID
	if ref.InstanceRef != "" {
		if parsed, parseErr := uuid.Parse(ref.InstanceRef); parseErr == nil {
			instanceRef = &parsed
		}
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE catalog_instances
		SET ref_type = $2, instance_ref = $3, host = $4, port = $5, conn_string = $6, status = $7, error_message = $8, updated_at = NOW()
		WHERE id = $1
	`, id, ref.RefType, instanceRef, ref.Host, ref.Port, ref.ConnString, ref.Status, ref.Error)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("catalog instance not found")
	}
	return nil
}

// CreateCatalogAttachLink records that an instance's vars were injected into
// an environment. Idempotent per (instance, environment).
func (s *Store) CreateCatalogAttachLink(ctx context.Context, instanceID, envID, varPrefix string) (CatalogAttachLink, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO catalog_attach_links (id, catalog_instance_id, environment_id, var_prefix)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (catalog_instance_id, environment_id) DO UPDATE SET var_prefix = EXCLUDED.var_prefix
	`, id, instanceID, envID, varPrefix)
	if err != nil {
		return CatalogAttachLink{}, err
	}
	return CatalogAttachLink{ID: id, CatalogInstanceID: instanceID, EnvironmentID: envID, VarPrefix: varPrefix}, nil
}

// ListCatalogAttachLinks returns the environments a catalog instance has been
// attached to.
func (s *Store) ListCatalogAttachLinks(ctx context.Context, instanceID string) ([]CatalogAttachLink, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, catalog_instance_id::text, environment_id::text, var_prefix, created_at
		FROM catalog_attach_links WHERE catalog_instance_id = $1 ORDER BY created_at DESC
	`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	links := []CatalogAttachLink{}
	for rows.Next() {
		var l CatalogAttachLink
		var createdAt time.Time
		if err := rows.Scan(&l.ID, &l.CatalogInstanceID, &l.EnvironmentID, &l.VarPrefix, &createdAt); err != nil {
			return nil, err
		}
		l.CreatedAt = createdAt.Format(time.RFC3339)
		links = append(links, l)
	}
	return links, rows.Err()
}

// GetRetentionPolicy returns one backup retention policy.
func (s *Store) GetRetentionPolicy(ctx context.Context, kind string) (*BackupRetention, error) {
	var r BackupRetention
	var updatedAt time.Time
	err := s.db.QueryRow(ctx, `
		SELECT kind, enabled, retention_days, retention_max, updated_at
		FROM backup_retention WHERE kind = $1
	`, strings.ToLower(strings.TrimSpace(kind))).Scan(&r.Kind, &r.Enabled, &r.RetentionDays, &r.RetentionMax, &updatedAt)
	if err != nil {
		return nil, err
	}
	r.UpdatedAt = updatedAt.Format(time.RFC3339)
	return &r, nil
}

// ListRetentionPolicies returns every backup retention policy.
func (s *Store) ListRetentionPolicies(ctx context.Context) ([]BackupRetention, error) {
	rows, err := s.db.Query(ctx, `SELECT kind, enabled, retention_days, retention_max, updated_at FROM backup_retention ORDER BY kind ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	policies := []BackupRetention{}
	for rows.Next() {
		var r BackupRetention
		var updatedAt time.Time
		if err := rows.Scan(&r.Kind, &r.Enabled, &r.RetentionDays, &r.RetentionMax, &updatedAt); err != nil {
			return nil, err
		}
		r.UpdatedAt = updatedAt.Format(time.RFC3339)
		policies = append(policies, r)
	}
	return policies, rows.Err()
}

// SetRetentionPolicy upserts a backup retention policy for a kind.
func (s *Store) SetRetentionPolicy(ctx context.Context, kind string, retentionDays, retentionMax int, enabled bool) (BackupRetention, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		return BackupRetention{}, errors.New("kind is required")
	}
	if retentionDays <= 0 {
		retentionDays = 30
	}
	if retentionMax <= 0 {
		retentionMax = 8
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO backup_retention (kind, enabled, retention_days, retention_max, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (kind) DO UPDATE SET enabled = EXCLUDED.enabled, retention_days = EXCLUDED.retention_days, retention_max = EXCLUDED.retention_max, updated_at = NOW()
	`, kind, enabled, retentionDays, retentionMax); err != nil {
		return BackupRetention{}, err
	}
	r, err := s.GetRetentionPolicy(ctx, kind)
	if err != nil {
		return BackupRetention{}, err
	}
	return *r, nil
}

// DeleteExpiredManagedBackups deletes completed managed database backups whose
// engine matches kind, that are older than the cutoff, excluding the newest
// retentionMax backups per managed database. Returns the number of rows
// deleted.
func (s *Store) DeleteExpiredManagedBackups(ctx context.Context, kind string, until time.Time, retentionMax int) (int64, error) {
	if retentionMax <= 0 {
		retentionMax = 1
	}
	tag, err := s.db.Exec(ctx, `
		WITH candidate AS (
		    SELECT b.id
		    FROM managed_database_backups b
		    JOIN managed_databases m ON m.id = b.managed_database_id
		    WHERE LOWER(b.engine) = LOWER($1)
		      AND b.status = 'completed'
		      AND b.created_at <= $2
		      AND b.id NOT IN (
		          SELECT keep.id FROM (
		              SELECT id, ROW_NUMBER() OVER (PARTITION BY managed_database_id ORDER BY created_at DESC) AS rn
		              FROM managed_database_backups
		              WHERE LOWER(engine) = LOWER($1) AND status = 'completed'
		          ) keep
		          WHERE keep.rn <= $3
		      )
		)
		DELETE FROM managed_database_backups WHERE id IN (SELECT id FROM candidate)
	`, kind, until, retentionMax)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

var catalogSelect = `id::text, key, display_name, description, category, versions, default_version,
	icon, requires, enabled, sort_order, created_at, updated_at FROM catalog_entries`
