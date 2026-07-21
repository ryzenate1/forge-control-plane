package store

import (
	"context"
	"time"

	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ListServers(ctx context.Context) ([]Server, error) {
	rows, err := s.db.Query(ctx, `
		SELECT s.id::text, s.name, COALESCE(s.description, ''), s.status, s.desired_state::text, s.actual_state::text, s.config_sync_pending, s.suspended, s.transferring, s.transfer_target_node_id::text, s.transfer_state, s.transfer_error, s.transfer_run_token::text, s.memory_mb, s.cpu_shares, s.disk_mb, n.name, u.email, e.name
		FROM servers s
		JOIN nodes n ON n.id = s.node_id
		JOIN users u ON u.id = s.owner_id
		JOIN eggs e ON e.id = s.egg_id
		ORDER BY s.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	servers := []Server{}
	for rows.Next() {
		var server Server
		if err := rows.Scan(&server.ID, &server.Name, &server.Description, &server.Status, &server.DesiredState, &server.ActualState, &server.ConfigSyncPending, &server.Suspended, &server.Transferring, &server.TransferTargetNodeID, &server.TransferState, &server.TransferError, &server.TransferRunToken, &server.MemoryMB, &server.CPUShares, &server.DiskMB, &server.Node, &server.Owner, &server.Template); err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	return servers, rows.Err()
}

func (s *Store) ListServersForUser(ctx context.Context, userID, role string, page, perPage int, search string) ([]Server, int, error) {
	if role == "admin" {
		return s.ListServersPaginated(ctx, page, perPage, search)
	}

	offset := (page - 1) * perPage
	baseQuery := `
		SELECT id, name, description, status, desired_state, actual_state, config_sync_pending, suspended, transferring, transfer_target_node_id, transfer_state, transfer_error, transfer_run_token, memory_mb, cpu_shares, disk_mb, node_name, owner_email, template_name
		FROM (
			SELECT DISTINCT s.id::text AS id, s.name, COALESCE(s.description, '') AS description, s.status, s.desired_state::text AS desired_state, s.actual_state::text AS actual_state, s.config_sync_pending, s.suspended, s.transferring, s.transfer_target_node_id::text AS transfer_target_node_id, s.transfer_state, s.transfer_error, s.transfer_run_token::text AS transfer_run_token, s.memory_mb, s.cpu_shares, s.disk_mb, n.name AS node_name, u.email AS owner_email, e.name AS template_name, s.created_at
			FROM servers s
			JOIN nodes n ON n.id = s.node_id
			JOIN users u ON u.id = s.owner_id
			JOIN eggs e ON e.id = s.egg_id
			LEFT JOIN subusers su ON su.server_id = s.id AND su.user_id = $1
			WHERE s.owner_id = $1 OR su.user_id IS NOT NULL
		) AS server_inventory
		WHERE 1=1`

	countQuery := `
		SELECT COUNT(DISTINCT s.id)
		FROM servers s
		JOIN users u ON u.id = s.owner_id
		LEFT JOIN subusers su ON su.server_id = s.id AND su.user_id = $1
		WHERE s.owner_id = $1 OR su.user_id IS NOT NULL`

	args := []any{userID}
	argIndex := 2

	if search != "" {
		baseQuery += fmt.Sprintf(" AND (name ILIKE $%d OR description ILIKE $%d OR owner_email ILIKE $%d)", argIndex, argIndex+1, argIndex+2)
		countQuery += fmt.Sprintf(" AND (s.name ILIKE $%d OR s.description ILIKE $%d OR u.email ILIKE $%d)", argIndex, argIndex+1, argIndex+2)
		searchPattern := "%" + search + "%"
		args = append(args, searchPattern, searchPattern, searchPattern)
		argIndex += 3
	}

	baseQuery += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, perPage, offset)

	// Get total count
	var total int
	countArgs := args[:len(args)-2] // Remove limit and offset
	if err := s.db.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	servers := []Server{}
	for rows.Next() {
		var server Server
		if err := rows.Scan(&server.ID, &server.Name, &server.Description, &server.Status, &server.DesiredState, &server.ActualState, &server.ConfigSyncPending, &server.Suspended, &server.Transferring, &server.TransferTargetNodeID, &server.TransferState, &server.TransferError, &server.TransferRunToken, &server.MemoryMB, &server.CPUShares, &server.DiskMB, &server.Node, &server.Owner, &server.Template); err != nil {
			return nil, 0, err
		}
		servers = append(servers, server)
	}
	return servers, total, rows.Err()
}

func (s *Store) ListServersPaginated(ctx context.Context, page, perPage int, search string) ([]Server, int, error) {
	offset := (page - 1) * perPage
	baseQuery := `
		SELECT id, name, description, status, desired_state, actual_state, config_sync_pending, suspended, transferring, transfer_target_node_id, transfer_state, transfer_error, transfer_run_token, memory_mb, cpu_shares, disk_mb, node_name, owner_email, template_name
		FROM (
			SELECT DISTINCT s.id::text AS id, s.name, COALESCE(s.description, '') AS description, s.status, s.desired_state::text AS desired_state, s.actual_state::text AS actual_state, s.config_sync_pending, s.suspended, s.transferring, s.transfer_target_node_id::text AS transfer_target_node_id, s.transfer_state, s.transfer_error, s.transfer_run_token::text AS transfer_run_token, s.memory_mb, s.cpu_shares, s.disk_mb, n.name AS node_name, u.email AS owner_email, e.name AS template_name, s.created_at
			FROM servers s
			JOIN nodes n ON n.id = s.node_id
			JOIN users u ON u.id = s.owner_id
			JOIN eggs e ON e.id = s.egg_id
			WHERE 1=1
		) AS server_inventory
		WHERE 1=1`

	countQuery := `
		SELECT COUNT(DISTINCT s.id)
		FROM servers s
		JOIN users u ON u.id = s.owner_id
		WHERE 1=1`

	args := []any{}
	argIndex := 1

	if search != "" {
		baseQuery += fmt.Sprintf(" AND (name ILIKE $%d OR description ILIKE $%d OR owner_email ILIKE $%d)", argIndex, argIndex+1, argIndex+2)
		countQuery += fmt.Sprintf(" AND (s.name ILIKE $%d OR s.description ILIKE $%d OR u.email ILIKE $%d)", argIndex, argIndex+1, argIndex+2)
		searchPattern := "%" + search + "%"
		args = append(args, searchPattern, searchPattern, searchPattern)
		argIndex += 3
	}

	baseQuery += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, perPage, offset)

	// Get total count
	var total int
	countArgs := args[:len(args)-2] // Remove limit and offset
	if err := s.db.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	servers := []Server{}
	for rows.Next() {
		var server Server
		if err := rows.Scan(&server.ID, &server.Name, &server.Description, &server.Status, &server.DesiredState, &server.ActualState, &server.ConfigSyncPending, &server.Suspended, &server.Transferring, &server.TransferTargetNodeID, &server.TransferState, &server.TransferError, &server.TransferRunToken, &server.MemoryMB, &server.CPUShares, &server.DiskMB, &server.Node, &server.Owner, &server.Template); err != nil {
			return nil, 0, err
		}
		servers = append(servers, server)
	}
	return servers, total, rows.Err()
}

func (s *Store) CreateServer(ctx context.Context, req CreateServerRequest) (Server, error) {
	serverID := uuid.NewString()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Server{}, err
	}
	defer tx.Rollback(ctx)

	var ownerExists, nodeExists bool
	var defaultMemoryMB int
	var defaultImage string
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM users WHERE id = $1),
		       EXISTS (SELECT 1 FROM nodes WHERE id = $3),
		       COALESCE((SELECT default_memory_mb FROM eggs WHERE id = $2), 0),
		       COALESCE((SELECT value FROM eggs e, LATERAL jsonb_each_text(e.docker_images) WHERE e.id = $2 ORDER BY key LIMIT 1), '')
	`, req.OwnerID, req.TemplateID, req.NodeID).Scan(&ownerExists, &nodeExists, &defaultMemoryMB, &defaultImage); err != nil {
		return Server{}, err
	}
	templateExists := defaultMemoryMB > 0
	if req.MemoryMB <= 0 {
		req.MemoryMB = defaultMemoryMB
	}
	if req.MemoryMB <= 0 || req.CPUShares <= 0 || req.CPULimit < 0 || req.DiskMB <= 0 || req.DatabaseLimit < 0 || req.BackupLimit < 0 || req.AllocationLimit < 0 || req.IOWeight < 10 || req.IOWeight > 1000 || req.SwapMB < -1 {
		return Server{}, errors.New("invalid server resource limits")
	}
	if !ownerExists {
		return Server{}, errors.New("owner not found")
	}
	if !templateExists {
		return Server{}, errors.New("egg not found")
	}
	if strings.TrimSpace(req.DockerImage) == "" && defaultImage == "" {
		return Server{}, errors.New("egg has no docker images")
	}
	if !nodeExists {
		return Server{}, errors.New("node not found")
	}

	allocationIDs := append([]string{req.AllocationID}, req.AdditionalAllocationIDs...)
	seen := map[string]bool{}
	for _, allocationID := range allocationIDs {
		if allocationID == "" || seen[allocationID] {
			continue
		}
		seen[allocationID] = true
		var allocationNodeID string
		var assignedServerID *string
		if err := tx.QueryRow(ctx, `SELECT node_id::text, server_id::text FROM allocations WHERE id = $1 FOR UPDATE`, allocationID).Scan(&allocationNodeID, &assignedServerID); err != nil {
			return Server{}, errors.New("allocation not found")
		}
		if assignedServerID != nil {
			return Server{}, errors.New("allocation already assigned")
		}
		if allocationNodeID != req.NodeID {
			return Server{}, errors.New("allocation does not belong to selected node")
		}
	}
	if req.AllocationID == "" {
		return Server{}, errors.New("primary allocation is required")
	}
	if req.AllocationLimit > 0 && len(seen) > req.AllocationLimit {
		return Server{}, errors.New("requested allocations exceed allocation limit")
	}

	serverUUID := uuid.NewString()
	uuidShort := serverUUID[:8]
	_, err = tx.Exec(ctx, `
		INSERT INTO servers (
			id, node_id, owner_id, template_id, egg_id, name, status, desired_state, actual_state,
			memory_mb, cpu_shares, cpu_limit, disk_mb, database_limit, backup_limit,
			allocation_limit, io_weight, swap_mb, threads, oom_disabled, docker_image,
			startup_command, primary_allocation_id, installed, config_sync_pending,
			uuid, uuid_short, skip_scripts, docker_labels
		)
		VALUES ($1, $2, $3, $4, $4, $5, 'provisioning', 'stopped', 'stopped',
			$6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, false, false,
			$20, $21, $22, $23::jsonb)
	`, serverID, req.NodeID, req.OwnerID, req.TemplateID, strings.TrimSpace(req.Name), req.MemoryMB, req.CPUShares, req.CPULimit, req.DiskMB, req.DatabaseLimit, req.BackupLimit, req.AllocationLimit, req.IOWeight, req.SwapMB, strings.TrimSpace(req.Threads), req.OOMDisabled, strings.TrimSpace(req.DockerImage), strings.TrimSpace(req.StartupCommand), req.AllocationID, serverUUID, uuidShort, req.SkipScripts, req.DockerLabels)
	if err != nil {
		return Server{}, err
	}
	for allocationID := range seen {
		if _, err = tx.Exec(ctx, `UPDATE allocations SET server_id = $1, assigned_at = now() WHERE id = $2`, serverID, allocationID); err != nil {
			return Server{}, err
		}
	}
	for key, value := range req.StartupVariables {
		var rules string
		if err := tx.QueryRow(ctx, `SELECT rules FROM egg_variables WHERE egg_id = $1 AND env_variable = $2`, req.TemplateID, key).Scan(&rules); err != nil {
			return Server{}, fmt.Errorf("startup variable %q is not defined by egg", key)
		}
		if err := validateVariableValue(value, rules); err != nil {
			return Server{}, fmt.Errorf("startup variable %q: %w", key, err)
		}
		commandTag, err := tx.Exec(ctx, `
			INSERT INTO server_variables (server_id, variable_id, variable_value, updated_at)
			SELECT $1, ev.id, $3, now()
			FROM egg_variables ev
			WHERE ev.egg_id = $2 AND ev.env_variable = $4
			ON CONFLICT (server_id, variable_id)
			DO UPDATE SET variable_value = EXCLUDED.variable_value, updated_at = now()
		`, serverID, req.TemplateID, value, key)
		if err != nil {
			return Server{}, err
		}
		if commandTag.RowsAffected() == 0 {
			return Server{}, fmt.Errorf("startup variable %q is not defined by egg", key)
		}
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO audit_events (id, actor_id, action, target_type, target_id, metadata)
		VALUES ($1, $2, 'server provisioning started', 'server', $3, $4::jsonb)
	`, uuid.NewString(), req.OwnerID, serverID, fmt.Sprintf(`{"allocationId":"%s"}`, req.AllocationID)); err != nil {
		return Server{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Server{}, err
	}
	return s.GetServer(ctx, serverID)
}

func (s *Store) GetServer(ctx context.Context, serverID string) (Server, error) {
	var server Server
	err := s.db.QueryRow(ctx, `
		SELECT s.id::text, s.name, COALESCE(s.description, ''), s.status,
		       s.desired_state::text, s.actual_state::text, s.suspended, s.transferring,
		       s.transfer_target_node_id::text, s.transfer_state, s.transfer_error, s.transfer_run_token::text,
		       s.memory_mb, s.cpu_shares, s.cpu_limit, s.disk_mb, s.database_limit, s.backup_limit,
		       s.allocation_limit, s.io_weight, s.swap_mb, COALESCE(s.threads, ''), s.oom_disabled,
		       COALESCE(NULLIF(s.docker_image, ''), (SELECT value FROM jsonb_each_text(e.docker_images) ORDER BY key LIMIT 1), ''),
		       COALESCE(NULLIF(s.startup_command, ''), e.startup),
		       s.primary_allocation_id::text, s.config_sync_pending, s.config_sync_error,
		       n.name, n.id::text, COALESCE(NULLIF(n.fqdn, ''), n.base_url), COALESCE(n.daemon_sftp, 2022),
		       u.email, u.id::text, e.name,
		       COALESCE(s.uuid, ''), COALESCE(s.uuid_short, ''), s.installed_at, s.skip_scripts, s.docker_labels,
		       COALESCE(s.generation, 0), s.workload_lease_expiry
		FROM servers s
		JOIN nodes n ON n.id = s.node_id
		JOIN users u ON u.id = s.owner_id
		JOIN eggs e ON e.id = s.egg_id
		WHERE s.id = $1
	`, serverID).Scan(
		&server.ID, &server.Name, &server.Description, &server.Status, &server.DesiredState, &server.ActualState,
		&server.Suspended, &server.Transferring, &server.TransferTargetNodeID, &server.TransferState,
		&server.TransferError, &server.TransferRunToken, &server.MemoryMB, &server.CPUShares, &server.CPULimit,
		&server.DiskMB, &server.DatabaseLimit, &server.BackupLimit, &server.AllocationLimit, &server.IOWeight,
		&server.SwapMB, &server.Threads, &server.OOMDisabled, &server.DockerImage, &server.StartupCommand,
		&server.PrimaryAllocationID, &server.ConfigSyncPending, &server.ConfigSyncError,
		&server.Node, &server.NodeID, &server.SFTPHost, &server.SFTPPort,
		&server.Owner, &server.OwnerID, &server.Template,
		&server.Uuid, &server.UuidShort, &server.InstalledAt, &server.SkipScripts, &server.DockerLabels,
		&server.Generation, &server.WorkloadLeaseExpiry,
	)
	if err != nil {
		return Server{}, err
	}
	return server, nil
}

func (s *Store) IsServerTransferBlocking(ctx context.Context, serverID string) (bool, error) {
	var state string
	err := s.db.QueryRow(ctx, `
		SELECT transfer_state
		FROM servers
		WHERE id = $1
	`, serverID).Scan(&state)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, errors.New("server not found")
		}
		return false, err
	}
	return state == "queued" || state == "running", nil
}

func (s *Store) UpdateServerTransferState(ctx context.Context, serverID, state string, targetNodeID *string, error *string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE servers
		SET transfer_state = $1,
		    transfer_target_node_id = $2,
		    transfer_error = $3,
		    transferring = ($1 = 'queued' OR $1 = 'running')
		WHERE id = $4
	`, state, targetNodeID, error, serverID)
	return err
}

func (s *Store) SetServerSuspension(ctx context.Context, serverID string, suspended bool) error {
	commandTag, err := s.db.Exec(ctx, `UPDATE servers SET suspended = $1 WHERE id = $2`, suspended, serverID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("server not found")
	}
	return nil
}

func (s *Store) SetServerSuspended(ctx context.Context, serverID string, suspended bool, actorID *string) error {
	commandTag, err := s.db.Exec(ctx, `UPDATE servers SET suspended = $1 WHERE id = $2`, suspended, serverID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("server not found")
	}
	action := "server unsuspended"
	if suspended {
		action = "server suspended"
	}
	return s.AppendAudit(ctx, actorID, action, "server", &serverID, `{"scope":"admin"}`)
}

func (s *Store) ToggleServerInstallStatus(ctx context.Context, serverID string, actorID *string) (string, error) {
	var status string
	if err := s.db.QueryRow(ctx, `SELECT status FROM servers WHERE id = $1`, serverID).Scan(&status); err != nil {
		return "", errors.New("server not found")
	}
	next := "installing"
	if status == "installing" {
		next = "stopped"
	}
	if err := s.SetServerStatus(ctx, serverID, next, "admin install status toggled"); err != nil {
		return "", err
	}
	_ = s.AppendAudit(ctx, actorID, "server install status toggled", "server", &serverID, fmt.Sprintf(`{"from":"%s","to":"%s"}`, status, next))
	return next, nil
}

func (s *Store) UpdateServer(ctx context.Context, serverID string, req UpdateServerRequest, actorID *string) (Server, error) {
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			return Server{}, errors.New("name cannot be empty")
		}
		req.Name = &trimmed
	}
	if req.OwnerID != nil && strings.TrimSpace(*req.OwnerID) == "" {
		return Server{}, errors.New("ownerId cannot be empty")
	}
	if invalidOptionalInt(req.MemoryMB, 0, false) || invalidOptionalInt(req.CPUShares, 0, false) || invalidOptionalInt(req.CPULimit, 0, false) || invalidOptionalInt(req.DiskMB, 0, false) || invalidOptionalInt(req.DatabaseLimit, 0, false) || invalidOptionalInt(req.BackupLimit, 0, false) || invalidOptionalInt(req.AllocationLimit, 0, false) || invalidOptionalInt(req.IOWeight, 10, true) || invalidOptionalInt(req.SwapMB, -1, true) {
		return Server{}, errors.New("invalid server resource limits")
	}
	if req.IOWeight != nil && *req.IOWeight > 1000 {
		return Server{}, errors.New("ioWeight must be between 10 and 1000")
	}
	trimOptionalString(req.Description)
	trimOptionalString(req.Threads)
	trimOptionalString(req.DockerImage)
	trimOptionalString(req.StartupCommand)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Server{}, err
	}
	defer tx.Rollback(ctx)
	var serverExists int
	if err := tx.QueryRow(ctx, `SELECT 1 FROM servers WHERE id = $1 FOR UPDATE`, serverID).Scan(&serverExists); err != nil {
		return Server{}, errors.New("server not found")
	}
	if req.OwnerID != nil {
		var ownerExists int
		if err := tx.QueryRow(ctx, `SELECT 1 FROM users WHERE id = $1`, *req.OwnerID).Scan(&ownerExists); err != nil {
			return Server{}, errors.New("owner not found")
		}
	}
	if req.PrimaryAllocationID != nil {
		if strings.TrimSpace(*req.PrimaryAllocationID) == "" {
			return Server{}, errors.New("primaryAllocationId cannot be empty")
		}
		var exists int
		if err := tx.QueryRow(ctx, `SELECT 1 FROM allocations WHERE id = $1 AND server_id = $2 FOR UPDATE`, *req.PrimaryAllocationID, serverID).Scan(&exists); err != nil {
			return Server{}, errors.New("allocation not assigned to server")
		}
	}
	runtimeChanged := req.Name != nil || req.MemoryMB != nil || req.CPUShares != nil || req.CPULimit != nil || req.DiskMB != nil || req.IOWeight != nil || req.SwapMB != nil || req.Threads != nil || req.OOMDisabled != nil || req.DockerImage != nil || req.StartupCommand != nil || req.PrimaryAllocationID != nil
	commandTag, err := tx.Exec(ctx, `
		UPDATE servers SET
			name = COALESCE($2, name), description = COALESCE($3, description), owner_id = COALESCE($4::uuid, owner_id),
			memory_mb = COALESCE($5, memory_mb), cpu_shares = COALESCE($6, cpu_shares), cpu_limit = COALESCE($7, cpu_limit),
			disk_mb = COALESCE($8, disk_mb), database_limit = COALESCE($9, database_limit), backup_limit = COALESCE($10, backup_limit),
			allocation_limit = COALESCE($11, allocation_limit), io_weight = COALESCE($12, io_weight), swap_mb = COALESCE($13, swap_mb),
			threads = COALESCE($14, threads), oom_disabled = COALESCE($15, oom_disabled), docker_image = COALESCE($16, docker_image),
			startup_command = COALESCE($17, startup_command), primary_allocation_id = COALESCE($18::uuid, primary_allocation_id),
			config_sync_pending = CASE WHEN $19 THEN TRUE ELSE config_sync_pending END,
			config_sync_error = CASE WHEN $19 THEN NULL ELSE config_sync_error END, updated_at = now()
		WHERE id = $1
	`, serverID, req.Name, req.Description, req.OwnerID, req.MemoryMB, req.CPUShares, req.CPULimit, req.DiskMB, req.DatabaseLimit, req.BackupLimit, req.AllocationLimit, req.IOWeight, req.SwapMB, req.Threads, req.OOMDisabled, req.DockerImage, req.StartupCommand, req.PrimaryAllocationID, runtimeChanged)
	if err != nil {
		return Server{}, err
	}
	if commandTag.RowsAffected() == 0 {
		return Server{}, errors.New("server not found")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (id, actor_id, action, target_type, target_id, metadata) VALUES ($1, $2, 'server:settings.update', 'server', $3, '{}'::jsonb)`, uuid.NewString(), actorID, serverID); err != nil {
		return Server{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Server{}, err
	}
	return s.GetServer(ctx, serverID)
}

func (s *Store) UpdateServerGeneration(ctx context.Context, serverID string, generation int64, leaseExpiry *time.Time) error {
	commandTag, err := s.db.Exec(ctx, `
		UPDATE servers SET generation = $2, workload_lease_expiry = $3, updated_at = now()
		WHERE id = $1
	`, serverID, generation, leaseExpiry)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("server not found")
	}
	return nil
}

func invalidOptionalInt(value *int, minimum int, strict bool) bool {
	if value == nil {
		return false
	}
	if strict {
		return *value < minimum
	}
	return *value < 0
}

func trimOptionalString(value *string) {
	if value != nil {
		trimmed := strings.TrimSpace(*value)
		*value = trimmed
	}
}
