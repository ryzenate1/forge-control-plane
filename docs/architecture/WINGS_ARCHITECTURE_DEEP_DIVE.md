# Wings Architecture - Deep Dive Reference

> Complete architectural analysis of Pterodactyl Wings for GamePanel parity implementation
> Created: 2026-06-16

## Table of Contents

1. [System Overview](#system-overview)
2. [Core Components](#core-components)
3. [Communication Flow](#communication-flow)
4. [Server Lifecycle](#server-lifecycle)
5. [Resource Management](#resource-management)
6. [Event System](#event-system)
7. [Critical Implementation Details](#critical-implementation-details)

---

## System Overview

Wings is the daemon that runs on game server hosts, communicating with the Panel via REST API.

### Key Architectural Principles

- **Autonomous Operation**: Wings operates independently, syncing state with Panel
- **Container-Based**: Uses Docker for process isolation and resource management
- **Event-Driven**: Publishes events for state changes, console output, and stats
- **Crash Detection**: Automatic server restart on unexpected crashes
- **Resource Polling**: Continuous monitoring of CPU, memory, disk, network

---

## Core Components

### 1. Server Manager (`server.Manager`)

**Purpose**: Central registry for all server instances on a node

```go
type Manager struct {
    mu      sync.RWMutex
    client  remote.Client
    servers []*Server
}
```

**Responsibilities**:
- Boot all servers on Wings startup
- Load server configurations from Panel API
- Persist server states to disk (every minute)
- Restore server states after Wings restart
- Provide server lookup and filtering

**Key Methods**:
- `NewManager(ctx, client)` - Fetch all servers from Panel
- `InitServer(data)` - Create server instance with config
- `PersistStates()` - Save running states to disk
- `ReadStates()` - Load persisted states
- `Get(uuid)` - Retrieve server by UUID

### 2. Server Instance (`server.Server`)

**Purpose**: Represents a single game server instance

```go
type Server struct {
    sync.RWMutex
    ctx         context.Context
    ctxCancel   *context.CancelFunc
    cfg         Configuration
    client      remote.Client
    Environment environment.ProcessEnvironment
    fs          *filesystem.Filesystem
    emitter     *events.Bus
    resources   ResourceUsage
    powerLock   *system.Locker
    installing  *system.AtomicBool
    transferring *system.AtomicBool
    restoring   *system.AtomicBool
}
```

**Responsibilities**:
- Manage individual server lifecycle
- Handle power actions (start/stop/restart/kill)
- Process console output
- Track resource usage
- Emit events for state changes
- Sync with Panel configuration

**State Machine**:
- `offline` - Container stopped
- `starting` - Container booting
- `running` - Container active and process running
- `stopping` - Graceful shutdown in progress

### 3. Environment Interface (`environment.ProcessEnvironment`)

**Purpose**: Abstract interface for different process isolation methods (Docker, etc.)

**Core Operations**:
```go
type ProcessEnvironment interface {
    Type() string
    Exists() (bool, error)
    IsRunning(ctx) (bool, error)
    Create() error
    Start(ctx) error
    Stop(ctx) error
    Terminate(ctx, signal) error
    Destroy() error
    Attach(ctx) error
    SendCommand(string) error
    State() string
    SetState(string)
    Events() *events.Bus
}
```


### 4. Docker Environment (`docker.Environment`)

**Purpose**: Docker-specific implementation of ProcessEnvironment

**Key Features**:
- Container lifecycle management
- Resource limit enforcement
- Stats polling (CPU, memory, network, disk)
- Console output streaming via hijacked connection
- Image pulling with progress events
- Network configuration
- Volume/mount management

**Container Configuration**:
```go
- User: pterodactyl UID:GID
- Security: no-new-privileges, readonly rootfs, dropped capabilities
- Logging: json-file with size/rotation limits
- Resources: CPU limits, memory limits, IO limits
- Network: Custom or bridge mode with port bindings
- Tmpfs: /tmp with configurable size
```

### 5. Remote Client (`remote.Client`)

**Purpose**: HTTP client for Panel API communication

**Key Endpoints**:
- `GET /servers` - Fetch all servers (paginated)
- `GET /servers/{uuid}` - Get server configuration
- `POST /servers/reset` - Reset installing/restoring states
- `POST /activity` - Send activity logs
- `POST /servers/{uuid}/install` - Installation status
- `POST /servers/{uuid}/archive` - Archive status
- `POST /servers/{uuid}/transfer/{state}` - Transfer status

**Authentication**:
- Bearer token: `{token_id}.{token}`
- Exponential backoff on failures
- Max retries configurable

### 6. Cron System (`cron`)

**Purpose**: Scheduled background tasks

**Jobs**:

1. **Activity Cron** (configurable interval, default 60s):
   - Fetch activity events from local SQLite
   - Send to Panel in batches (max configurable)
   - Delete after successful send
   - Skip SFTP events (handled separately)

2. **SFTP Cron** (same interval):
   - Fetch SFTP events
   - De-duplicate by user+server+ip+event+minute
   - Merge file lists for same operation
   - Send consolidated events
   - Reduces Panel DB load

3. **State Persistence** (every 60 seconds):
   - Save all server states to disk
   - Enables seamless restart after crash

---

## Communication Flow

### Panel → Wings

**Authenticated Endpoints** (Bearer token required):

```
POST /api/servers                     - Create new server
GET  /api/servers                     - List all servers
GET  /api/servers/{uuid}              - Get server details
DELETE /api/servers/{uuid}            - Delete server
POST /api/servers/{uuid}/power        - Power action (start/stop/restart/kill)
POST /api/servers/{uuid}/commands     - Send console command
POST /api/servers/{uuid}/sync         - Force config sync
POST /api/servers/{uuid}/install      - Install server
POST /api/servers/{uuid}/reinstall    - Reinstall server
POST /api/servers/{uuid}/transfer     - Initiate transfer
GET  /api/servers/{uuid}/logs         - Get console logs
POST /api/servers/{uuid}/files/*      - File operations
POST /api/servers/{uuid}/backup       - Create backup
```

**Unauthenticated Endpoints** (JWT signed URLs):
```
GET  /api/servers/{uuid}/ws           - WebSocket connection (JWT)
GET  /download/backup?token=...       - Download backup (signed)
GET  /download/file?token=...         - Download file (signed)
POST /upload/file                     - Upload files (signed)
```

### Wings → Panel

**Sync Operations**:
```
GET  /api/remote/servers?page=X&per_page=Y  - Boot-time server fetch
GET  /api/remote/servers/{uuid}             - Get server config
POST /api/remote/activity                   - Send activity logs
POST /api/remote/servers/reset              - Reset stuck states
```


**Status Updates**:
```
POST /api/remote/servers/{uuid}/install     - Installation progress
POST /api/remote/servers/{uuid}/archive     - Archive completion
POST /api/remote/servers/{uuid}/transfer/*  - Transfer status
POST /api/remote/backups/{uuid}             - Backup status
POST /api/remote/backups/{uuid}/restore     - Restore status
```

---

## Server Lifecycle

### 1. Wings Boot Sequence

```
1. Load configuration from disk
2. Connect to Docker daemon
3. Initialize SQLite database (activity logs)
4. Create HTTP client for Panel
5. Fetch all servers from Panel (paginated)
6. Parse server configurations in parallel (workerpool)
7. Create server instances
8. Create Docker environments
9. Load persisted states from disk
10. Check if containers exist and are running
11. Restore servers to previous state (if running before restart)
12. Start SFTP server
13. Start HTTP API server
14. Start cron jobs (activity, SFTP, state persistence)
```

### 2. Server Start Flow

```
HandlePowerAction(start) →
  ├─ Acquire power lock (prevents concurrent actions)
  ├─ onBeforeStart()
  │   ├─ Sync() - Fetch latest config from Panel
  │   ├─ Check if suspended
  │   ├─ SyncWithEnvironment() - Apply new config
  │   ├─ Check disk space
  │   ├─ Update configuration files (egg configs)
  │   └─ Chown server directory (if enabled)
  ├─ Environment.OnBeforeStart()
  │   ├─ Remove existing container
  │   └─ Create new container
  ├─ Environment.Attach()
  │   ├─ Docker attach to container
  │   ├─ Start resource polling goroutine
  │   └─ Start console output scanner
  ├─ Environment.Start()
  │   ├─ SetState(starting)
  │   ├─ Truncate log file
  │   └─ Docker container start
  ├─ Wait for "done" line in console output
  ├─ SetState(running)
  └─ Release power lock
```

### 3. Server Stop Flow

```
HandlePowerAction(stop) →
  ├─ Acquire power lock
  ├─ Environment.WaitForStop(10 minutes)
  │   ├─ SetState(stopping)
  │   ├─ Check stop configuration type:
  │   │   ├─ "command" → SendCommand(stop_value)
  │   │   ├─ "signal" → SignalContainer(signal)
  │   │   └─ "" → Docker native stop (SIGTERM)
  │   ├─ Wait for container to stop
  │   ├─ If timeout and terminate=true → SIGKILL
  │   └─ SetState(offline)
  └─ Release power lock
```


### 4. Crash Detection & Recovery

```
OnStateChange() triggers when state changes from running→offline:
  ├─ Check if transition is unexpected (crash vs. clean stop)
  ├─ handleServerCrash()
  │   ├─ Check crash detection enabled
  │   ├─ Get crash time and count
  │   ├─ If within threshold → Don't restart
  │   ├─ Increment crash counter
  │   ├─ Wait cooldown period
  │   ├─ Reset crash counter if threshold passed
  │   ├─ PublishConsoleOutput("Server detected as crashed, restarting...")
  │   └─ HandlePowerAction(start)
  └─ resources.Reset() - Zero out CPU/memory stats
```

**Crash Detection Config**:
- `crash_detection.enabled` - Enable/disable
- `crash_detection.timeout` - Time window (default 60s)
- `crash_detection.count` - Max crashes in window (default 3)

---

## Resource Management

### 1. Stats Collection

**Polling Loop** (runs in goroutine per server):
```go
pollResources(ctx) {
    stats := Docker.ContainerStats(streaming=true)
    for stat := range stats {
        calculate memory (accounting for cache)
        calculate CPU percentage
        sum network RX/TX across interfaces
        calculate uptime
        publish ResourceEvent with Stats
    }
}
```


**Stats Structure**:
```go
type Stats struct {
    Memory      uint64   // Bytes used (minus cache)
    MemoryLimit uint64   // Container limit
    CpuAbsolute float64  // % of total system CPU
    Network     NetworkStats {
        RxBytes uint64
        TxBytes uint64
    }
    Uptime      int64    // Milliseconds
}
```

**Memory Calculation**:
```
// Match docker stats CLI output
if total_inactive_file exists and < usage:
    memory = usage - total_inactive_file
else:
    memory = usage - inactive_file
```

**CPU Calculation**:
```
cpuDelta = current.TotalUsage - previous.TotalUsage
systemDelta = current.SystemUsage - previous.SystemUsage
cpus = OnlineCPUs count
percent = (cpuDelta / systemDelta) * 100.0 * cpus
```

### 2. Disk Space Management

**diskSpaceLimiter**:
- Runs on each resource event
- Checks `Filesystem().HasSpaceAvailable(force=true)`
- If exceeded → Trigger() (once per boot)
- Publishes console warning
- Stops server (WaitForStop with terminate)


### 3. Resource Limits

**Applied at Container Level**:
```go
Resources: container.Resources{
    Memory:            cfg.MemoryLimit,
    MemoryReservation: 0,
    MemorySwap:        cfg.MemoryLimit + cfg.SwapLimit,
    CpuQuota:          cfg.CpuLimit * 1000,  // Per 100ms
    CpuPeriod:         100000,  // 100ms
    CpuShares:         cfg.CpuShares,
    BlkioWeight:       cfg.IoWeight,
    CpusetCpus:        cfg.CpuPinning,
}
```

**In-Situ Updates**:
- `Environment.InSituUpdate()` called when limits change
- Uses `Docker.ContainerUpdate()` API
- No container restart required
- **Limitation**: Cannot remove CPU pinning or memory limits

---

## Event System

### 1. Event Bus (`events.Bus`)

**Based on `system.SinkPool`**:
- Thread-safe publish/subscribe
- Supports multiple listeners per topic
- JSON-encoded event payloads

**Event Structure**:
```go
type Event struct {
    Topic string
    Data  interface{}
}
```


### 2. Environment Events

**Published by Docker environment**:
- `state change` - State transitions (offline/starting/running/stopping)
- `resources` - Stats updates (continuous during running)
- `docker image pull started` - Image pull begins
- `docker image pull status` - Pull progress updates
- `docker image pull completed` - Pull finished

### 3. Server Events

**Published by Server instance**:
- `status` - State changes (propagated from environment)
- `stats` - Resource usage (with disk added)
- `console output` - Console line output
- `install output` - Installation progress
- `backup complete` - Backup finished
- `backup restore completed` - Restore finished
- `transfer status` - Transfer progress

### 4. Event Listeners

**StartEventListeners()** - Called once per server:
```go
1. Create buffered channel for environment events
2. Subscribe to environment event bus
3. Set log callback for console output
4. Start goroutine to process events:
   - ResourceEvent → Update stats, check disk, publish to websockets
   - StateChangeEvent → OnStateChange(), reset throttler
   - DockerImagePull* → Publish install output
```


### 5. WebSocket Integration

**Console Streaming**:
```
Client → WebSocket → Server.Websockets()
  ├─ Subscribe to server event bus
  ├─ Listen for:
  │   ├─ console output → Send to client
  │   ├─ status → Send state change
  │   ├─ stats → Send resource usage
  │   └─ install output → Send progress
  └─ Receive from client:
      ├─ send command → Server.SendCommand()
      ├─ send stats → Trigger stats event
      └─ set state → (auth required for state changes)
```

---

## Critical Implementation Details

### 1. Power Action Locking

**Problem**: Prevent concurrent power actions
**Solution**: `system.Locker` with UUID-tracked locks

```go
powerLock.Acquire() - Blocks until lock acquired
powerLock.TryAcquire(ctx) - Timeout-based acquire
powerLock.Release() - Release lock
powerLock.IsLocked() - Check if locked
```

**Exception**: Terminate action can bypass lock

### 2. Console Output Throttling

**ConsoleThrottle**:
- Token bucket algorithm
- Configurable rate and burst
- Prevents spam from overwhelming system
- Does NOT terminate server (old behavior removed)
- Drops excess output when throttled


### 3. Container Recreation on Start

**Always Recreates Container**:
```go
OnBeforeStart() {
    // Always destroy and re-create
    Docker.ContainerRemove(e.Id, RemoveVolumes: true)
    e.Create()
}
```

**Reasons**:
- Ensures latest Panel config is applied
- Prevents config drift
- Avoids stale environment variables
- Fresh container state

### 4. State Persistence

**Persisted to**: `{data_path}/.states.json`
**Format**: `{"uuid": "state", ...}`
**Frequency**: Every 60 seconds
**Purpose**: Restore servers after Wings crash/restart

**Boot Restoration Logic**:
```
if persisted_state == "running" or "starting":
    if container is running:
        Attach to it (don't restart)
    else:
        Start server
else:
    Mark as offline
```

### 5. Activity Log Batching

**Local Storage**: SQLite database
**Schema**:
```sql
CREATE TABLE activity (
    id INTEGER PRIMARY KEY,
    event TEXT,
    user TEXT,
    server TEXT,
    ip TEXT,
    timestamp DATETIME,
    metadata JSON
)
```

