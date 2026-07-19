package store

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// GetNodeDaemonToken returns the node's current daemon bearer token (the part
// after the `.` separator in `<id>.<token>`). It is reserved for onboarding
// responses that must configure the daemon.
func (s *Store) GetNodeDaemonToken(ctx context.Context, nodeID string) (string, error) {
	if s.db == nil {
		return "", errors.New("no database connection")
	}
	var plaintext, encrypted string
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(daemon_token, ''), COALESCE(daemon_token_encrypted, '')
		FROM nodes
		WHERE id::text = $1
	`, nodeID).Scan(&plaintext, &encrypted)
	if err != nil {
		return "", err
	}
	return s.decryptSecret(encrypted, plaintext, secretAAD("nodes", nodeID, "daemon_token"))
}

// SetNodeCloudEndpoint records the private/public address assigned by a cloud
// provider so Forge can reach the bootstrapped Beacon immediately.
func (s *Store) SetNodeCloudEndpoint(ctx context.Context, nodeID, host string, port int) error {
	if net.ParseIP(host) == nil {
		return errors.New("cloud instance did not return a valid node IP")
	}
	if port < 1 || port > 65535 {
		return errors.New("invalid Beacon port")
	}
	baseURL := fmt.Sprintf("http://%s", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	tag, err := s.db.Exec(ctx, `UPDATE nodes SET base_url=$2, fqdn=$3, scheme='http', daemon_listen=$4, updated_at=now() WHERE id=$1`, nodeID, baseURL, host, port)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("node not found")
	}
	return nil
}

// GetNodeDaemonCredential returns the complete credential used to sign panel
// requests to a node. Unlike GetNodeDaemonToken, this is not an onboarding DTO.
func (s *Store) GetNodeDaemonCredential(ctx context.Context, nodeID string) (string, error) {
	if s.db == nil {
		return "", errors.New("no database connection")
	}
	var tokenID string
	if err := s.db.QueryRow(ctx, `SELECT COALESCE(daemon_token_id, '') FROM nodes WHERE id::text = $1`, nodeID).Scan(&tokenID); err != nil {
		return "", err
	}
	token, err := s.GetNodeDaemonToken(ctx, nodeID)
	if err != nil || tokenID == "" || token == "" {
		return "", err
	}
	return tokenID + "." + token, nil
}
