package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// EnvContext is the tenancy context (environment + project + org) resolved
// for a single environment id. Used by the environment manifest and domain
// provisioning engines so they can create applications and services under
// the correct org/project scopes.
type EnvContext struct {
	Environment Environment
	Project     Project
	Org         Organization
}

// ResolveEnvContext loads the environment, its project and the owning
// organization in one helper. An error is returned if any hop is missing.
func (s *Store) ResolveEnvContext(ctx context.Context, envID string) (EnvContext, error) {
	var env Environment
	err := s.db.QueryRow(ctx, `
		SELECT id::text, project_id::text, name, color, protected, created_at
		FROM environments WHERE id = $1
	`, envID).Scan(&env.ID, &env.ProjectID, &env.Name, &env.Color, &env.Protected, &env.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EnvContext{}, fmt.Errorf("environment %s not found", envID)
		}
		return EnvContext{}, fmt.Errorf("load environment: %w", err)
	}

	var project Project
	err = s.db.QueryRow(ctx, `
		SELECT id::text, org_id::text, name, slug, COALESCE(description, ''), created_at
		FROM projects WHERE id = $1
	`, env.ProjectID).Scan(&project.ID, &project.OrgID, &project.Name, &project.Slug, &project.Description, &project.CreatedAt)
	if err != nil {
		return EnvContext{}, fmt.Errorf("load project: %w", err)
	}

	var org Organization
	err = s.db.QueryRow(ctx, `
		SELECT o.id::text, o.name, o.slug, o.owner_id::text, COALESCE(u.email, ''), o.created_at
		FROM organizations o
		JOIN users u ON u.id = o.owner_id
		WHERE o.id = $1
	`, project.OrgID).Scan(&org.ID, &org.Name, &org.Slug, &org.OwnerID, &org.OwnerName, &org.CreatedAt)
	if err != nil {
		return EnvContext{}, fmt.Errorf("load organization: %w", err)
	}

	return EnvContext{Env: env, Project: project, Org: org}, nil
}

// ---- Environment manifests ----

// EnvManifestRow is the persisted environment manifest record. Manifest
// holds the normalized JSON document, ManifestYAML the raw applied text so
// renders round-trip faithfully.
type EnvManifestRow struct {
	EnvID        string
	Version      string
	Manifest     json.RawMessage
	ManifestYAML string
	AppliedBy    *string
	AppliedAt    time.Time
	UpdatedAt    time.Time
}

// GetEnvManifest loads the last applied manifest for an environment. It
// returns nil when no manifest has been applied yet.
func (s *Store) GetEnvManifest(ctx context.Context, envID string) (*EnvManifestRow, error) {
	var row EnvManifestRow
	var raw []byte
	err := s.db.QueryRow(ctx, `
		SELECT env_id::text, version, manifest::text, manifest_yaml, applied_by::text, applied_at, updated_at
		FROM env_manifests WHERE env_id = $1
	`, envID).Scan(&row.EnvID, &row.Version, &raw, &row.ManifestYAML, &row.AppliedBy, &row.AppliedAt, &row.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("load env manifest: %w", err)
	}
	row.Manifest = raw
	return &row, nil
}

// SaveEnvManifest upserts the applied manifest document for an environment.
func (s *Store) SaveEnvManifest(ctx context.Context, envID, version string, manifest json.RawMessage, manifestYAML string, actorID *string) error {
	if manifest == nil {
		manifest = json.RawMessage("{}")
	}
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO env_manifests (env_id, version, manifest, manifest_yaml, applied_by, applied_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6)
		ON CONFLICT (env_id) DO UPDATE SET
			version = EXCLUDED.version,
			manifest = EXCLUDED.manifest,
			manifest_yaml = EXCLUDED.manifest_yaml,
			applied_by = EXCLUDED.applied_by,
			applied_at = EXCLUDED.applied_at,
			updated_at = EXCLUDED.updated_at
	`, envID, version, string(manifest), manifestYAML, actorID, now)
	if err != nil {
		return fmt.Errorf("save env manifest: %w", err)
	}
	_ = s.AppendAudit(ctx, actorID, "env manifest applied", "environment", &envID, version)
	return nil
}

// ---- Env-var groups ----

// EnvVarGroup is a named bundle of KEY=VALUE entries scoped to one
// environment. The variables map is stored as JSON on env_var_groups.
type EnvVarGroup struct {
	ID            string            `json:"id"`
	EnvironmentID string            `json:"environmentId"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Variables     map[string]string `json:"variables"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
}

// ListEnvVarGroups returns all groups for an environment, ordered by name.
func (s *Store) ListEnvVarGroups(ctx context.Context, envID string) ([]EnvVarGroup, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, environment_id::text, name, COALESCE(description, ''), variables::text, created_at, updated_at
		FROM env_var_groups WHERE environment_id = $1 ORDER BY name ASC
	`, envID)
	if err != nil {
		return nil, fmt.Errorf("list env var groups: %w", err)
	}
	defer rows.Close()

	groups := []EnvVarGroup{}
	for rows.Next() {
		var g EnvVarGroup
		var raw string
		if err := rows.Scan(&g.ID, &g.EnvironmentID, &g.Name, &g.Description, &raw, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan env var group: %w", err)
		}
		if err := json.Unmarshal([]byte(raw), &g.Variables); err != nil || g.Variables == nil {
			g.Variables = map[string]string{}
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// GetEnvVarGroup returns a single group by environment id and name.
func (s *Store) GetEnvVarGroup(ctx context.Context, envID, name string) (EnvVarGroup, error) {
	var g EnvVarGroup
	var raw string
	err := s.db.QueryRow(ctx, `
		SELECT id::text, environment_id::text, name, COALESCE(description, ''), variables::text, created_at, updated_at
		FROM env_var_groups WHERE environment_id = $1 AND name = $2
	`, envID, name).Scan(&g.ID, &g.EnvironmentID, &g.Name, &g.Description, &raw, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EnvVarGroup{}, fmt.Errorf("group %q not found", name)
		}
		return EnvVarGroup{}, fmt.Errorf("get env var group: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &g.Variables); err != nil || g.Variables == nil {
		g.Variables = map[string]string{}
	}
	return g, nil
}

// UpsertEnvVarGroup creates or replaces a group (variables map is stored as
// JSON). Passing variables=nil clears the bundle.
func (s *Store) UpsertEnvVarGroup(ctx context.Context, envID, name, description string, variables map[string]string, actorID *string) (EnvVarGroup, error) {
	if variables == nil {
		variables = map[string]string{}
	}
	raw, err := json.Marshal(variables)
	if err != nil {
		return EnvVarGroup{}, fmt.Errorf("marshal group variables: %w", err)
	}
	if err := s.EnsureEnvironmentExists(ctx, envID); err != nil {
		return EnvVarGroup{}, err
	}
	now := time.Now().UTC()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO env_var_groups (id, environment_id, name, description, variables, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6)
		ON CONFLICT (environment_id, name) DO UPDATE SET
			description = EXCLUDED.description,
			variables = EXCLUDED.variables,
			updated_at = EXCLUDED.updated_at
	`, uuid.NewString(), envID, name, description, string(raw), now); err != nil {
		return EnvVarGroup{}, fmt.Errorf("upsert env var group: %w", err)
	}
	_ = s.AppendAudit(ctx, actorID, "env var group saved", "environment", &envID, `{"name":"`+name+`"}`)
	return s.GetEnvVarGroup(ctx, envID, name)
}

// DeleteEnvVarGroup removes a single group from an environment.
func (s *Store) DeleteEnvVarGroup(ctx context.Context, envID, name string, actorID *string) error {
	tag, err := s.db.Exec(ctx, `
		DELETE FROM env_var_groups WHERE environment_id = $1 AND name = $2
	`, envID, name)
	if err != nil {
		return fmt.Errorf("delete env var group: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("group %q not found", name)
	}
	_ = s.AppendAudit(ctx, actorID, "env var group deleted", "environment", &envID, `{"name":"`+name+`"}`)
	return nil
}

// EnsureEnvironmentExists guards group writes with a clear error for missing
// environments instead of a FK violation.
func (s *Store) EnsureEnvironmentExists(ctx context.Context, envID string) error {
	var id string
	err := s.db.QueryRow(ctx, `SELECT id::text FROM environments WHERE id = $1`, envID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("environment %s not found", envID)
		}
		return fmt.Errorf("check environment: %w", err)
	}
	return nil
}

// ---- Env domain provisioning ----

// EnvDomainProvisioning is the provisioning intent/status ledger row written
// by the domainsenv orchestrator.
type EnvDomainProvisioning struct {
	EnvID        string     `json:"envId"`
	Domain       string     `json:"domain"`
	WildcardHost string     `json:"wildcardHost"`
	Target       string     `json:"target"`
	DNSStatus    string     `json:"dnsStatus"`
	TLSStatus    string     `json:"tlsStatus"`
	CertID       *string    `json:"certId,omitempty"`
	LastError    string     `json:"lastError,omitempty"`
	AttemptedAt  *time.Time `json:"attemptedAt,omitempty"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

// GetEnvDomainProvisioning returns the ledger row for an environment. nil is
// returned when the environment has never been provisioned.
func (s *Store) GetEnvDomainProvisioning(ctx context.Context, envID string) (*EnvDomainProvisioning, error) {
	var p EnvDomainProvisioning
	err := s.db.QueryRow(ctx, `
		SELECT env_id::text, domain, wildcard_host, target, dns_status, tls_status, cert_id::text, last_error, attempted_at, updated_at
		FROM env_domain_provisioning WHERE env_id = $1
	`, envID).Scan(&p.EnvID, &p.Domain, &p.WildcardHost, &p.Target, &p.DNSStatus, &p.TLSStatus, &p.CertID, &p.LastError, &p.AttemptedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("load env provisioning: %w", err)
	}
	return &p, nil
}

// UpsertEnvDomainProvisioning records the latest provisioning state.
func (s *Store) UpsertEnvDomainProvisioning(ctx context.Context, p EnvDomainProvisioning) error {
	now := time.Now().UTC()
	p.UpdatedAt = now
	_, err := s.db.Exec(ctx, `
		INSERT INTO env_domain_provisioning (env_id, domain, wildcard_host, target, dns_status, tls_status, cert_id, last_error, attempted_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (env_id) DO UPDATE SET
			domain = EXCLUDED.domain,
			wildcard_host = EXCLUDED.wildcard_host,
			target = EXCLUDED.target,
			dns_status = EXCLUDED.dns_status,
			tls_status = EXCLUDED.tls_status,
			cert_id = EXCLUDED.cert_id,
			last_error = EXCLUDED.last_error,
			attempted_at = EXCLUDED.attempted_at,
			updated_at = EXCLUDED.updated_at
	`, p.EnvID, p.Domain, p.WildcardHost, p.Target, p.DNSStatus, p.TLSStatus, p.CertID, p.LastError, p.AttemptedAt, now)
	if err != nil {
		return fmt.Errorf("upsert env provisioning: %w", err)
	}
	return nil
}

// ListEnvironmentIDsForProvisioning returns every environment id that either
// has no provisioning row or whose provisioning is not fully "ok". The
// reconciler reconciles these.
func (s *Store) ListEnvironmentIDsForProvisioning(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT e.id::text
		FROM environments e
		LEFT JOIN env_domain_provisioning p ON p.env_id = e.id
		WHERE p.env_id IS NULL
		   OR p.dns_status <> 'ok'
		   OR p.tls_status <> 'ok'
		ORDER BY e.created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list environments for provisioning: %w", err)
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan environment id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ---- Apps / services under an environment ----

// ListApplicationsByEnvironment returns applications attached to an
// environment. Passing orgID filters by the application's org; pass "" to
// skip the filter.
func (s *Store) ListApplicationsByEnvironment(ctx context.Context, envID, orgID string) ([]Application, error) {
	query := `
		SELECT id::text, name, COALESCE(description, ''), org_id::text,
			project_id::text, environment_id::text, server_id::text,
			source_type, source_config::text,
			desired_state, observed_status, current_deployment_id::text,
			COALESCE(created_at, now()), COALESCE(updated_at, now())
		FROM applications
		WHERE environment_id = $1
	` + optionalOrgFilter(orgID) + ` ORDER BY name ASC`

	args := []any{envID}
	if orgID != "" {
		args = append(args, orgID)
	}

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list apps by environment: %w", err)
	}
	defer rows.Close()

	apps := []Application{}
	for rows.Next() {
		var a Application
		var projectID, envIDPtr, serverID, currentDeplID *string
		var configBytes []byte
		var description string
		if err := rows.Scan(&a.ID, &a.Name, &description, &a.OrgID,
			&projectID, &envIDPtr, &serverID,
			&a.SourceType, &configBytes,
			&a.DesiredState, &a.ObservedStatus, &currentDeplID,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan application: %w", err)
		}
		a.Description = description
		a.ProjectID = projectID
		a.EnvironmentID = envIDPtr
		a.ServerID = serverID
		a.CurrentDeploymentID = currentDeplID
		if len(configBytes) > 0 {
			a.SourceConfig = configBytes
		}
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

func optionalOrgFilter(orgID string) string {
	if orgID == "" {
		return ""
	}
	return " AND org_id = $2"
}

// ---- Service logs aggregation ----

// ServiceLogEntry is a flattened log line for the env service logs endpoint.
// It aggregates deployment_logs of every application attached to the
// environment (compose/apphosting workloads both deploy through the
// deployments table); Stage is the dataset origin.
type ServiceLogEntry struct {
	ID        string    `json:"id"`
	EnvID     string    `json:"envId"`
	AppName   string    `json:"appName"`
	AppID     string    `json:"appId"`
	Service   string    `json:"service"`
	Stream    string    `json:"stream"`
	Stage     string    `json:"stage"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

// ListEnvServiceLogs returns aggregated log entries for an environment's
// applications. When service is non-empty only applications whose name
// matches are returned. since filters on log timestamp; limit caps rows
// (default 500).
func (s *Store) ListEnvServiceLogs(ctx context.Context, envID, service string, since *time.Time, limit int) ([]ServiceLogEntry, error) {
	if limit <= 0 || limit > 5000 {
		limit = 500
	}

	query := `
		SELECT dl.id::text, $1::text AS env_id,
			a.name,
			a.id::text,
			COALESCE(dl.log_level, 'info'),
			COALESCE(dl.message, ''),
			dl.created_at
		FROM deployment_logs dl
		JOIN deployments d ON d.id = dl.deployment_id
		JOIN applications a ON a.current_deployment_id = d.id
		WHERE a.environment_id = $1
		  AND a.desired_state <> 'removed'
		  AND ($2::text = '' OR a.name = $2)
		  AND ($3::timestamptz IS NULL OR dl.created_at >= $3)
		ORDER BY dl.created_at ASC
		LIMIT $4
	`
	var sinceArg any
	if since != nil {
		sinceArg = *since
	}
	rows, err := s.db.Query(ctx, query, envID, service, sinceArg, limit)
	if err != nil {
		return nil, fmt.Errorf("list env service logs: %w", err)
	}
	defer rows.Close()

	entries := []ServiceLogEntry{}
	for rows.Next() {
		var e ServiceLogEntry
		if err := rows.Scan(&e.ID, &e.EnvID, &e.AppName, &e.AppID, &e.Level, &e.Message, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan service log: %w", err)
		}
		e.Service = e.AppName
		e.Stage = "deploy"
		e.Stream = "stdout"
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
