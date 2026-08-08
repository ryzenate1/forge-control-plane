// Package catalog implements the one-click managed service catalog: the
// unified catalog of provisionable services (databases, caches and queues),
// sync provisioning onto the existing db_containers / compose runtimes,
// connection-string injection into environments, and backup retention for
// Phase 3.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gamepanel/forge/internal/services/compose"
	"gamepanel/forge/internal/services/envvars"
	"gamepanel/forge/internal/store"
)

// Provision statuses persisted on catalog_instances rows.
const (
	StatusProvisioning = "provisioning"
	StatusRunning      = "running"
	StatusError        = "error"

	RefTypeDBContainer = "db_container"
	RefTypeCompose     = "compose_stack"
)

// DBProvider provisions a database container through the managed databases /
// db_containers runtime. Satisfied by *dbprovisioner.DBContainerService.
type DBProvider interface {
	Provision(ctx context.Context, serverID, engine, version string, memoryMB, cpuShares int) (store.DBContainer, error)
}

// ComposeProvider deploys a compose project stack. Satisfied by
// *compose.Service.
type ComposeProvider interface {
	DeployComposeStack(ctx context.Context, req compose.DeployComposeRequest) (*compose.ComposeStack, error)
}

// catalogStore is the store surface the catalog service consumes.
type catalogStore interface {
	ListCatalogEntries(ctx context.Context, enabledOnly bool) ([]store.CatalogEntry, error)
	GetCatalogEntry(ctx context.Context, key string) (*store.CatalogEntry, error)
	CreateCatalogInstance(ctx context.Context, inst store.CatalogInstance) (store.CatalogInstance, error)
	GetCatalogInstance(ctx context.Context, id string) (*store.CatalogInstance, error)
	ListCatalogInstances(ctx context.Context, entryKey, envID string) ([]store.CatalogInstance, error)
	UpdateCatalogInstanceStatus(ctx context.Context, id string, ref store.CatalogInstanceRef) error
	CreateCatalogAttachLink(ctx context.Context, instanceID, envID, varPrefix string) (store.CatalogAttachLink, error)
	ListCatalogAttachLinks(ctx context.Context, instanceID string) ([]store.CatalogAttachLink, error)
	GetNode(ctx context.Context, nodeID string) (store.Node, error)
	GetRetentionPolicy(ctx context.Context, kind string) (*store.BackupRetention, error)
	ListRetentionPolicies(ctx context.Context) ([]store.BackupRetention, error)
	SetRetentionPolicy(ctx context.Context, kind string, retentionDays, retentionMax int, enabled bool) (store.BackupRetention, error)
	DeleteExpiredManagedBackups(ctx context.Context, kind string, until time.Time, retentionMax int) (int64, error)
}

// Service implements the one-click catalog provisioning.
type Service struct {
	store      catalogStore
	dbProvider DBProvider
	composeSvc ComposeProvider
	envSvc     *envvars.Service
	logger     *slog.Logger
}

// Options configures the catalog service.
type Options struct {
	Store        catalogStore
	DBProvider   DBProvider
	ComposeStack ComposeProvider
	EnvSvc       *envvars.Service
	Logger       *slog.Logger
}

// New validates options and builds the catalog service.
func New(opts Options) (*Service, error) {
	if opts.Store == nil {
		return nil, errors.New("catalog: store is required")
	}
	if opts.DBProvider == nil {
		return nil, errors.New("catalog: database container provider is required")
	}
	if opts.ComposeStack == nil {
		return nil, errors.New("catalog: compose provider is required")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Service{
		store:      opts.Store,
		dbProvider: opts.DBProvider,
		composeSvc: opts.ComposeStack,
		envSvc:     opts.EnvSvc,
		logger:     opts.Logger,
	}, nil
}

// ProvisionInput is a one-click provision request.
type ProvisionInput struct {
	Kind          string
	Version       string
	EnvironmentID string
	NodeID        string
	MemoryMB      int
	CPUShares     int
	DiskMB        int
}

// Provision creates a catalog instance and provisions its backing runtime.
// DB kinds use the existing db_containers/managed_databases path
// (synchronously); cache/queue kinds deploy a baked compose template on the
// node (validated synchronously, warmed asynchronously like the existing
// managed-database handler). The resulting connection string is persisted on
// the instance row. When EnvironmentID is set the connection variables are
// injected into that environment too.
func (s *Service) Provision(ctx context.Context, in ProvisionInput) (*store.CatalogInstance, error) {
	entry, err := s.store.GetCatalogEntry(ctx, in.Kind)
	if err != nil {
		return nil, fmt.Errorf("catalog entry %q: %w", in.Kind, err)
	}
	if !entry.Enabled {
		return nil, fmt.Errorf("catalog entry %q is disabled", in.Kind)
	}
	version := strings.TrimSpace(in.Version)
	if version == "" {
		version = entry.DefaultVersion
	}
	if err := requireVersion(entry, version); err != nil {
		return nil, err
	}
	memoryMB := in.MemoryMB
	if memoryMB <= 0 {
		memoryMB = defaultMemoryFor(entry.Category)
	}
	cpuShares := in.CPUShares
	if cpuShares <= 0 {
		cpuShares = 512
	}

	created, err := s.store.CreateCatalogInstance(ctx, store.CatalogInstance{
		EntryKey:      entry.Key,
		Kind:          kindForEntry(entry.Key),
		Version:       version,
		EnvironmentID: in.EnvironmentID,
		NodeID:        in.NodeID,
		Status:        StatusProvisioning,
	})
	if err != nil {
		return nil, fmt.Errorf("create catalog instance: %w", err)
	}

	var ref store.CatalogInstanceRef
	if isManagedDBKind(entry.Key) {
		ref, err = s.provisionDB(ctx, entry, in.NodeID, version, memoryMB, cpuShares)
	} else {
		ref, err = s.provisionCompose(ctx, entry, in.NodeID, version, memoryMB, cpuShares, in.EnvironmentID)
	}
	if err != nil {
		_ = s.store.UpdateCatalogInstanceStatus(ctx, created.ID, store.CatalogInstanceRef{
			Status: StatusError,
			Error:  err.Error(),
		})
		return &created, fmt.Errorf("provision %s: %w", entry.Key, err)
	}

	ref.Status = StatusRunning
	ref.Error = ""
	if err := s.store.UpdateCatalogInstanceStatus(ctx, created.ID, ref); err != nil {
		return &created, fmt.Errorf("update catalog instance status: %w", err)
	}

	inst, err := s.store.GetCatalogInstance(ctx, created.ID)
	if err != nil {
		return &created, fmt.Errorf("reload catalog instance: %w", err)
	}

	if envID := strings.TrimSpace(in.EnvironmentID); envID != "" {
		if _, err := s.Attach(ctx, inst.ID, envID); err != nil {
			s.logger.Warn("catalog: attach env vars failed after provision",
				slog.String("instance_id", inst.ID), slog.String("environment_id", envID), slog.String("err", err.Error()))
		}
	}
	return inst, nil
}

// ListEntries returns the enabled catalog entries.
func (s *Service) ListEntries(ctx context.Context) ([]store.CatalogEntry, error) {
	entries, err := s.store.ListCatalogEntries(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list catalog entries: %w", err)
	}
	return entries, nil
}

// GetEntry returns one catalog entry.
func (s *Service) GetEntry(ctx context.Context, key string) (*store.CatalogEntry, error) {
	entry, err := s.store.GetCatalogEntry(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get catalog entry: %w", err)
	}
	return entry, nil
}

// ListInstances lists catalog instances for an entry key, optionally scoped
// to a single environment ("my instances" view).
func (s *Service) ListInstances(ctx context.Context, entryKey, envID string) ([]store.CatalogInstance, error) {
	instances, err := s.store.ListCatalogInstances(ctx, entryKey, envID)
	if err != nil {
		return nil, fmt.Errorf("list catalog instances: %w", err)
	}
	return instances, nil
}

// GetInstance returns a single catalog instance.
func (s *Service) GetInstance(ctx context.Context, id string) (*store.CatalogInstance, error) {
	inst, err := s.store.GetCatalogInstance(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get catalog instance: %w", err)
	}
	return inst, nil
}

func (s *Service) provisionDB(ctx context.Context, entry *store.CatalogEntry, nodeID, version string, memoryMB, cpuShares int) (store.CatalogInstanceRef, error) {
	container, err := s.dbProvider.Provision(ctx, nodeID, engineForEntry(entry.Key), version, memoryMB, cpuShares)
	if err != nil {
		return store.CatalogInstanceRef{}, err
	}
	return store.CatalogInstanceRef{
		RefType:     RefTypeDBContainer,
		InstanceRef: container.ID,
		Host:        extractHost(container.ConnectionString),
		Port:        container.Port,
		ConnString:  container.ConnectionString,
	}, nil
}

func (s *Service) provisionCompose(ctx context.Context, entry *store.CatalogEntry, nodeID, version string, memoryMB, cpuShares int, envID string) (store.CatalogInstanceRef, error) {
	password := randomPassword(24)
	template := composeTemplateFor(entry, version, password)
	host, err := s.nodeHost(ctx, nodeID)
	if err != nil {
		return store.CatalogInstanceRef{}, err
	}
	stack, err := s.composeSvc.DeployComposeStack(ctx, compose.DeployComposeRequest{
		UserID:        "catalog",
		Name:          templateName(entry, version, nodeID),
		NodeID:        nodeID,
		ComposeYAML:   template,
		EnvVars:       map[string]string{"CATALOG_VERSION": version},
		MemoryMB:      int64(memoryMB),
		CPUShares:     int64(cpuShares),
		DiskMB:        int64(defaultDiskMB(entry.Category)),
		EnvironmentID: envID,
	})
	if err != nil {
		return store.CatalogInstanceRef{}, fmt.Errorf("deploy compose stack: %w", err)
	}
	port := defaultPortFor(entry.Key)
	username := ""
	if entry.Key == "rabbitmq" || entry.Key == "clickhouse" {
		username = "app"
	}
	if entry.Key == "postgres" || entry.Key == "mysql" || entry.Key == "mariadb" || entry.Key == "mongodb" {
		username = "app"
	}
	return store.CatalogInstanceRef{
		RefType:     RefTypeCompose,
		InstanceRef: stack.ID,
		Host:        host,
		Port:        port,
		ConnString:  connectionStringFor(entry.Key, username, password, host, port, version),
	}, nil
}

func (s *Service) nodeHost(ctx context.Context, nodeID string) (string, error) {
	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		return "", fmt.Errorf("get node: %w", err)
	}
	if host := strings.TrimSpace(node.FQDN); host != "" {
		return host, nil
	}
	if host := strings.TrimSpace(node.PublicHostname); host != "" {
		return host, nil
	}
	return strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(node.BaseURL), "http://"), "/"), nil
}

func requireVersion(entry *store.CatalogEntry, version string) error {
	for _, v := range entry.Versions {
		if v == version {
			return nil
		}
	}
	return fmt.Errorf("unsupported version %q for %q; supported: %v", version, entry.Key, entry.Versions)
}

func defaultMemoryFor(category string) int {
	if category == "database" {
		return 512
	}
	return 256
}

func defaultDiskMB(category string) int {
	if category == "database" {
		return 20480
	}
	return 10240
}
