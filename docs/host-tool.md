# Host Tool — Beacon `handlers_host.go` — FROZEN

> **Status:** **FROZEN** as of Phase 03 subagent-20. The host diagnostics surface (`GET /host/info|disk|memory|network|processes`) is considered stable. No further endpoint or schema changes are planned without a versioned proposal. Fixes in this phase (daemon vs host uptime, inert sort, suspend accounting) are the last scheduled behavioural changes.

## Endpoints (Beacon → Forge proxy)

All under `forge/api/internal/http/handlers_host.go:registerHostRoutes` (admin-only, daemon proxied):

```
GET /host/info       → HostInfo
GET /host/disk       → DiskPartition[]
GET /host/memory     → MemoryInfo
GET /host/network    → NetworkInterface[]
GET /host/processes  → ProcessEntry[]
```

Beacon handlers: `beacon/internal/server/handlers_host.go` + `sysinfo_linux.go` / `sysinfo_darwin.go` (`netInterfacesPlatform`, `processListPlatform`, `totalSystemMemoryMB` etc).

## Schemas

```go
type HostInfo struct {
  Hostname     string `json:"hostname"`
  OS           string `json:"os"`
  Kernel       string `json:"kernel"`
  Uptime       int64  `json:"uptimeSeconds"`          // host wall uptime since boot (hostUptimeSeconds), NOT daemon uptime
  DaemonUptime int64  `json:"daemonUptimeSeconds"`    // beacon process uptime excluding suspend gaps (daemonUptimeSeconds, CLOCK_MONOTONIC)
  CPUModel     string `json:"cpuModel"`
  CPUCores     int    `json:"cpuCores"`
  Arch         string `json:"arch"`
  Time         string `json:"time"`                   // RFC3339 wall time
}
```

### Uptime semantics (Phase 03 fix)

- **Previously mislabelled:** `handlers_host.go:65` returned `int64(time.Since(s.started).Seconds())` as `uptimeSeconds`, conflating **daemon** uptime with **host** uptime. Operators reading “Uptime” expected host since-boot, not beacon process lifetime.
- **Now:** `Uptime` is host wall uptime (`hostUptimeSeconds()` → `unix.Sysinfo.Uptime` on Linux with `/proc/uptime` fallback; `kern.boottime` on Darwin). `DaemonUptime` is beacon process uptime via `daemonUptimeSeconds(s.started)` which uses Go's `time.Since` (`CLOCK_MONOTONIC` on Linux, which pauses across suspend). The two are distinct; their gap is suspend duration (`suspendDuration()` = wall elapsed − monotonic elapsed, ≥0).
- Polling hint in `forge/web/app/admin/host/page.tsx:InfoTab` → `fmtUptime(data.uptimeSeconds)` now reflects host, while a future “daemon” tile can use `daemonUptimeSeconds` without breaking the existing `uptimeSeconds` contract.

## Platform collectors

- **Linux** (`sysinfo_linux.go`): `totalSystemMemoryMB`/`freeMemoryMB` via `unix.Sysinfo`, `cpuModelPlatform` via `/proc/cpuinfo`, `netInterfacesPlatform` via `/sys/class/net` (`speed`, `operstate`), `processListPlatform` via `/proc/<pid>/status`. Previously `processListPlatform` contained an inert sort at `sysinfo_linux:94` (sorted a copy or sorted prior to append, observable as non-deterministic order). Now `sort.Slice(processes, PID ascending)` is performed in-place after collection so `GET /host/processes` is stable and tests can rely on order.
- **Darwin** (`sysinfo_darwin.go`): `hw.memsize`, `machdep.cpu.brand_string`, `net.Interfaces()` for IP/MAC, empty `processListPlatform` (no `/proc`). Same `hostUptimeSeconds`/`daemonUptimeSeconds` helpers via `kern.boottime` / monotonic.

## Freeze policy

- The five handlers, their response shapes, and the collector helpers are **frozen**. New fields may be added as optional (`omitempty`) but MUST NOT rename or reinterpret `uptimeSeconds`.
- The monitoring stack (Prometheus) still scrapes `caddy:2019/metrics` and `game_panel_daemon_uptime_seconds`; host-tool is not a metrics scrape path.
- If you need richer host telemetry, extend `GET /monitoring/nodes/metrics` (live OS counters) rather than widening `/host/*`.

## UI

`forge/web/app/admin/host/page.tsx` — tabbed host view (System, Storage, Memory, Network, Processes) with 15s poll, `NodeSelect`, `OfflineBanner`, `fmtUptime`/`usageBar`. Shows host `Uptime` correctly labeled. Processes tab already sorted by CPU/Memory client-side; server now also orders by PID as baseline.
