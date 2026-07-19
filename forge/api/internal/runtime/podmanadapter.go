package runtime

import (
	"context"

	"gamepanel/forge/internal/daemon"
)

type PodmanAdapter struct {
	client *daemon.Client
}

func NewPodmanAdapter(client *daemon.Client) *PodmanAdapter {
	return &PodmanAdapter{client: client}
}

func (r *PodmanAdapter) Name() string {
	return PodmanProvider
}

func (r *PodmanAdapter) Capabilities() Capabilities {
	return PodmanCapabilities()
}

func (r *PodmanAdapter) SupportsMigration() bool {
	return r != nil && r.client != nil
}

func (r *PodmanAdapter) CreateServer(ctx context.Context, target Target, req CreateServerRequest) (CreateResponse, error) {
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
		RegistryAuth: daemonRegistryAuth(req.RegistryAuth), DiskMB: req.DiskMB, Provider: PodmanProvider,
	})
	if err != nil {
		return CreateResponse{}, err
	}
	return CreateResponse{ServerID: response.ServerID, Accepted: response.Accepted, Mode: response.Mode}, nil
}

func (r *PodmanAdapter) InstallServer(ctx context.Context, target Target, req InstallRequest) (InstallResponse, error) {
	return r.runInstaller(ctx, target, req, false)
}

func (r *PodmanAdapter) ReinstallServer(ctx context.Context, target Target, req InstallRequest) (InstallResponse, error) {
	return r.runInstaller(ctx, target, req, true)
}

func (r *PodmanAdapter) runInstaller(ctx context.Context, target Target, req InstallRequest, reinstall bool) (InstallResponse, error) {
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

func (r *PodmanAdapter) SyncServerConfiguration(ctx context.Context, target Target, config ServerConfiguration) error {
	if r == nil || r.client == nil {
		return ErrRuntimeUnavailable
	}
	return r.client.SyncServerConfiguration(ctx, target.NodeURL, target.NodeToken, target.ServerID, daemon.ServerConfiguration{
		UUID: config.UUID, Name: config.Name, Suspended: config.Suspended, Environment: config.Environment,
		Invocation: config.Invocation, DockerImage: config.DockerImage, Egg: config.Egg, Build: config.Build,
		Allocations: config.Allocations, Config: config.Config, Mounts: daemonMounts(config.Mounts),
		Provider: PodmanProvider,
	})
}

func (r *PodmanAdapter) ResizeServer(ctx context.Context, target Target, memoryMB, cpu int64) error {
	if r == nil || r.client == nil {
		return ErrRuntimeUnavailable
	}
	return r.client.SyncServerConfiguration(ctx, target.NodeURL, target.NodeToken, target.ServerID, daemon.ServerConfiguration{
		UUID: target.ServerID,
		Build: map[string]any{"memoryLimit": memoryMB, "cpuShares": cpu},
	})
}

func (r *PodmanAdapter) DeleteServer(ctx context.Context, target Target) (PowerResponse, error) {
	if r == nil || r.client == nil {
		return PowerResponse{}, ErrRuntimeUnavailable
	}
	response, err := r.client.DeleteServer(ctx, target.NodeURL, target.NodeToken, target.ServerID)
	return powerResponse(response), err
}

func (r *PodmanAdapter) StartServer(ctx context.Context, target Target) (PowerResponse, error) {
	return r.sendPower(ctx, target, "start")
}

func (r *PodmanAdapter) StopServer(ctx context.Context, target Target) (PowerResponse, error) {
	return r.sendPower(ctx, target, "stop")
}

func (r *PodmanAdapter) RestartServer(ctx context.Context, target Target) (PowerResponse, error) {
	return r.sendPower(ctx, target, "restart")
}

func (r *PodmanAdapter) KillServer(ctx context.Context, target Target) (PowerResponse, error) {
	return r.sendPower(ctx, target, "kill")
}

func (r *PodmanAdapter) Stats(ctx context.Context, target Target) (Stats, error) {
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

func (r *PodmanAdapter) Exists(ctx context.Context, target Target) (bool, error) {
	if _, err := r.Stats(ctx, target); err != nil {
		return false, err
	}
	return true, nil
}

func (r *PodmanAdapter) Inspect(ctx context.Context, target Target) (Inspection, error) {
	exists, err := r.Exists(ctx, target)
	if err != nil {
		return Inspection{}, err
	}
	return Inspection{ServerID: target.ServerID, Exists: exists, Provider: PodmanProvider}, nil
}

func (r *PodmanAdapter) PrepareMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	return MigrationResponse{MigrationID: req.MigrationID, Accepted: false, Mode: "not_implemented"}, ErrNotImplemented
}

func (r *PodmanAdapter) ExecuteMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	return MigrationResponse{MigrationID: req.MigrationID, Accepted: false, Mode: "not_implemented"}, ErrNotImplemented
}

func (r *PodmanAdapter) CancelMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	return MigrationResponse{MigrationID: req.MigrationID, Accepted: false, Mode: "not_implemented"}, ErrNotImplemented
}

func (r *PodmanAdapter) sendPower(ctx context.Context, target Target, signal string) (PowerResponse, error) {
	if r == nil || r.client == nil {
		return PowerResponse{}, ErrRuntimeUnavailable
	}
	response, err := r.client.SendPower(ctx, target.NodeURL, target.NodeToken, target.ServerID, signal)
	return powerResponse(response), err
}
