package compose

import (
	"context"
	"encoding/json"
	"fmt"
)

type ComposeQueueHandler struct {
	service *Service
}

func NewQueueHandler(service *Service) *ComposeQueueHandler {
	return &ComposeQueueHandler{service: service}
}

type composeDeployPayload struct {
	UserID      string            `json:"userId"`
	Name        string            `json:"name"`
	NodeID      string            `json:"nodeId"`
	ComposeYAML string            `json:"composeYaml"`
	EnvVars     map[string]string `json:"envVars,omitempty"`
	MemoryMB    int64             `json:"memoryMb"`
	CPUShares   int64             `json:"cpuShares"`
	DiskMB      int64             `json:"diskMb"`
}

type composeUpdatePayload struct {
	StackID     string            `json:"stackId"`
	ComposeYAML string            `json:"composeYaml"`
	EnvVars     map[string]string `json:"envVars,omitempty"`
	MemoryMB    int64             `json:"memoryMb"`
	CPUShares   int64             `json:"cpuShares"`
	DiskMB      int64             `json:"diskMb"`
}

type composeDeletePayload struct {
	StackID string `json:"stackId"`
}

type composeStartStopPayload struct {
	StackID string `json:"stackId"`
}

func (h *ComposeQueueHandler) HandleDeploy(ctx context.Context, payload json.RawMessage) error {
	var req composeDeployPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return fmt.Errorf("invalid compose deploy payload: %w", err)
	}
	_, err := h.service.DeployComposeStack(ctx, DeployComposeRequest{
		UserID:      req.UserID,
		Name:        req.Name,
		NodeID:      req.NodeID,
		ComposeYAML: req.ComposeYAML,
		EnvVars:     req.EnvVars,
		MemoryMB:    req.MemoryMB,
		CPUShares:   req.CPUShares,
		DiskMB:      req.DiskMB,
	})
	return err
}

func (h *ComposeQueueHandler) HandleUpdate(ctx context.Context, payload json.RawMessage) error {
	var req composeUpdatePayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return fmt.Errorf("invalid compose update payload: %w", err)
	}
	_, err := h.service.UpdateComposeStack(ctx, req.StackID, UpdateComposeRequest{
		ComposeYAML: req.ComposeYAML,
		EnvVars:     req.EnvVars,
		MemoryMB:    req.MemoryMB,
		CPUShares:   req.CPUShares,
		DiskMB:      req.DiskMB,
	})
	return err
}

func (h *ComposeQueueHandler) HandleDelete(ctx context.Context, payload json.RawMessage) error {
	var req composeDeletePayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return fmt.Errorf("invalid compose delete payload: %w", err)
	}
	return h.service.DeleteComposeStack(ctx, req.StackID)
}

func (h *ComposeQueueHandler) HandleStart(ctx context.Context, payload json.RawMessage) error {
	var req composeStartStopPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return fmt.Errorf("invalid compose start payload: %w", err)
	}
	_, err := h.service.StartStack(ctx, req.StackID)
	return err
}

func (h *ComposeQueueHandler) HandleStop(ctx context.Context, payload json.RawMessage) error {
	var req composeStartStopPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return fmt.Errorf("invalid compose stop payload: %w", err)
	}
	_, err := h.service.StopStack(ctx, req.StackID)
	return err
}

func (h *ComposeQueueHandler) HandleRestart(ctx context.Context, payload json.RawMessage) error {
	var req composeStartStopPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return fmt.Errorf("invalid compose restart payload: %w", err)
	}
	_, err := h.service.RestartStack(ctx, req.StackID)
	return err
}
