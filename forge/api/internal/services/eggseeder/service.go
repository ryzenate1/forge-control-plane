package eggseeder

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	"gamepanel/forge/internal/store"
	"github.com/google/uuid"
)

//go:embed templates/*.json
var templateFS embed.FS

type Service struct {
	store *store.Store
}

func New(s *store.Store) *Service {
	return &Service{store: s}
}

type templateFile struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Version       string            `json:"version"`
	Game          string            `json:"game"`
	Author        string            `json:"author"`
	UpdateURL     string            `json:"update_url"`
	Image         string            `json:"image"`
	Images        map[string]string `json:"images"`
	Startup       string            `json:"startup"`
	Config        json.RawMessage   `json:"config"`
	Env           []templateEnv     `json:"env"`
	Resources     templateResources `json:"resources"`
	InstallScript templateInstall   `json:"install_script"`
	FileDenylist  []string          `json:"file_denylist"`
	Features      []string          `json:"features"`
}

type templateEnv struct {
	Name         string `json:"name"`
	EnvVariable  string `json:"env_variable"`
	Description  string `json:"description"`
	DefaultValue string `json:"default_value"`
	UserViewable bool   `json:"user_viewable"`
	UserEditable bool   `json:"user_editable"`
	Rules        string `json:"rules"`
}

type templateResources struct {
	CPU       int `json:"cpu"`
	MemoryMB  int `json:"memory_mb"`
	DiskMB    int `json:"disk_mb"`
	CPUShares int `json:"cpu_shares"`
	IOWeight  int `json:"io_weight"`
	SwapMB    int `json:"swap_mb"`
}

type templateInstall struct {
	Container  string `json:"container"`
	Entrypoint string `json:"entrypoint"`
	Script     string `json:"script"`
}

// SeedDefaultEggs seeds game templates into eggs+egg_variables idempotently.
// It mirrors appstore.Service.SeedDefaultApps: upsert on (nest_id, name).
func (s *Service) SeedDefaultEggs(ctx context.Context) error {
	if s.store == nil {
		return fmt.Errorf("store required")
	}
	// Ensure Games nest exists.
	var nestID string
	// Try to fetch existing nest first.
	err := s.store.GetDB().QueryRow(ctx, `SELECT id::text FROM nests WHERE name = 'Games' LIMIT 1`).Scan(&nestID)
	if err != nil {
		// Create nest directly via store or raw SQL.
		nestID = uuid.NewString()
		_, execErr := s.store.GetDB().Exec(ctx, `INSERT INTO nests (id, name, description) VALUES ($1, 'Games', 'Default game nest') ON CONFLICT (name) DO NOTHING`, nestID)
		if execErr != nil {
			return fmt.Errorf("ensure Games nest: %w", execErr)
		}
		// Re-fetch id (handles race where another process inserted).
		if err := s.store.GetDB().QueryRow(ctx, `SELECT id::text FROM nests WHERE name = 'Games' LIMIT 1`).Scan(&nestID); err != nil {
			return fmt.Errorf("fetch Games nest: %w", err)
		}
	}

	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		return fmt.Errorf("read embedded templates: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := templateFS.ReadFile("templates/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read template %s: %w", entry.Name(), err)
		}
		var tmpl templateFile
		if err := json.Unmarshal(data, &tmpl); err != nil {
			return fmt.Errorf("parse template %s: %w", entry.Name(), err)
		}
		if err := s.upsertEgg(ctx, nestID, tmpl); err != nil {
			return fmt.Errorf("seed template %s: %w", tmpl.Name, err)
		}
	}
	return nil
}

func (s *Service) upsertEgg(ctx context.Context, nestID string, tmpl templateFile) error {
	name := strings.TrimSpace(tmpl.Name)
	if name == "" {
		return fmt.Errorf("template name required")
	}
	// Build docker_images JSON.
	images := tmpl.Images
	if len(images) == 0 {
		if strings.TrimSpace(tmpl.Image) == "" {
			return fmt.Errorf("template %q missing image", name)
		}
		images = map[string]string{tmpl.Image: tmpl.Image}
	}
	dockerImages, err := json.Marshal(images)
	if err != nil {
		return fmt.Errorf("marshal docker_images: %w", err)
	}
	// Config JSON.
	config := tmpl.Config
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}
	// File denylist.
	fileDenylist := tmpl.FileDenylist
	if fileDenylist == nil {
		fileDenylist = []string{}
	}
	fileDenylistJSON, _ := json.Marshal(fileDenylist)
	// Features.
	features := tmpl.Features
	if features == nil {
		features = []string{}
	}
	featuresJSON, _ := json.Marshal(features)

	memory := tmpl.Resources.MemoryMB
	if memory <= 0 {
		memory = 1024
	}
	installScript := tmpl.InstallScript.Script
	installContainer := strings.TrimSpace(tmpl.InstallScript.Container)
	if installContainer == "" {
		installContainer = "alpine:3.21"
	}
	installEntrypoint := strings.TrimSpace(tmpl.InstallScript.Entrypoint)
	if installEntrypoint == "" {
		installEntrypoint = "sh"
	}
	author := strings.TrimSpace(tmpl.Author)
	if author == "" {
		author = "GamePanel"
	}
	updateURL := strings.TrimSpace(tmpl.UpdateURL)
	description := strings.TrimSpace(tmpl.Description)
	startup := tmpl.Startup

	eggID := uuid.NewString()
	_, err = s.store.GetDB().Exec(ctx, `
		INSERT INTO eggs (id, nest_id, name, description, docker_images, startup, config,
		                  default_memory_mb, install_script, install_container, install_entrypoint,
		                  file_denylist, author, features, startup_commands, update_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, '[]'::jsonb, $15)
		ON CONFLICT (nest_id, name) DO UPDATE SET
			description = EXCLUDED.description,
			docker_images = EXCLUDED.docker_images,
			startup = EXCLUDED.startup,
			config = EXCLUDED.config,
			default_memory_mb = EXCLUDED.default_memory_mb,
			install_script = EXCLUDED.install_script,
			install_container = EXCLUDED.install_container,
			install_entrypoint = EXCLUDED.install_entrypoint,
			file_denylist = EXCLUDED.file_denylist,
			author = EXCLUDED.author,
			features = EXCLUDED.features,
			update_url = EXCLUDED.update_url
	`, eggID, nestID, name, description, dockerImages, startup, config,
		memory, installScript, installContainer, installEntrypoint,
		fileDenylistJSON, author, featuresJSON, updateURL)
	if err != nil {
		return fmt.Errorf("upsert egg: %w", err)
	}
	// Fetch canonical egg ID for variable upsert (handles both insert and conflict).
	var canonicalEggID string
	if err := s.store.GetDB().QueryRow(ctx, `SELECT id::text FROM eggs WHERE nest_id = $1 AND name = $2`, nestID, name).Scan(&canonicalEggID); err != nil {
		return fmt.Errorf("fetch egg id: %w", err)
	}
	// Upsert env variables.
	for idx, env := range tmpl.Env {
		if strings.TrimSpace(env.EnvVariable) == "" {
			continue
		}
		rules := strings.TrimSpace(env.Rules)
		if rules == "" {
			rules = "nullable|string"
		}
		// Ensure rules are valid; fallback to safe default if store validation would reject.
		// We keep original rules but allow nullable etc.
		varID := uuid.NewString()
		// Use ON CONFLICT (egg_id, env_variable) to make idempotent.
		_, err := s.store.GetDB().Exec(ctx, `
			INSERT INTO egg_variables (id, egg_id, name, description, env_variable, default_value,
			                           user_viewable, user_editable, rules, sort)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (egg_id, env_variable) DO UPDATE SET
				name = EXCLUDED.name,
				description = EXCLUDED.description,
				default_value = EXCLUDED.default_value,
				user_viewable = EXCLUDED.user_viewable,
				user_editable = EXCLUDED.user_editable,
				rules = EXCLUDED.rules,
				sort = EXCLUDED.sort
		`, varID, canonicalEggID, strings.TrimSpace(env.Name), strings.TrimSpace(env.Description),
			strings.TrimSpace(env.EnvVariable), env.DefaultValue, env.UserViewable, env.UserEditable, rules, idx*10)
		if err != nil {
			return fmt.Errorf("upsert egg variable %s: %w", env.EnvVariable, err)
		}
	}
	return nil
}
