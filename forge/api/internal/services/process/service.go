package process

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"gamepanel/forge/internal/store"
)

var validProcessTypes = map[string]bool{
	"web":     true,
	"worker":  true,
	"clock":   true,
	"release": true,
}

var procfileLineRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_]*)\s*:\s*(.+)$`)

type Service struct {
	store  *store.Store
	daemon DaemonClient
	logger *slog.Logger
}

type DaemonClient interface {
	StartContainer(ctx context.Context, serverID, processType string) error
	StopContainer(ctx context.Context, serverID, processType string) error
	RunContainer(ctx context.Context, serverID, command string) (string, error)
}

func New(s *store.Store, daemon DaemonClient, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:  s,
		daemon: daemon,
		logger: logger,
	}
}

func ValidateProcessType(pt string) error {
	if !validProcessTypes[pt] {
		return fmt.Errorf("invalid process type %q: must be one of web, worker, clock, release", pt)
	}
	return nil
}

type ProcfileEntry struct {
	ProcessType string `json:"processType"`
	Command     string `json:"command"`
}

func ParseProcfile(content string) ([]ProcfileEntry, error) {
	var entries []ProcfileEntry
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		matches := procfileLineRe.FindStringSubmatch(line)
		if matches == nil {
			return nil, fmt.Errorf("line %d: invalid Procfile format (expected TYPE: command)", i+1)
		}
		processType := strings.ToLower(matches[1])
		if !validProcessTypes[processType] {
			return nil, fmt.Errorf("line %d: invalid process type %q (must be web, worker, clock, release)", i+1, processType)
		}
		entries = append(entries, ProcfileEntry{
			ProcessType: processType,
			Command:     strings.TrimSpace(matches[2]),
		})
	}
	return entries, nil
}

func (s *Service) SetProcesses(ctx context.Context, serverID string, entries []ProcfileEntry) ([]store.ProcessType, error) {
	var results []store.ProcessType
	for _, entry := range entries {
		pt, err := s.store.UpsertProcessType(ctx, serverID, entry.ProcessType, entry.Command, 1)
		if err != nil {
			return nil, fmt.Errorf("upsert process type %q: %w", entry.ProcessType, err)
		}
		results = append(results, pt)
	}
	return results, nil
}

func (s *Service) ScaleProcess(ctx context.Context, serverID, processType string, quantity int, triggeredBy string) (store.ProcessType, error) {
	if quantity < 0 {
		return store.ProcessType{}, errors.New("quantity must be >= 0")
	}

	pt, err := s.store.GetProcessType(ctx, serverID, processType)
	if err != nil {
		return store.ProcessType{}, fmt.Errorf("process type %q not found: %w", processType, err)
	}

	oldQuantity := pt.Quantity
	if oldQuantity == quantity {
		return pt, nil
	}

	if _, err := s.store.CreateProcessScalingEvent(ctx, serverID, processType, oldQuantity, quantity, triggeredBy); err != nil {
		s.logger.Error("failed to record scaling event", "error", err)
	}

	if err := s.store.SetProcessTypeQuantity(ctx, serverID, processType, quantity); err != nil {
		return store.ProcessType{}, fmt.Errorf("update quantity: %w", err)
	}

	for i := quantity; i < oldQuantity; i++ {
		label := fmt.Sprintf("%s-%d", processType, i)
		if err := s.daemon.StopContainer(ctx, serverID, label); err != nil {
			s.logger.Error("failed to stop container", "processType", processType, "container", label, "error", err)
		}
	}

	for i := oldQuantity; i < quantity; i++ {
		label := fmt.Sprintf("%s-%d", processType, i)
		if err := s.daemon.StartContainer(ctx, serverID, label); err != nil {
			s.logger.Error("failed to start container", "processType", processType, "container", label, "error", err)
		}
	}

	pt, _ = s.store.GetProcessType(ctx, serverID, processType)
	return pt, nil
}

func (s *Service) RunOneOffTask(ctx context.Context, serverID, command string) (store.OneOffTask, error) {
	task, err := s.store.CreateOneOffTask(ctx, serverID, command)
	if err != nil {
		return store.OneOffTask{}, fmt.Errorf("create task: %w", err)
	}

	go func() {
		taskCtx := context.Background()
		output, runErr := s.daemon.RunContainer(taskCtx, serverID, command)
		status := "completed"
		if runErr != nil {
			status = "failed"
			output = runErr.Error()
		}
		if updateErr := s.store.UpdateOneOffTask(taskCtx, task.ID, status, output); updateErr != nil {
			s.logger.Error("failed to update one-off task", "id", task.ID, "error", updateErr)
		}
	}()

	return task, nil
}

func (s *Service) ListProcesses(ctx context.Context, serverID string) ([]store.ProcessType, error) {
	return s.store.ListProcessTypes(ctx, serverID)
}

func (s *Service) GetScalingHistory(ctx context.Context, serverID string) ([]store.ProcessScalingEvent, error) {
	return s.store.GetScalingHistory(ctx, serverID)
}

func (s *Service) ListTasks(ctx context.Context, serverID string) ([]store.OneOffTask, error) {
	return s.store.ListOneOffTasks(ctx, serverID)
}
