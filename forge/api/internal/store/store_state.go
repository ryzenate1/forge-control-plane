package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

func (s *Store) SetServerDesiredState(ctx context.Context, serverID string, desired ServerDesiredState, reason string) error {
	var previous string
	if err := s.db.QueryRow(ctx, `SELECT desired_state::text FROM servers WHERE id = $1`, serverID).Scan(&previous); err != nil {
		return errors.New("server not found")
	}
	commandTag, err := s.db.Exec(ctx, `UPDATE servers SET
		desired_generation = desired_generation + CASE WHEN desired_state IS DISTINCT FROM $1::server_desired_state THEN 1 ELSE 0 END,
		desired_state = $1::server_desired_state,
		last_reconcile_error = '' WHERE id = $2`, string(desired), serverID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("server not found")
	}
	if previous != string(desired) {
		return s.recordStateTransition(ctx, "server", serverID, "desired", previous, string(desired), reason)
	}
	return nil
}

func (s *Store) SetServerActualState(ctx context.Context, serverID string, actual ServerActualState, reason string) error {
	var previous string
	if err := s.db.QueryRow(ctx, `SELECT actual_state::text FROM servers WHERE id = $1`, serverID).Scan(&previous); err != nil {
		return errors.New("server not found")
	}
	status := serverStatusFromActual(actual)
	commandTag, err := s.db.Exec(ctx, `UPDATE servers SET actual_state = $1::server_actual_state, status = $2,
		last_observation_at = NOW(),
		observed_generation = CASE
			WHEN (desired_state='running' AND $1::text='running') OR (desired_state='stopped' AND $1::text='stopped')
			THEN desired_generation ELSE observed_generation END,
		last_reconcile_error = '' WHERE id = $3`, string(actual), status, serverID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("server not found")
	}
	if previous != string(actual) {
		return s.recordStateTransition(ctx, "server", serverID, "actual", previous, string(actual), reason)
	}
	return nil
}

func (s *Store) SetNodeDesiredState(ctx context.Context, nodeID string, desired NodeDesiredState, reason string) error {
	var previous string
	if err := s.db.QueryRow(ctx, `SELECT desired_state::text FROM nodes WHERE id = $1`, nodeID).Scan(&previous); err != nil {
		return errors.New("node not found")
	}
	maintenance := desired == NodeDesiredStateMaintenance
	draining := desired == NodeDesiredStateDraining
	commandTag, err := s.db.Exec(ctx, `
		UPDATE nodes
		SET desired_state = $1::node_desired_state,
		    desired_generation = desired_generation + CASE WHEN desired_state IS DISTINCT FROM $1::node_desired_state THEN 1 ELSE 0 END,
		    maintenance_mode = $2,
		    draining = $3,
		    status = CASE WHEN $1::text = 'active' THEN actual_state::text ELSE $1::text END
		WHERE id = $4
	`, string(desired), maintenance, draining, nodeID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("node not found")
	}
	if previous != string(desired) {
		return s.recordStateTransition(ctx, "node", nodeID, "desired", previous, string(desired), reason)
	}
	return nil
}

func (s *Store) SetNodeActualState(ctx context.Context, nodeID string, actual NodeActualState, reason string) error {
	var previous string
	if err := s.db.QueryRow(ctx, `SELECT actual_state::text FROM nodes WHERE id = $1`, nodeID).Scan(&previous); err != nil {
		return errors.New("node not found")
	}
	commandTag, err := s.db.Exec(ctx, `
		UPDATE nodes
		SET actual_state = $1::node_actual_state,
		    last_observation_at = NOW(),
		    observed_generation = CASE WHEN desired_state='active' AND $1::text='online' THEN desired_generation ELSE observed_generation END,
		    status = CASE WHEN desired_state = 'active' THEN $1::text ELSE desired_state::text END
		WHERE id = $2
	`, string(actual), nodeID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("node not found")
	}
	if previous != string(actual) {
		return s.recordStateTransition(ctx, "node", nodeID, "actual", previous, string(actual), reason)
	}
	return nil
}

func (s *Store) StateTransitionsTotal(ctx context.Context) (int64, error) {
	var total int64
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM state_transitions`).Scan(&total)
	return total, err
}

func (s *Store) NodesDrainingTotal(ctx context.Context) (int64, error) {
	var total int64
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM nodes WHERE desired_state = 'draining' OR COALESCE(draining, false) = true`).Scan(&total)
	return total, err
}

func (s *Store) recordStateTransition(ctx context.Context, resourceType, resourceID, stateKind, fromState, toState, reason string) error {
	if reason == "" {
		reason = "state updated"
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO state_transitions (id, resource_type, resource_id, state_kind, from_state, to_state, reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, uuid.NewString(), resourceType, resourceID, stateKind, fromState, toState, reason)
	return err
}

func serverStatusFromActual(actual ServerActualState) string {
	switch actual {
	case ServerActualStateRunning:
		return "running"
	case ServerActualStateInstalling:
		return "installing"
	case ServerActualStateRestoringBackup:
		return "restoring_backup"
	case ServerActualStateCrashed:
		return "install_failed"
	case ServerActualStateUnknown:
		return "unknown"
	default:
		return "stopped"
	}
}

func serverActualFromStatus(status string) ServerActualState {
	switch status {
	case "running":
		return ServerActualStateRunning
	case "installing":
		return ServerActualStateInstalling
	case "restoring_backup":
		return ServerActualStateRestoringBackup
	case "install_failed":
		return ServerActualStateCrashed
	case "stopped":
		return ServerActualStateStopped
	default:
		return ServerActualStateUnknown
	}
}

func serverDesiredFromSignal(signal string) ServerDesiredState {
	if signal == "start" || signal == "restart" {
		return ServerDesiredStateRunning
	}
	return ServerDesiredStateStopped
}

func serverActualFromSignal(signal string) ServerActualState {
	if signal == "start" || signal == "restart" {
		return ServerActualStateRunning
	}
	return ServerActualStateStopped
}
