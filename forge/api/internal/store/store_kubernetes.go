package store

import (
	"context"
)

type NodeDaemonTarget struct {
	NodeID    string
	NodeURL   string
	NodeToken string
}

func (s *Store) NodeDaemonTarget(ctx context.Context, nodeID string) (NodeDaemonTarget, error) {
	var target NodeDaemonTarget
	var tokenID, daemonToken, daemonTokenEncrypted string
	err := s.db.QueryRow(ctx, `
		SELECT n.id::text, n.base_url,
		       COALESCE(n.daemon_token_id, ''),
		       COALESCE(n.daemon_token, ''),
		       COALESCE(n.daemon_token_encrypted, '')
		FROM nodes n
		WHERE n.id = $1
	`, nodeID).Scan(&target.NodeID, &target.NodeURL, &tokenID, &daemonToken, &daemonTokenEncrypted)
	if err != nil {
		return NodeDaemonTarget{}, err
	}
	token, err := s.decryptSecret(daemonTokenEncrypted, daemonToken, secretAAD("nodes", nodeID, "daemon_token"))
	if err != nil {
		return NodeDaemonTarget{}, err
	}
	if tokenID != "" && token != "" {
		target.NodeToken = tokenID + "." + token
	}
	return target, nil
}
