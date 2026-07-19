package runtime

import (
	"context"

	"gamepanel/forge/internal/daemon"
)

type FirecrackerAdapter struct {
	client *daemon.Client
}

func NewFirecrackerAdapter(client *daemon.Client) *FirecrackerAdapter {
	return &FirecrackerAdapter{client: client}
}

func (r *FirecrackerAdapter) Name() string {
	return FirecrackerProvider
}

func (r *FirecrackerAdapter) Capabilities() Capabilities {
	return FirecrackerCapabilities()
}

func (r *FirecrackerAdapter) SupportsMigration() bool {
	return r != nil && r.client != nil
}

func (r *FirecrackerAdapter) CreateServer(ctx context.Context, target Target, req CreateServerRequest) (CreateResponse, error) {
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
		RegistryAuth: daemonRegistryAuth(req.RegistryAuth), DiskMB: req.DiskMB, Provider: FirecrackerProvider,
	})
	if err != nil {
		return CreateResponse{}, err
	}
	return CreateResponse{ServerID: response.ServerID, Accepted: response.Accepted, Mode: response.Mode}, nil
}

func (r *FirecrackerAdapter) InstallServer(ctx context.Context, target Target, req InstallRequest) (InstallResponse, error) {
	return r.runInstaller(ctx, target, req, false)
}

func (r *FirecrackerAdapter) ReinstallServer(ctx context.Context, target Target, req InstallRequest) (InstallResponse, error) {
	return r.runInstaller(ctx, target, req, true)
}

func (r *FirecrackerAdapter) runInstaller(ctx context.Context, target Target, req InstallRequest, reinstall bool) (InstallResponse, error) {
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

func (r *FirecrackerAdapter) SyncServerConfiguration(ctx context.Context, target Target, config ServerConfiguration) error {
	if r == nil || r.client == nil {
		return ErrRuntimeUnavailable
	}
	return r.client.SyncServerConfiguration(ctx, target.NodeURL, target.NodeToken, target.ServerID, daemon.ServerConfiguration{
		UUID: config.UUID, Name: config.Name, Suspended: config.Suspended, Environment: config.Environment,
		Invocation: config.Invocation, DockerImage: config.DockerImage, Egg: config.Egg, Build: config.Build,
		Allocations: config.Allocations, Config: config.Config, Mounts: daemonMounts(config.Mounts),
		Provider: FirecrackerProvider,
	})
}

func (r *FirecrackerAdapter) ResizeServer(ctx context.Context, target Target, memoryMB, cpu int64) error {
	if r == nil || r.client == nil {
		return ErrRuntimeUnavailable
	}
	return r.client.SyncServerConfiguration(ctx, target.NodeURL, target.NodeToken, target.ServerID, daemon.ServerConfiguration{
		UUID: target.ServerID,
		Build: map[string]any{"memoryLimit": memoryMB, "cpuShares": cpu},
	})
}

func (r *FirecrackerAdapter) DeleteServer(ctx context.Context, target Target) (PowerResponse, error) {
	if r == nil || r.client == nil {
		return PowerResponse{}, ErrRuntimeUnavailable
	}
	response, err := r.client.DeleteServer(ctx, target.NodeURL, target.NodeToken, target.ServerID)
	return powerResponse(response), err
}

func (r *FirecrackerAdapter) StartServer(ctx context.Context, target Target) (PowerResponse, error) {
	return r.sendPower(ctx, target, "start")
}

func (r *FirecrackerAdapter) StopServer(ctx context.Context, target Target) (PowerResponse, error) {
	return r.sendPower(ctx, target, "stop")
}

func (r *FirecrackerAdapter) RestartServer(ctx context.Context, target Target) (PowerResponse, error) {
	return r.sendPower(ctx, target, "restart")
}

func (r *FirecrackerAdapter) KillServer(ctx context.Context, target Target) (PowerResponse, error) {
	return r.sendPower(ctx, target, "kill")
}

func (r *FirecrackerAdapter) Stats(ctx context.Context, target Target) (Stats, error) {
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

func (r *FirecrackerAdapter) Exists(ctx context.Context, target Target) (bool, error) {
	if _, err := r.Stats(ctx, target); err != nil {
		return false, err
	}
	return true, nil
}

func (r *FirecrackerAdapter) Inspect(ctx context.Context, target Target) (Inspection, error) {
	exists, err := r.Exists(ctx, target)
	if err != nil {
		return Inspection{}, err
	}
	return Inspection{ServerID: target.ServerID, Exists: exists, Provider: FirecrackerProvider}, nil
}

func (r *FirecrackerAdapter) PrepareMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	return MigrationResponse{MigrationID: req.MigrationID, Accepted: false, Mode: "not_implemented"}, ErrNotImplemented
}

func (r *FirecrackerAdapter) ExecuteMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	return MigrationResponse{MigrationID: req.MigrationID, Accepted: false, Mode: "not_implemented"}, ErrNotImplemented
}

func (r *FirecrackerAdapter) CancelMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	return MigrationResponse{MigrationID: req.MigrationID, Accepted: false, Mode: "not_implemented"}, ErrNotImplemented
}

func (r *FirecrackerAdapter) sendPower(ctx context.Context, target Target, signal string) (PowerResponse, error) {
	if r == nil || r.client == nil {
		return PowerResponse{}, ErrRuntimeUnavailable
	}
	response, err := r.client.SendPower(ctx, target.NodeURL, target.NodeToken, target.ServerID, signal)
	return powerResponse(response), err
}
