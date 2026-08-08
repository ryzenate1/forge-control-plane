package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gamepanel/beacon/internal/remote"
	"gamepanel/beacon/internal/rootfs"
	"gamepanel/beacon/internal/runtime"
)

type PowerState string

const (
	PowerStateOffline  PowerState = "offline"
	PowerStateStarting PowerState = "starting"
	PowerStateRunning  PowerState = "running"
	PowerStateStopping PowerState = "stopping"
)

type ServerState struct {
	mu                    sync.Mutex
	PowerState            PowerState
	InstallationState     string
	StartupState          string
	StartupCommand        string
	RunningAction         string
	RootDir               string
	MemoryMB              int64
	AllocationIP          string
	AllocationPort        int
	StopType              string
	StopValue             string
	StopTimeout           time.Duration
	DiskLimitBytes        int64
	ConfigurationSynced   bool
	ExpectedStop          bool
	CrashDetectionEnabled bool
	CrashCooldown         time.Duration
	LastCrash             time.Time
	LastStartedAt         time.Time
	Suspended             bool
	// DetectCleanExitAsCrash matches Wings' config of the same name: when
	// false (the recommended default), an exit code of 0 is treated as a
	// clean shutdown and does NOT trigger crash auto-restart. When true,
	// even exit code 0 is considered a crash and triggers restart.
	DetectCleanExitAsCrash bool
	ChownOnBoot            bool
	UID                    int
	GID                    int
	PanelURL               string
	PanelToken             string
	EnvVars                map[string]string
	ContainerExists        bool
}

// Reconstruction describes panel-owned server state restored during daemon
// boot. Container power state is always inspected from the runtime.
type Reconstruction struct {
	ServerID            string
	RootDir             string
	DiskLimitMB         int64
	ConfigurationSynced bool
	InstallationState   string
	Suspended           bool
}

type ServerManager struct {
	runtime                runtime.Runtime
	states                 sync.Map
	crashCooldown          time.Duration
	detectCleanExitAsCrash bool
	onRunning              func(string)
	onStopped              func(string)
	sendConsole            func(string, string) error
	crashHandler           func(ctx context.Context, serverID string, exitCode int, oomKilled bool)
	panelSyncMu            sync.Mutex
	stateDir               string
}

func NewServerManager(rt runtime.Runtime) *ServerManager {
	return &ServerManager{runtime: rt, crashCooldown: time.Minute, detectCleanExitAsCrash: false}
}

// SetConsoleLifecycle registers callbacks invoked when a server transitions
// to a running or stopped power state. The onRunning callback is called when
// the server starts; onStopped is called on any stop or crash.
func (m *ServerManager) SetConsoleLifecycle(onRunning, onStopped func(string)) {
	m.onRunning = onRunning
	m.onStopped = onStopped
}

func (m *ServerManager) SetConsoleCommand(send func(string, string) error) { m.sendConsole = send }

func (m *ServerManager) SetCrashHandler(handler func(ctx context.Context, serverID string, exitCode int, oomKilled bool)) {
	m.crashHandler = handler
}

// SetStateDir configures the directory used to persist per-server power state
// (last start time and expected-stop flag) so crash auto-restart survives a
// daemon restart. A nil or empty path disables persistence.
func (m *ServerManager) SetStateDir(dir string) {
	m.stateDir = strings.TrimSpace(dir)
}

func (m *ServerManager) Reconcile(ctx context.Context, reconstruction Reconstruction) error {
	state := m.State(reconstruction.ServerID)
	persisted := m.loadPowerState(reconstruction.ServerID)
	state.mu.Lock()
	state.RootDir = filepath.Clean(reconstruction.RootDir)
	state.DiskLimitBytes = MbToBytes(reconstruction.DiskLimitMB)
	state.ConfigurationSynced = reconstruction.ConfigurationSynced
	state.Suspended = reconstruction.Suspended
	state.InstallationState = reconstruction.InstallationState
	if state.InstallationState == "" {
		state.InstallationState = "installed"
	}
	state.ExpectedStop = false
	state.RunningAction = ""
	// Restore crash-recovery state persisted before a daemon restart. This is
	// what lets the daemon honour crash auto-restart across its own restarts:
	// a workload that crashed while the daemon was down is restarted instead
	// of being left offline forever.
	state.ExpectedStop = persisted.ExpectedStop
	state.LastStartedAt = persisted.LastStartedAt
	state.mu.Unlock()
	if m.runtime == nil {
		return errRuntimeUnavailable
	}

	actual, err := m.runtime.Inspect(ctx, reconstruction.ServerID)
	if err != nil {
		return err
	}
	state.mu.Lock()
	state.ContainerExists = actual.Exists
	if actual.Exists && actual.Running {
		state.PowerState = PowerStateRunning
	} else {
		state.PowerState = PowerStateOffline
	}
	state.mu.Unlock()
	if actual.Exists && actual.Running && m.onRunning != nil {
		m.onRunning(reconstruction.ServerID)
	}
	if err := m.autoRestartCrashed(ctx, reconstruction.ServerID, state, actual); err != nil {
		return err
	}
	return nil
}

// autoRestartCrashed restarts a workload that is down but was started shortly
// before the daemon restarted and was not expected to stop. This preserves
// crash auto-restart semantics across daemon restarts.
func (m *ServerManager) autoRestartCrashed(ctx context.Context, serverID string, state *ServerState, actual runtime.ContainerState) error {
	state.mu.Lock()
	lastStarted := state.LastStartedAt
	expectedStop := state.ExpectedStop
	state.mu.Unlock()
	if !actual.Exists || actual.Running || expectedStop || lastStarted.IsZero() {
		return nil
	}
	if time.Since(lastStarted) > crashAutoRestartWindow {
		return nil
	}
	log.Printf("beacon: restarting workload %s that stopped near the daemon restart (last started %s, not expected to stop)", serverID, lastStarted.Format(time.RFC3339))
	if err := m.runtime.Start(ctx, serverID); err != nil {
		if isContainerMissing(err) {
			return nil
		}
		return err
	}
	state.mu.Lock()
	state.PowerState = PowerStateRunning
	state.ContainerExists = true
	state.ExpectedStop = false
	state.LastStartedAt = time.Now()
	state.mu.Unlock()
	m.persistPowerState(serverID, state)
	if m.onRunning != nil {
		m.onRunning(serverID)
	}
	return nil
}

// crashAutoRestartWindow bounds how long after its last start a workload is
// eligible for crash auto-restart after a daemon restart. Workloads that have
// been down longer than this are treated as intentionally stopped.
const crashAutoRestartWindow = 24 * time.Hour

func (m *ServerManager) SetDetectCleanExitAsCrash(value bool) {
	if m == nil {
		return
	}
	m.detectCleanExitAsCrash = value
}

// ServerIDs returns the identifiers of every server currently tracked by the
// manager. It is used by the /metrics endpoint to enumerate workloads for
// per-container runtime stats.
func (m *ServerManager) ServerIDs() []string {
	if m == nil {
		return nil
	}
	ids := make([]string, 0, 16)
	m.states.Range(func(key, _ any) bool {
		if id, ok := key.(string); ok && id != "" {
			ids = append(ids, id)
		}
		return true
	})
	return ids
}

func (m *ServerManager) State(serverID string) *ServerState {
	value, _ := m.states.LoadOrStore(serverID, &ServerState{
		PowerState:             PowerStateOffline,
		InstallationState:      "unknown",
		StartupState:           "unknown",
		CrashDetectionEnabled:  true,
		CrashCooldown:          m.crashCooldown,
		DetectCleanExitAsCrash: m.detectCleanExitAsCrash,
		StopTimeout:            30 * time.Second,
		Suspended:              false,
	})
	state, _ := value.(*ServerState)
	return state
}

func (m *ServerManager) MarkInstalling(serverID string, installing bool) {
	state := m.State(serverID)
	state.mu.Lock()
	defer state.mu.Unlock()
	if installing {
		state.InstallationState = "installing"
		state.RunningAction = "install"
		return
	}
	if state.RunningAction == "install" {
		state.RunningAction = ""
	}
	state.InstallationState = "installed"
}

func (m *ServerManager) MarkCreated(serverID, rootDir string, diskLimitMB int64) {
	state := m.State(serverID)
	state.mu.Lock()
	defer state.mu.Unlock()
	state.InstallationState = "installed"
	state.ContainerExists = true
	state.RootDir = rootDir
	state.DiskLimitBytes = MbToBytes(diskLimitMB)
	if state.PowerState == "" {
		state.PowerState = PowerStateOffline
	}
}

func (m *ServerManager) MarkConfigurationSynced(serverID string, diskLimitMB int64) {
	state := m.State(serverID)
	state.mu.Lock()
	defer state.mu.Unlock()
	state.ConfigurationSynced = true
	if diskLimitMB >= 0 {
		state.DiskLimitBytes = MbToBytes(diskLimitMB)
	}
}

func (m *ServerManager) UpdateRuntimeConfig(serverID string, memoryMB int64, allocationIP string, allocationPort int, stopType, stopValue string, stopTimeout time.Duration) {
	state := m.State(serverID)
	state.mu.Lock()
	defer state.mu.Unlock()
	if memoryMB > 0 {
		state.MemoryMB = memoryMB
	}
	if strings.TrimSpace(allocationIP) != "" {
		state.AllocationIP = allocationIP
	}
	if allocationPort > 0 {
		state.AllocationPort = allocationPort
	}
	if strings.TrimSpace(stopType) != "" {
		state.StopType = stopType
	}
	if strings.TrimSpace(stopValue) != "" {
		state.StopValue = stopValue
	}
	if stopTimeout > 0 {
		state.StopTimeout = stopTimeout
	}
}

// persistedPowerState is the subset of ServerState that must survive a daemon
// restart so crash auto-restart stays correct.
type persistedPowerState struct {
	LastStartedAt time.Time `json:"lastStartedAt,omitempty"`
	ExpectedStop  bool      `json:"expectedStop"`
}

func (m *ServerManager) powerStatePath(serverID string) string {
	return filepath.Join(m.stateDir, serverID+".json")
}

func (m *ServerManager) loadPowerState(serverID string) persistedPowerState {
	if m.stateDir == "" {
		return persistedPowerState{}
	}
	body, err := os.ReadFile(m.powerStatePath(serverID))
	if err != nil {
		return persistedPowerState{}
	}
	var persisted persistedPowerState
	if err := json.Unmarshal(body, &persisted); err != nil {
		return persistedPowerState{}
	}
	return persisted
}

// persistPowerState writes per-server power state atomically. Errors are
// logged but never fail power operations.
func (m *ServerManager) persistPowerState(serverID string, state *ServerState) {
	if m.stateDir == "" {
		return
	}
	state.mu.Lock()
	persisted := persistedPowerState{LastStartedAt: state.LastStartedAt, ExpectedStop: state.ExpectedStop}
	state.mu.Unlock()
	body, err := json.Marshal(persisted)
	if err != nil {
		log.Printf("beacon: marshal power state for %s: %v", serverID, err)
		return
	}
	if err := os.MkdirAll(m.stateDir, 0o700); err != nil {
		log.Printf("beacon: create power state directory: %v", err)
		return
	}
	path := m.powerStatePath(serverID)
	temp, err := os.CreateTemp(m.stateDir, "."+serverID+".state-*.tmp")
	if err != nil {
		log.Printf("beacon: create power state temp for %s: %v", serverID, err)
		return
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		log.Printf("beacon: secure power state for %s: %v", serverID, err)
		return
	}
	if _, err := temp.Write(body); err != nil {
		_ = temp.Close()
		log.Printf("beacon: write power state for %s: %v", serverID, err)
		return
	}
	if err := temp.Close(); err != nil {
		log.Printf("beacon: close power state for %s: %v", serverID, err)
		return
	}
	if err := os.Rename(tempName, path); err != nil {
		log.Printf("beacon: replace power state for %s: %v", serverID, err)
	}
}

func (m *ServerManager) deletePowerState(serverID string) {
	if m.stateDir == "" {
		return
	}
	_ = os.Remove(m.powerStatePath(serverID))
}

// stopServer stops the workload. Configuration is snapshotted under the state
// lock before any blocking runtime call so the caller never needs to hold the
// lock across network/container operations.
func (m *ServerManager) stopServer(ctx context.Context, state *ServerState, serverID string) error {
	state.mu.Lock()
	stopType := strings.TrimSpace(state.StopType)
	stopValue := strings.TrimSpace(state.StopValue)
	timeout := state.StopTimeout
	state.mu.Unlock()

	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if stopType == "command" && stopValue != "" {
		if m.sendConsole == nil {
			return errors.New("configured stop command requires a live console")
		}
		if err := m.sendConsole(serverID, stopValue); err != nil {
			return fmt.Errorf("send stop command: %w", err)
		}
		return m.runtime.WaitForStop(ctx, serverID, timeout, true)
	}
	if stopType == "signal" && stopValue != "" {
		if strings.EqualFold(stopValue, "C") {
			stopValue = "SIGINT"
		}
		if strings.EqualFold(stopValue, "SIGKILL") {
			return m.runtime.Kill(ctx, serverID)
		}
		if err := m.runtime.Signal(ctx, serverID, stopValue); err != nil {
			return err
		}
		return m.runtime.WaitForStop(ctx, serverID, timeout, true)
	}
	if err := m.runtime.Signal(ctx, serverID, "SIGTERM"); err != nil {
		return err
	}
	return m.runtime.WaitForStop(ctx, serverID, timeout, true)
}

func (m *ServerManager) Delete(serverID string) {
	m.states.Delete(serverID)
	m.deletePowerState(serverID)
}

func (m *ServerManager) HandlePower(ctx context.Context, serverID, signal string) error {
	if m.runtime == nil {
		return errRuntimeUnavailable
	}
	state := m.State(serverID)
	if !state.mu.TryLock() {
		return errors.New("another server action is already running")
	}
	if state.RunningAction != "" {
		state.mu.Unlock()
		return errors.New("another server action is already running")
	}
	if state.InstallationState == "installing" {
		state.mu.Unlock()
		return errors.New("server is installing")
	}

	state.RunningAction = signal
	state.mu.Unlock()
	defer func() {
		state.mu.Lock()
		if state.RunningAction == signal {
			state.RunningAction = ""
		}
		state.mu.Unlock()
	}()

	var err error
	switch signal {
	case "start":
		if err := m.onBeforeStart(serverID, state); err != nil {
			return err
		}
		state.mu.Lock()
		state.PowerState = PowerStateStarting
		state.ExpectedStop = false
		state.mu.Unlock()
		err = m.runtime.Start(ctx, serverID)
		if err == nil {
			state.mu.Lock()
			state.PowerState = PowerStateRunning
			state.ContainerExists = true
			state.LastStartedAt = time.Now()
			state.mu.Unlock()
			m.persistPowerState(serverID, state)
			if m.onRunning != nil {
				m.onRunning(serverID)
			}
		}
	case "stop":
		if m.onStopped != nil {
			m.onStopped(serverID)
		}
		state.mu.Lock()
		state.PowerState = PowerStateStopping
		state.ExpectedStop = true
		state.mu.Unlock()
		m.persistPowerState(serverID, state)
		err = m.stopServer(ctx, state, serverID)
		if err == nil {
			state.mu.Lock()
			state.PowerState = PowerStateOffline
			state.mu.Unlock()
		}
	case "restart":
		if err := m.onBeforeStart(serverID, state); err != nil {
			return err
		}
		if m.onStopped != nil {
			m.onStopped(serverID)
		}
		state.mu.Lock()
		state.PowerState = PowerStateStopping
		state.ExpectedStop = true
		state.mu.Unlock()
		err = m.stopServer(ctx, state, serverID)
		if err == nil {
			err = m.runtime.Start(ctx, serverID)
		}
		state.mu.Lock()
		if err == nil {
			state.PowerState = PowerStateRunning
			state.ExpectedStop = false
			state.LastStartedAt = time.Now()
		} else {
			// A failed restart must not leave the server stuck in "stopping"
			// or with a pending expected-stop: reset to offline so the crash
			// watcher and panel see a consistent state.
			state.PowerState = PowerStateOffline
			state.ExpectedStop = false
		}
		state.mu.Unlock()
		m.persistPowerState(serverID, state)
		if err == nil && m.onRunning != nil {
			m.onRunning(serverID)
		}
	case "kill":
		if m.onStopped != nil {
			m.onStopped(serverID)
		}
		state.mu.Lock()
		state.PowerState = PowerStateStopping
		state.ExpectedStop = true
		state.mu.Unlock()
		m.persistPowerState(serverID, state)
		err = m.runtime.Kill(ctx, serverID)
		if err == nil {
			state.mu.Lock()
			state.PowerState = PowerStateOffline
			state.mu.Unlock()
		}
	default:
		return errors.New("invalid power signal")
	}
	if err != nil {
		if isContainerMissing(err) {
			state.mu.Lock()
			state.PowerState = PowerStateOffline
			state.mu.Unlock()
		}
		return err
	}
	return nil
}

func (m *ServerManager) onBeforeStart(serverID string, state *ServerState) error {
	state.mu.Lock()
	installing := state.InstallationState == "installing"
	suspended := state.Suspended
	synced := state.ConfigurationSynced
	root := state.RootDir
	chownOnBoot := state.ChownOnBoot
	uid, gid := state.UID, state.GID
	panelURL, panelToken := state.PanelURL, state.PanelToken
	diskLimit := state.DiskLimitBytes
	state.mu.Unlock()

	// Check if server is installing
	if installing {
		return errors.New("server is installing")
	}

	// Check if server is suspended
	if suspended {
		return errors.New("server is suspended")
	}

	// Configuration must be synced
	if !synced {
		return errors.New("server configuration has not been synced")
	}

	// Root directory must be known
	if root == "" {
		return errors.New("server root directory is unknown")
	}

	// Chown server directory on boot if enabled
	if chownOnBoot {
		if err := chownRecursive(root, uid, gid); err != nil {
			// Log but don't fail - chown errors shouldn't block startup
			fmt.Printf("warning: chown failed: %v\n", err)
		}
	}

	// Sync latest server state from Panel if available.
	if panelURL != "" && panelToken != "" {
		if err := m.syncServerStateFromPanel(serverID, panelURL, panelToken); err != nil {
			// Log but don't fail - Panel sync errors shouldn't block startup
			fmt.Printf("warning: panel sync failed: %v\n", err)
		}
	}

	// Check disk usage
	if diskLimit <= 0 {
		return nil
	}

	usage, err := diskUsageBytes(root)
	if err != nil {
		return err
	}

	if usage > diskLimit {
		return fmt.Errorf("server disk usage %d exceeds limit %d", usage, diskLimit)
	}

	return nil
}

// chownRecursive changes ownership of all files in a directory
func chownRecursive(root string, uid, gid int) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chown(path, uid, gid)
	})
}

// syncServerStateFromPanel fetches the latest server configuration from the
// panel and updates in-memory daemon state to better mirror Wings' source of
// truth model. Panel sync is serialized with its own mutex so concurrent power
// operations cannot interleave HTTP fetches against the panel.
func (m *ServerManager) syncServerStateFromPanel(serverID, panelURL, token string) error {
	if strings.TrimSpace(panelURL) == "" || strings.TrimSpace(token) == "" {
		return nil
	}
	if !m.panelSyncMu.TryLock() {
		return errors.New("panel state sync is already in progress")
	}
	defer m.panelSyncMu.Unlock()

	client := remote.NewClient(panelURL, token)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := client.GetServerConfiguration(ctx, serverID)
	if err != nil {
		return err
	}

	var settings struct {
		Suspended   bool              `json:"suspended"`
		Invocation  string            `json:"invocation"`
		Environment map[string]string `json:"environment"`
		Build       struct {
			MemoryLimit int64 `json:"memory_limit"`
			MemoryMB    int64 `json:"memoryMb"`
			MemoryMb    int64 `json:"memory_mb"`
		} `json:"build"`
		Allocations struct {
			Default struct {
				IP   string `json:"ip"`
				Port int    `json:"port"`
			} `json:"default"`
		} `json:"allocations"`
		ProcessConfiguration *remote.ProcessConfiguration `json:"process_configuration"`
	}
	merged := map[string]any{}
	if err := json.Unmarshal(cfg.Settings, &merged); err != nil {
		return err
	}
	if cfg.ProcessConfiguration != nil {
		merged["process_configuration"] = cfg.ProcessConfiguration
	}
	if cfg.Mounts != nil {
		merged["mounts"] = cfg.Mounts
	}
	encoded, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(encoded, &settings); err != nil {
		return err
	}

	envVars := make(map[string]string, len(settings.Environment))
	for key, value := range settings.Environment {
		envVars[key] = value
	}

	state := m.State(serverID)
	state.mu.Lock()
	state.Suspended = settings.Suspended
	state.StartupCommand = settings.Invocation
	if strings.TrimSpace(settings.Invocation) != "" {
		state.StartupState = "synced"
	}
	if settings.Build.MemoryLimit > 0 {
		state.MemoryMB = settings.Build.MemoryLimit
	} else if settings.Build.MemoryMB > 0 {
		state.MemoryMB = settings.Build.MemoryMB
	} else if settings.Build.MemoryMb > 0 {
		state.MemoryMB = settings.Build.MemoryMb
	}
	if strings.TrimSpace(settings.Allocations.Default.IP) != "" {
		state.AllocationIP = settings.Allocations.Default.IP
	}
	if settings.Allocations.Default.Port > 0 {
		state.AllocationPort = settings.Allocations.Default.Port
	}
	if settings.ProcessConfiguration != nil {
		state.StopType = settings.ProcessConfiguration.Stop.Type
		state.StopValue = settings.ProcessConfiguration.Stop.Value
	}
	state.EnvVars = envVars
	state.mu.Unlock()
	return nil
}

func (m *ServerManager) HasSpaceForWrite(serverID string, additionalBytes int64) error {
	return m.hasSpaceForWrite(serverID, additionalBytes, nil)
}

func (m *ServerManager) HasSpaceForWriteFS(serverID string, additionalBytes int64, fsys *rootfs.FS) error {
	return m.hasSpaceForWrite(serverID, additionalBytes, fsys)
}

func (m *ServerManager) hasSpaceForWrite(serverID string, additionalBytes int64, fsys *rootfs.FS) error {
	if additionalBytes <= 0 {
		return nil
	}
	state := m.State(serverID)
	state.mu.Lock()
	root := state.RootDir
	limit := state.DiskLimitBytes
	state.mu.Unlock()
	if limit <= 0 {
		return nil
	}
	var usage int64
	var err error
	if fsys != nil {
		usage, err = fsys.Usage()
	} else if root != "" {
		usage, err = diskUsageBytes(root)
	} else {
		return nil
	}
	if err != nil {
		return err
	}
	if usage > limit || additionalBytes > limit-usage {
		return fmt.Errorf("server disk usage %d plus write %d exceeds limit %d", usage, additionalBytes, limit)
	}
	return nil
}

func diskUsageBytes(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total, err
}

func MbToBytes(value int64) int64 {
	if value <= 0 {
		return 0
	}
	return value * 1024 * 1024
}

func (m *ServerManager) StartEventWatcher(ctx context.Context) {
	watcher, ok := m.runtime.(runtime.EventWatcher)
	if !ok || watcher == nil {
		return
	}
	events, errs := watcher.WatchEvents(ctx)
	go func() {
		for {
			select {
			case event, ok := <-events:
				if !ok {
					return
				}
				m.HandleContainerEvent(ctx, event)
			case _, ok := <-errs:
				if !ok {
					return
				}
				// The runtime watcher reconnects internally; errors are advisory.
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (m *ServerManager) HandleContainerEvent(ctx context.Context, event runtime.ContainerEvent) {
	if event.ServerID == "" {
		return
	}
	if strings.EqualFold(event.Action, "start") {
		state := m.State(event.ServerID)
		state.mu.Lock()
		state.ContainerExists = true
		state.PowerState = PowerStateRunning
		state.ExpectedStop = false
		state.LastStartedAt = time.Now()
		state.mu.Unlock()
		m.persistPowerState(event.ServerID, state)
		if m.onRunning != nil {
			m.onRunning(event.ServerID)
		}
		return
	}
	if !isExitEvent(event.Action) {
		return
	}
	if m.onStopped != nil {
		m.onStopped(event.ServerID)
	}
	state := m.State(event.ServerID)
	state.mu.Lock()
	if state.ExpectedStop {
		state.PowerState = PowerStateOffline
		state.ExpectedStop = false
		state.RunningAction = ""
		state.mu.Unlock()
		m.persistPowerState(event.ServerID, state)
		return
	}
	// Determine crash state honouring DetectCleanExitAsCrash (Wings parity).
	crashed := event.OOMKilled || event.ExitCode != 0
	if !crashed && state.DetectCleanExitAsCrash {
		crashed = true
	}
	if !crashed || !state.CrashDetectionEnabled {
		state.PowerState = PowerStateOffline
		state.RunningAction = ""
		state.mu.Unlock()
		return
	}
	if state.CrashCooldown > 0 && !state.LastCrash.IsZero() && state.LastCrash.Add(state.CrashCooldown).After(time.Now()) {
		state.PowerState = PowerStateOffline
		state.RunningAction = ""
		state.mu.Unlock()
		return
	}
	state.LastCrash = time.Now()
	state.PowerState = PowerStateOffline
	state.RunningAction = ""
	state.mu.Unlock()

	if m.crashHandler != nil {
		m.crashHandler(ctx, event.ServerID, event.ExitCode, event.OOMKilled)
	}

	_ = m.HandlePower(ctx, event.ServerID, "start")
}

func isExitEvent(action string) bool {
	action = strings.ToLower(action)
	return action == "die" || action == "oom" || action == "stop"
}
