package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type ComposeStack struct {
	ID            string            `json:"id"`
	UserID        string            `json:"userId"`
	Name          string            `json:"name"`
	NodeID        string            `json:"nodeId"`
	Status        string            `json:"status"`
	ComposeYAML   string            `json:"composeYaml"`
	ComposeHash   string            `json:"composeHash"`
	EnvVars       map[string]string `json:"envVars,omitempty"`
	MemoryMB      int64             `json:"memoryMb"`
	CPUShares     int64             `json:"cpuShares"`
	DiskMB        int64             `json:"diskMb"`
	Error         string            `json:"error,omitempty"`
	ReservationID string            `json:"reservationId,omitempty"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`

	GitSourceID          string     `json:"gitSourceId,omitempty"`
	GitRepositoryURL     string     `json:"gitRepositoryUrl,omitempty"`
	GitRepositoryPath    string     `json:"gitRepositoryPath,omitempty"`
	ComposePath          string     `json:"composePath,omitempty"`
	GitBranch            string     `json:"gitBranch,omitempty"`
	GitCommitSHA         string     `json:"gitCommitSha,omitempty"`
	GitDesiredCommitSHA  string     `json:"gitDesiredCommitSha,omitempty"`
	GitPreviousCommitSHA string          `json:"gitPreviousCommitSha,omitempty"`
	GitPreviousCompose   string          `json:"gitPreviousCompose,omitempty"`
	GitPreviousManifest  *json.RawMessage `json:"gitPreviousManifest,omitempty"`
	GitAutoUpdate        bool            `json:"gitAutoUpdate"`
	GitPollIntervalSec   int        `json:"gitPollIntervalSec,omitempty"`
	GitWebhookSecret     string     `json:"-"`
	GitWebhookID         string     `json:"gitWebhookId,omitempty"`
	GitLastWebhookAt     *time.Time `json:"gitLastWebhookAt,omitempty"`
	GitUpdateStatus      string     `json:"gitUpdateStatus,omitempty"`
	GitUpdateError       string     `json:"gitUpdateError,omitempty"`
	GitReconcileMode     string     `json:"gitReconcileMode,omitempty"`
	GitFailedSHA         string     `json:"gitFailedSha,omitempty"`
	GitNextPollAt        *time.Time `json:"gitNextPollAt,omitempty"`
	GitCredentialID      string     `json:"gitCredentialId,omitempty"`
	GitUpdateClaimedBy   *string    `json:"-"`
	GitUpdateClaimedAt   *time.Time `json:"-"`
	GitLastDeliveryID    string     `json:"-"`

	ComposeType    string          `json:"composeType"`
	SourceType     string          `json:"sourceType"`
	EnvironmentID  string          `json:"environmentId,omitempty"`
	ParsedConfig   json.RawMessage `json:"parsedConfig,omitempty"`
}

type ComposeService struct {
	ID        string    `json:"id"`
	StackID   string    `json:"stackId"`
	Name      string    `json:"name"`
	Image     string    `json:"image"`
	Status    string    `json:"status"`
	State     string    `json:"state"`
	Ports     string    `json:"ports"`
	Health    string    `json:"health"`
	NodeID    string    `json:"nodeId"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

var composeStackCols = []string{
	"id", "user_id", "name", "node_id", "status", "compose_yaml", "compose_hash",
	"env_vars", "memory_mb", "cpu_shares", "disk_mb", "error", "reservation_id",
	"created_at", "updated_at",
	"compose_type", "source_type", "environment_id",
	"git_source_id", "git_repository_url", "git_repository_path", "compose_path", "git_branch",
	"git_commit_sha", "git_desired_commit_sha", "git_previous_commit_sha", "git_previous_compose_yaml",
	"git_previous_manifest",
	"git_auto_update", "git_poll_interval_seconds", "git_webhook_secret", "git_webhook_id",
	"git_last_webhook_at", "git_update_status", "git_update_error", "git_next_poll_at", "git_credential_id",
	"git_update_claimed_by", "git_update_claimed_at", "git_last_delivery_id",
	"git_reconcile_mode", "git_failed_sha",
	"parsed_config",
}

const composeStackScanExpr = `
	cs.id, cs.user_id, cs.name, cs.node_id, cs.status,
	cs.compose_yaml, cs.compose_hash, cs.env_vars,
	cs.memory_mb, cs.cpu_shares, cs.disk_mb,
	COALESCE(cs.error, ''), cs.reservation_id,
	cs.created_at, cs.updated_at,
	COALESCE(cs.compose_type, 'docker-compose'), COALESCE(cs.source_type, 'raw'), COALESCE(cs.environment_id, ''),
	cs.git_source_id, cs.git_repository_url, cs.git_repository_path, COALESCE(cs.compose_path, ''), cs.git_branch,
	cs.git_commit_sha, cs.git_desired_commit_sha, cs.git_previous_commit_sha, cs.git_previous_compose_yaml,
	cs.git_previous_manifest,
	cs.git_auto_update, cs.git_poll_interval_seconds,
	COALESCE(cs.git_webhook_secret, ''), COALESCE(cs.git_webhook_id, ''),
	cs.git_last_webhook_at, COALESCE(cs.git_update_status, 'idle'), COALESCE(cs.git_update_error, ''),
	cs.git_next_poll_at, cs.git_credential_id,
	cs.git_update_claimed_by, cs.git_update_claimed_at, COALESCE(cs.git_last_delivery_id, ''),
	COALESCE(cs.git_reconcile_mode, ''), cs.git_failed_sha,
	COALESCE(cs.parsed_config, '{}'::jsonb)`

const composeStackFromExpr = `FROM compose_stacks cs`

func (s *Store) CreateComposeStack(ctx context.Context, stack *ComposeStack) error {
	if s.db == nil {
		return nil
	}
	if stack.CreatedAt.IsZero() {
		stack.CreatedAt = time.Now().UTC()
	}
	if stack.UpdatedAt.IsZero() {
		stack.UpdatedAt = stack.CreatedAt
	}
	envJSON, err := marshalEnvVars(stack.EnvVars)
	if err != nil {
		return fmt.Errorf("marshal env vars: %w", err)
	}

	cols := composeStackCols
	placeholders := "$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37,$38,$39,$40,$41,$42,$43,$44"
	updateSet := ""
	for _, c := range cols {
		if c == "id" || c == "created_at" {
			continue
		}
		if updateSet != "" {
			updateSet += ", "
		}
		updateSet += fmt.Sprintf("%s = EXCLUDED.%s", c, c)
	}

	_, err = s.db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO compose_stacks (%s) VALUES (%s)
		ON CONFLICT (id) DO UPDATE SET %s
	`, joinCols(cols), placeholders, updateSet),
		stack.ID, stack.UserID, stack.Name, stack.NodeID, stack.Status,
		stack.ComposeYAML, stack.ComposeHash, envJSON, stack.MemoryMB, stack.CPUShares,
		stack.DiskMB, stack.Error, nullIfEmpty(stack.ReservationID),
		stack.CreatedAt, stack.UpdatedAt,
		nullIfEmpty(stack.GitSourceID), nullIfEmpty(stack.GitRepositoryURL),
		nullIfEmpty(stack.GitRepositoryPath), nullIfEmpty(stack.ComposePath), nullIfEmpty(stack.GitBranch),
		nullIfEmpty(stack.GitCommitSHA), nullIfEmpty(stack.GitDesiredCommitSHA),
		nullIfEmpty(stack.GitPreviousCommitSHA), nullIfEmpty(stack.GitPreviousCompose),
		manifestJSON(stack.GitPreviousManifest),
		stack.GitAutoUpdate, stack.GitPollIntervalSec,
		nullIfEmpty(stack.GitWebhookSecret), nullIfEmpty(stack.GitWebhookID),
		stack.GitLastWebhookAt, stack.GitUpdateStatus, stack.GitUpdateError,
		stack.GitNextPollAt, nullIfEmpty(stack.GitCredentialID),
		stack.GitUpdateClaimedBy, stack.GitUpdateClaimedAt, nullIfEmpty(stack.GitLastDeliveryID),
		stack.GitReconcileMode, nullIfEmpty(stack.GitFailedSHA),
		stack.ComposeType, stack.SourceType, nullIfEmpty(stack.EnvironmentID),
		parsedConfigJSON(stack.ParsedConfig))
	return err
}

func parsedConfigJSON(pc json.RawMessage) *string {
	if len(pc) == 0 {
		return nil
	}
	s := string(pc)
	return &s
}

func (s *Store) UpdateComposeStack(ctx context.Context, stack *ComposeStack) error {
	return s.CreateComposeStack(ctx, stack)
}

func scanComposeStack(scanner interface{ Scan(dest ...any) error }) *ComposeStack {
	var stack ComposeStack
	var reservationID, errorMsg *string
	var envJSON []byte
	var gitSourceID, gitRepoURL, gitRepoPath, gitComposePath, gitBranch, gitCommitSHA, gitDesiredSHA *string
	var gitPrevSHA, gitPrevCompose, gitPrevManifest, gitUpdateStatus, gitUpdateError, gitWebhookSecret, gitWebhookID, gitCredID *string
	var gitLastWebhookAt, gitNextPollAt *time.Time
	var gitUpdateClaimedBy *string
	var gitUpdateClaimedAt *time.Time
	var gitLastDeliveryID, gitReconcileMode, gitFailedSHA *string
	var composeType, sourceType, environmentID *string
	var parsedConfigRaw []byte
	if err := scanner.Scan(
		&stack.ID, &stack.UserID, &stack.Name, &stack.NodeID, &stack.Status,
		&stack.ComposeYAML, &stack.ComposeHash, &envJSON, &stack.MemoryMB, &stack.CPUShares,
		&stack.DiskMB, &errorMsg, &reservationID, &stack.CreatedAt, &stack.UpdatedAt,
		&composeType, &sourceType, &environmentID,
		&gitSourceID, &gitRepoURL, &gitRepoPath, &gitComposePath, &gitBranch,
		&gitCommitSHA, &gitDesiredSHA, &gitPrevSHA, &gitPrevCompose,
		&gitPrevManifest,
		&stack.GitAutoUpdate, &stack.GitPollIntervalSec,
		&gitWebhookSecret, &gitWebhookID,
		&gitLastWebhookAt, &gitUpdateStatus, &gitUpdateError, &gitNextPollAt, &gitCredID,
		&gitUpdateClaimedBy, &gitUpdateClaimedAt, &gitLastDeliveryID,
		&gitReconcileMode, &gitFailedSHA,
		&parsedConfigRaw,
	); err != nil {
		return nil
	}
	if errorMsg != nil {
		stack.Error = *errorMsg
	}
	if reservationID != nil {
		stack.ReservationID = *reservationID
	}
	if gitSourceID != nil {
		stack.GitSourceID = *gitSourceID
	}
	if gitRepoURL != nil {
		stack.GitRepositoryURL = *gitRepoURL
	}
	if gitRepoPath != nil {
		stack.GitRepositoryPath = *gitRepoPath
	}
	if gitComposePath != nil {
		stack.ComposePath = *gitComposePath
	}
	if gitBranch != nil {
		stack.GitBranch = *gitBranch
	}
	if gitCommitSHA != nil {
		stack.GitCommitSHA = *gitCommitSHA
	}
	if gitDesiredSHA != nil {
		stack.GitDesiredCommitSHA = *gitDesiredSHA
	}
	if gitPrevSHA != nil {
		stack.GitPreviousCommitSHA = *gitPrevSHA
	}
	if gitPrevCompose != nil {
		stack.GitPreviousCompose = *gitPrevCompose
	}
	if gitPrevManifest != nil {
		var raw json.RawMessage
		if json.Unmarshal([]byte(*gitPrevManifest), &raw) == nil {
			stack.GitPreviousManifest = &raw
		}
	}
	if gitWebhookSecret != nil {
		stack.GitWebhookSecret = *gitWebhookSecret
	}
	if gitWebhookID != nil {
		stack.GitWebhookID = *gitWebhookID
	}
	stack.GitLastWebhookAt = gitLastWebhookAt
	if gitUpdateStatus != nil {
		stack.GitUpdateStatus = *gitUpdateStatus
	}
	if gitUpdateError != nil {
		stack.GitUpdateError = *gitUpdateError
	}
	stack.GitNextPollAt = gitNextPollAt
	if gitCredID != nil {
		stack.GitCredentialID = *gitCredID
	}
	stack.GitUpdateClaimedBy = gitUpdateClaimedBy
	stack.GitUpdateClaimedAt = gitUpdateClaimedAt
	if gitLastDeliveryID != nil {
		stack.GitLastDeliveryID = *gitLastDeliveryID
	}
	if gitReconcileMode != nil {
		stack.GitReconcileMode = *gitReconcileMode
	}
	if gitFailedSHA != nil {
		stack.GitFailedSHA = *gitFailedSHA
	}
	if len(envJSON) > 0 {
		_ = json.Unmarshal(envJSON, &stack.EnvVars)
	}
	if stack.EnvVars == nil {
		stack.EnvVars = make(map[string]string)
	}
	if len(parsedConfigRaw) > 0 {
		stack.ParsedConfig = json.RawMessage(parsedConfigRaw)
	}
	if composeType != nil {
		stack.ComposeType = *composeType
	}
	if stack.ComposeType == "" {
		stack.ComposeType = "docker-compose"
	}
	if sourceType != nil {
		stack.SourceType = *sourceType
	}
	if stack.SourceType == "" {
		stack.SourceType = "raw"
	}
	if environmentID != nil {
		stack.EnvironmentID = *environmentID
	}
	if stack.GitBranch == "" {
		stack.GitBranch = "main"
	}
	if stack.GitUpdateStatus == "" {
		stack.GitUpdateStatus = "idle"
	}
	if stack.GitPollIntervalSec <= 0 {
		stack.GitPollIntervalSec = 300
	}
	return &stack
}

func (s *Store) GetComposeStack(ctx context.Context, stackID string) (ComposeStack, error) {
	if s.db == nil {
		return ComposeStack{}, nil
	}
	query := fmt.Sprintf(`SELECT %s %s WHERE cs.id = $1`, composeStackScanExpr, composeStackFromExpr)
	rows, err := s.db.Query(ctx, query, stackID)
	if err != nil {
		return ComposeStack{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return ComposeStack{}, errors.New("compose stack not found")
	}
	stack := scanComposeStack(rows)
	if stack == nil {
		return ComposeStack{}, errors.New("scan compose stack failed")
	}
	return *stack, rows.Err()
}

func (s *Store) ListComposeStacks(ctx context.Context, userID string) ([]ComposeStack, error) {
	if s.db == nil {
		return nil, nil
	}
	query := fmt.Sprintf(`SELECT %s %s`, composeStackScanExpr, composeStackFromExpr)
	args := []any{}
	if userID != "" {
		query += ` WHERE cs.user_id = $1`
		args = append(args, userID)
	}
	query += ` ORDER BY cs.created_at DESC`
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stacks := []ComposeStack{}
	for rows.Next() {
		stack := scanComposeStack(rows)
		if stack != nil {
			stacks = append(stacks, *stack)
		}
	}
	return stacks, rows.Err()
}

func (s *Store) DeleteComposeStack(ctx context.Context, stackID string) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.Exec(ctx, `DELETE FROM compose_stacks WHERE id = $1`, stackID)
	return err
}

func (s *Store) ListComposeStacksDueForPoll(ctx context.Context) ([]ComposeStack, error) {
	if s.db == nil {
		return nil, nil
	}
	query := fmt.Sprintf(`SELECT %s %s WHERE cs.git_auto_update = true AND (cs.git_next_poll_at IS NULL OR cs.git_next_poll_at <= NOW()) ORDER BY cs.created_at DESC`, composeStackScanExpr, composeStackFromExpr)
	rows, err := s.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stacks := []ComposeStack{}
	for rows.Next() {
		stack := scanComposeStack(rows)
		if stack != nil {
			stacks = append(stacks, *stack)
		}
	}
	return stacks, rows.Err()
}

func (s *Store) ListComposeStacksForReconciliation(ctx context.Context) ([]ComposeStack, error) {
	if s.db == nil {
		return nil, nil
	}
	query := fmt.Sprintf(`SELECT %s %s WHERE cs.status != 'deleted' AND cs.status != 'deleting' ORDER BY cs.created_at ASC`, composeStackScanExpr, composeStackFromExpr)
	rows, err := s.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stacks := []ComposeStack{}
	for rows.Next() {
		stack := scanComposeStack(rows)
		if stack != nil {
			stacks = append(stacks, *stack)
		}
	}
	return stacks, rows.Err()
}

func (s *Store) FindComposeStackByGitSource(ctx context.Context, gitSourceID string) (*ComposeStack, error) {
	if s.db == nil {
		return nil, nil
	}
	query := fmt.Sprintf(`SELECT %s %s WHERE cs.git_source_id = $1 LIMIT 1`, composeStackScanExpr, composeStackFromExpr)
	rows, err := s.db.Query(ctx, query, gitSourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	stack := scanComposeStack(rows)
	if stack == nil {
		return nil, errors.New("scan compose stack failed")
	}
	return stack, nil
}

func (s *Store) GetComposeStackByWebhookID(ctx context.Context, webhookID string) (*ComposeStack, error) {
	if s.db == nil {
		return nil, nil
	}
	query := fmt.Sprintf(`SELECT %s %s WHERE cs.git_webhook_id = $1 LIMIT 1`, composeStackScanExpr, composeStackFromExpr)
	rows, err := s.db.Query(ctx, query, webhookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	stack := scanComposeStack(rows)
	if stack == nil {
		return nil, errors.New("scan compose stack failed")
	}
	return stack, nil
}

func nullIfEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func manifestJSON(m *json.RawMessage) *string {
	if m == nil {
		return nil
	}
	s := string(*m)
	return &s
}

func marshalEnvVars(envVars map[string]string) ([]byte, error) {
	if envVars == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(envVars)
}

func joinCols(cols []string) string {
	out := ""
	for i, c := range cols {
		if i > 0 {
			out += ", "
		}
		out += c
	}
	return out
}

func (s *Store) ClaimComposeStackForUpdate(ctx context.Context, stackID, workerID string) (*ComposeStack, error) {
	if s.db == nil {
		return nil, nil
	}
	query := fmt.Sprintf(`UPDATE compose_stacks SET git_update_status = 'deploying', git_update_claimed_by = $2, git_update_claimed_at = NOW() WHERE id = $1 AND git_update_status = 'pending' RETURNING %s`, composeStackScanExpr)
	rows, err := s.db.Query(ctx, query, stackID, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	stack := scanComposeStack(rows)
	if stack == nil {
		return nil, nil
	}
	return stack, rows.Err()
}

func (s *Store) ReleaseComposeStackClaim(ctx context.Context, stackID, workerID string) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.Exec(ctx, `UPDATE compose_stacks SET git_update_claimed_by = NULL, git_update_claimed_at = NULL, git_update_status = 'idle' WHERE id = $1 AND git_update_claimed_by = $2`, stackID, workerID)
	return err
}

func (s *Store) ListComposeStacksPendingUpdate(ctx context.Context) ([]ComposeStack, error) {
	if s.db == nil {
		return nil, nil
	}
	query := fmt.Sprintf(`SELECT %s %s WHERE cs.git_update_status = 'pending' ORDER BY cs.created_at ASC`, composeStackScanExpr, composeStackFromExpr)
	rows, err := s.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stacks := []ComposeStack{}
	for rows.Next() {
		stack := scanComposeStack(rows)
		if stack != nil {
			stacks = append(stacks, *stack)
		}
	}
	return stacks, rows.Err()
}

func (s *Store) ListComposeStacksStaleClaims(ctx context.Context, staleDuration time.Duration) ([]ComposeStack, error) {
	if s.db == nil {
		return nil, nil
	}
	query := fmt.Sprintf(`SELECT %s %s WHERE cs.git_update_status IN ('deploying', 'rolling_back') AND cs.git_update_claimed_at IS NOT NULL AND cs.git_update_claimed_at < NOW() - $1::interval ORDER BY cs.git_update_claimed_at ASC`, composeStackScanExpr, composeStackFromExpr)
	rows, err := s.db.Query(ctx, query, staleDuration.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stacks := []ComposeStack{}
	for rows.Next() {
		stack := scanComposeStack(rows)
		if stack != nil {
			stacks = append(stacks, *stack)
		}
	}
	return stacks, rows.Err()
}

var composeServiceCols = []string{
	"id", "stack_id", "name", "image", "status", "state", "ports", "health", "node_id", "created_at", "updated_at",
}

const composeServiceScanExpr = `
	svc.id, svc.stack_id, svc.name, COALESCE(svc.image, ''), COALESCE(svc.status, 'unknown'),
	COALESCE(svc.state, ''), COALESCE(svc.ports, ''), COALESCE(svc.health, ''),
	svc.node_id, svc.created_at, svc.updated_at`

const composeServiceFromExpr = `FROM compose_services svc`

func scanComposeService(scanner interface{ Scan(dest ...any) error }) *ComposeService {
	var svc ComposeService
	if err := scanner.Scan(
		&svc.ID, &svc.StackID, &svc.Name, &svc.Image, &svc.Status,
		&svc.State, &svc.Ports, &svc.Health, &svc.NodeID, &svc.CreatedAt, &svc.UpdatedAt,
	); err != nil {
		return nil
	}
	return &svc
}

func (s *Store) UpsertComposeService(ctx context.Context, svc *ComposeService) error {
	if s.db == nil {
		return nil
	}
	if svc.CreatedAt.IsZero() {
		svc.CreatedAt = time.Now().UTC()
	}
	if svc.UpdatedAt.IsZero() {
		svc.UpdatedAt = svc.CreatedAt
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO compose_services (id, stack_id, name, image, status, state, ports, health, node_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status, state = EXCLUDED.state, ports = EXCLUDED.ports,
			health = EXCLUDED.health, image = EXCLUDED.image, name = EXCLUDED.name,
			updated_at = EXCLUDED.updated_at
	`, svc.ID, svc.StackID, svc.Name, svc.Image, svc.Status, svc.State, svc.Ports, svc.Health, svc.NodeID, svc.CreatedAt, svc.UpdatedAt)
	return err
}

func (s *Store) ListComposeServicesByStack(ctx context.Context, stackID string) ([]ComposeService, error) {
	if s.db == nil {
		return nil, nil
	}
	query := fmt.Sprintf(`SELECT %s %s WHERE svc.stack_id = $1 ORDER BY svc.name`, composeServiceScanExpr, composeServiceFromExpr)
	rows, err := s.db.Query(ctx, query, stackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var services []ComposeService
	for rows.Next() {
		svc := scanComposeService(rows)
		if svc != nil {
			services = append(services, *svc)
		}
	}
	return services, rows.Err()
}

func (s *Store) DeleteComposeServicesByStack(ctx context.Context, stackID string) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.Exec(ctx, `DELETE FROM compose_services WHERE stack_id = $1`, stackID)
	return err
}

type ComposeLogEntry struct {
	ID          int64     `json:"id"`
	StackID     string    `json:"stackId"`
	ServiceName string    `json:"serviceName"`
	Stream      string    `json:"stream"`
	Message     string    `json:"message"`
	Timestamp   time.Time `json:"timestamp"`
}

func (s *Store) InsertComposeLog(ctx context.Context, entry *ComposeLogEntry) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO compose_logs (stack_id, service_name, stream, message, timestamp)
		VALUES ($1, $2, $3, $4, $5)
	`, entry.StackID, entry.ServiceName, entry.Stream, entry.Message, entry.Timestamp)
	return err
}

func (s *Store) ListComposeLogs(ctx context.Context, stackID string, service string, tail int) ([]ComposeLogEntry, error) {
	if s.db == nil {
		return nil, nil
	}
	if tail <= 0 {
		tail = 100
	}
	query := `SELECT id, stack_id, COALESCE(service_name, ''), COALESCE(stream, 'stdout'), message, timestamp FROM compose_logs WHERE stack_id = $1`
	args := []any{stackID}
	if service != "" {
		query += ` AND service_name = $2`
		args = append(args, service)
	}
	query += ` ORDER BY timestamp DESC LIMIT $` + fmt.Sprintf("%d", len(args)+1)
	args = append(args, tail)
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []ComposeLogEntry
	for rows.Next() {
		var e ComposeLogEntry
		if err := rows.Scan(&e.ID, &e.StackID, &e.ServiceName, &e.Stream, &e.Message, &e.Timestamp); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

type ProjectDocument struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ServerID       string `json:"serverId,omitempty"`
	ComposeContent string `json:"composeContent"`
	ParsedConfig   []byte `json:"parsedConfig"`
	Status         string `json:"status"`
	Revision       int    `json:"revision"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

func (s *Store) CreateComposeProject(ctx context.Context, p *ProjectDocument) error {
	if p.ID == "" {
		return fmt.Errorf("project ID is required")
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO compose_projects (id, name, server_id, compose_content, parsed_config, status, revision, created_at, updated_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, NOW(), NOW())
	`, p.ID, p.Name, p.ServerID, p.ComposeContent, p.ParsedConfig, p.Status, p.Revision)
	return err
}

func (s *Store) GetComposeProject(ctx context.Context, id string) (*ProjectDocument, error) {
	var p ProjectDocument
	err := s.db.QueryRow(ctx, `
		SELECT id::text, name, COALESCE(server_id::text, ''), compose_content, parsed_config::text, status, revision,
		       TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at,
		       TO_CHAR(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS updated_at
		FROM compose_projects
		WHERE id = $1
	`, id).Scan(&p.ID, &p.Name, &p.ServerID, &p.ComposeContent, &p.ParsedConfig, &p.Status, &p.Revision, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) ListComposeProjects(ctx context.Context) ([]ProjectDocument, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, COALESCE(server_id::text, ''), compose_content, COALESCE(parsed_config::text, '{}'), status, revision,
		       TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at,
		       TO_CHAR(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS updated_at
		FROM compose_projects
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []ProjectDocument
	for rows.Next() {
		var p ProjectDocument
		if err := rows.Scan(&p.ID, &p.Name, &p.ServerID, &p.ComposeContent, &p.ParsedConfig, &p.Status, &p.Revision, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func (s *Store) UpdateComposeProject(ctx context.Context, p *ProjectDocument) error {
	_, err := s.db.Exec(ctx, `
		UPDATE compose_projects
		SET name = $2, compose_content = $3, parsed_config = $4, status = $5, revision = $6, updated_at = NOW()
		WHERE id = $1
	`, p.ID, p.Name, p.ComposeContent, p.ParsedConfig, p.Status, p.Revision)
	return err
}

func (s *Store) DeleteComposeProject(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM compose_projects WHERE id = $1`, id)
	return err
}
