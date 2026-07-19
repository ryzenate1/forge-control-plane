package store

import (
	"context"
	"errors"
	"strings"
	"time"
)

type CloudNodeLink struct {
	Provider   string    `json:"provider"`
	InstanceID string    `json:"instanceId"`
	NodeID     string    `json:"nodeId"`
	CreatedAt  time.Time `json:"createdAt"`
}

func (s *Store) ListCloudNodeLinks(ctx context.Context) ([]CloudNodeLink, error) {
	rows, err := s.db.Query(ctx, `
		SELECT provider, instance_id, node_id::text, created_at
		FROM cloud_node_links
		ORDER BY provider, instance_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	links := make([]CloudNodeLink, 0)
	for rows.Next() {
		var link CloudNodeLink
		if err := rows.Scan(&link.Provider, &link.InstanceID, &link.NodeID, &link.CreatedAt); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (s *Store) CreateCloudNodeLink(ctx context.Context, link CloudNodeLink) error {
	link.Provider = strings.TrimSpace(link.Provider)
	link.InstanceID = strings.TrimSpace(link.InstanceID)
	link.NodeID = strings.TrimSpace(link.NodeID)
	if link.Provider == "" || link.InstanceID == "" || link.NodeID == "" {
		return errors.New("provider, instance ID, and node ID are required")
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO cloud_node_links (provider, instance_id, node_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (provider, instance_id) DO UPDATE SET node_id = EXCLUDED.node_id
	`, link.Provider, link.InstanceID, link.NodeID)
	return err
}

func (s *Store) DeleteCloudNodeLink(ctx context.Context, provider, instanceID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM cloud_node_links WHERE provider = $1 AND instance_id = $2`, strings.TrimSpace(provider), strings.TrimSpace(instanceID))
	return err
}
