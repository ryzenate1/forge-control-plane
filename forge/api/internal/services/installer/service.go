package installer

import (
	"context"
	"encoding/json"
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

type Service struct {
	store Store
}

func New(store Store) *Service {
	return &Service{store: store}
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
