package runtime

import (
	"context"

	"gamepanel/forge/internal/daemon"
)

type LXCAdapter struct {
	client *daemon.Client
}

func NewLXCAdapter(client *daemon.Client) *LXCAdapter {
	return &LXCAdapter{client: client}
}

func (r *LXCAdapter) Name() string { return LXCProvider }

func (r *LXCAdapter) Capabilities() Capabilities { return Capabilities{} }

func (r *LXCAdapter) SupportsMigration() bool { return false }

func (r *LXCAdapter) CreateServer(ctx context.Context, target Target, req CreateServerRequest) (CreateResponse, error) {
	if r == nil || r.client == nil {
		return CreateResponse{}, ErrRuntimeUnavailable
	}
	// LXC is proxied via the beacon's generic server endpoint; reuse docker-style mapping.
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
		NetworkName: req.NetworkName, NetworkSubnet: req.NetworkSubnet, NetworkGateway: req.NetworkGateway, NetworkIP: req.NetworkIP,
		RegistryAuth: daemonRegistryAuth(req.RegistryAuth), DiskMB: req.DiskMB, Provider: LXCProvider,
	})
	if err != nil {
		return CreateResponse{}, err
	}
	return CreateResponse{ServerID: response.ServerID, Accepted: response.Accepted, Mode: response.Mode}, nil
}

func (r *LXCAdapter) InstallServer(ctx context.Context, target Target, req InstallRequest) (InstallResponse, error) {
	return r.runInstaller(ctx, target, req, false)
}

func (r *LXCAdapter) ReinstallServer(ctx context.Context, target Target, req InstallRequest) (InstallResponse, error) {
	return r.runInstaller(ctx, target, req, true)
}

func (r *LXCAdapter) runInstaller(ctx context.Context, target Target, req InstallRequest, reinstall bool) (InstallResponse, error) {
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

func (r *LXCAdapter) SyncServerConfiguration(ctx context.Context, target Target, config ServerConfiguration) error {
	if r == nil || r.client == nil {
		return ErrRuntimeUnavailable
	}
	return r.client.SyncServerConfiguration(ctx, target.NodeURL, target.NodeToken, target.ServerID, daemon.ServerConfiguration{
		UUID: config.UUID, Name: config.Name, Suspended: config.Suspended, Environment: config.Environment,
		Invocation: config.Invocation, DockerImage: config.DockerImage, Egg: config.Egg, Build: config.Build,
		Allocations: config.Allocations, Config: config.Config, Mounts: daemonMounts(config.Mounts),
		UID: config.UID, GID: config.GID, Provider: LXCProvider,
	})
}

func (r *LXCAdapter) ResizeServer(ctx context.Context, target Target, memoryMB, cpu int64) error {
	if r == nil || r.client == nil {
		return ErrRuntimeUnavailable
	}
	return r.client.SyncServerConfiguration(ctx, target.NodeURL, target.NodeToken, target.ServerID, daemon.ServerConfiguration{
		UUID:  target.ServerID,
		Build: map[string]any{"memoryLimit": memoryMB, "cpuShares": cpu},
	})
}

func (r *LXCAdapter) DeleteServer(ctx context.Context, target Target) (PowerResponse, error) {
	if r == nil || r.client == nil {
		return PowerResponse{}, ErrRuntimeUnavailable
	}
	response, err := r.client.DeleteServer(ctx, target.NodeURL, target.NodeToken, target.ServerID)
	return powerResponse(response), err
}

func (r *LXCAdapter) StartServer(ctx context.Context, target Target) (PowerResponse, error) {
	return r.sendPower(ctx, target, "start")
}
func (r *LXCAdapter) StopServer(ctx context.Context, target Target) (PowerResponse, error) {
	return r.sendPower(ctx, target, "stop")
}
func (r *LXCAdapter) RestartServer(ctx context.Context, target Target) (PowerResponse, error) {
	return r.sendPower(ctx, target, "restart")
}
func (r *LXCAdapter) KillServer(ctx context.Context, target Target) (PowerResponse, error) {
	return r.sendPower(ctx, target, "kill")
}

func (r *LXCAdapter) Stats(ctx context.Context, target Target) (Stats, error) {
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

func (r *LXCAdapter) Exists(ctx context.Context, target Target) (bool, error) {
	return existsWorkload(ctx, r.client, target, LXCProvider)
}

func (r *LXCAdapter) Inspect(ctx context.Context, target Target) (Inspection, error) {
	return inspectWorkload(ctx, r.client, target, LXCProvider)
}

func (r *LXCAdapter) PrepareMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	return MigrationResponse{MigrationID: req.MigrationID, Accepted: false, Mode: "control_plane"}, ErrMigrationManagedByControlPlane
}

func (r *LXCAdapter) ExecuteMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	return MigrationResponse{MigrationID: req.MigrationID, Accepted: false, Mode: "control_plane"}, ErrMigrationManagedByControlPlane
}

func (r *LXCAdapter) CancelMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	return MigrationResponse{MigrationID: req.MigrationID, Accepted: false, Mode: "control_plane"}, ErrMigrationManagedByControlPlane
}

func (r *LXCAdapter) sendPower(ctx context.Context, target Target, signal string) (PowerResponse, error) {
	if r == nil || r.client == nil {
		return PowerResponse{}, ErrRuntimeUnavailable
	}
	response, err := r.client.SendPower(ctx, target.NodeURL, target.NodeToken, target.ServerID, signal)
	return powerResponse(response), err
}
