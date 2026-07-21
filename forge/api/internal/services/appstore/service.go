package appstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"gamepanel/forge/internal/services/compose"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

type Service struct {
	store      *store.Store
	composeSvc *compose.Service
}

func New(store *store.Store, composeSvc *compose.Service) (*Service, error) {
	if store == nil {
		return nil, errors.New("store required")
	}
	if composeSvc == nil {
		return nil, errors.New("compose service required")
	}
	return &Service{store: store, composeSvc: composeSvc}, nil
}

func (s *Service) ListApps(ctx context.Context, category, search string) ([]store.AppStoreApp, error) {
	return s.store.ListAppStoreApps(ctx, category, search)
}

func (s *Service) GetApp(ctx context.Context, key string) (*store.AppStoreApp, error) {
	return s.store.GetAppStoreApp(ctx, key)
}

func (s *Service) ListInstalls(ctx context.Context) ([]store.AppStoreInstall, error) {
	return s.store.ListAppStoreInstalls(ctx)
}

type InstallRequest struct {
	AppKey        string            `json:"appKey"`
	Name          string            `json:"name"`
	ProjectID     string            `json:"projectId"`
	EnvironmentID string            `json:"environmentId"`
	Params        map[string]string `json:"params"`
	NodeID        string            `json:"nodeId"`
	MemoryMB      int64             `json:"memoryMb"`
	CPUShares     int64             `json:"cpuShares"`
	DiskMB        int64             `json:"diskMb"`
	UserID        string            `json:"userId"`
}

func (s *Service) InstallApp(ctx context.Context, req *InstallRequest) (*store.AppStoreInstall, error) {
	app, err := s.store.GetAppStoreApp(ctx, req.AppKey)
	if err != nil {
		return nil, fmt.Errorf("app not found: %w", err)
	}

	paramsJSON, _ := json.Marshal(req.Params)
	resolvedCompose := resolveTemplate(app.ComposeContent, req.Params)

	inst := &store.AppStoreInstall{
		ID:             uuid.NewString(),
		AppKey:         app.Key,
		AppVersion:     app.Version,
		ProjectID:      req.ProjectID,
		EnvironmentID:  req.EnvironmentID,
		Name:           req.Name,
		Status:         "installing",
		Params:         paramsJSON,
		ComposeContent: resolvedCompose,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	if err := s.store.CreateAppStoreInstall(ctx, inst); err != nil {
		return nil, fmt.Errorf("create install record: %w", err)
	}

	composeReq := compose.DeployComposeRequest{
		UserID:      req.UserID,
		Name:        req.Name,
		NodeID:      req.NodeID,
		ComposeYAML: resolvedCompose,
		EnvVars:     req.Params,
		MemoryMB:    req.MemoryMB,
		CPUShares:   req.CPUShares,
		DiskMB:      req.DiskMB,
	}

	stack, deployErr := s.composeSvc.DeployComposeStack(ctx, composeReq)
	if deployErr != nil {
		_ = s.store.UpdateAppStoreInstallStatus(ctx, inst.ID, "error", deployErr.Error())
		inst.Status = "error"
		inst.ErrorMessage = deployErr.Error()
		return inst, fmt.Errorf("deploy compose stack: %w", deployErr)
	}

	_ = s.store.UpdateAppStoreInstallComposeProject(ctx, inst.ID, stack.ID)
	inst.ComposeProjectID = stack.ID

	status := "running"
	if string(stack.Status) == "degraded" || string(stack.Status) == "failed" {
		status = "error"
	}
	_ = s.store.UpdateAppStoreInstallStatus(ctx, inst.ID, status, stack.Error)
	inst.Status = status
	inst.ErrorMessage = stack.Error

	return inst, nil
}

func (s *Service) UninstallApp(ctx context.Context, installID string) error {
	inst, err := s.store.GetAppStoreInstall(ctx, installID)
	if err != nil {
		return fmt.Errorf("install not found: %w", err)
	}

	if inst.ComposeProjectID != "" {
		if err := s.composeSvc.DeleteComposeStack(ctx, inst.ComposeProjectID); err != nil {
			slog.Warn("uninstall: delete compose stack", "id", inst.ComposeProjectID, "error", err)
		}
	}

	_ = s.store.UpdateAppStoreInstallStatus(ctx, installID, "uninstalling", "")
	return s.store.DeleteAppStoreInstall(ctx, installID)
}

func (s *Service) UpgradeApp(ctx context.Context, installID string) (*store.AppStoreInstall, error) {
	inst, err := s.store.GetAppStoreInstall(ctx, installID)
	if err != nil {
		return nil, fmt.Errorf("install not found: %w", err)
	}

	app, err := s.store.GetAppStoreApp(ctx, inst.AppKey)
	if err != nil {
		return nil, fmt.Errorf("app not found: %w", err)
	}

	var params map[string]string
	if len(inst.Params) > 0 {
		_ = json.Unmarshal(inst.Params, &params)
	}

	resolvedCompose := resolveTemplate(app.ComposeContent, params)

	_ = s.store.UpdateAppStoreInstallStatus(ctx, installID, "upgrading", "")
	inst.Status = "upgrading"
	inst.AppVersion = app.Version
	inst.ComposeContent = resolvedCompose

	updateReq := compose.UpdateComposeRequest{
		ComposeYAML: resolvedCompose,
		EnvVars:     params,
	}

	if inst.ComposeProjectID != "" {
		_, updateErr := s.composeSvc.UpdateComposeStack(ctx, inst.ComposeProjectID, updateReq)
		if updateErr != nil {
			_ = s.store.UpdateAppStoreInstallStatus(ctx, installID, "error", updateErr.Error())
			inst.Status = "error"
			inst.ErrorMessage = updateErr.Error()
			return inst, updateErr
		}
	}

	_ = s.store.UpdateAppStoreInstallStatus(ctx, installID, "running", "")
	inst.Status = "running"
	return inst, nil
}

func (s *Service) SyncFromRemote(ctx context.Context, registryURL string) error {
	if registryURL == "" {
		return fmt.Errorf("registry URL is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registryURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch registry: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("registry returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return fmt.Errorf("failed to read registry response: %w", err)
	}
	var apps []store.AppStoreApp
	if err := json.Unmarshal(body, &apps); err != nil {
		slog.Warn("failed to parse registry response as app list, storing raw response", "error", err)
		return nil
	}
	for _, app := range apps {
		if err := s.store.UpsertAppStoreApp(ctx, &app); err != nil {
			slog.Warn("failed to store app from registry", "name", app.Name, "error", err)
		}
	}
	slog.Info("synced apps from remote registry", "url", registryURL, "count", len(apps))
	return nil
}

func resolveTemplate(tmpl string, params map[string]string) string {
	if params == nil {
		return tmpl
	}
	result := tmpl
	for k, v := range params {
		result = strings.ReplaceAll(result, "${"+k+"}", v)
		result = strings.ReplaceAll(result, "${"+k+":-}", v)
	}
	return result
}
