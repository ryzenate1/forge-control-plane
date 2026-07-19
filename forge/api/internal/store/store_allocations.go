package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

type AllocationNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListAllocationNodes returns the minimal node data required to create and filter allocations.
// It intentionally does not depend on the full node inventory query, which includes operational
// and configuration fields that are neither required nor appropriate for allocation management.
func (s *Store) ListAllocationNodes(ctx context.Context) ([]AllocationNode, error) {
	rows, err := s.db.Query(ctx, `SELECT id::text, name FROM nodes ORDER BY name LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes := []AllocationNode{}
	for rows.Next() {
		var node AllocationNode
		if err := rows.Scan(&node.ID, &node.Name); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func (s *Store) ListAllocations(ctx context.Context) ([]Allocation, error) {
	return s.ListAllocationsPaginated(ctx, 0, 1000)
}

func (s *Store) ListAllocationsPaginated(ctx context.Context, offset, limit int) ([]Allocation, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.Query(ctx, `
		SELECT a.id::text, n.name, s.name, a.ip::text, a.port, a.container_port, a.protocol, a.alias, COALESCE(a.notes, '')
		FROM allocations a
		JOIN nodes n ON n.id = a.node_id
		LEFT JOIN servers s ON s.id = a.server_id
		ORDER BY n.name, a.port
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	allocations := []Allocation{}
	for rows.Next() {
		var allocation Allocation
		var server, alias sql.NullString
		if err := rows.Scan(&allocation.ID, &allocation.Node, &server, &allocation.IP, &allocation.Port, &allocation.ContainerPort, &allocation.Protocol, &alias, &allocation.Notes); err != nil {
			return nil, err
		}
		if server.Valid {
			allocation.Server = &server.String
		}
		if alias.Valid && alias.String != "" {
			allocation.Alias = &alias.String
		}
		allocations = append(allocations, allocation)
	}
	return allocations, rows.Err()
}

func (s *Store) CreateAllocation(ctx context.Context, req CreateAllocationRequest, actorID *string) (Allocation, error) {
	allocations, err := s.CreateAllocations(ctx, []CreateAllocationRequest{req}, actorID)
	if err != nil {
		return Allocation{}, err
	}
	return allocations[0], nil
}

// CreateAllocations creates the complete request atomically so a range cannot be partially persisted.
func (s *Store) CreateAllocations(ctx context.Context, requests []CreateAllocationRequest, actorID *string) ([]Allocation, error) {
	if len(requests) == 0 {
		return nil, errors.New("at least one allocation is required")
	}

	allocations := make([]Allocation, 0, len(requests))
	for _, req := range requests {
		if req.NodeID == "" || strings.TrimSpace(req.IP) == "" || req.Port <= 0 || req.Port > 65535 {
			return nil, errors.New("nodeId, ip, and valid port are required")
		}
		ip := strings.TrimSpace(req.IP)
		if net.ParseIP(ip) == nil {
			return nil, errors.New("invalid IP address format")
		}
		protocol := strings.ToLower(strings.TrimSpace(req.Protocol))
		if protocol == "" {
			protocol = "tcp"
		}
		if protocol != "tcp" && protocol != "udp" {
			return nil, errors.New("protocol must be tcp or udp")
		}
		containerPort := req.ContainerPort
		if containerPort == 0 {
			containerPort = req.Port
		}
		if containerPort < 1 || containerPort > 65535 {
			return nil, errors.New("container port must be between 1 and 65535")
		}
		allocation := Allocation{ID: uuid.NewString(), IP: ip, Port: req.Port, ContainerPort: containerPort, Protocol: protocol, Notes: strings.TrimSpace(req.Notes)}
		if alias := strings.TrimSpace(req.Alias); alias != "" {
			allocation.Alias = &alias
		}
		allocations = append(allocations, allocation)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	for index, allocation := range allocations {
		req := requests[index]
		if _, err := tx.Exec(ctx, `
			INSERT INTO allocations (id, node_id, server_id, ip, port, container_port, protocol, alias, notes)
			VALUES ($1, $2, NULL, $3, $4, $5, $6, $7, $8)
		`, allocation.ID, req.NodeID, allocation.IP, allocation.Port, allocation.ContainerPort, allocation.Protocol, allocation.Alias, allocation.Notes); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return nil, fmt.Errorf("allocation with ip %q and port %d already exists on this node", allocation.IP, allocation.Port)
			}
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO audit_events (id, actor_id, action, target_type, target_id, metadata)
			VALUES ($1, $2, 'allocation created', 'allocation', $3, $4::jsonb)
		`, uuid.NewString(), actorID, allocation.ID, fmt.Sprintf(`{"nodeId":"%s","ip":"%s","port":%d}`, req.NodeID, allocation.IP, allocation.Port)); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Return the canonical list representation (with node/server display names).
	canonical, listErr := s.ListAllocations(ctx)
	if listErr == nil {
		byID := make(map[string]Allocation, len(canonical))
		for _, a := range canonical {
			byID[a.ID] = a
		}
		for index, a := range allocations {
			if candidate, ok := byID[a.ID]; ok {
				allocations[index] = candidate
			}
		}
	}
	return allocations, nil
}

func (s *Store) DeleteAllocation(ctx context.Context, allocationID string, actorID *string) error {
	return s.DeleteAllocations(ctx, []string{allocationID}, actorID)
}

// DeleteAllocations removes only unassigned allocations and records all audit events atomically.
// A failed ID rolls back the entire batch so administrators never receive a partial bulk delete.
func (s *Store) DeleteAllocations(ctx context.Context, allocationIDs []string, actorID *string) error {
	if len(allocationIDs) == 0 {
		return errors.New("at least one allocation id is required")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, allocationID := range allocationIDs {
		commandTag, err := tx.Exec(ctx, `DELETE FROM allocations WHERE id = $1 AND server_id IS NULL`, allocationID)
		if err != nil {
			return err
		}
		if commandTag.RowsAffected() == 0 {
			return errors.New("allocation not found or assigned")
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO audit_events (id, actor_id, action, target_type, target_id, metadata)
			VALUES ($1, $2, 'allocation deleted', 'allocation', $3, '{"reason":"admin delete"}'::jsonb)
		`, uuid.NewString(), actorID, allocationID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (s *Store) UpdateAllocation(ctx context.Context, allocationID string, req UpdateAllocationRequest, actorID *string) (Allocation, error) {
	var alias *string
	if strings.TrimSpace(req.Alias) != "" {
		val := strings.TrimSpace(req.Alias)
		alias = &val
	}
	notes := strings.TrimSpace(req.Notes)
	commandTag, err := s.db.Exec(ctx, `UPDATE allocations SET alias = $1, notes = $2 WHERE id = $3`, alias, notes, allocationID)
	if err != nil {
		return Allocation{}, err
	}
	if commandTag.RowsAffected() == 0 {
		return Allocation{}, errors.New("allocation not found")
	}
	_ = s.AppendAudit(ctx, actorID, "allocation updated", "allocation", &allocationID, fmt.Sprintf(`{"alias":"%s"}`, strings.TrimSpace(req.Alias)))
	allocations, listErr := s.ListAllocations(ctx)
	if listErr == nil {
		for _, candidate := range allocations {
			if candidate.ID == allocationID {
				return candidate, nil
			}
		}
	}
	return Allocation{ID: allocationID, Alias: alias, Notes: notes}, nil
}

func (s *Store) UpdateServerAllocation(ctx context.Context, serverID, allocationID string, req UpdateAllocationRequest, actorID *string) (Allocation, error) {
	var alias *string
	if strings.TrimSpace(req.Alias) != "" {
		value := strings.TrimSpace(req.Alias)
		alias = &value
	}
	notes := strings.TrimSpace(req.Notes)
	tag, err := s.db.Exec(ctx, `UPDATE allocations SET alias = $1, notes = $2 WHERE id = $3 AND server_id = $4`, alias, notes, allocationID, serverID)
	if err != nil {
		return Allocation{}, err
	}
	if tag.RowsAffected() == 0 {
		return Allocation{}, errors.New("allocation is not assigned to this server")
	}
	_ = s.AppendAudit(ctx, actorID, "server allocation updated", "server", &serverID, fmt.Sprintf(`{"allocationId":"%s","alias":"%s"}`, allocationID, strings.TrimSpace(req.Alias)))
	for _, allocation := range mustListServerAllocations(ctx, s, serverID) {
		if allocation.ID == allocationID {
			return allocation, nil
		}
	}
	return Allocation{ID: allocationID, Alias: alias, Notes: notes}, nil
}

func mustListServerAllocations(ctx context.Context, s *Store, serverID string) []Allocation {
	allocations, err := s.ListServerAllocations(ctx, serverID)
	if err != nil {
		return nil
	}
	return allocations
}

func (s *Store) ListServerAllocations(ctx context.Context, serverID string) ([]Allocation, error) {
	rows, err := s.db.Query(ctx, `
		SELECT a.id::text, n.name, s.name, host(a.ip), a.port, a.container_port, a.protocol, a.alias, a.notes
		FROM allocations a
		JOIN nodes n ON n.id = a.node_id
		LEFT JOIN servers s ON s.id = a.server_id
		WHERE a.server_id = $1
		ORDER BY a.port
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	allocations := []Allocation{}
	for rows.Next() {
		var allocation Allocation
		if err := rows.Scan(&allocation.ID, &allocation.Node, &allocation.Server, &allocation.IP, &allocation.Port, &allocation.ContainerPort, &allocation.Protocol, &allocation.Alias, &allocation.Notes); err != nil {
			return nil, err
		}
		allocations = append(allocations, allocation)
	}
	return allocations, rows.Err()
}

func (s *Store) AssignAllocationToServer(ctx context.Context, serverID, allocationID string, actorID *string) error {
	var serverNodeID string
	if err := s.db.QueryRow(ctx, `SELECT node_id::text FROM servers WHERE id = $1`, serverID).Scan(&serverNodeID); err != nil {
		return err
	}
	var allocationNodeID string
	var assignedServerID *string
	if err := s.db.QueryRow(ctx, `SELECT node_id::text, server_id::text FROM allocations WHERE id = $1`, allocationID).Scan(&allocationNodeID, &assignedServerID); err != nil {
		return err
	}
	if assignedServerID != nil {
		return errors.New("allocation already assigned")
	}
	if allocationNodeID != serverNodeID {
		return errors.New("allocation does not belong to server node")
	}
	_, err := s.db.Exec(ctx, `UPDATE allocations SET server_id = $1 WHERE id = $2`, serverID, allocationID)
	if err != nil {
		return err
	}
	return s.AppendAudit(ctx, actorID, "allocation assigned", "server", &serverID, fmt.Sprintf(`{"allocationId":"%s"}`, allocationID))
}

func (s *Store) UnassignAllocationFromServer(ctx context.Context, serverID, allocationID string, actorID *string) error {
	// Prevent removing primary allocation.
	var primary sql.NullString
	_ = s.db.QueryRow(ctx, `SELECT primary_allocation_id::text FROM servers WHERE id = $1`, serverID).Scan(&primary)
	if primary.Valid && primary.String == allocationID {
		return errors.New("cannot unassign primary allocation")
	}
	commandTag, err := s.db.Exec(ctx, `UPDATE allocations SET server_id = NULL WHERE id = $1 AND server_id = $2`, allocationID, serverID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("allocation not assigned to server")
	}
	return s.AppendAudit(ctx, actorID, "allocation unassigned", "server", &serverID, fmt.Sprintf(`{"allocationId":"%s"}`, allocationID))
}

func (s *Store) SetPrimaryAllocation(ctx context.Context, serverID, allocationID string, actorID *string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var exists int
	if err := tx.QueryRow(ctx, `SELECT 1 FROM allocations WHERE id = $1 AND server_id = $2 FOR UPDATE`, allocationID, serverID).Scan(&exists); err != nil {
		return errors.New("allocation not assigned to server")
	}
	commandTag, err := tx.Exec(ctx, `UPDATE servers SET primary_allocation_id = $1, config_sync_pending = true, config_sync_error = NULL, updated_at = now() WHERE id = $2`, allocationID, serverID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("server not found")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (id, actor_id, action, target_type, target_id, metadata)
		VALUES ($1, $2, 'primary allocation set', 'server', $3, $4::jsonb)
	`, uuid.NewString(), actorID, serverID, fmt.Sprintf(`{"primaryAllocationId":"%s"}`, allocationID)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
