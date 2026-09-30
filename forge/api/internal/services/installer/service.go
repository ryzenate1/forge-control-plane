package installer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/google/uuid"
)

type InstallStatus string

const (
	InstallPending   InstallStatus = "pending"
	InstallRunning   InstallStatus = "running"
	InstallCompleted InstallStatus = "completed"
	InstallFailed    InstallStatus = "failed"
)

type WorkflowType string

const (
	WorkflowInstall   WorkflowType = "install"
	WorkflowUninstall WorkflowType = "uninstall"
	WorkflowReinstall WorkflowType = "reinstall"
)

type InstallStep struct {
	ID          string        `json:"id"`
	WorkflowID  string        `json:"workflowId"`
	Sequence    int           `json:"sequence"`
	Name        string        `json:"name"`
	Action      string        `json:"action"`
	Status      InstallStatus `json:"status"`
	StartedAt   *time.Time    `json:"startedAt,omitempty"`
	CompletedAt *time.Time    `json:"completedAt,omitempty"`
	Error       string        `json:"error,omitempty"`
}

type Workflow struct {
	ID          string          `json:"id"`
	ServerID    string          `json:"serverId"`
	Type        WorkflowType    `json:"type"`
	Status      InstallStatus   `json:"status"`
	Steps       []InstallStep   `json:"steps"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	CompletedAt *time.Time      `json:"completedAt,omitempty"`
}

type Store interface {
	CreateWorkflow(ctx context.Context, wf *Workflow) error
	GetWorkflow(ctx context.Context, id string) (*Workflow, error)
	ListWorkflows(ctx context.Context, serverID string) ([]Workflow, error)
	UpdateStep(ctx context.Context, stepID string, status InstallStatus, err string) error
}

// WorkflowExecutor runs a workflow's steps against a node. No implementation
// exists yet; the canonical install path is beacon's POST /servers/:id/install
// driven by clustermanager. Until one is wired, ExecuteWorkflow refuses rather
// than leaving a workflow claiming to run.
type WorkflowExecutor interface {
	ExecuteWorkflow(ctx context.Context, wf *Workflow) error
}

// ErrNoWorkflowExecutor reports that nothing can carry out a workflow's steps.
var ErrNoWorkflowExecutor = errors.New("installer workflow execution is not implemented; the canonical install path is beacon POST /servers/:id/install")

type Service struct {
	store    Store
	executor WorkflowExecutor
}

func New(store Store) *Service {
	return &Service{store: store}
}

// SetExecutor attaches the component that actually runs workflow steps.
func (s *Service) SetExecutor(exec WorkflowExecutor) {
	if s == nil {
		return
	}
	s.executor = exec
}

// CanExecute reports whether both the operator has opted in and something
// exists to carry the work out. Both must be true; an opted-in flag with no
// executor is what previously produced workflows stuck at "running".
func (s *Service) CanExecute() bool {
	return IsEnabled() && s != nil && s.executor != nil
}

func IsEnabled() bool {
	return os.Getenv("INSTALLER_WORKFLOW_ENABLED") == "1"
}

func (s *Service) ListWorkflows(ctx context.Context, serverID string) ([]Workflow, error) {
	return s.store.ListWorkflows(ctx, serverID)
}

func (s *Service) GetWorkflow(ctx context.Context, id string) (*Workflow, error) {
	return s.store.GetWorkflow(ctx, id)
}

func (s *Service) ListRecentWorkflows(ctx context.Context, limit int) ([]Workflow, error) {
	if pg, ok := s.store.(*PostgresStore); ok {
		return pg.ListRecentWorkflows(ctx, limit)
	}
	return nil, nil
}

func (s *Service) CreateReinstallWorkflow(ctx context.Context, serverID string) (*Workflow, error) {
	wf := &Workflow{
		ID:        uuid.NewString(),
		ServerID:  serverID,
		Type:      WorkflowReinstall,
		Status:    InstallPending,
		Steps:     defaultInstallSteps(),
		CreatedAt: time.Now().UTC(),
	}
	return wf, s.store.CreateWorkflow(ctx, wf)
}

// ExecuteWorkflow runs a workflow's steps. It reported a workflow as running
// and then did nothing, so an operator who enabled INSTALLER_WORKFLOW_ENABLED
// saw installs that never progressed and never failed. It now refuses when
// there is no executor, and marks every step failed so the workflow reaches a
// terminal state instead of sitting at "running" forever.
//
// The previous implementation also passed a workflow ID to UpdateStep, which
// looks rows up by step ID, so even the status write did not land.
func (s *Service) ExecuteWorkflow(ctx context.Context, workflowID string) error {
	wf, err := s.store.GetWorkflow(ctx, workflowID)
	if err != nil {
		return err
	}
	if s.executor == nil {
		s.failWorkflow(ctx, wf, ErrNoWorkflowExecutor.Error())
		return ErrNoWorkflowExecutor
	}
	if err := s.executor.ExecuteWorkflow(ctx, wf); err != nil {
		s.failWorkflow(ctx, wf, err.Error())
		return err
	}
	return nil
}

func (s *Service) failWorkflow(ctx context.Context, wf *Workflow, reason string) {
	if wf == nil {
		return
	}
	for _, step := range wf.Steps {
		if step.Status == InstallCompleted || step.Status == InstallFailed {
			continue
		}
		_ = s.store.UpdateStep(ctx, step.ID, InstallFailed, reason)
	}
}

func (s *Service) CreateInstallWorkflow(ctx context.Context, serverID string) (*Workflow, error) {
	wf := &Workflow{
		ID:        uuid.NewString(),
		ServerID:  serverID,
		Type:      WorkflowInstall,
		Status:    InstallPending,
		Steps:     defaultInstallSteps(),
		CreatedAt: time.Now().UTC(),
	}
	return wf, s.store.CreateWorkflow(ctx, wf)
}

func (s *Service) CreateUninstallWorkflow(ctx context.Context, serverID string) (*Workflow, error) {
	wf := &Workflow{
		ID:        uuid.NewString(),
		ServerID:  serverID,
		Type:      WorkflowUninstall,
		Status:    InstallPending,
		Steps:     defaultUninstallSteps(),
		CreatedAt: time.Now().UTC(),
	}
	return wf, s.store.CreateWorkflow(ctx, wf)
}

func defaultInstallSteps() []InstallStep {
	return []InstallStep{
		{ID: uuid.NewString(), Sequence: 1, Name: "Create Container", Action: "docker.create"},
		{ID: uuid.NewString(), Sequence: 2, Name: "Setup Data Directory", Action: "filesystem.setup"},
		{ID: uuid.NewString(), Sequence: 3, Name: "Download Server Files", Action: "download.server"},
		{ID: uuid.NewString(), Sequence: 4, Name: "Run Install Script", Action: "script.install"},
		{ID: uuid.NewString(), Sequence: 5, Name: "Configure Server", Action: "config.apply"},
		{ID: uuid.NewString(), Sequence: 6, Name: "Start Server", Action: "server.start"},
	}
}

func defaultUninstallSteps() []InstallStep {
	return []InstallStep{
		{ID: uuid.NewString(), Sequence: 1, Name: "Stop Server", Action: "server.stop"},
		{ID: uuid.NewString(), Sequence: 2, Name: "Delete Container", Action: "docker.delete"},
		{ID: uuid.NewString(), Sequence: 3, Name: "Remove Data Directory", Action: "filesystem.cleanup"},
		{ID: uuid.NewString(), Sequence: 4, Name: "Cleanup Resources", Action: "resources.cleanup"},
	}
}
