package store

import (
	"context"
	"fmt"
)

type SeedEntry struct {
	Name string
	Run  func(ctx context.Context, store *Store) error
}

type Seeder struct {
	store   *Store
	entries []SeedEntry
}

func NewSeeder(store *Store) *Seeder {
	return &Seeder{
		store:   store,
		entries: make([]SeedEntry, 0),
	}
}

func (s *Seeder) Register(name string, fn func(ctx context.Context, store *Store) error) {
	s.entries = append(s.entries, SeedEntry{Name: name, Run: fn})
}

func (s *Seeder) Run(ctx context.Context) error {
	for _, entry := range s.entries {
		if err := entry.Run(ctx, s.store); err != nil {
			return fmt.Errorf("seed %s: %w", entry.Name, err)
		}
	}
	return nil
}

func DefaultSeeder(store *Store) *Seeder {
	s := NewSeeder(store)

	s.Register("default-roles", func(ctx context.Context, store *Store) error {
		roles := []struct {
			Name        string
			Key         string
			Description string
			IsAdmin     bool
		}{
			{Name: "Admin", Key: "admin", Description: "Full administrative access", IsAdmin: true},
			{Name: "User", Key: "user", Description: "Standard user access", IsAdmin: false},
		}
		for _, role := range roles {
			_, err := store.db.Exec(ctx, `
				INSERT INTO roles (name, key, description, is_admin)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (key) DO NOTHING
			`, role.Name, role.Key, role.Description, role.IsAdmin)
			if err != nil {
				return err
			}
		}
		return nil
	})

	s.Register("default-settings", func(ctx context.Context, store *Store) error {
		settings := []struct {
			Key   string
			Value string
		}{
			{Key: "app:name", Value: "GamePanel"},
			{Key: "app:locale", Value: "en"},
			{Key: "theme:primary", Value: "#ef4444"},
			{Key: "backup:retention_days", Value: "30"},
			{Key: "backup:schedule", Value: "0 3 * * *"},
			{Key: "node:heartbeat_interval", Value: "30"},
			{Key: "node:offline_threshold", Value: "90"},
			{Key: "mail:driver", Value: "log"},
		}
		for _, s := range settings {
			_, err := store.db.Exec(ctx, `
				INSERT INTO settings (key, value)
				VALUES ($1, $2)
				ON CONFLICT (key) DO NOTHING
			`, s.Key, s.Value)
			if err != nil {
				return err
			}
		}
		return nil
	})

	return s
}
