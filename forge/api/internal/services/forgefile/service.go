// Package forgefile implements declarative forge.yaml configuration for
// Phase 8 (env-as-code). It validates a forge.yaml manifest against a known
// key schema, persists it idempotently (one row per project slug, replacing
// older forge.yaml updates) and materializes app + service targets through
// the apphosting service. It deliberately does NOT hook apphosting deep
// APIs — apply is validate + persist + construct + links.
package forgefile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"gamepanel/forge/internal/services/apphosting"
	"gamepanel/forge/internal/services/tenancy"
	"gamepanel/forge/internal/store"

	"gopkg.in/yaml.v3"
)

const (
	MaxManifest = 256 * 1024

	DeployTypeApp     = "app"
	DeployTypeCompose = "compose"
	DeployTypeDB      = "db"

	BuilderDockerfile = "dockerfile"
	BuilderNixpacks   = "nixpacks"
	BuilderHeroku     = "heroku"
	BuilderStatic     = "static"

	// BaseDomainEnv is the env var read at registrar build time. When unset,
	// Apply returns placeholder internal URLs (http://<name>.local).
	BaseDomainEnv = "FORGEFILE_BASE_DOMAIN"
)

var slugRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Project identifies the forge.yaml project.
type Project struct {
	Name string `yaml:"name" json:"name"`
	Slug string `yaml:"slug" json:"slug"`
}

// Source selects the repository and branch to deploy.
type Source struct {
	Repo   string `yaml:"repo" json:"repo"`
	Branch string `yaml:"branch" json:"branch"`
	Root   string `yaml:"root" json:"root"`
}

// Build describes how the app is turned into a runnable artifact.
type Build struct {
	Builder    string `yaml:"builder" json:"builder"`
	Dockerfile string `yaml:"dockerfile" json:"dockerfile"`
}

// Resources is the per-app resource budget.
type Resources struct {
	CPU      string `yaml:"cpu" json:"cpu"`
	MemoryMB int    `yaml:"memory" json:"memory"`
	Replicas int    `yaml:"replicas" json:"replicas"`
}

// Deploy is one entry of the deploy array (app | compose | db).
type Deploy struct {
	Name      string            `yaml:"name" json:"name"`
	Type      string            `yaml:"type" json:"type"`
	Source    Source            `yaml:"source" json:"source"`
	Build     Build             `yaml:"build" json:"build"`
	Ports     []int             `yaml:"ports" json:"ports"`
	Env       map[string]string `yaml:"env" json:"env"`
	Resources Resources         `yaml:"resources" json:"resources"`
	Kind      string            `yaml:"kind" json:"kind,omitempty"`
	Version   string            `yaml:"version" json:"version,omitempty"`
}

// Database is the shorthand `database:` block: type database, kind, version.
type Database struct {
	Name    string `yaml:"name" json:"name"`
	Kind    string `yaml:"kind" json:"kind"`
	Version string `yaml:"version" json:"version"`
}

// Manifest is the parsed forge.yaml document.
type Manifest struct {
	Project      Project   `yaml:"project" json:"project"`
	Deploy       []Deploy  `yaml:"deploy" json:"deploy"`
	Database     *Database `yaml:"database" json:"database,omitempty"`
	Environments []string  `yaml:"environments" json:"environments"`
}

// LinkPayload is one target returned by Apply.
type LinkPayload struct {
	AppID     string `json:"appId"`
	AppName   string `json:"appName"`
	ServiceID string `json:"serviceId"`
	Domain    string `json:"domain"`
	URL       string `json:"url"`
	Status    string `json:"status"`
}

// ApplyResult is the apply response for the UI.
type ApplyResult struct {
	ProjectSlug string            `json:"projectSlug"`
	ProjectName string            `json:"projectName"`
	Version     int               `json:"version"`
	Apps        []LinkPayload     `json:"apps"`
	Links       map[string]string `json:"links"`
	Warnings    []string          `json:"warnings"`
	AppliedAt   time.Time         `json:"appliedAt"`
}

// Service validates, persists and applies forge.yaml manifests.
type Service struct {
	store      *store.Store
	appSvc     *apphosting.Service
	logger     *slog.Logger
	baseDomain string
}

func NewService(st *store.Store, appSvc *apphosting.Service, logger *slog.Logger, baseDomain string) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: st, appSvc: appSvc, logger: logger, baseDomain: strings.TrimSpace(baseDomain)}
}

// --- validation (validate via check keys) ---

var knownTopLevel = map[string]bool{
	"project": true, "deploy": true, "database": true, "environments": true,
}
var knownDeployKeys = map[string]bool{
	"name": true, "type": true, "source": true, "build": true, "ports": true,
	"env": true, "resources": true, "kind": true, "version": true,
}
var knownBuilders = map[string]bool{
	BuilderDockerfile: true, BuilderNixpacks: true, BuilderHeroku: true, BuilderStatic: true,
}
var knownDeployTypes = map[string]bool{
	DeployTypeApp: true, DeployTypeCompose: true, DeployTypeDB: true,
}

// Validate parses content and returns a normalized manifest plus warnings for
// unknown keys. Structural violations return an error.
func Validate(content []byte) (*Manifest, []string, error) {
	if len(content) == 0 {
		return nil, nil, errors.New("forge.yaml body is empty")
	}
	if len(content) > MaxManifest {
		return nil, nil, errors.New("forge.yaml body exceeds 256 KiB")
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, nil, fmt.Errorf("forge.yaml is not valid YAML: %w", err)
	}
	warnings := checkKeys(&doc)
	var m Manifest
	if err := doc.Decode(&m); err != nil {
		return nil, warnings, fmt.Errorf("forge.yaml does not match the schema: %w", err)
	}
	m.normalize()
	warnings = append(warnings, checkMinimal(&m)...)
	return &m, warnings, nil
}

func (m *Manifest) normalize() {
	m.Project.Name = strings.TrimSpace(m.Project.Name)
	m.Project.Slug = slugify(m.Project.Slug, m.Project.Name)
	for i := range m.Deploy {
		d := &m.Deploy[i]
		d.Name = strings.TrimSpace(d.Name)
		d.Type = strings.ToLower(strings.TrimSpace(d.Type))
		d.Build.Builder = strings.ToLower(strings.TrimSpace(d.Build.Builder))
		if d.Type == "" {
			d.Type = DeployTypeApp
		}
	}
}

// checkKeys walks the YAML tree and reports keys outside the known schema.
func checkKeys(doc *yaml.Node) []string {
	var warnings []string
	if len(doc.Content) == 0 {
		return warnings
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return warnings
	}
	for i := 0; i < len(root.Content); i += 2 {
		key := root.Content[i].Value
		value := root.Content[i+1]
		if !knownTopLevel[key] {
			warnings = append(warnings, fmt.Sprintf("unknown top-level key %q (known: project, deploy, database, environments)", key))
			continue
		}
		if key == "deploy" && value.Kind == yaml.SequenceNode {
			for _, item := range value.Content {
				if item.Kind != yaml.MappingNode {
					continue
				}
				for j := 0; j < len(item.Content); j += 2 {
					dk := item.Content[j].Value
					if !knownDeployKeys[dk] {
						warnings = append(warnings, fmt.Sprintf("unknown deploy key %q", dk))
					}
				}
			}
		}
	}
	return warnings
}

func checkMinimal(m *Manifest) []string {
	var warnings []string
	if m.Project.Slug == "" {
		warnings = append(warnings, "project.slug is required")
	} else if !slugRE.MatchString(m.Project.Slug) {
		warnings = append(warnings, "project.slug must be lowercase alphanumeric words joined by hyphens")
	}
	if len(m.Deploy) == 0 {
		warnings = append(warnings, "no deploy entries — nothing will be materialized")
	}
	for i := range m.Deploy {
		d := &m.Deploy[i]
		if d.Name == "" {
			warnings = append(warnings, fmt.Sprintf("deploy[%d].name is required", i))
		}
		if !knownDeployTypes[d.Type] {
			warnings = append(warnings, fmt.Sprintf("deploy[%d].type %q invalid (must be app|compose|db)", i, d.Type))
		}
		if d.Type != DeployTypeDB && strings.TrimSpace(d.Source.Repo) == "" {
			warnings = append(warnings, fmt.Sprintf("deploy[%d].source.repo is required for type %q", i, d.Type))
		}
		if d.Build.Builder != "" && d.Build.Builder != "auto" && !knownBuilders[d.Build.Builder] {
			warnings = append(warnings, fmt.Sprintf("deploy[%d].build.builder %q invalid (dockerfile|nixpacks|heroku|static)", i, d.Build.Builder))
		}
		for _, port := range d.Ports {
			if port < 1 || port > 65535 {
				warnings = append(warnings, fmt.Sprintf("deploy[%d] port %d is out of range 1-65535", i, port))
			}
		}
		if d.Resources.Replicas < 0 {
			warnings = append(warnings, fmt.Sprintf("deploy[%d].resources.replicas must be >= 0", i))
		}
	}
	return warnings
}

// --- persistence ---

func (s *Service) GetManifest(ctx context.Context, slug string) (*Manifest, int, time.Time, error) {
	if s.store == nil || s.store.DB() == nil {
		return nil, 0, time.Time{}, errors.New("postgres is required")
	}
	var content []byte
	var version int
	var updatedAt time.Time
	err := s.store.DB().QueryRow(ctx, `
		SELECT content, version, updated_at FROM forge_manifests WHERE project_slug = $1
	`, slug).Scan(&content, &version, &updatedAt)
	if err != nil {
		return nil, 0, time.Time{}, err
	}
	var m Manifest
	if err := yaml.Unmarshal(content, &m); err != nil {
		return nil, version, updatedAt, fmt.Errorf("stored manifest is invalid: %w", err)
	}
	return &m, version, updatedAt, nil
}

func (s *Service) ListManifests(ctx context.Context) ([]string, error) {
	if s.store == nil || s.store.DB() == nil {
		return nil, errors.New("postgres is required")
	}
	rows, err := s.store.DB().Query(ctx, `SELECT project_slug FROM forge_manifests ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	slugs := []string{}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		slugs = append(slugs, slug)
	}
	return slugs, rows.Err()
}

// Apply validates, persists (idempotent upsert on project slug) and
// materializes one app + one service per deploy entry. Domains are derived
// from the configured base domain or fall back to internal placeholder URLs.
func (s *Service) Apply(ctx context.Context, userID, role string, content []byte, orgID string) (*ApplyResult, error) {
	if orgID == "" {
		orgID = resolveOrg(ctx, s.store, userID, role)
	}
	m, warnings, err := Validate(content)
	if err != nil {
		return nil, err
	}
	slug := m.Project.Slug

	result := &ApplyResult{
		ProjectSlug: slug,
		ProjectName: m.Project.Name,
		Warnings:    warnings,
		AppliedAt:   time.Now().UTC(),
		Links:       map[string]string{},
	}

	for _, d := range m.Deploy {
		if d.Type == DeployTypeDB {
			result.Warnings = append(result.Warnings, fmt.Sprintf("deploy %q: database targets are recorded but provisioned by the platform databases service", d.Name))
			continue
		}
		link, err := s.materializeDeploy(ctx, orgID, slug, d)
		if err != nil {
			return result, fmt.Errorf("apply deploy %q: %w", d.Name, err)
		}
		result.Apps = append(result.Apps, link)
		result.Links[link.AppName] = link.URL
	}

	version, err := s.persist(ctx, slug, m.Project.Name, userID, content)
	if err != nil {
		return result, fmt.Errorf("persist manifest: %w", err)
	}
	result.Version = version
	return result, nil
}

func (s *Service) persist(ctx context.Context, slug, projectName, userID string, content []byte) (int, error) {
	encoded, err := json.Marshal(content)
	if err != nil {
		return 0, err
	}
	var version int
	err = s.store.DB().QueryRow(ctx, `
		INSERT INTO forge_manifests (project_slug, project_name, content, version, applied_by, updated_at)
		VALUES ($1, $2, $3, 1, $4, now())
		ON CONFLICT (project_slug) DO UPDATE SET
			project_name = EXCLUDED.project_name,
			content = EXCLUDED.content,
			version = forge_manifests.version + 1,
			applied_by = EXCLUDED.applied_by,
			updated_at = now()
		RETURNING version
	`, slug, projectName, encoded, userID).Scan(&version)
	return version, err
}

func (s *Service) materializeDeploy(ctx context.Context, orgID, projectSlug string, d Deploy) (LinkPayload, error) {
	sourceCfg, _ := json.Marshal(map[string]any{
		"repository": d.Source.Repo,
		"repo":       d.Source.Repo,
		"branch":     d.Source.Branch,
		"root":       d.Source.Root,
		"builder":    d.Build.Builder,
		"dockerfile": d.Build.Dockerfile,
		"kind":       d.Type,
		"project":    projectSlug,
	})
	app, err := s.appSvc.CreateApp(ctx, orgCtx(orgID), apphosting.CreateAppRequest{
		Name:         d.Name,
		Description:  "materialized from forge.yaml (" + projectSlug + ")",
		OrgID:        orgID,
		SourceType:   "GIT",
		SourceConfig: sourceCfg,
	})
	if err != nil {
		return LinkPayload{}, err
	}
	ports := make([]store.AppPort, 0, len(d.Ports))
	for _, p := range d.Ports {
		ports = append(ports, store.AppPort{ContainerPort: p, Protocol: "tcp"})
	}
	if len(ports) == 0 {
		ports = append(ports, store.AppPort{ContainerPort: 8080, Protocol: "tcp"})
	}
	env := d.Env
	if env == nil {
		env = map[string]string{}
	}
	replicas := d.Resources.Replicas
	if replicas < 1 {
		replicas = 1
	}
	svcID := ""
	svc, err := s.appSvc.CreateService(ctx, app.ID, orgID, apphosting.CreateServiceRequest{
		Name:     d.Name,
		Replicas: replicas,
		Ports:    ports,
		EnvVars:  env,
	})
	if err != nil {
		s.logger.Warn("forgefile: service creation failed; app remains", "appID", app.ID, "error", err)
	} else {
		svcID = svc.ID
	}

	link := LinkPayload{
		AppID:     app.ID,
		AppName:   d.Name,
		ServiceID: svcID,
		Domain:    domainFor(d.Name, projectSlug, s.baseDomain),
		URL:       urlFor(d.Name, projectSlug, app.ID, s.baseDomain),
		Status:    "queued",
	}
	_ = s.persistAppLink(ctx, projectSlug, link)
	return link, nil
}

func (s *Service) persistAppLink(ctx context.Context, slug string, link LinkPayload) error {
	if s.store == nil || s.store.DB() == nil {
		return nil
	}
	_, err := s.store.DB().Exec(ctx, `
		INSERT INTO forge_manifest_apps (manifest_slug, app_id, app_name, service_id, domain, url, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (manifest_slug, app_id) DO UPDATE SET
			app_name = EXCLUDED.app_name,
			service_id = EXCLUDED.service_id,
			domain = EXCLUDED.domain,
			url = EXCLUDED.url,
			status = EXCLUDED.status
	`, slug, link.AppID, link.AppName, link.ServiceID, link.Domain, link.URL, link.Status)
	return err
}

// --- helpers ---

func resolveOrg(ctx context.Context, st *store.Store, userID, role string) string {
	if st == nil || st.DB() == nil {
		return "default"
	}
	if role == "admin" {
		return "default"
	}
	orgs, err := st.ListOrganizationsForUser(ctx, userID)
	if err != nil || len(orgs) == 0 {
		return "default"
	}
	return orgs[0].ID
}

func slugify(slug string, name string) string {
	slug = strings.TrimSpace(strings.ToLower(slug))
	if slug != "" {
		return slug
	}
	re := regexp.MustCompile(`[^a-z0-9]+`)
	base := re.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	base = strings.Trim(base, "-")
	if base == "" {
		return ""
	}
	return base
}

func domainFor(name, projectSlug, baseDomain string) string {
	host := slugify(name, name)
	if host == "" {
		host = projectSlug
	}
	if baseDomain != "" {
		return host + "." + baseDomain
	}
	return host + ".local"
}

func urlFor(name, projectSlug, appID, baseDomain string) string {
	if baseDomain != "" {
		return "https://" + domainFor(name, projectSlug, baseDomain)
	}
	// Placeholder internal URL until DNS is configured for the base domain.
	return "http://" + domainFor(name, projectSlug, baseDomain) + "/deploy/" + appID
}

func orgCtx(orgID string) tenancy.OrgContext {
	return tenancy.OrgContext{OrgID: orgID, Role: "admin"}
}
