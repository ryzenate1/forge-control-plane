package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type BuildLogEntry struct {
	ID            string         `json:"id"`
	BuildID       string         `json:"buildId"`
	CorrelationID string         `json:"correlationId"`
	NodeID        string         `json:"nodeId"`
	ServerID      string         `json:"serverId"`
	SourceType    string         `json:"sourceType"`
	LogLevel      string         `json:"logLevel"`
	Message       string         `json:"message"`
	Metadata      map[string]any `json:"metadata"`
	Sequence      int            `json:"sequence"`
	CreatedAt     time.Time      `json:"createdAt"`
}

type CreateBuildLogEntryRequest struct {
	BuildID       string
	CorrelationID string
	NodeID        string
	ServerID      string
	SourceType    string
	LogLevel      string
	Message       string
	Metadata      map[string]any
	Sequence      int
}

type DeploymentLogEntry struct {
	ID            string         `json:"id"`
	DeploymentID  string         `json:"deploymentId"`
	CorrelationID string         `json:"correlationId"`
	ServerID      string         `json:"serverId"`
	NodeID        string         `json:"nodeId"`
	LogLevel      string         `json:"logLevel"`
	Message       string         `json:"message"`
	Metadata      map[string]any `json:"metadata"`
	Sequence      int            `json:"sequence"`
	CreatedAt     time.Time      `json:"createdAt"`
}

type CreateDeploymentLogEntryRequest struct {
	DeploymentID  string
	CorrelationID string
	ServerID      string
	NodeID        string
	LogLevel      string
	Message       string
	Metadata      map[string]any
	Sequence      int
}

type BeaconCommandLog struct {
	ID              string         `json:"id"`
	CommandID       string         `json:"commandId"`
	OperationID     string         `json:"operationId"`
	CorrelationID   string         `json:"correlationId"`
	NodeID          string         `json:"nodeId"`
	ServerID        string         `json:"serverId"`
	CommandType     string         `json:"commandType"`
	Status          string         `json:"status"`
	RequestPayload  map[string]any `json:"requestPayload"`
	ResponsePayload map[string]any `json:"responsePayload"`
	ExitCode        *int           `json:"exitCode"`
	DurationMs      int64          `json:"durationMs"`
	ErrorMessage    string         `json:"errorMessage"`
	ExecutedAt      *time.Time     `json:"executedAt"`
	CreatedAt       time.Time      `json:"createdAt"`
}

type CreateBeaconCommandLogRequest struct {
	CommandID       string
	OperationID     string
	CorrelationID   string
	NodeID          string
	ServerID        string
	CommandType     string
	Status          string
	RequestPayload  map[string]any
	ResponsePayload map[string]any
	ExitCode        *int
	DurationMs      int64
	ErrorMessage    string
	ExecutedAt      *time.Time
}

type CorrelationLink struct {
	ID                string    `json:"id"`
	OperationID       string    `json:"operationId"`
	CommandID         string    `json:"commandId"`
	DeploymentID      string    `json:"deploymentId"`
	BuildID           string    `json:"buildId"`
	ResourceType      string    `json:"resourceType"`
	ResourceID        string    `json:"resourceId"`
	ParentOperationID string    `json:"parentOperationId"`
	CreatedAt         time.Time `json:"createdAt"`
}

type CreateCorrelationLinkRequest struct {
	OperationID       string
	CommandID         string
	DeploymentID      string
	BuildID           string
	ResourceType      string
	ResourceID        string
	ParentOperationID string
}

// Build Logs
func (s *Store) CreateBuildLog(ctx context.Context, req CreateBuildLogEntryRequest) (BuildLogEntry, error) {
	id := uuid.NewString()
	if req.SourceType == "" {
		req.SourceType = "build"
	}
	if req.LogLevel == "" {
		req.LogLevel = "info"
	}
	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	metaBytes, _ := json.Marshal(metadata)

	_, err := s.db.Exec(ctx, `
		INSERT INTO build_logs (id, build_id, correlation_id, node_id, server_id, source_type, log_level, message, metadata, sequence)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10)
	`, id, req.BuildID, req.CorrelationID, nullableUUID(req.NodeID), nullableUUID(req.ServerID),
		req.SourceType, req.LogLevel, req.Message, string(metaBytes), req.Sequence)
	if err != nil {
		return BuildLogEntry{}, err
	}
	return s.GetBuildLog(ctx, id)
}

func (s *Store) GetBuildLog(ctx context.Context, id string) (BuildLogEntry, error) {
	var entry BuildLogEntry
	var nodeID, serverID sql.NullString
	var metaBytes []byte
	err := s.db.QueryRow(ctx, `
		SELECT id::text, build_id, COALESCE(correlation_id,''), COALESCE(node_id::text,''), COALESCE(server_id::text,''),
			source_type, log_level, message, metadata, sequence, created_at
		FROM build_logs WHERE id = $1
	`, id).Scan(&entry.ID, &entry.BuildID, &entry.CorrelationID, &nodeID, &serverID,
		&entry.SourceType, &entry.LogLevel, &entry.Message, &metaBytes, &entry.Sequence, &entry.CreatedAt)
	if err != nil {
		return BuildLogEntry{}, err
	}
	entry.NodeID = nodeID.String
	entry.ServerID = serverID.String
	if len(metaBytes) > 0 {
		json.Unmarshal(metaBytes, &entry.Metadata)
	}
	if entry.Metadata == nil {
		entry.Metadata = map[string]any{}
	}
	return entry, nil
}

func (s *Store) ListBuildLogs(ctx context.Context, buildID string, limit int) ([]BuildLogEntry, error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, build_id, COALESCE(correlation_id,''), COALESCE(node_id::text,''), COALESCE(server_id::text,''),
			source_type, log_level, message, metadata, sequence, created_at
		FROM build_logs WHERE build_id = $1
		ORDER BY sequence ASC LIMIT $2
	`, buildID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []BuildLogEntry{}
	for rows.Next() {
		var entry BuildLogEntry
		var nodeID, serverID sql.NullString
		var metaBytes []byte
		if err := rows.Scan(&entry.ID, &entry.BuildID, &entry.CorrelationID, &nodeID, &serverID,
			&entry.SourceType, &entry.LogLevel, &entry.Message, &metaBytes, &entry.Sequence, &entry.CreatedAt); err != nil {
			return nil, err
		}
		entry.NodeID = nodeID.String
		entry.ServerID = serverID.String
		if len(metaBytes) > 0 {
			json.Unmarshal(metaBytes, &entry.Metadata)
		}
		if entry.Metadata == nil {
			entry.Metadata = map[string]any{}
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

func (s *Store) ListBuildLogsByCorrelation(ctx context.Context, correlationID string, limit int) ([]BuildLogEntry, error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, build_id, COALESCE(correlation_id,''), COALESCE(node_id::text,''), COALESCE(server_id::text,''),
			source_type, log_level, message, metadata, sequence, created_at
		FROM build_logs WHERE correlation_id = $1
		ORDER BY sequence ASC LIMIT $2
	`, correlationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []BuildLogEntry{}
	for rows.Next() {
		var entry BuildLogEntry
		var nodeID, serverID sql.NullString
		var metaBytes []byte
		if err := rows.Scan(&entry.ID, &entry.BuildID, &entry.CorrelationID, &nodeID, &serverID,
			&entry.SourceType, &entry.LogLevel, &entry.Message, &metaBytes, &entry.Sequence, &entry.CreatedAt); err != nil {
			return nil, err
		}
		entry.NodeID = nodeID.String
		entry.ServerID = serverID.String
		if len(metaBytes) > 0 {
			json.Unmarshal(metaBytes, &entry.Metadata)
		}
		if entry.Metadata == nil {
			entry.Metadata = map[string]any{}
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

// Deployment Logs
func (s *Store) CreateDeploymentLog(ctx context.Context, req CreateDeploymentLogEntryRequest) (DeploymentLogEntry, error) {
	id := uuid.NewString()
	if req.LogLevel == "" {
		req.LogLevel = "info"
	}
	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	metaBytes, _ := json.Marshal(metadata)

	_, err := s.db.Exec(ctx, `
		INSERT INTO deployment_logs (id, deployment_id, correlation_id, server_id, node_id, log_level, message, metadata, sequence)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9)
	`, id, req.DeploymentID, req.CorrelationID, nullableUUID(req.ServerID), nullableUUID(req.NodeID),
		req.LogLevel, req.Message, string(metaBytes), req.Sequence)
	if err != nil {
		return DeploymentLogEntry{}, err
	}
	var entry DeploymentLogEntry
	var serverID, nodeID sql.NullString
	var metaBytes2 []byte
	err = s.db.QueryRow(ctx, `
		SELECT id::text, deployment_id, COALESCE(correlation_id,''), COALESCE(server_id::text,''), COALESCE(node_id::text,''),
			log_level, message, metadata, sequence, created_at
		FROM deployment_logs WHERE id = $1
	`, id).Scan(&entry.ID, &entry.DeploymentID, &entry.CorrelationID, &serverID, &nodeID,
		&entry.LogLevel, &entry.Message, &metaBytes2, &entry.Sequence, &entry.CreatedAt)
	if err != nil {
		return DeploymentLogEntry{}, err
	}
	entry.ServerID = serverID.String
	entry.NodeID = nodeID.String
	if len(metaBytes2) > 0 {
		json.Unmarshal(metaBytes2, &entry.Metadata)
	}
	if entry.Metadata == nil {
		entry.Metadata = map[string]any{}
	}
	return entry, nil
}

func (s *Store) ListDeploymentLogs(ctx context.Context, deploymentID string, limit int) ([]DeploymentLogEntry, error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, deployment_id, COALESCE(correlation_id,''), COALESCE(server_id::text,''), COALESCE(node_id::text,''),
			log_level, message, metadata, sequence, created_at
		FROM deployment_logs WHERE deployment_id = $1
		ORDER BY sequence ASC LIMIT $2
	`, deploymentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []DeploymentLogEntry{}
	for rows.Next() {
		var entry DeploymentLogEntry
		var serverID, nodeID sql.NullString
		var metaBytes []byte
		if err := rows.Scan(&entry.ID, &entry.DeploymentID, &entry.CorrelationID, &serverID, &nodeID,
			&entry.LogLevel, &entry.Message, &metaBytes, &entry.Sequence, &entry.CreatedAt); err != nil {
			return nil, err
		}
		entry.ServerID = serverID.String
		entry.NodeID = nodeID.String
		if len(metaBytes) > 0 {
			json.Unmarshal(metaBytes, &entry.Metadata)
		}
		if entry.Metadata == nil {
			entry.Metadata = map[string]any{}
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

// Beacon Command Logs
func (s *Store) CreateBeaconCommandLog(ctx context.Context, req CreateBeaconCommandLogRequest) (BeaconCommandLog, error) {
	id := uuid.NewString()
	if req.Status == "" {
		req.Status = "pending"
	}
	reqPayload := req.RequestPayload
	if reqPayload == nil {
		reqPayload = map[string]any{}
	}
	respPayload := req.ResponsePayload
	if respPayload == nil {
		respPayload = map[string]any{}
	}
	reqBytes, _ := json.Marshal(reqPayload)
	respBytes, _ := json.Marshal(respPayload)

	_, err := s.db.Exec(ctx, `
		INSERT INTO beacon_command_logs (id, command_id, operation_id, correlation_id, node_id, server_id,
			command_type, status, request_payload, response_payload, exit_code, duration_ms, error_message, executed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10::jsonb,$11,$12,$13,$14)
	`, id, req.CommandID, req.OperationID, req.CorrelationID,
		nullableUUID(req.NodeID), nullableUUID(req.ServerID),
		req.CommandType, req.Status, string(reqBytes), string(respBytes),
		req.ExitCode, req.DurationMs, req.ErrorMessage, req.ExecutedAt)
	if err != nil {
		return BeaconCommandLog{}, err
	}
	var log BeaconCommandLog
	var nodeID, serverID sql.NullString
	var reqBytes2, respBytes2 []byte
	var exitCode sql.NullInt64
	var executedAt sql.NullTime
	err = s.db.QueryRow(ctx, `
		SELECT id::text, command_id, COALESCE(operation_id,''), COALESCE(correlation_id,''),
			COALESCE(node_id::text,''), COALESCE(server_id::text,''),
			command_type, status, request_payload, response_payload, exit_code, duration_ms,
			COALESCE(error_message,''), executed_at, created_at
		FROM beacon_command_logs WHERE id = $1
	`, id).Scan(&log.ID, &log.CommandID, &log.OperationID, &log.CorrelationID,
		&nodeID, &serverID,
		&log.CommandType, &log.Status, &reqBytes2, &respBytes2, &exitCode, &log.DurationMs,
		&log.ErrorMessage, &executedAt, &log.CreatedAt)
	if err != nil {
		return BeaconCommandLog{}, err
	}
	log.NodeID = nodeID.String
	log.ServerID = serverID.String
	if exitCode.Valid {
		v := int(exitCode.Int64)
		log.ExitCode = &v
	}
	if executedAt.Valid {
		log.ExecutedAt = &executedAt.Time
	}
	if len(reqBytes2) > 0 {
		json.Unmarshal(reqBytes2, &log.RequestPayload)
	}
	if len(respBytes2) > 0 {
		json.Unmarshal(respBytes2, &log.ResponsePayload)
	}
	if log.RequestPayload == nil {
		log.RequestPayload = map[string]any{}
	}
	if log.ResponsePayload == nil {
		log.ResponsePayload = map[string]any{}
	}
	return log, nil
}

func (s *Store) GetBeaconCommandLog(ctx context.Context, id string) (BeaconCommandLog, error) {
	var log BeaconCommandLog
	var nodeID, serverID sql.NullString
	var reqBytes, respBytes []byte
	var exitCode sql.NullInt64
	var executedAt sql.NullTime
	err := s.db.QueryRow(ctx, `
		SELECT id::text, command_id, COALESCE(operation_id,''), COALESCE(correlation_id,''),
			COALESCE(node_id::text,''), COALESCE(server_id::text,''),
			command_type, status, request_payload, response_payload, exit_code, duration_ms,
			COALESCE(error_message,''), executed_at, created_at
		FROM beacon_command_logs WHERE id = $1
	`, id).Scan(&log.ID, &log.CommandID, &log.OperationID, &log.CorrelationID,
		&nodeID, &serverID,
		&log.CommandType, &log.Status, &reqBytes, &respBytes, &exitCode, &log.DurationMs,
		&log.ErrorMessage, &executedAt, &log.CreatedAt)
	if err != nil {
		return BeaconCommandLog{}, err
	}
	log.NodeID = nodeID.String
	log.ServerID = serverID.String
	if exitCode.Valid {
		v := int(exitCode.Int64)
		log.ExitCode = &v
	}
	if executedAt.Valid {
		log.ExecutedAt = &executedAt.Time
	}
	if len(reqBytes) > 0 {
		json.Unmarshal(reqBytes, &log.RequestPayload)
	}
	if len(respBytes) > 0 {
		json.Unmarshal(respBytes, &log.ResponsePayload)
	}
	if log.RequestPayload == nil {
		log.RequestPayload = map[string]any{}
	}
	if log.ResponsePayload == nil {
		log.ResponsePayload = map[string]any{}
	}
	return log, nil
}

func (s *Store) ListBeaconCommandLogs(ctx context.Context, filter BeaconCommandLogFilter) ([]BeaconCommandLog, error) {
	if filter.Limit <= 0 || filter.Limit > 1000 {
		filter.Limit = 100
	}
	query := `SELECT id::text, command_id, COALESCE(operation_id,''), COALESCE(correlation_id,''),
		COALESCE(node_id::text,''), COALESCE(server_id::text,''),
		command_type, status, request_payload, response_payload, exit_code, duration_ms,
		COALESCE(error_message,''), executed_at, created_at
		FROM beacon_command_logs WHERE 1=1`
	args := []any{}
	argN := 1

	if filter.CommandID != "" {
		query += ` AND command_id = $` + strconv.Itoa(argN)
		args = append(args, filter.CommandID)
		argN++
	}
	if filter.OperationID != "" {
		query += ` AND operation_id = $` + strconv.Itoa(argN)
		args = append(args, filter.OperationID)
		argN++
	}
	if filter.CorrelationID != "" {
		query += ` AND correlation_id = $` + strconv.Itoa(argN)
		args = append(args, filter.CorrelationID)
		argN++
	}
	if filter.NodeID != "" {
		query += ` AND node_id = $` + strconv.Itoa(argN)
		args = append(args, filter.NodeID)
		argN++
	}
	if filter.Status != "" {
		query += ` AND status = $` + strconv.Itoa(argN)
		args = append(args, filter.Status)
		argN++
	}
	query += ` ORDER BY created_at DESC LIMIT $` + strconv.Itoa(argN)
	args = append(args, filter.Limit)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []BeaconCommandLog{}
	for rows.Next() {
		var log BeaconCommandLog
		var nodeID, serverID sql.NullString
		var reqBytes, respBytes []byte
		var exitCode sql.NullInt64
		var executedAt sql.NullTime
		if err := rows.Scan(&log.ID, &log.CommandID, &log.OperationID, &log.CorrelationID,
			&nodeID, &serverID,
			&log.CommandType, &log.Status, &reqBytes, &respBytes, &exitCode, &log.DurationMs,
			&log.ErrorMessage, &executedAt, &log.CreatedAt); err != nil {
			return nil, err
		}
		log.NodeID = nodeID.String
		log.ServerID = serverID.String
		if exitCode.Valid {
			v := int(exitCode.Int64)
			log.ExitCode = &v
		}
		if executedAt.Valid {
			log.ExecutedAt = &executedAt.Time
		}
		if len(reqBytes) > 0 {
			json.Unmarshal(reqBytes, &log.RequestPayload)
		}
		if len(respBytes) > 0 {
			json.Unmarshal(respBytes, &log.ResponsePayload)
		}
		if log.RequestPayload == nil {
			log.RequestPayload = map[string]any{}
		}
		if log.ResponsePayload == nil {
			log.ResponsePayload = map[string]any{}
		}
		result = append(result, log)
	}
	return result, rows.Err()
}

func (s *Store) UpdateBeaconCommandLogStatus(ctx context.Context, commandID, status string, exitCode int, responsePayload map[string]any) error {
	respBytes, _ := json.Marshal(responsePayload)
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		UPDATE beacon_command_logs SET status = $1, exit_code = $2, response_payload = $3::jsonb, executed_at = $4
		WHERE command_id = $5
	`, status, exitCode, string(respBytes), now, commandID)
	return err
}

// Correlation Links
func (s *Store) CreateCorrelationLink(ctx context.Context, req CreateCorrelationLinkRequest) (CorrelationLink, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO correlation_links (id, operation_id, command_id, deployment_id, build_id, resource_type, resource_id, parent_operation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, id, req.OperationID, req.CommandID, req.DeploymentID, req.BuildID,
		req.ResourceType, req.ResourceID, req.ParentOperationID)
	if err != nil {
		return CorrelationLink{}, err
	}
	var link CorrelationLink
	err = s.db.QueryRow(ctx, `
		SELECT id::text, operation_id, COALESCE(command_id,''), COALESCE(deployment_id,''), COALESCE(build_id,''),
			COALESCE(resource_type,''), COALESCE(resource_id,''), COALESCE(parent_operation_id,''), created_at
		FROM correlation_links WHERE id = $1
	`, id).Scan(&link.ID, &link.OperationID, &link.CommandID, &link.DeploymentID, &link.BuildID,
		&link.ResourceType, &link.ResourceID, &link.ParentOperationID, &link.CreatedAt)
	return link, err
}

func (s *Store) GetCorrelationLinks(ctx context.Context, operationID string) ([]CorrelationLink, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, operation_id, COALESCE(command_id,''), COALESCE(deployment_id,''), COALESCE(build_id,''),
			COALESCE(resource_type,''), COALESCE(resource_id,''), COALESCE(parent_operation_id,''), created_at
		FROM correlation_links
		WHERE operation_id = $1
		ORDER BY created_at ASC
	`, operationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []CorrelationLink{}
	for rows.Next() {
		var link CorrelationLink
		if err := rows.Scan(&link.ID, &link.OperationID, &link.CommandID, &link.DeploymentID, &link.BuildID,
			&link.ResourceType, &link.ResourceID, &link.ParentOperationID, &link.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, link)
	}
	return result, rows.Err()
}

type BeaconCommandLogFilter struct {
	CommandID     string
	OperationID   string
	CorrelationID string
	NodeID        string
	Status        string
	Limit         int
}
