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

// ---------- Nest types ----------

type Nest struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	EggCount    int       `json:"eggCount"`
	CreatedAt   time.Time `json:"createdAt"`
}

type CreateNestRequest struct {
	Name        string
	Description string
}

type UpdateNestRequest struct {
	Name        string
	Description string
}

// ---------- Egg types ----------

type Egg struct {
	ID                string          `json:"id"`
	NestID            string          `json:"nestId"`
	NestName          string          `json:"nestName,omitempty"`
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	DockerImages      json.RawMessage `json:"dockerImages"`
	Startup           string          `json:"startup"`
	Config            json.RawMessage `json:"config"`
	DefaultMemoryMB   int             `json:"defaultMemoryMb"`
	InstallScript     string          `json:"installScript"`
	InstallContainer  string          `json:"installContainer"`
	InstallEntrypoint string          `json:"installEntrypoint"`
	FileDenylist      json.RawMessage `json:"fileDenylist"`
	ConfigFrom        *string         `json:"configFrom,omitempty"`
	CopyScriptFrom    *string         `json:"copyScriptFrom,omitempty"`
	UpdateURL         string          `json:"updateUrl"`
	Author            string          `json:"author"`
	Features          json.RawMessage `json:"features"`
	StartupCommands   json.RawMessage `json:"startupCommands"`
	CreatedAt         time.Time       `json:"createdAt"`
}

type CreateEggRequest struct {
	NestID            string
	Name              string
	Description       string
	DockerImages      json.RawMessage
	Startup           string
	Config            json.RawMessage
	DefaultMemoryMB   int
	InstallScript     string
	InstallContainer  string
	InstallEntrypoint string
	FileDenylist      json.RawMessage
	ConfigFrom        *string
	CopyScriptFrom    *string
	UpdateURL         string
	Author            string
	Features          json.RawMessage
	StartupCommands   json.RawMessage
}

type UpdateEggRequest struct {
	Name              string
	Description       string
	DockerImages      json.RawMessage
	Startup           string
	Config            json.RawMessage
	DefaultMemoryMB   int
	InstallScript     string
	InstallContainer  string
	InstallEntrypoint string
	FileDenylist      json.RawMessage
	ConfigFrom        *string
	CopyScriptFrom    *string
	UpdateURL         string
	Author            string
	Features          json.RawMessage
	StartupCommands   json.RawMessage
}

// ---------- Nest CRUD ----------

func (s *Store) ListNests(ctx context.Context) ([]Nest, error) {
	rows, err := s.db.Query(ctx, `
		SELECT n.id::text, n.name, COALESCE(n.description, ''), n.created_at,
		       COALESCE((SELECT COUNT(*) FROM eggs e WHERE e.nest_id = n.id), 0)
		FROM nests n
		ORDER BY n.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nests := []Nest{}
	for rows.Next() {
		var nest Nest
		if err := rows.Scan(&nest.ID, &nest.Name, &nest.Description, &nest.CreatedAt, &nest.EggCount); err != nil {
			return nil, err
		}
		nests = append(nests, nest)
	}
	return nests, rows.Err()
}

func (s *Store) GetNest(ctx context.Context, id string) (Nest, error) {
	var nest Nest
	err := s.db.QueryRow(ctx, `
		SELECT n.id::text, n.name, COALESCE(n.description, ''), n.created_at,
		       COALESCE((SELECT COUNT(*) FROM eggs e WHERE e.nest_id = n.id), 0)
		FROM nests n
		WHERE n.id = $1
	`, id).Scan(&nest.ID, &nest.Name, &nest.Description, &nest.CreatedAt, &nest.EggCount)
	if err != nil {
		return Nest{}, fmt.Errorf("nest not found: %w", err)
	}
	return nest, nil
}

func (s *Store) CreateNest(ctx context.Context, req CreateNestRequest, actorID *string) (Nest, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Nest{}, errors.New("name is required")
	}
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO nests (id, name, description)
		VALUES ($1, $2, $3)
	`, id, name, strings.TrimSpace(req.Description))
	if err != nil {
		return Nest{}, fmt.Errorf("create nest: %w", err)
	}
	_ = s.AppendAudit(ctx, actorID, "nest created", "nest", &id, fmt.Sprintf(`{"name":"%s"}`, name))
	return s.GetNest(ctx, id)
}

func (s *Store) UpdateNest(ctx context.Context, id string, req UpdateNestRequest, actorID *string) (Nest, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Nest{}, errors.New("name is required")
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE nests SET name = $1, description = $2 WHERE id = $3
	`, name, strings.TrimSpace(req.Description), id)
	if err != nil {
		return Nest{}, fmt.Errorf("update nest: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Nest{}, errors.New("nest not found")
	}
	_ = s.AppendAudit(ctx, actorID, "nest updated", "nest", &id, fmt.Sprintf(`{"name":"%s"}`, name))
	return s.GetNest(ctx, id)
}

func (s *Store) DeleteNest(ctx context.Context, id string, actorID *string) error {
	var eggCount int
	_ = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM eggs WHERE nest_id = $1`, id).Scan(&eggCount)
	if eggCount > 0 {
		return fmt.Errorf("cannot delete nest with %d egg(s) — delete or move eggs first", eggCount)
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM nests WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("nest not found")
	}
	return s.AppendAudit(ctx, actorID, "nest deleted", "nest", &id, `{"reason":"admin delete"}`)
}

// ---------- Egg CRUD ----------

func (s *Store) ListEggs(ctx context.Context, nestID string) ([]Egg, error) {
	rows, err := s.db.Query(ctx, `
		SELECT e.id::text, e.nest_id::text, n.name, e.name, COALESCE(e.description, ''),
		       e.docker_images, e.startup, e.config, e.default_memory_mb,
		       e.install_script, e.install_container, e.install_entrypoint, e.file_denylist,
		       e.config_from::text, e.copy_script_from::text,
		       COALESCE(e.update_url, ''), COALESCE(e.author, ''),
		       e.features, e.startup_commands, e.created_at
		FROM eggs e
		JOIN nests n ON n.id = e.nest_id
		WHERE ($1 = '' OR e.nest_id = $1::uuid)
		ORDER BY n.name, e.name
	`, nestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	eggs := []Egg{}
	for rows.Next() {
		var egg Egg
		if err := rows.Scan(&egg.ID, &egg.NestID, &egg.NestName, &egg.Name, &egg.Description,
			&egg.DockerImages, &egg.Startup, &egg.Config, &egg.DefaultMemoryMB, &egg.InstallScript,
			&egg.InstallContainer, &egg.InstallEntrypoint, &egg.FileDenylist,
			&egg.ConfigFrom, &egg.CopyScriptFrom,
			&egg.UpdateURL, &egg.Author,
			&egg.Features, &egg.StartupCommands, &egg.CreatedAt); err != nil {
			return nil, err
		}
		eggs = append(eggs, egg)
	}
	return eggs, rows.Err()
}

func (s *Store) GetEgg(ctx context.Context, id string) (Egg, error) {
	var egg Egg
	err := s.db.QueryRow(ctx, `
		SELECT e.id::text, e.nest_id::text, n.name, e.name, COALESCE(e.description, ''),
		       e.docker_images, e.startup, e.config, e.default_memory_mb,
		       e.install_script, e.install_container, e.install_entrypoint, e.file_denylist,
		       e.config_from::text, e.copy_script_from::text,
		       COALESCE(e.update_url, ''), COALESCE(e.author, ''),
		       e.features, e.startup_commands, e.created_at
		FROM eggs e
		JOIN nests n ON n.id = e.nest_id
		WHERE e.id = $1
	`, id).Scan(&egg.ID, &egg.NestID, &egg.NestName, &egg.Name, &egg.Description,
		&egg.DockerImages, &egg.Startup, &egg.Config, &egg.DefaultMemoryMB, &egg.InstallScript,
		&egg.InstallContainer, &egg.InstallEntrypoint, &egg.FileDenylist,
		&egg.ConfigFrom, &egg.CopyScriptFrom,
		&egg.UpdateURL, &egg.Author,
		&egg.Features, &egg.StartupCommands, &egg.CreatedAt)
	if err != nil {
		return Egg{}, fmt.Errorf("egg not found: %w", err)
	}
	return egg, nil
}

func (s *Store) CreateEgg(ctx context.Context, req CreateEggRequest, actorID *string) (Egg, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || req.NestID == "" {
		return Egg{}, errors.New("nestId and name are required")
	}
	dockerImages, err := normalizeDockerImages(req.DockerImages)
	if err != nil {
		return Egg{}, err
	}
	config, err := normalizeJSONObject(req.Config, "config")
	if err != nil {
		return Egg{}, err
	}
	fileDenylist, err := normalizeJSONArray(req.FileDenylist, "fileDenylist")
	if err != nil {
		return Egg{}, err
	}
	if req.DefaultMemoryMB <= 0 {
		req.DefaultMemoryMB = 1024
	}
	if strings.TrimSpace(req.InstallContainer) == "" {
		req.InstallContainer = "alpine:3.21"
	}
	if strings.TrimSpace(req.InstallEntrypoint) == "" {
		req.InstallEntrypoint = "sh"
	}
	id := uuid.NewString()
	_, err = s.db.Exec(ctx, `
		INSERT INTO eggs (id, nest_id, name, description, docker_images, startup, config,
		                  default_memory_mb, install_script, install_container, install_entrypoint,
		                  file_denylist, config_from, copy_script_from, update_url, author,
		                  features, startup_commands)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
	`, id, req.NestID, name, strings.TrimSpace(req.Description), dockerImages, req.Startup, config,
		req.DefaultMemoryMB, req.InstallScript, req.InstallContainer, req.InstallEntrypoint, fileDenylist,
		req.ConfigFrom, req.CopyScriptFrom, req.UpdateURL, req.Author,
		req.Features, req.StartupCommands)
	if err != nil {
		return Egg{}, fmt.Errorf("create egg: %w", err)
	}
	_ = s.AppendAudit(ctx, actorID, "egg created", "egg", &id, fmt.Sprintf(`{"name":"%s","nestId":"%s"}`, name, req.NestID))
	return s.GetEgg(ctx, id)
}

func (s *Store) UpdateEgg(ctx context.Context, id string, req UpdateEggRequest, actorID *string) (Egg, error) {
	current, err := s.GetEgg(ctx, id)
	if err != nil {
		return Egg{}, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Egg{}, errors.New("name is required")
	}
	if len(req.DockerImages) == 0 {
		req.DockerImages = current.DockerImages
	}
	if len(req.Config) == 0 {
		req.Config = current.Config
	}
	if len(req.FileDenylist) == 0 {
		req.FileDenylist = current.FileDenylist
	}
	if req.DefaultMemoryMB <= 0 {
		req.DefaultMemoryMB = current.DefaultMemoryMB
	}
	if strings.TrimSpace(req.InstallContainer) == "" {
		req.InstallContainer = current.InstallContainer
	}
	if strings.TrimSpace(req.InstallEntrypoint) == "" {
		req.InstallEntrypoint = current.InstallEntrypoint
	}
	if req.InstallScript == "" {
		req.InstallScript = current.InstallScript
	}
	dockerImages, err := normalizeDockerImages(req.DockerImages)
	if err != nil {
		return Egg{}, err
	}
	config, err := normalizeJSONObject(req.Config, "config")
	if err != nil {
		return Egg{}, err
	}
	fileDenylist, err := normalizeJSONArray(req.FileDenylist, "fileDenylist")
	if err != nil {
		return Egg{}, err
	}
	if req.DefaultMemoryMB <= 0 {
		return Egg{}, errors.New("defaultMemoryMb must be greater than zero")
	}
	if strings.TrimSpace(req.InstallContainer) == "" || strings.TrimSpace(req.InstallEntrypoint) == "" {
		return Egg{}, errors.New("installContainer and installEntrypoint are required")
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE eggs SET name = $1, description = $2, docker_images = $3, startup = $4, config = $5,
		                default_memory_mb = $6, install_script = $7, install_container = $8,
		                install_entrypoint = $9, file_denylist = $10,
		                config_from = $11, copy_script_from = $12, update_url = $13, author = $14,
		                features = $15, startup_commands = $16
		WHERE id = $17
	`, name, strings.TrimSpace(req.Description), dockerImages, req.Startup, config, req.DefaultMemoryMB,
		req.InstallScript, req.InstallContainer, req.InstallEntrypoint, fileDenylist,
		req.ConfigFrom, req.CopyScriptFrom, req.UpdateURL, req.Author,
		req.Features, req.StartupCommands, id)
	if err != nil {
		return Egg{}, fmt.Errorf("update egg: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Egg{}, errors.New("egg not found")
	}
	_ = s.AppendAudit(ctx, actorID, "egg updated", "egg", &id, fmt.Sprintf(`{"name":"%s"}`, name))
	return s.GetEgg(ctx, id)
}

func normalizeDockerImages(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, errors.New("dockerImages must contain at least one image")
	}
	var images map[string]string
	if err := json.Unmarshal(raw, &images); err == nil {
		if len(images) == 0 {
			return nil, errors.New("dockerImages must contain at least one image")
		}
		for label, image := range images {
			if strings.TrimSpace(label) == "" || strings.TrimSpace(image) == "" {
				return nil, errors.New("dockerImages labels and image values must not be empty")
			}
		}
		return json.Marshal(images)
	}
	var legacy []string
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return nil, errors.New("dockerImages must be an object map")
	}
	images = make(map[string]string, len(legacy))
	for _, image := range legacy {
		image = strings.TrimSpace(image)
		if image != "" {
			images[image] = image
		}
	}
	if len(images) == 0 {
		return nil, errors.New("dockerImages must contain at least one image")
	}
	return json.Marshal(images)
}

func normalizeJSONObject(raw json.RawMessage, field string) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("%s must be a JSON object", field)
	}
	return json.Marshal(value)
}

func normalizeJSONArray(raw json.RawMessage, field string) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`[]`), nil
	}
	var value []any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("%s must be a JSON array", field)
	}
	return json.Marshal(value)
}

func (s *Store) DeleteEgg(ctx context.Context, id string, actorID *string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM eggs WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("egg not found")
	}
	return s.AppendAudit(ctx, actorID, "egg deleted", "egg", &id, `{"reason":"admin delete"}`)
}
