package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

type OnboardingToken struct {
	ID           string    `json:"id"`
	TokenHash    string    `json:"-"`
	NodeID       string    `json:"nodeId"`
	CreatedAt    time.Time `json:"createdAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
	ApprovedAt   *time.Time `json:"approvedAt,omitempty"`
	ApprovedBy   string    `json:"approvedBy,omitempty"`
	RevokedAt    *time.Time `json:"revokedAt,omitempty"`
	RevokedReason string   `json:"revokedReason,omitempty"`
	State        string    `json:"state"`
}

type NodeCapability struct {
	ID              string    `json:"id"`
	NodeID          string    `json:"nodeId"`
	BeaconVersion   string    `json:"beaconVersion"`
	OS              string    `json:"os"`
	Architecture    string    `json:"architecture"`
	CPUThreads      int       `json:"cpuThreads"`
	MemoryMB        int64     `json:"memoryMb"`
	DiskMB          int64     `json:"diskMb"`
	UptimeSeconds   int64     `json:"uptimeSeconds"`

	RuntimeAvailable bool   `json:"runtimeAvailable"`
	RuntimeStatus    string `json:"runtimeStatus"`
	RuntimeVersion   string `json:"runtimeVersion"`
	RuntimeProvider  string `json:"runtimeProvider"`

	DockerBuildEnabled bool `json:"dockerBuildEnabled"`
	NixpacksEnabled    bool `json:"nixpacksEnabled"`

	ComposeEnabled bool   `json:"composeEnabled"`
	ComposeVersion string `json:"composeVersion,omitempty"`
	StackCount     int    `json:"stackCount"`

	LocalBackups    bool `json:"localBackups"`
	S3Backups       bool `json:"s3Backups"`
	TransferEnabled bool `json:"transferEnabled"`

	SFTPEnabled      bool `json:"sftpEnabled"`
	WebSocketEnabled bool `json:"webSocketEnabled"`
	ConsoleEnabled   bool `json:"consoleEnabled"`

	DatabaseProvisioningEnabled bool `json:"databaseProvisioningEnabled"`

	RawReport json.RawMessage `json:"rawReport"`

	FetchedAt time.Time `json:"fetchedAt"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type CapabilityInventoryFilter struct {
	Offset int
	Limit  int
}

func (s *Store) CreateOnboardingToken(ctx context.Context, nodeID string, expiresAt time.Time) (*OnboardingToken, error) {
	id := uuid.NewString()
	tokenHash := uuid.NewString()
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO onboarding_tokens (id, token_hash, node_id, created_at, expires_at, state)
		VALUES ($1, $2, $3, $4, $5, 'pending')
	`, id, tokenHash, nodeID, now, expiresAt)
	if err != nil {
		return nil, err
	}
	return &OnboardingToken{
		ID:        id,
		TokenHash: tokenHash,
		NodeID:    nodeID,
		CreatedAt: now,
		ExpiresAt: expiresAt,
		State:     "pending",
	}, nil
}

func (s *Store) GetOnboardingToken(ctx context.Context, tokenID string) (*OnboardingToken, error) {
	var t OnboardingToken
	err := s.db.QueryRow(ctx, `
		SELECT id, token_hash, node_id, created_at, expires_at,
		       approved_at, COALESCE(approved_by, ''),
		       revoked_at, COALESCE(revoked_reason, ''),
		       state
		FROM onboarding_tokens WHERE id = $1
	`, tokenID).Scan(
		&t.ID, &t.TokenHash, &t.NodeID, &t.CreatedAt, &t.ExpiresAt,
		&t.ApprovedAt, &t.ApprovedBy,
		&t.RevokedAt, &t.RevokedReason,
		&t.State,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) ApproveOnboardingToken(ctx context.Context, tokenID, approvedBy string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE onboarding_tokens
		SET state = 'approved', approved_at = now(), approved_by = $2
		WHERE id = $1 AND state = 'pending'
	`, tokenID, approvedBy)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("token not found or already processed")
	}
	return nil
}

func (s *Store) RejectOnboardingToken(ctx context.Context, tokenID, reason string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE onboarding_tokens
		SET state = 'rejected', revoked_at = now(), revoked_reason = $2
		WHERE id = $1 AND state = 'pending'
	`, tokenID, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("token not found or already processed")
	}
	return nil
}

func (s *Store) RevokeOnboardingToken(ctx context.Context, tokenID, reason string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE onboarding_tokens
		SET state = 'revoked', revoked_at = now(), revoked_reason = $2
		WHERE id = $1 AND state IN ('pending', 'approved')
	`, tokenID, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("token not found or not active")
	}
	return nil
}

func (s *Store) ListOnboardingTokens(ctx context.Context, nodeID string) ([]OnboardingToken, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, token_hash, node_id, created_at, expires_at,
		       approved_at, COALESCE(approved_by, ''),
		       revoked_at, COALESCE(revoked_reason, ''),
		       state
		FROM onboarding_tokens
		WHERE node_id = $1
		ORDER BY created_at DESC
	`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []OnboardingToken
	for rows.Next() {
		var t OnboardingToken
		if err := rows.Scan(
			&t.ID, &t.TokenHash, &t.NodeID, &t.CreatedAt, &t.ExpiresAt,
			&t.ApprovedAt, &t.ApprovedBy,
			&t.RevokedAt, &t.RevokedReason,
			&t.State,
		); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (s *Store) UpsertNodeCapability(ctx context.Context, nc *NodeCapability) error {
	id := uuid.NewString()
	now := time.Now().UTC()

	rawJSON, err := json.Marshal(nc.RawReport)
	if err != nil {
		return err
	}

	tag, err := s.db.Exec(ctx, `
		INSERT INTO node_capabilities (
			id, node_id, beacon_version, os, architecture, cpu_threads, memory_mb, disk_mb, uptime_seconds,
			runtime_available, runtime_status, runtime_version, runtime_provider,
			docker_build_enabled, nixpacks_enabled,
			compose_enabled, compose_version, compose_stack_count,
			local_backups, s3_backups, transfer_enabled,
			sftp_enabled, websocket_enabled, console_enabled,
			database_provisioning_enabled,
			raw_report, fetched_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9,
			$10, $11, $12, $13,
			$14, $15,
			$16, $17, $18,
			$19, $20, $21,
			$22, $23, $24,
			$25,
			$26, $27, $28, $29
		) ON CONFLICT (node_id) DO UPDATE SET
			beacon_version = EXCLUDED.beacon_version,
			os = EXCLUDED.os,
			architecture = EXCLUDED.architecture,
			cpu_threads = EXCLUDED.cpu_threads,
			memory_mb = EXCLUDED.memory_mb,
			disk_mb = EXCLUDED.disk_mb,
			uptime_seconds = EXCLUDED.uptime_seconds,
			runtime_available = EXCLUDED.runtime_available,
			runtime_status = EXCLUDED.runtime_status,
			runtime_version = EXCLUDED.runtime_version,
			runtime_provider = EXCLUDED.runtime_provider,
			docker_build_enabled = EXCLUDED.docker_build_enabled,
			nixpacks_enabled = EXCLUDED.nixpacks_enabled,
			compose_enabled = EXCLUDED.compose_enabled,
			compose_version = EXCLUDED.compose_version,
			compose_stack_count = EXCLUDED.compose_stack_count,
			local_backups = EXCLUDED.local_backups,
			s3_backups = EXCLUDED.s3_backups,
			transfer_enabled = EXCLUDED.transfer_enabled,
			sftp_enabled = EXCLUDED.sftp_enabled,
			websocket_enabled = EXCLUDED.websocket_enabled,
			console_enabled = EXCLUDED.console_enabled,
			database_provisioning_enabled = EXCLUDED.database_provisioning_enabled,
			raw_report = EXCLUDED.raw_report,
			fetched_at = EXCLUDED.fetched_at,
			updated_at = EXCLUDED.updated_at
	`, id, nc.NodeID, nc.BeaconVersion, nc.OS, nc.Architecture, nc.CPUThreads, nc.MemoryMB, nc.DiskMB, nc.UptimeSeconds,
		nc.RuntimeAvailable, nc.RuntimeStatus, nc.RuntimeVersion, nc.RuntimeProvider,
		nc.DockerBuildEnabled, nc.NixpacksEnabled,
		nc.ComposeEnabled, nc.ComposeVersion, nc.StackCount,
		nc.LocalBackups, nc.S3Backups, nc.TransferEnabled,
		nc.SFTPEnabled, nc.WebSocketEnabled, nc.ConsoleEnabled,
		nc.DatabaseProvisioningEnabled,
		rawJSON, nc.FetchedAt, now, now,
	)
	if err != nil {
		return err
	}

	// Record capability snapshot in history
	historyID := uuid.NewString()
	capEntries, _ := json.Marshal([]map[string]any{
		{"type": "runtime", "available": nc.RuntimeAvailable, "status": nc.RuntimeStatus},
		{"type": "build", "available": nc.DockerBuildEnabled || nc.NixpacksEnabled},
		{"type": "compose", "available": nc.ComposeEnabled},
		{"type": "storage", "local": nc.LocalBackups, "s3": nc.S3Backups, "transfer": nc.TransferEnabled},
		{"type": "gateway", "sftp": nc.SFTPEnabled, "websocket": nc.WebSocketEnabled, "console": nc.ConsoleEnabled},
		{"type": "database", "available": nc.DatabaseProvisioningEnabled},
	})
	_, _ = s.db.Exec(ctx, `
		INSERT INTO node_capability_history (id, node_id, beacon_version, capabilities, raw_report, observed_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, historyID, nc.NodeID, nc.BeaconVersion, capEntries, rawJSON, nc.FetchedAt)

	if tag.RowsAffected() == 0 {
		return errors.New("upsert affected no rows")
	}
	return nil
}

func (s *Store) GetNodeCapability(ctx context.Context, nodeID string) (*NodeCapability, error) {
	var nc NodeCapability
	var raw []byte
	err := s.db.QueryRow(ctx, `
		SELECT id, node_id, beacon_version, os, architecture, cpu_threads, memory_mb, disk_mb, uptime_seconds,
		       runtime_available, runtime_status, runtime_version, runtime_provider,
		       docker_build_enabled, nixpacks_enabled,
		       compose_enabled, compose_version, compose_stack_count,
		       local_backups, s3_backups, transfer_enabled,
		       sftp_enabled, websocket_enabled, console_enabled,
		       database_provisioning_enabled,
		       raw_report, fetched_at, created_at, updated_at
		FROM node_capabilities
		WHERE node_id = $1
	`, nodeID).Scan(
		&nc.ID, &nc.NodeID, &nc.BeaconVersion, &nc.OS, &nc.Architecture, &nc.CPUThreads, &nc.MemoryMB, &nc.DiskMB, &nc.UptimeSeconds,
		&nc.RuntimeAvailable, &nc.RuntimeStatus, &nc.RuntimeVersion, &nc.RuntimeProvider,
		&nc.DockerBuildEnabled, &nc.NixpacksEnabled,
		&nc.ComposeEnabled, &nc.ComposeVersion, &nc.StackCount,
		&nc.LocalBackups, &nc.S3Backups, &nc.TransferEnabled,
		&nc.SFTPEnabled, &nc.WebSocketEnabled, &nc.ConsoleEnabled,
		&nc.DatabaseProvisioningEnabled,
		&raw, &nc.FetchedAt, &nc.CreatedAt, &nc.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	nc.RawReport = raw
	return &nc, nil
}

func (s *Store) ListCapabilities(ctx context.Context, filter CapabilityInventoryFilter) ([]NodeCapability, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT nc.id, nc.node_id, nc.beacon_version, nc.os, nc.architecture, nc.cpu_threads, nc.memory_mb, nc.disk_mb, nc.uptime_seconds,
		       nc.runtime_available, nc.runtime_status, nc.runtime_version, nc.runtime_provider,
		       nc.docker_build_enabled, nc.nixpacks_enabled,
		       nc.compose_enabled, nc.compose_version, nc.compose_stack_count,
		       nc.local_backups, nc.s3_backups, nc.transfer_enabled,
		       nc.sftp_enabled, nc.websocket_enabled, nc.console_enabled,
		       nc.database_provisioning_enabled,
		       nc.raw_report, nc.fetched_at, nc.created_at, nc.updated_at,
		       n.name AS node_name, n.status AS node_status
		FROM node_capabilities nc
		JOIN nodes n ON n.id = nc.node_id
		ORDER BY nc.fetched_at DESC
		LIMIT $1 OFFSET $2
	`, limit, filter.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var caps []NodeCapability
	for rows.Next() {
		var nc NodeCapability
		var raw []byte
		var nodeName, nodeStatus string
		if err := rows.Scan(
			&nc.ID, &nc.NodeID, &nc.BeaconVersion, &nc.OS, &nc.Architecture, &nc.CPUThreads, &nc.MemoryMB, &nc.DiskMB, &nc.UptimeSeconds,
			&nc.RuntimeAvailable, &nc.RuntimeStatus, &nc.RuntimeVersion, &nc.RuntimeProvider,
			&nc.DockerBuildEnabled, &nc.NixpacksEnabled,
			&nc.ComposeEnabled, &nc.ComposeVersion, &nc.StackCount,
			&nc.LocalBackups, &nc.S3Backups, &nc.TransferEnabled,
			&nc.SFTPEnabled, &nc.WebSocketEnabled, &nc.ConsoleEnabled,
			&nc.DatabaseProvisioningEnabled,
			&raw, &nc.FetchedAt, &nc.CreatedAt, &nc.UpdatedAt,
			&nodeName, &nodeStatus,
		); err != nil {
			return nil, err
		}
		nc.RawReport = raw
		caps = append(caps, nc)
	}
	return caps, rows.Err()
}

func (s *Store) GetCapabilityHistory(ctx context.Context, nodeID string, limit int) ([]NodeCapabilityHistoryEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, node_id, beacon_version, capabilities, raw_report, observed_at
		FROM node_capability_history
		WHERE node_id = $1
		ORDER BY observed_at DESC
		LIMIT $2
	`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []NodeCapabilityHistoryEntry
	for rows.Next() {
		var e NodeCapabilityHistoryEntry
		var caps []byte
		var raw []byte
		if err := rows.Scan(&e.ID, &e.NodeID, &e.BeaconVersion, &caps, &raw, &e.ObservedAt); err != nil {
			return nil, err
		}
		e.Capabilities = caps
		e.RawReport = raw
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

type NodeCapabilityHistoryEntry struct {
	ID            string          `json:"id"`
	NodeID        string          `json:"nodeId"`
	BeaconVersion string          `json:"beaconVersion"`
	Capabilities  json.RawMessage `json:"capabilities"`
	RawReport     json.RawMessage `json:"rawReport"`
	ObservedAt    time.Time       `json:"observedAt"`
}
