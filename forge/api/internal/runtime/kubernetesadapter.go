package runtime

import (
	"context"

	"gamepanel/forge/internal/daemon"
)

type KubernetesAdapter struct {
	client *daemon.Client
}

func NewKubernetesAdapter(client *daemon.Client) *KubernetesAdapter {
	return &KubernetesAdapter{client: client}
}

func (r *KubernetesAdapter) Name() string {
	return KubernetesProvider
}

func (r *KubernetesAdapter) Capabilities() Capabilities {
	return KubernetesCapabilities()
}

func (r *KubernetesAdapter) SupportsMigration() bool {
	return r != nil && r.client != nil
}

func (r *KubernetesAdapter) CreateServer(ctx context.Context, target Target, req CreateServerRequest) (CreateResponse, error) {
	if r == nil || r.client == nil {
		return CreateResponse{}, ErrRuntimeUnavailable
	}
	networkName := req.NetworkName
	if networkName == "" {
		networkName = "gamepanel"
	}
	response, err := r.client.CreateServer(ctx, target.NodeURL, target.NodeToken, daemon.CreateRequest{
		ServerID: firstNonEmpty(req.ServerID, target.ServerID),
		Image:    req.Image,
		Command:  req.Command,
		Env:      req.Env,
		Ports:    daemonPorts(req.Ports),
		Mounts:   daemonMounts(req.Mounts),
		MemoryMB: req.MemoryMB, SwapMB: req.SwapMB, CPUShares: req.CPUShares, CPUPercent: req.CPULimit,
		CPUSet: req.Threads, IOWeight: req.IOWeight, OOMKillDisabled: req.OOMDisabled, PIDLimit: req.PIDLimit,
		StopSignal: req.StopSignal, StopTimeout: req.StopTimeout, UID: req.UID, GID: req.GID, DNS: req.DNS,
		NetworkName: networkName, NetworkSubnet: req.NetworkSubnet, NetworkGateway: req.NetworkGateway, NetworkIP: req.NetworkIP,
		RegistryAuth: daemonRegistryAuth(req.RegistryAuth), DiskMB: req.DiskMB, Provider: KubernetesProvider,
	})
	if err != nil {
		return CreateResponse{}, err
	}
	return CreateResponse{ServerID: response.ServerID, Accepted: response.Accepted, Mode: response.Mode}, nil
}

func (r *KubernetesAdapter) InstallServer(ctx context.Context, target Target, req InstallRequest) (InstallResponse, error) {
	return r.runInstaller(ctx, target, req, false)
}

func (r *KubernetesAdapter) ReinstallServer(ctx context.Context, target Target, req InstallRequest) (InstallResponse, error) {
	return r.runInstaller(ctx, target, req, true)
}

func (r *KubernetesAdapter) runInstaller(ctx context.Context, target Target, req InstallRequest, reinstall bool) (InstallResponse, error) {
	if r == nil || r.client == nil {
		return InstallResponse{}, ErrRuntimeUnavailable
	}
	request := daemon.InstallRequest{ServerID: req.ServerID, Image: req.Image, Entrypoint: req.Entrypoint, Script: req.Script, Env: req.Env}
	var response daemon.InstallResponse
	var err error
	if reinstall {
		response, err = r.client.ReinstallServer(ctx, target.NodeURL, target.NodeToken, target.ServerID, request)
	} else {
		response, err = r.client.InstallServer(ctx, target.NodeURL, target.NodeToken, target.ServerID, request)
	}
	if err != nil {
		return InstallResponse{}, err
	}
	return InstallResponse{ServerID: response.ServerID, Accepted: response.Accepted, Mode: response.Mode, ExitCode: response.ExitCode, Logs: response.Logs}, nil
}

func (r *KubernetesAdapter) SyncServerConfiguration(ctx context.Context, target Target, config ServerConfiguration) error {
	if r == nil || r.client == nil {
		return ErrRuntimeUnavailable
	}
	return r.client.SyncServerConfiguration(ctx, target.NodeURL, target.NodeToken, target.ServerID, daemon.ServerConfiguration{
		UUID: config.UUID, Name: config.Name, Suspended: config.Suspended, Environment: config.Environment,
		Invocation: config.Invocation, DockerImage: config.DockerImage, Egg: config.Egg, Build: config.Build,
		Allocations: config.Allocations, Config: config.Config, Mounts: daemonMounts(config.Mounts),
		Provider: KubernetesProvider,
	})
}

func (r *KubernetesAdapter) ResizeServer(ctx context.Context, target Target, memoryMB, cpu int64) error {
	if r == nil || r.client == nil {
		return ErrRuntimeUnavailable
	}
	return r.client.SyncServerConfiguration(ctx, target.NodeURL, target.NodeToken, target.ServerID, daemon.ServerConfiguration{
		UUID: target.ServerID,
		Build: map[string]any{"memoryLimit": memoryMB, "cpuShares": cpu},
	})
}

func (r *KubernetesAdapter) DeleteServer(ctx context.Context, target Target) (PowerResponse, error) {
	if r == nil || r.client == nil {
		return PowerResponse{}, ErrRuntimeUnavailable
	}
	response, err := r.client.DeleteServer(ctx, target.NodeURL, target.NodeToken, target.ServerID)
	return powerResponse(response), err
}

func (r *KubernetesAdapter) StartServer(ctx context.Context, target Target) (PowerResponse, error) {
	return r.sendPower(ctx, target, "start")
}

func (r *KubernetesAdapter) StopServer(ctx context.Context, target Target) (PowerResponse, error) {
	return r.sendPower(ctx, target, "stop")
}

func (r *KubernetesAdapter) RestartServer(ctx context.Context, target Target) (PowerResponse, error) {
	return r.sendPower(ctx, target, "restart")
}

func (r *KubernetesAdapter) KillServer(ctx context.Context, target Target) (PowerResponse, error) {
	return r.sendPower(ctx, target, "kill")
}

func (r *KubernetesAdapter) Stats(ctx context.Context, target Target) (Stats, error) {
	if r == nil || r.client == nil {
		return Stats{}, ErrRuntimeUnavailable
	}
	stats, err := r.client.Stats(ctx, target.NodeURL, target.NodeToken, target.ServerID)
	if err != nil {
		return Stats{}, err
	}
	return Stats{
		CPUPercent:     stats.CPUPercent,
		MemoryBytes:    stats.MemoryBytes,
		MemoryLimit:    stats.MemoryLimit,
		NetworkRxBytes: stats.NetworkRxBytes,
		NetworkTxBytes: stats.NetworkTxBytes,
	}, nil
}

func (r *KubernetesAdapter) Exists(ctx context.Context, target Target) (bool, error) {
	if _, err := r.Stats(ctx, target); err != nil {
		return false, err
	}
	return true, nil
}

func (r *KubernetesAdapter) Inspect(ctx context.Context, target Target) (Inspection, error) {
	exists, err := r.Exists(ctx, target)
	if err != nil {
		return Inspection{}, err
	}
	return Inspection{ServerID: target.ServerID, Exists: exists, Provider: KubernetesProvider}, nil
}

func (r *KubernetesAdapter) PrepareMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	return MigrationResponse{MigrationID: req.MigrationID, Accepted: false, Mode: "not_implemented"}, ErrNotImplemented
}

func (r *KubernetesAdapter) ExecuteMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	return MigrationResponse{MigrationID: req.MigrationID, Accepted: false, Mode: "not_implemented"}, ErrNotImplemented
}

func (r *KubernetesAdapter) CancelMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	return MigrationResponse{MigrationID: req.MigrationID, Accepted: false, Mode: "not_implemented"}, ErrNotImplemented
}

func (r *KubernetesAdapter) sendPower(ctx context.Context, target Target, signal string) (PowerResponse, error) {
	if r == nil || r.client == nil {
		return PowerResponse{}, ErrRuntimeUnavailable
	}
	response, err := r.client.SendPower(ctx, target.NodeURL, target.NodeToken, target.ServerID, signal)
	return powerResponse(response), err
}
