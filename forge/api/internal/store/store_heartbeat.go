package store

import (
	"context"
	"errors"
	"fmt"
)

func (s *Store) SetNodeHeartbeatClassification(ctx context.Context, nodeID string, heartbeatState NodeHeartbeatState, actualState NodeActualState, recoveryCount int, reason string) (Node, Node, error) {
	previous, err := s.GetNode(ctx, nodeID)
	if err != nil {
		return Node{}, Node{}, fmt.Errorf("get node for heartbeat classification: %w", err)
	}
	commandTag, err := s.db.Exec(ctx, `
		UPDATE nodes
		SET heartbeat_state = $2::node_heartbeat_state,
		    heartbeat_state_changed_at = CASE WHEN heartbeat_state::text <> $2::text THEN now() ELSE heartbeat_state_changed_at END,
		    heartbeat_recovery_count = $3,
		    actual_state = $4::node_actual_state,
		    status = CASE WHEN desired_state = 'active' THEN $4::text ELSE desired_state::text END
		WHERE id = $1
	`, nodeID, heartbeatState, recoveryCount, actualState)
	if err != nil {
		return Node{}, Node{}, err
	}
	if commandTag.RowsAffected() == 0 {
		return Node{}, Node{}, errors.New("node not found")
	}
	if previous.HeartbeatState != string(heartbeatState) {
		if err := s.recordStateTransition(ctx, "node", nodeID, "heartbeat", previous.HeartbeatState, string(heartbeatState), reason); err != nil {
			return Node{}, Node{}, err
		}
	}
	if previous.ActualState != string(actualState) {
		if err := s.recordStateTransition(ctx, "node", nodeID, "actual", previous.ActualState, string(actualState), reason); err != nil {
			return Node{}, Node{}, err
		}
	}
	updated, err := s.GetNode(ctx, nodeID)
	if err != nil {
		return Node{}, Node{}, err
	}
	return previous, updated, nil
}
