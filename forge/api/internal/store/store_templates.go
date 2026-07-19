package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ListTemplates is a compatibility transform over canonical eggs. It does not
// read from or create rows in the legacy server_templates table.
func (s *Store) ListTemplates(ctx context.Context) ([]Template, error) {
	eggs, err := s.ListEggs(ctx, "")
	if err != nil {
		return nil, err
	}
	templates := make([]Template, 0, len(eggs))
	for _, egg := range eggs {
		template, err := templateFromEgg(egg)
		if err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}
	return templates, nil
}

func (s *Store) GetTemplate(ctx context.Context, id string) (Template, error) {
	egg, err := s.GetEgg(ctx, id)
	if err != nil {
		return Template{}, err
	}
	return templateFromEgg(egg)
}

func (s *Store) CreateTemplate(ctx context.Context, req CreateTemplateRequest, actorID *string) (Template, error) {
	name := strings.TrimSpace(req.Name)
	image := strings.TrimSpace(req.Image)
	if name == "" || image == "" {
		return Template{}, errors.New("name and image are required")
	}
	if req.DefaultMemoryMB <= 0 {
		req.DefaultMemoryMB = 1024
	}
	var nestID string
	if err := s.db.QueryRow(ctx, `SELECT id::text FROM nests WHERE name = 'Games' LIMIT 1`).Scan(&nestID); err != nil {
		return Template{}, fmt.Errorf("default Games nest not found: %w", err)
	}
	images, err := json.Marshal(map[string]string{image: image})
	if err != nil {
		return Template{}, err
	}
	egg, err := s.CreateEgg(ctx, CreateEggRequest{
		NestID:            nestID,
		Name:              name,
		DockerImages:      images,
		Startup:           req.StartupCommand,
		DefaultMemoryMB:   req.DefaultMemoryMB,
		InstallContainer:  "alpine:3.21",
		InstallEntrypoint: "sh",
	}, actorID)
	if err != nil {
		return Template{}, err
	}
	return templateFromEgg(egg)
}

func (s *Store) UpdateTemplate(ctx context.Context, id string, req CreateTemplateRequest, actorID *string) (Template, error) {
	current, err := s.GetEgg(ctx, id)
	if err != nil {
		return Template{}, err
	}
	name := strings.TrimSpace(req.Name)
	image := strings.TrimSpace(req.Image)
	if name == "" || image == "" {
		return Template{}, errors.New("name and image are required")
	}
	if req.DefaultMemoryMB <= 0 {
		req.DefaultMemoryMB = current.DefaultMemoryMB
	}
	images, err := json.Marshal(map[string]string{image: image})
	if err != nil {
		return Template{}, err
	}
	egg, err := s.UpdateEgg(ctx, id, UpdateEggRequest{
		Name: name, Description: current.Description, DockerImages: images,
		Startup: req.StartupCommand, Config: current.Config, DefaultMemoryMB: req.DefaultMemoryMB,
		InstallScript: current.InstallScript, InstallContainer: current.InstallContainer,
		InstallEntrypoint: current.InstallEntrypoint, FileDenylist: current.FileDenylist,
	}, actorID)
	if err != nil {
		return Template{}, err
	}
	return templateFromEgg(egg)
}

func templateFromEgg(egg Egg) (Template, error) {
	var images map[string]string
	if err := json.Unmarshal(egg.DockerImages, &images); err != nil {
		return Template{}, fmt.Errorf("egg %s has invalid docker image map: %w", egg.ID, err)
	}
	keys := make([]string, 0, len(images))
	for key := range images {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	image := ""
	if len(keys) > 0 {
		image = images[keys[0]]
	}
	return Template{
		ID:                egg.ID,
		Name:              egg.Name,
		Image:             image,
		StartupCommand:    egg.Startup,
		DefaultMemoryMB:   egg.DefaultMemoryMB,
		InstallScript:     egg.InstallScript,
		InstallContainer:  egg.InstallContainer,
		InstallEntrypoint: egg.InstallEntrypoint,
		ConfigJSON:        string(egg.Config),
		FileDenylist:      string(egg.FileDenylist),
	}, nil
}
