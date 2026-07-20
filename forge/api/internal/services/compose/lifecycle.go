package compose

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

type StackStatus string

const (
	StackStatusDeploying   StackStatus = "deploying"
	StackStatusRunning     StackStatus = "running"
	StackStatusStopped     StackStatus = "stopped"
	StackStatusDegraded    StackStatus = "degraded"
	StackStatusUpdating    StackStatus = "updating"
	StackStatusRollingBack StackStatus = "rolling_back"
	StackStatusDeleting    StackStatus = "deleting"
	StackStatusDeleted     StackStatus = "deleted"
	StackStatusFailed      StackStatus = "failed"
)

type ComposeStack struct {
	ID           string     `json:"id"`
	UserID       string     `json:"userId"`
	Name         string     `json:"name"`
	NodeID       string     `json:"nodeId"`
	Status       StackStatus `json:"status"`
	ComposeYAML  string     `json:"composeYaml"`
	ComposeHash  string     `json:"composeHash"`
	EnvVars      map[string]string `json:"envVars,omitempty"`
	MemoryMB     int64      `json:"memoryMb"`
	CPUShares    int64      `json:"cpuShares"`
	DiskMB       int64      `json:"diskMb"`
	Error        string     `json:"error,omitempty"`
	ReservationID string    `json:"reservationId,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

type DeployComposeRequest struct {
	UserID      string            `json:"userId"`
	Name        string            `json:"name"`
	NodeID      string            `json:"nodeId"`
	ComposeYAML string            `json:"composeYaml"`
	EnvVars     map[string]string `json:"envVars,omitempty"`
	MemoryMB    int64             `json:"memoryMb"`
	CPUShares   int64             `json:"cpuShares"`
	DiskMB      int64             `json:"diskMb"`
}

type UpdateComposeRequest struct {
	ComposeYAML string            `json:"composeYaml"`
	EnvVars     map[string]string `json:"envVars,omitempty"`
	MemoryMB    int64             `json:"memoryMb"`
	CPUShares   int64             `json:"cpuShares"`
	DiskMB      int64             `json:"diskMb"`
}

type ServiceState struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	Status string `json:"status"`
	State  string `json:"state"`
	Ports  string `json:"ports"`
}

type StackStatusResponse struct {
	Stack    ComposeStack   `json:"stack"`
	Services []ServiceState `json:"services"`
}

type StackLogsResponse struct {
	StackID  string            `json:"stackId"`
	Services map[string]string `json:"services"`
}

type Service struct {
	store     *store.Store
	publisher events.Publisher
}

var (
	ErrStackNotFound     = errors.New("compose stack not found")
	ErrStackNotRunnable  = errors.New("compose stack is not in a runnable state")
	ErrInvalidCompose    = errors.New("invalid compose yaml")
	ErrNodeNotAvailable  = errors.New("node is not available")
	ErrReservationFailed = errors.New("resource reservation failed")
)

func New(store *store.Store, publishers ...events.Publisher) *Service {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Service{
		store:     store,
		publisher: publisher,
	}
}

func (s *Service) getClient() *daemon.Client {
	return daemon.NewClient()
}

func (s *Service) createStackID() string {
	return "cps-" + uuid.NewString()[:12]
}

func (s *Service) DeployComposeStack(ctx context.Context, req DeployComposeRequest) (*ComposeStack, error) {
	if req.Name == "" || req.ComposeYAML == "" {
		return nil, fmt.Errorf("name and composeYaml are required")
	}
	if req.UserID == "" {
		return nil, fmt.Errorf("userId is required")
	}

	hash := computeHash(req.ComposeYAML)

	if req.NodeID == "" {
		return nil, errors.New("node selection is required for compose deployment")
	}

	node, err := s.store.GetNode(ctx, req.NodeID)
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}

	nodeCredential, err := s.store.GetNodeDaemonCredential(ctx, req.NodeID)
	if err != nil {
		return nil, fmt.Errorf("node credential not found: %w", err)
	}

	reservation, err := s.store.CreatePlacementReservation(ctx, store.CreatePlacementReservationRequest{
		NodeID:          req.NodeID,
		CPU:             int(req.CPUShares / 100),
		Memory:          req.MemoryMB,
		Disk:            req.DiskMB,
		ReservationType: store.PlacementReservationTypePlacement,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReservationFailed, err)
	}

	stackID := s.createStackID()
	now := time.Now().UTC()

	stack := &ComposeStack{
		ID:            stackID,
		UserID:        req.UserID,
		Name:          req.Name,
		NodeID:        req.NodeID,
		Status:        StackStatusDeploying,
		ComposeYAML:   req.ComposeYAML,
		ComposeHash:   hash,
		EnvVars:       req.EnvVars,
		MemoryMB:      req.MemoryMB,
		CPUShares:     req.CPUShares,
		DiskMB:        req.DiskMB,
		ReservationID: reservation.ID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.store.CreateComposeStack(ctx, toStoreComposeStack(stack)); err != nil {
		_, _ = s.store.UpdatePlacementReservationStatus(ctx, reservation.ID, store.PlacementReservationStatusCancelled)
		return nil, fmt.Errorf("create compose stack record: %w", err)
	}

	client := s.getClient()
	deployResp, err := client.ComposeDeploy(ctx, node.BaseURL, nodeCredential, daemon.ComposeDeployRequest{
		StackID:     stackID,
		ComposeYAML: req.ComposeYAML,
		EnvVars:     req.EnvVars,
	})

	if err != nil {
		s.markFailed(ctx, stack, err.Error())
		return nil, fmt.Errorf("compose deploy to node: %w", err)
	}

	_ = deployResp

	stack.Status = StackStatusRunning
	stack.UpdatedAt = time.Now().UTC()
	_ = s.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))

	_, _ = s.store.UpdatePlacementReservationStatus(ctx, reservation.ID, store.PlacementReservationStatusCompleted)

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("compose_stack_deployed", "compose", "stack", stackID, map[string]any{
			"name": req.Name, "nodeId": req.NodeID,
		}))
	}

	return stack, nil
}

func (s *Service) UpdateComposeStack(ctx context.Context, stackID string, req UpdateComposeRequest) (*ComposeStack, error) {
	existing, err := s.store.GetComposeStack(ctx, stackID)
	if err != nil {
		return nil, ErrStackNotFound
	}

	stack := fromStoreComposeStack(existing)

	if stack.Status != StackStatusRunning && stack.Status != StackStatusStopped && stack.Status != StackStatusDegraded {
		return nil, ErrStackNotRunnable
	}

	newHash := computeHash(req.ComposeYAML)
	if newHash == stack.ComposeHash && mapsEqual(stack.EnvVars, req.EnvVars) {
		return stack, nil
	}

	rollbackHash := stack.ComposeHash
	rollbackYAML := stack.ComposeYAML
	rollbackEnv := stack.EnvVars

	stack.Status = StackStatusUpdating
	stack.ComposeYAML = req.ComposeYAML
	stack.ComposeHash = newHash
	if req.MemoryMB > 0 {
		stack.MemoryMB = req.MemoryMB
	}
	if req.CPUShares > 0 {
		stack.CPUShares = req.CPUShares
	}
	if req.DiskMB > 0 {
		stack.DiskMB = req.DiskMB
	}
	stack.UpdatedAt = time.Now().UTC()

	if err := s.store.UpdateComposeStack(ctx, toStoreComposeStack(stack)); err != nil {
		return nil, fmt.Errorf("update compose stack record: %w", err)
	}

	node, err := s.store.GetNode(ctx, stack.NodeID)
	if err != nil {
		s.rollbackStack(ctx, stack, rollbackYAML, rollbackHash, rollbackEnv)
		return nil, fmt.Errorf("node not found: %w", err)
	}

	nodeCredential, err := s.store.GetNodeDaemonCredential(ctx, stack.NodeID)
	if err != nil {
		s.rollbackStack(ctx, stack, rollbackYAML, rollbackHash, rollbackEnv)
		return nil, fmt.Errorf("node credential not found: %w", err)
	}

	client := s.getClient()

	stopResp, stopErr := client.ComposeStop(ctx, node.BaseURL, nodeCredential, stackID)
	if stopErr != nil {
		s.rollbackStack(ctx, stack, rollbackYAML, rollbackHash, rollbackEnv)
		return nil, fmt.Errorf("stop stack for update: %w", stopErr)
	}
	_ = stopResp

	deleteResp, delErr := client.ComposeDelete(ctx, node.BaseURL, nodeCredential, stackID)
	if delErr != nil && !isNotFoundError(delErr) {
		s.rollbackStack(ctx, stack, rollbackYAML, rollbackHash, rollbackEnv)
		return nil, fmt.Errorf("delete old stack for update: %w", delErr)
	}
	_ = deleteResp

	deployResp, deployErr := client.ComposeDeploy(ctx, node.BaseURL, nodeCredential, daemon.ComposeDeployRequest{
		StackID:     stackID,
		ComposeYAML: req.ComposeYAML,
		EnvVars:     req.EnvVars,
	})
	if deployErr != nil {
		s.rollbackStack(ctx, stack, rollbackYAML, rollbackHash, rollbackEnv)
		return nil, fmt.Errorf("deploy updated stack: %w", deployErr)
	}

	stack.Status = StackStatusRunning
	stack.Error = ""
	stack.UpdatedAt = time.Now().UTC()
	_ = s.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
	_ = deployResp

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("compose_stack_updated", "compose", "stack", stackID, map[string]any{
			"nodeId": stack.NodeID,
		}))
	}

	return stack, nil
}

func (s *Service) DeleteComposeStack(ctx context.Context, stackID string) error {
	existing, err := s.store.GetComposeStack(ctx, stackID)
	if err != nil {
		return ErrStackNotFound
	}

	stack := fromStoreComposeStack(existing)

	if stack.Status == StackStatusDeleting || stack.Status == StackStatusDeleted {
		return nil
	}

	stack.Status = StackStatusDeleting
	stack.UpdatedAt = time.Now().UTC()
	_ = s.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))

	node, err := s.store.GetNode(ctx, stack.NodeID)
	if err != nil {
		s.markFailed(ctx, stack, "node not found during delete: "+err.Error())
		return fmt.Errorf("node not found: %w", err)
	}

	nodeCredential, err := s.store.GetNodeDaemonCredential(ctx, stack.NodeID)
	if err != nil {
		s.markFailed(ctx, stack, "node credential not found during delete: "+err.Error())
		return fmt.Errorf("node credential not found: %w", err)
	}

	client := s.getClient()
	delResp, delErr := client.ComposeDelete(ctx, node.BaseURL, nodeCredential, stackID)
	_ = delResp

	if delErr != nil && !isNotFoundError(delErr) {
		s.markFailed(ctx, stack, "delete compose from node: "+delErr.Error())
		return fmt.Errorf("delete compose from node: %w", delErr)
	}

	if stack.ReservationID != "" {
		_, _ = s.store.UpdatePlacementReservationStatus(ctx, stack.ReservationID, store.PlacementReservationStatusExpired)
	}

	stack.Status = StackStatusDeleted
	stack.Error = ""
	stack.UpdatedAt = time.Now().UTC()
	_ = s.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("compose_stack_deleted", "compose", "stack", stackID, map[string]any{
			"nodeId": stack.NodeID,
		}))
	}

	return nil
}

func (s *Service) GetStackStatus(ctx context.Context, stackID string) (*StackStatusResponse, error) {
	existing, err := s.store.GetComposeStack(ctx, stackID)
	if err != nil {
		return nil, ErrStackNotFound
	}

	stack := fromStoreComposeStack(existing)

	node, err := s.store.GetNode(ctx, stack.NodeID)
	if err != nil {
		return &StackStatusResponse{Stack: *stack, Services: nil}, nil
	}

	nodeCredential, err := s.store.GetNodeDaemonCredential(ctx, stack.NodeID)
	if err != nil {
		return &StackStatusResponse{Stack: *stack, Services: nil}, nil
	}

	client := s.getClient()
	statusResp, err := client.ComposeStatus(ctx, node.BaseURL, nodeCredential, stackID)
	if err != nil {
		return &StackStatusResponse{Stack: *stack, Services: nil}, nil
	}

	services := make([]ServiceState, 0, len(statusResp.Services))
	for _, svc := range statusResp.Services {
		services = append(services, ServiceState{
			Name:   svc.Name,
			Image:  svc.Image,
			Status: svc.Status,
			State:  svc.State,
			Ports:  svc.Ports,
		})
	}

	return &StackStatusResponse{Stack: *stack, Services: services}, nil
}

func (s *Service) GetStackLogs(ctx context.Context, stackID, service string, tail int) (*StackLogsResponse, error) {
	existing, err := s.store.GetComposeStack(ctx, stackID)
	if err != nil {
		return nil, ErrStackNotFound
	}

	stack := fromStoreComposeStack(existing)

	node, err := s.store.GetNode(ctx, stack.NodeID)
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}

	nodeCredential, err := s.store.GetNodeDaemonCredential(ctx, stack.NodeID)
	if err != nil {
		return nil, fmt.Errorf("node credential not found: %w", err)
	}

	if tail <= 0 {
		tail = 100
	}

	client := s.getClient()
	logs, err := client.ComposeLogs(ctx, node.BaseURL, nodeCredential, stackID, service, tail)
	if err != nil {
		return nil, fmt.Errorf("get compose logs: %w", err)
	}

	return &StackLogsResponse{
		StackID:  stackID,
		Services: map[string]string{"_all": logs},
	}, nil
}

func (s *Service) StartStack(ctx context.Context, stackID string) (*ComposeStack, error) {
	existing, err := s.store.GetComposeStack(ctx, stackID)
	if err != nil {
		return nil, ErrStackNotFound
	}

	stack := fromStoreComposeStack(existing)
	if stack.Status != StackStatusStopped {
		return nil, ErrStackNotRunnable
	}

	node, err := s.store.GetNode(ctx, stack.NodeID)
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}

	nodeCredential, err := s.store.GetNodeDaemonCredential(ctx, stack.NodeID)
	if err != nil {
		return nil, fmt.Errorf("node credential not found: %w", err)
	}

	client := s.getClient()
	startResp, startErr := client.ComposeStart(ctx, node.BaseURL, nodeCredential, stackID)
	_ = startResp

	if startErr != nil {
		return nil, fmt.Errorf("start compose stack: %w", startErr)
	}

	stack.Status = StackStatusRunning
	stack.UpdatedAt = time.Now().UTC()
	_ = s.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))

	return stack, nil
}

func (s *Service) StopStack(ctx context.Context, stackID string) (*ComposeStack, error) {
	existing, err := s.store.GetComposeStack(ctx, stackID)
	if err != nil {
		return nil, ErrStackNotFound
	}

	stack := fromStoreComposeStack(existing)
	if stack.Status != StackStatusRunning && stack.Status != StackStatusDegraded {
		return nil, ErrStackNotRunnable
	}

	node, err := s.store.GetNode(ctx, stack.NodeID)
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}

	nodeCredential, err := s.store.GetNodeDaemonCredential(ctx, stack.NodeID)
	if err != nil {
		return nil, fmt.Errorf("node credential not found: %w", err)
	}

	client := s.getClient()
	stopResp, stopErr := client.ComposeStop(ctx, node.BaseURL, nodeCredential, stackID)
	_ = stopResp

	if stopErr != nil {
		return nil, fmt.Errorf("stop compose stack: %w", stopErr)
	}

	stack.Status = StackStatusStopped
	stack.UpdatedAt = time.Now().UTC()
	_ = s.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))

	return stack, nil
}

func (s *Service) RestartStack(ctx context.Context, stackID string) (*ComposeStack, error) {
	if _, err := s.StopStack(ctx, stackID); err != nil {
		return nil, err
	}
	return s.StartStack(ctx, stackID)
}

func (s *Service) PullStack(ctx context.Context, stackID string) (*ComposeStack, error) {
	existing, err := s.store.GetComposeStack(ctx, stackID)
	if err != nil {
		return nil, ErrStackNotFound
	}

	stack := fromStoreComposeStack(existing)

	node, err := s.store.GetNode(ctx, stack.NodeID)
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}

	nodeCredential, err := s.store.GetNodeDaemonCredential(ctx, stack.NodeID)
	if err != nil {
		return nil, fmt.Errorf("node credential not found: %w", err)
	}

	client := s.getClient()
	pullResp, pullErr := client.ComposePull(ctx, node.BaseURL, nodeCredential, stackID)
	_ = pullResp

	if pullErr != nil {
		return nil, fmt.Errorf("pull compose images: %w", pullErr)
	}

	return stack, nil
}

func (s *Service) ListStacks(ctx context.Context, userID string) ([]*ComposeStack, error) {
	stacks, err := s.store.ListComposeStacks(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]*ComposeStack, 0, len(stacks))
	for _, st := range stacks {
		result = append(result, fromStoreComposeStack(st))
	}
	return result, nil
}

func (s *Service) GetStack(ctx context.Context, stackID string) (*ComposeStack, error) {
	existing, err := s.store.GetComposeStack(ctx, stackID)
	if err != nil {
		return nil, ErrStackNotFound
	}
	return fromStoreComposeStack(existing), nil
}

func (s *Service) markFailed(ctx context.Context, stack *ComposeStack, errMsg string) {
	stack.Status = StackStatusFailed
	stack.Error = errMsg
	stack.UpdatedAt = time.Now().UTC()
	_ = s.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
}

func (s *Service) rollbackStack(ctx context.Context, stack *ComposeStack, yaml, hash string, env map[string]string) {
	stack.Status = StackStatusRollingBack
	stack.ComposeYAML = yaml
	stack.ComposeHash = hash
	stack.EnvVars = env
	stack.Error = "update failed; rolling back to previous configuration"
	stack.UpdatedAt = time.Now().UTC()
	_ = s.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))

	node, err := s.store.GetNode(ctx, stack.NodeID)
	if err != nil {
		s.markFailed(ctx, stack, "rollback failed: node not found: "+err.Error())
		return
	}

	nodeCredential, err := s.store.GetNodeDaemonCredential(ctx, stack.NodeID)
	if err != nil {
		s.markFailed(ctx, stack, "rollback failed: node credential not found: "+err.Error())
		return
	}

	client := s.getClient()
	deployResp, deployErr := client.ComposeDeploy(ctx, node.BaseURL, nodeCredential, daemon.ComposeDeployRequest{
		StackID:     stack.ID,
		ComposeYAML: yaml,
		EnvVars:     env,
	})
	_ = deployResp

	if deployErr != nil {
		s.markFailed(ctx, stack, "rollback deploy failed: "+deployErr.Error())
		return
	}

	stack.Status = StackStatusRunning
	stack.Error = "rolled back after update failure"
	stack.UpdatedAt = time.Now().UTC()
	_ = s.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
}

func toStoreComposeStack(s *ComposeStack) *store.ComposeStack {
	return &store.ComposeStack{
		ID:            s.ID,
		UserID:        s.UserID,
		Name:          s.Name,
		NodeID:        s.NodeID,
		Status:        string(s.Status),
		ComposeYAML:   s.ComposeYAML,
		ComposeHash:   s.ComposeHash,
		EnvVars:       s.EnvVars,
		MemoryMB:      s.MemoryMB,
		CPUShares:     s.CPUShares,
		DiskMB:        s.DiskMB,
		Error:         s.Error,
		ReservationID: s.ReservationID,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
}

func fromStoreComposeStack(s store.ComposeStack) *ComposeStack {
	return &ComposeStack{
		ID:            s.ID,
		UserID:        s.UserID,
		Name:          s.Name,
		NodeID:        s.NodeID,
		Status:        StackStatus(s.Status),
		ComposeYAML:   s.ComposeYAML,
		ComposeHash:   s.ComposeHash,
		EnvVars:       s.EnvVars,
		MemoryMB:      s.MemoryMB,
		CPUShares:     s.CPUShares,
		DiskMB:        s.DiskMB,
		Error:         s.Error,
		ReservationID: s.ReservationID,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
}

func computeHash(yaml string) string {
	// Simple FNV-like hash for config change detection
	h := uint64(14695981039346656037)
	for i := 0; i < len(yaml); i++ {
		h ^= uint64(yaml[i])
		h *= 1099511628211
	}
	return fmt.Sprintf("%x", h)
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// isNotFoundError checks if the error is about a missing stack/container.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return containsAny(msg, "not found", "No such container", "does not exist", "404")
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		lowerS := strings.ToLower(s)
		lowerSub := strings.ToLower(sub)
		if len(lowerS) >= len(lowerSub) {
			for i := 0; i <= len(lowerS)-len(lowerSub); i++ {
				if lowerS[i:i+len(lowerSub)] == lowerSub {
					return true
				}
			}
		}
	}
	return false
}
