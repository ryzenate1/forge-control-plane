package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// Phase 6 env-affinity rows. These types read straight from the migration
// 190 columns (servers.env_affinity, nodes.env_groups) without touching the
// frozen Node/Server structs in store.go.

// ServerEnvAffinity pairs a server with the environment group it pins.
type ServerEnvAffinity struct {
	ServerID  string    `json:"serverId"`
	Name      string    `json:"name"`
	EnvGroup  string    `json:"envGroup"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// NodeEnvGroup advertises the environment groups a node serves.
type NodeEnvGroup struct {
	NodeID     string    `json:"nodeId"`
	Name       string    `json:"name"`
	EnvGroups  []string  `json:"envGroups"`
	UpdatedAt  time.Time `json:"updatedAt"`
	LabelCount int       `json:"labelCount"`
}

// SetServerEnvAffinity pins a server to an environment group.
func (s *Store) SetServerEnvAffinity(ctx context.Context, serverID, envGroup string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE servers SET env_affinity = $2, updated_at = now() WHERE id = $1
	`, serverID, strings.TrimSpace(envGroup))
	return err
}

// ListServerEnvAffinities returns every server that declared env_affinity.
func (s *Store) ListServerEnvAffinities(ctx context.Context) ([]ServerEnvAffinity, error) {
	rows, err := s.db.Query(ctx, `
		SELECT s.id::text, s.name, COALESCE(s.env_affinity, ''), s.updated_at
		FROM servers s
		WHERE COALESCE(s.env_affinity, '') <> ''
		ORDER BY s.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	affinities := []ServerEnvAffinity{}
	for rows.Next() {
		var a ServerEnvAffinity
		if err := rows.Scan(&a.ServerID, &a.Name, &a.EnvGroup, &a.UpdatedAt); err != nil {
			return nil, err
		}
		affinities = append(affinities, a)
	}
	return affinities, rows.Err()
}

// SetNodeEnvGroups replaces the environment groups advertised by a node.
func (s *Store) SetNodeEnvGroups(ctx context.Context, nodeID string, envGroups []string) error {
	if envGroups == nil {
		envGroups = []string{}
	}
	_, err := s.db.Exec(ctx, `
		UPDATE nodes SET env_groups = $2 WHERE id = $1
	`, nodeID, envGroups)
	return err
}

// ListNodeEnvGroups returns all nodes with at least one environment group.
func (s *Store) ListNodeEnvGroups(ctx context.Context) ([]NodeEnvGroup, error) {
	rows, err := s.db.Query(ctx, `
		SELECT n.id::text, n.name, COALESCE(n.env_groups, '{}'), COALESCE(n.labels, '[]'), n.updated_at
		FROM nodes n
		WHERE cardinality(COALESCE(n.env_groups, '{}')) > 0
		ORDER BY n.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := []NodeEnvGroup{}
	for rows.Next() {
		var g NodeEnvGroup
		var labelsRaw string
		if err := rows.Scan(&g.NodeID, &g.Name, &g.EnvGroups, &labelsRaw, &g.UpdatedAt); err != nil {
			return nil, err
		}
		var labels []LabelPair
		if json.Unmarshal([]byte(labelsRaw), &labels) == nil {
			g.LabelCount = len(labels)
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

