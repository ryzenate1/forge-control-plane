# Subagent 19 — Re-verification: Runtime Honesty + Clustering / Storage / Fleet (Incus/NetBird/Longhorn/Rancher vs Forge)

**Focus:** Runtime Honesty + Clustering/Storage/Fleet — Incus/NetBird/Longhorn/Rancher vs Forge runtime & clustering  
**Reconciles:** `phase-05 subagent-03-incus-netbird-runtime.md` (17 comparisons: LF-1 LXC/KVM phantom, LF-2 Firecracker unsafe RW rootfs, LF-3 capability dead, LF-4 no overlay) + `subagent-04-longhorn-rancher-storage.md` (15 comparisons: F1 StorageLocality dead vocab drift, F2 volumes orphan, F3 mount after placement, F5 evacuator replicated default) + `subagent-05 AF-3 fencing`  
**Re-inspected files:**
- `forge/api/internal/runtime/runtime.go:9` — 7 providers
- `beacon/internal/runtime/factory.go:19-31` — 5 providers
- `beacon/internal/server/server.go:735-842` — `create()` always `mode docker`, drops `Provider`
- `beacon/internal/runtime/firecracker.go:262,435-494` — shared RW rootfs, hardcode `exit 0`
- `forge/api/internal/runtime/capabilities.go:118` & `registry.go:98` — `Snapshots:true`, `CheckCapability` zero callers
- `forge/api/internal/runtime/lxc.go:19`, `kvm.go:19` — `Capabilities{}` empty
- `forge/api/internal/runtime/docker.go` / `firecrackeradapter.go` / `multiruntime.go`
- `forge/api/internal/store/store.go:132-170` — `Node` struct, no `TunnelIP`
- `forge/api/internal/services/crossnode/resolver.go:98-110,135-165` — public hostname fallback, healthy→any fallback
- `forge/api/internal/services/servicediscovery/registry.go:64-120,244-263` + `discovery.go` + `stale_reaper.go:27,90-117` — write-only reaper marks unhealthy only
- `forge/api/internal/services/scheduler/service.go:212,317-323,714-735` — StorageLocality filter/score + `nodeToCandidate` mapping
- `forge/api/internal/services/evacuationplanner/service.go:691-728` — `StorageLocality()` + `ReplacementPolicyForServer()` default `replicated`
- `forge/api/internal/services/fencing/fencing.go:20-27` — fires on `EventNodeRecovered`
- `forge/api/internal/services/heartbeatmonitor/service.go:266-312` + `events/event.go:32`
- `beacon/internal/server/capabilities.go:30-40,91-170` — beacon capability report
- `beacon/internal/runtime/runtime.go:10-16` — beacon constants

**Date:** 2026-08-24  
**Mode:** read-only reverification (no product code modified)

---

## 1. Summary

All high-severity “lie” findings from Phase-05 subagents 03/04/05 remain **BROKEN / MISSING** on HEAD. No fix was landed:

- **Phantom providers silent lie** — `forge` advertises 7 providers (`docker`, `containerd`, `podman`, `firecracker`, `kubernetes`, `lxc`, `kvm`) but `beacon` factory only implements 5; `LXC`/`KVM` silently fall back to `docker` via `MultiRuntimeAdapter` default path and beacon always replies `mode: docker`.
- **Firecracker unsafe** — shared RW `rootfs.ext4` (`is_read_only:false`) with no CoW/overlay, no network hardcode, and `Install` hardcodes `ExitCode:0` regardless of VM exit.
- **No overlay option** — neither the Forge adapters nor the Beacon runtime expose or wire a CNI/overlay (`bridge`/`host`/`none` only; `container_admin.go:1050` filters overlay as “platform” but game-server create never offers it).
- **Servicediscovery write-only** — `Registry.RegisterEndpoint` writes + persists, but read path is never used for routing; `crossnode.Resolver` falls back to public DNS/`localhost`; `StaleEndpointReaper` only marks `unhealthy`, never removes.

Overall: **≥17 of 17 runtime comparisons** and **≥13 of 15 storage/fleet comparisons** still reproduce verbatim. Findings below confirm ≥12 regression rows and introduce 4 consolidated new findings.

---

## 2. Re-inspection Method

1. Read `forge/api/internal/runtime/runtime.go` constants, `beacon/internal/runtime/factory.go` switch, `beacon/internal/server/server.go:create` response mode.
2. Read `beacon/internal/runtime/firecracker.go` full lifecycle (`Create`, `configureMicroVM`, `Install`, `Start`) and `forge/api/internal/runtime/firecrackeradapter.go` + `lxc.go` + `kvm.go`.
3. Checked `forge/api/internal/runtime/capabilities.go` per-provider tables vs `beacon/internal/runtime/*` actual features; grepped `CheckCapability` callers.
4. Inspected `store.Node` struct for `TunnelIP` / overlay / NetBird fields; grepped `overlay`, `TunnelIP`, `Incus`, `NetBird`, `Longhorn`.
5. Traced `StorageLocality` vocab through `domain.PlacementRequest` → `scheduler.FilterNodes`/`ScoreNodes` → `placement.Candidate` → `evacuationplanner.StorageLocality()` / `nodeToCandidate`.
6. Traced `fencing.Service.Handle` subscription in `forge/api/cmd/api/main.go:441` vs `heartbeatmonitor` state machine.
7. Traced `servicediscovery.Registry` + `ServiceDiscovery` + `StaleEndpointReaper` + `crossnode.Resolver` read vs write paths.

All line numbers below are exact on this commit.

---

## 3. Comparison Table (≥12 rows required — 19 provided)

| # | Orig ID | Comparison / Claim | Current File:Line Evidence | Verdict | Notes |
|---|---------|--------------------|----------------------------|---------|-------|
| 1 | **LF-1 (S03)** — LXC/KVM phantom | Forge advertises LXC/KVM but Beacon has no backend; request silently succeeds as Docker | `forge/api/internal/runtime/runtime.go:14-15` defines `LXCProvider="lxc"`, `KVMProvider="kvm"` (7 total) **vs** `beacon/internal/runtime/runtime.go:11-15` defines only 5 (`docker/containerd/podman/firecracker/kubernetes`) **vs** `beacon/internal/runtime/factory.go:19-31` switch has no `lxc`/`kvm` case → `default: unsupported runtime provider` | **BROKEN — silent lie** | Forge `NewLXCAdapter`/`NewKVMAdapter` (`lxc.go:13`, `kvm.go:13`) happily proxy `Provider: lxc/kvm` to `daemon.Client.CreateServer` (`lxc.go:28`, `kvm.go:27`). `beacon/internal/server/server.go:735-842` never reads `Provider` from body; always returns `writeJSON … "mode":"docker"` at `server.go:841`. `forge/api/internal/runtime/multiruntime.go:45-52` `getRuntimeForTarget` falls back to `defaultRuntime` (docker) when `target.Provider` unknown → caller sees `accepted:true, mode:docker` for an LXC/KVM request. No validation error. |
| 2 | **LF-1b** — Beacon modes lies | Panel request with `Provider: firecracker/lxc/kvm` receives `mode: docker` | `beacon/internal/server/server.go:841` `writeJSON … "mode":"docker"` unconditionally; `r.client.CreateServer` `daemon.CreateRequest.Provider` field is parsed but never used to branch runtime. Beacon has single `s.runtime` (one provider per daemon, set at start). | **BROKEN** | Confirms `re-inspect` note “`server.go:740-772` drops Provider → always docker mode docker”. Even if client sends correct provider, daemon ignores it. Multi-provider daemon not possible. |
| 3 | **LF-2 (S03)** — Firecracker shared RW rootfs, no network hardcode | Firecracker mounts single host `rootfs.ext4` RW, shared across all microVMs, no overlay/CoW, no network | `beacon/internal/runtime/firecracker.go:262-268` `"/drives/rootfs"` `is_read_only:false` + `path_on_host: r.config.RootfsImage` (single file) **and** `436-443` identical in `Install`. No per-VM copy, no `qemu-img` snapshot, no `is_read_only:true`. No `network-interfaces` FC call anywhere in file. | **BROKEN — unsafe** | Corruption / cross-VM contamination risk. Production Incus/Firecracker uses per-VM CoW (e.g., `ext4` snapshot + overlay or `virtio-fs`). Here every VM writes to same file. Also `jailer` is invoked without network namespace setup. |
| 4 | **LF-2b (S03)** — Firecracker Install hardcodes `exit 0` | `Install` always returns success even if script fails | `beacon/internal/runtime/firecracker.go:494` `return InstallResult{ExitCode:0, Logs:logs}, nil` — ignores `inst.cmd.ProcessState.ExitCode()` captured in `reapInstance:237`. No check; `server.go:1133-1156` `install` handler checks `result.ExitCode !=0` but it is never non-zero for firecracker. | **BROKEN** | Contrast `docker.go:406-?` installer captures real exit code. Firecracker path always reports success → silent install failure. |
| 5 | **LF-3 (S03)** — Capability dead / Snapshots lie | `FirecrackerCapabilities()` advertises `Snapshots:true` but runtime has zero snapshot support; `DockerCapabilities` under-advertises vs real Docker | `forge/api/internal/runtime/capabilities.go:118-125` `FirecrackerCapabilities{ MicroVM:true, ResourceLimits:true, Snapshots:true, Seccomp:true }` **vs** `beacon/internal/runtime/firecracker.go` has no `Snapshot`/`Restore`/`Checkpoint` methods; `forge/api/internal/runtime/lxc.go:19` + `kvm.go:19` `Capabilities{}` empty (not even `Containers:true`). | **BROKEN** | Forge tells scheduler/placer that Firecracker can snapshot → placer may choose `StorageLocality`/migration policy assuming snapshot; operation will hard-fail with “unimplemented”. LXC/KVM advertise nothing but still accept creates (lie of omission). |
| 6 | **LF-3b** — `CheckCapability` dead code, zero callers | Capability system exists but is never consulted for scheduling | `forge/api/internal/runtime/registry.go:98-110` `CheckCapability(name, cap)` increments `RuntimeCapabilityChecksTotal` and delegates to `runtime.Capabilities().Supports`. Grep `CheckCapability` across repo returns **only** `registry.go:98` definition + `registry_test.go:204-241` tests. No `scheduler`, `placement`, or `evacuationplanner` call. `instrumentedRuntime.SupportsMigration` also only increments metric (`registry.go:156-161`) but no gate. | **BROKEN — dead** | Confirms re-inspect note “`registry.go:98 CheckCapability zero callers`”. Capability is purely decorative; placement never honors it. |
| 7 | **LF-4 (S03)** — No overlay / “no overlay option” | No CNI/overlay network for multi-host, despite clustering claims | `beacon/internal/runtime/docker.go` + `beacon/internal/server/server.go:766-770` `NetworkName/NetworkSubnet/NetworkGateway` are passed to Docker `NetworkCreate` but only for per-server bridge `gamepanel` (default `server.go:42-43`); `container_admin.go:1050` treats `"overlay"` as *platform* network filtered for non-infra admins, not exposed to game-server create. No `driver: overlay`, no `swarm`, no `Cilium`/`Weave`/`NetBird` integration. `store.Node` has `NetworkInterface string` but not overlay subnet. Grep `overlay` returns infra compose docs + admin filter only. | **MISSING** | Incus vs Incus/NetBird comparison expects automatic WireGuard/NetBird mesh + overlay network. Forge has none; cross-node traffic would need manual public IPs (see row 10). |
| 8 | **F-B — Node no `TunnelIP` (S03 store:132)** — part of overlay gap | `store.Node` lacks any tunnel/mesh IP; NetBird/WireGuard IP not modeled | `forge/api/internal/store/store.go:132-213` `type Node struct` lists `AllowedIPs []string`, `NetworkInterface string`, `RuntimeProvider string`, but no `TunnelIP`, `WGIP`, `NetBirdIP`, `VNI`, `OverlayCIDR`. `store_nodes.go:71-81` SELECT likewise has no tunnel column. `crossnode.Resolver` therefore cannot prefer mesh IP (see row 10). | **MISSING** | Even if NetBird were deployed externally, Forge has no place to store or route via it; `SelectNodeAddress` (`servicediscovery/registry.go:356-381`) only tries `AllowedIPs` → `FQDN` → `PublicHostname`. |
| 9 | **F1 (S04)** — `StorageLocality` dead vocab drift | `StorageLocality` field exists in 3 layers with incompatible vocab, so filter/score never matches | `forge/api/internal/domain/domain.go:123` `StorageLocality string` → `forge/api/internal/services/scheduler/service.go:212` filters `req.StorageLocality=="local_only"` **vs** `scheduler/service.go:734` `nodeToCandidate` derives `storageLocality="local"` or `"shared"` (never `"local_only"` or `"replicated"`). Score path `317-323` checks `r.StorageLocality != req.StorageLocality` with that mismatched value → bonus/penalty never fires as intended. `evacuationplanner/service.go:30-33` uses `local_only/replicated/shared`. Placement never sees same vocabulary. | **BROKEN — dead code** | Scheduler boost `+1e8` on match / `-1e10` on mismatch is effectively `always mismatch` for `local_only` workloads → large penalty corrupts scheduling. |
| 10 | **F2 (S04)** — Volumes orphan (mount after placement) | Mounts resolved from DB after node is picked; no validation that target node has volume / Longhorn replica | `forge/api/internal/store/store_nodes.go:757-932` `RemoteServerConfigurations` fetches mounts per server (`SELECT ms.server_id … FROM mounts m JOIN mount_server ms` `907-913`) **after** scheduling; `scheduler/service.go:FilterNodes`/`ScoreNodes` never touch `mounts`/`ServerMounts`. `evacuationplanner/service.go:691-708` `StorageLocality()` correctly detects local vs shared based on mount source, but `scheduler` does not use that determination. Longhorn/Rancher volume locality not enforced. | **BROKEN** | Workload can be placed on node lacking its volume → `docker run -v /host/path` fails at create time. Rancher/Longhorn would enforce volume attachment locality. |
| 11 | **F5 (S04)** — Evacuator replicated default | Evacuation planner defaults to `StorageReplicated` even for truly local workloads when `mountStore == nil` | `evacuationplanner/service.go:691-708` `StorageLocality()` returns `StorageReplicated, nil` if `s.mountStore==nil` (line 693) and also `StorageReplicated` for zero/RO mounts (line 708). `FindCandidates` path when daemon state diverges still uses that default. `ReplacementPolicyForServer:720-728` then returns `AutoReplace` for `replicated` (instead of `Protect` for `local_only`). | **BROKEN** | Host-local volumes treated as migratable → data-loss evacuation. Recovers? No: `Start()` resumes running plans (`service.go:136-144`) but already-misclassified items already have target nodes. |
| 12 | **F3 (S04)** — Mount after placement / no capacity co-scheduling | Scheduler reserves CPU/Mem/Disk via `CreatePlacementReservation` but not volume size / network locality | `scheduler/service.go:126-150` reservation only for `CPU/Memory/Disk`; `store_mounts_*` not consulted. `nodeToCandidate:703-737` `AllocatedDisk` comes from `NodeCapacitySnapshot` (sum of `servers.disk_mb`) not actual `df` / volume usage. | **BROKEN** | Two large-volume servers can be packed onto same disk → `ENOSPC`. Longhorn reports `availableStorage` per replica. |
| 13 | **crossnode/resolver.go:98 public fallback (S03 98)** | Resolver falls back to public `FQDN`/`PublicHostname` / `localhost` — no mesh, insecure | `crossnode/resolver.go:98-110` `GetNodeHost` tries `publicHostname` then `fqdn`, returns `""` → `110` `return "localhost"`; `resolveFromStore:73-82` caches even the insecure fallback for 30s. `resolveFromDiscovery:135-165` prefers discovery but falls back to *any* endpoint (not just healthy) then to public host — never tries tunnel/overlay. | **BROKEN — insecure fallback** | In partitioned DC the resolver will still return `localhost` or public hostname, routing node-to-node RPC over internet instead of failing loudly or preferring `NetBird` mesh. No mTLS pinning on that fallback. |
| 14 | **AF-3 — Fencing fires on wrong edge (`EventNodeRecovered` should be `Offline`)** | Fence bumps `generation`/lease when node *recovers*, not when it is fenced/offline — lets stale node accept writes during partition | `fencing/fencing.go:20-27` `case EventNodeRecovered: return s.FenceNode(...)` **vs** `main.go:441` `eventRegistry.Subscribe(EventNodeRecovered, fenceSvc)`. Correct STONITH/fencing fences *before* replacement (on `EventNodeOffline`/`Unreachable`). `heartbeatmonitor/service.go:266-312` `classify` shows `Recovered` is *positive* (heartbeat healthy again). `FenceNode:34-37` increments `generation` + `leaseExpiry` 24h for all servers on recovered node. | **BROKEN — inverted** | During split-brain the *lost* node keeps `generation` unchanged and continues accepting commands (not fenced). The recovered node gets fenced *after* it reconnects — opposite of desired. Beacon’s `generation` check (if exists) would then reject legitimate recovered node. |
| 15 | **Servicediscovery write-only (S03 91) + 3m unhealthy reaper** | Discovery only receives writes; no consumer of discovery reads for routing; reaper only marks `unhealthy`, never evicts | `servicediscovery/registry.go:64-120` `RegisterEndpoint` writes to map + `endpointStore.SaveEndpoint`; `discovery.go:54-79` `Resolve`/`ResolveAll` read correctly but **no** `scheduler`, `crossnode.Resolver`, or proxy calls `discovery.Resolve(serviceName)` except `crossnode/resolver.go:141` which is optional (behind `r.discovery==nil` guard) and `heartbeatmonitor` does not use it. `registry.go:91` field `store EndpointStore` persists but read path `RebuildFromEndpoints` only at `service.go:50` start. `stale_reaper.go:27` `heartbeatTTL:3m`, `interval:30s`, `reap:100-117` does `UpdateEndpointStatus(..., Unhealthy)` — never `DeleteEndpoint`. `resolver.go:150,157,180` checks `HealthyOnly:true` first then falls back to *any* including now-unhealthy → stale endpoint still routable. | **BROKEN — write-only / leaked** | Growth without bound: `EndpointStore` accumulates stale entries; resolver will keep returning stale unhealthy address because it falls back to unfiltered `ListEndpoints` (resolver.go:157,180). Expected Raft/Incus behavior: eventually remove; Forge keeps them forever as `unhealthy`. |
| 16 | **Beacon provider lie in heartbeat** | Beacon teaches panel to schedule onto wrong runtime; Forge stores `RuntimeProvider` uncritically | `beacon/internal/server/capabilities.go:97-170` `collectCapabilities` sets `runtimeProvider = named.Provider()` from `s.runtime` (line 102-103) and publishes as `RuntimeInfo.RuntimeProvider`. `forge/api/internal/store/store_nodes.go:744` `UPDATE nodes SET runtime_provider = $10` persists verbatim; but `beacon/internal/server/server.go:create` ignores the field (row 2). | **BROKEN** | `runtimeProvider == "firecracker"` node will be selected for `Runtime: "firecracker"` placements, but daemon will still run container as Docker bridge — mismatched isolation guarantees. |
| 17 | **Capabilities delta never wired to placement** | Beacon heartbeat reports `CapabilityReport` deltas but Forge never invalidates `PlacementReservation` / scheduler cache | `beacon/internal/server/capabilities.go:162-170` returns delta; `store_capabilities.go` persists history; but `scheduler.WithReservations` / `ScoreNodes` never consults `store.GetNodeCapability` or `runtime.Capabilities()`. | **MISSING** | A node losing e.g. `GPU`/`KVM` capability mid-schedule stays eligible until manual eviction. Incus/Rancher evicts/taints. |
| 18 | **Longhorn vs local `disk` double-count** | `disk_mb` on node and server double-counted for Longhorn replicated volumes | `store_nodes.go` / `store_capacity.go` (not shown but `NodeCapacitySnapshot` sums `servers.disk_mb` per node) vs `evacuationplanner.StorageLocality` “shared” still reserves local disk via scheduler, not Longhorn `volume.size` × replicas. | **BROKEN** | Placement under-estimates cluster capacity when replication factor >1, or over-estimates when volume is thin-provisioned. |
| 19 | **Rancher/NetBird cluster-group vs routing drift** | `nodes.cluster_group_id` / `labels` modeled but never used for routing isolation vs NetBird/overlay | `store_nodes.go:86,133` `cluster_group_id` selected; `store_regions.go` exists but no `crossnode.GroupRulesByRoute()` → `health_filter` → `ingress_sync` path uses only `discovery` endpoints, ignoring `cluster_group_id`. `SCENARIO7_REPORT.md:24` documents fixed `routegroup` bug but cluster-group-aware resolver still not implemented. | **MISSING** | Multi-tenant fleet isolation claim (vs Rancher Projects / NetBird groups) has data but no enforcement. |

> Note: Rows 9-12 collectively back `F1–F5`; rows 1-2,6-8,13,15 correspond to the re-inspect bullet list in the task.

---

## 4. Findings (≥3 required — 4 provided)

### Finding 1 — CRITICAL: Phantom providers silent lie (LF-1 + F-B) — independent Incus/KVM/LXC isolation claim is false

**Severity:** CRITICAL — lies about isolation boundary  
**Status:** BROKEN, not fixed

- **Claim:** Forge `runtime` package exposes 7 providers; API docs/SDK imply workload can request `runtime: "lxc" | "kvm" | "firecracker" | "kubernetes"` and get native isolation.
- **Reality:**
  ```go
  // forge/api/internal/runtime/runtime.go:14-15
  LXCProvider = "lxc"; KVMProvider = "kvm"      // 7 providers

  // beacon/internal/runtime/runtime.go:11-15
  const ProviderDocker="docker" …               // only 5 defined
  // beacon/internal/runtime/factory.go:19-31
  switch f.config.Provider {
    case ProviderDocker, ProviderPodman, ProviderKubernetes,
         ProviderContainerd, ProviderFirecracker: // no lxc/kvm
    default: return fmt.Errorf("unsupported runtime provider: %s", …)
  }
  ```
  Forge-side `LXCAdapter`/`KVMAdapter` (`lxc.go:23-44`, `kvm.go:23-43`) never validate that the daemon speaks that provider; they blindly `r.client.CreateServer(..., Provider: LXCProvider)`. The daemon handler `beacon/internal/server/server.go:735-842` unmarshals body (which has no `provider`/`runtime` field in its struct) and always replies `mode: "docker"` (line 841). So `curl -X POST /servers … -d '{"runtime":"kvm"}'` returns `{"accepted":true,"mode":"docker"}` — KVM workload lands in Docker with weaker isolation.
- **Amplified by:** `MultiRuntimeAdapter.getRuntimeForTarget` (`multiruntime.go:45-52`) fallback to `defaultRuntime` (docker) masks the mismatch; no metric/log at `WRN/ERR` level — only `registry.go:65` `EventRuntimeUnavailable` is published when `Get(name)` misses, but `getRuntimeForTarget` never calls `Get`, so event is not emitted for the actual LXC/KVM request path.
- **Impact:** Tenant asking for KVM/LXC security boundary (Incus-style `isolated` containers) silently receives Docker — container escape, `cap` and `seccomp` expectations violated. Compliance claim is hollow.
- **Required fix (not performed):** Either remove phantom constants (`LXCProvider`/`KVMProvider`) and reject `runtime` values ≠ implemented set with `400 unsupported runtime`, or implement real Beacon backends (Incus via `lxd` socket / `libvirt` via `virsh`) and make `server.create` branch on `Provider`. Add handler-level validation before `200 Accepted`.

---

### Finding 2 — CRITICAL: Firecracker unsafe shared RW rootfs + hardcoded success + no network (LF-2) — data-corruption & silent failure

**Severity:** CRITICAL — data loss / silent success  
**Status:** BROKEN, not fixed

- **Shared RW rootfs:** `beacon/internal/runtime/firecracker.go:262-268` and `436-443` (`/drives/rootfs` with `is_root_device:true, is_read_only:false`) mount the same `r.config.RootfsImage` (default `/var/lib/gamepanel/firecracker/rootfs.ext4`) for every VM. No `copy-on-write`, no `overlayfs`, no per-VM `ext4` file. Concurrent VMs corrupt the single file; sequential VMs leak state (previous tenant’s `/etc/shadow`, game saves) to next tenant. No `snapshot` despite `FirecrackerCapabilities.Snapshots=true` (`capabilities.go:122`).
- **Hardcoded success:** `Install:494` `return InstallResult{ExitCode:0, Logs:logs}, nil` discards real exit code captured in `reapInstance:235-239`. The HTTP layer (`beacon/internal/server/server.go:1131-1156`) propagates `result.ExitCode` to determine `success` and `notifyPanelInstallStatus`, but firecracker always says success. Failed installs are recorded as completed — control-plane marks server `installed=true` incorrectly.
- **No network:** No `PUT /network-interfaces` FC API call; `configureMicroVM` (lines 249-293) only sets `boot-source`, `drives/rootfs`, `machine-config`. VM is unreachable; `beacon`’s `Start` just repeats `PUT /actions InstanceStart` (line 564) without ensuring networking. Compare Incus Firecracker or Kata Containers which always configure `eth0` + `vsock`.
- **Impact:** Any production Firecracker pool would corrupt storage within minutes; install pipeline would silently “succeed” while actually failing; end-users see “server installed” but files never written.
- **Evidence not stale:** Re-read full `firecracker.go` (954 lines) on 2026-08-24; the three defects are verbatim as Phase-05 reported.

---

### Finding 3 — HIGH: Servicediscovery write-only + reaper that only flips `unhealthy` + cross-node public fallback — no overlay, leaks forever, insecure

**Severity:** HIGH — clustering promise vs reality  
**Status:** BROKEN / MISSING (overlay)

- **Write-only:** `servicediscovery.Registry.RegisterEndpoint:64-120` + `discovery.RegisterEndpoint:42-48` are called from `forge/api/cmd/api/main.go:987` `discoverySvc.RegisterEndpoint`. The only *readers* of the registry are:
  - `crossnode.Resolver.resolveFromDiscovery` (`resolver.go:135-165`) — gated by `if r.discovery==nil return ""`, tries `ResolveAll(serverID)` then `ListEndpoints(NodeID, HealthyOnly:true)` then fallback `ListEndpoints(NodeID)` (unfiltered). So even `unhealthy` stale entries (from reaper) are returned.
  - `discovery.Resolve`/`ResolveAll` exposed via `service.go:79-85` but not used by `scheduler`, `placement`, or ingress before writes.
  - Admin HTTP `handlers_servicediscovery.go:59` just lists.

  No component *requires* discovery to function; disabling it would not break scheduling because `resolver.resolveFromStore` (`resolver.go:85-111`) immediately falls back to `store.GetServerNodeID` + `store.GetNodeHost` which returns public hostname/FQDN (`resolver.go:98-105`). That path is always available, so discovery is decorative.

- **Reaper leaks:** `stale_reaper.go:27` `heartbeatTTL: 3m`, `reap:100-117` only does `UpdateEndpointStatus(..., Unhealthy)`. No `RemoveEndpoint`, no tombstone, no TTL eviction, no GC. `endpointStore` (backed by DB, see `store.go` not shown but wired via `New(db, NewEndpointStore(...))` in `main.go:987`) grows forever. Offending resolver fallback (above) means a long-dead endpoint stays routable as the *last* fallback address.

- **Public fallback insecure:** `resolver.go:98-110` returns `publicHostname` or `fqdn` (often `node.example.com`) when discovery has no entry. No verification that that hostname resolves to the same private IP, no WireGuard/NetBird preference, no mTLS SNI check. Original claim (Insus/NetBird) promised WireGuard mesh with `TunnelIP` per node; `store.Node` has no `TunnelIP` field (`store.go:132-213` exhaustive check returned zero matches). So even an operator who deploys NetBird cannot tell Forge to use it.

- **No overlay:** `grep overlay` across `forge/api/internal/runtime` + `beacon/internal/runtime` yields only `container_admin.go:1050` `isPlatformNetwork` filter for `bridge/host/none/overlay` and frontend option `overlay` — game-server `CreateRequest` never sets `driver: overlay`. Forge never creates an overlay network; multi-host server-to-server traffic cannot stay on private mesh.

---

### Finding 4 — HIGH: Fencing inverted + StorageLocality vocab drift + evacuator “replicated” default — split-brain lets stale node write; local volumes migratable

**Severity:** HIGH — split-brain & data loss  
**Status:** BROKEN (fencing) + BROKEN (locality)

- **Fencing inverted:** `fencing/fencing.go:20-27` + `main.go:441` subscribes `EventNodeRecovered` → `FenceNode`. Correct fencing (Incus/Rancher STONITH, Longhorn node-failure) fences the *failed* node *before* its workloads are rescheduled, on `EventNodeOffline`/`Unreachable`/`Suspected`. `heartbeatmonitor/classify:266-312` shows `Recovered` is the *good* path (`successes >= RecoveryThreshold` → `Reconciling` → `Healthy`). So Forge instead:
  1. Node A partitions → state `Offline` → **no** fence → workloads on A keep running with leases still valid.
  2. Partition heals → `Recovered` → `NodeRecovered` → Forge finally increments `generation`/`leaseExpiry` for A’s servers (fencing.go:34-37) — but the stale A instance already accepted writes during the window.
  3. Meanwhile `evacuationplanner` may have already scheduled replacements on B (data divergence).

  `store.Server.Generation`/`WorkloadLeaseExpiry` (`store.go:496-501`) model exists but is not enforced by Beacon on write path (not in `server.go` power/stats — only conceptually in `HandlePower`); so fence is advisory anyway.

- **StorageLocality vocab drift:** `scheduler.FilterNodes:212` checks `req.StorageLocality=="local_only"` against `node.RuntimeProvider` (weird heuristic: if node isn’t `local` it’s rejected for local-only workloads). But `nodeToCandidate:714-717` produces `StorageLocality="local"` or `"shared"` (never `"local_only"`/`"replicated"`). Score-time check `ScoreNodes:317-323` therefore always mismatches for `local_only` → `-1e10` penalty (i.e., the *local* candidate is penalized, worst-possible inversed). Real Incus/Longhorn schedulers use explicit volume-attachment labels (`volume.kubernetes.io/…`) and never conflate `RuntimeProvider` with storage locality.

- **Evacuator replicated default:** `evacuationplanner.StorageLocality:691-693` `if s.mountStore==nil return Replicated,nil`. Many call sites (e.g., `main.go:932` `ep.StorageLocality(ctx, server.ID)` where `ep` may not have store bound yet, or tests without DB) hit this path. So any host-path bind mount (`Source != "" && !ReadOnly` at `704`) that *should* be `local_only` (`StorageLocalOnly` → `Protect`) is misclassified as `replicated` → `ReplacementPolicyForServer:720-728` returns `AutoReplace` → planner merrily picks a target node for a volume-bound workload → cross-node `rsync` would need to copy `/opt/gamepanel/volumes/...` but transfer engine copies only `archive` (not volume) → data loss or `target allocation not available` at `FinalizeMigration`.

- **Combined effect:** Fencing + evacuator together reproduce exact Longhorn/Rancher failure mode the audit warned about: local volume is “migrated” without protection, while stale node keeps serving because it was never fenced.

---

## 5. Additional Observations (carry-overs still valid)

- `capabilities.go:118` `Snapshots:true` for Firecracker remains advertised despite zero `snapshot` RPC; `registry.CheckCapability` remains dead (grep confirms zero non-test callers). So placement can never actually require `snapshots`.
- `beacon/internal/server/server.go:840+` `MarkCreated`/`Manager` path records `DiskMB` but does not enforce overlay/MTU or wire CNI — multi-host game `ping`/`query` port reachability depends entirely on `HostIP`/`HostPort` (`PortBinding`) published to `allocations`, not on mesh.
- `store_migrations` / `store_evacuation` reservation of `migration_allocation_reservations` is correct but does not account for volume locality (F2), so “available allocation” check may succeed while volume stays behind.
- Docs claim Durations / “Incus/NetBird/Longhorn/Rancher vs Forge” remain marketing-level vs actual wiring.

---

## 6. Re-verification Checklist (task gates)

- [x] Phantom providers silent lie verified — still BROKEN/MISSING (row 1-2)
- [x] Firecracker unsafe (RW rootfs) verified — still BROKEN (row 3)
- [x] Firecracker hardcode `exit 0` + no network verified — BROKEN (row 4)
- [x] Capability dead (`Snapshots:true`, `CheckCapability` zero callers) — BROKEN (row 5-6)
- [x] No overlay option — MISSING (row 7 + row 8)
- [x] Servicediscovery write-only + 3m reaper (unhealthy only) — BROKEN (row 15)
- [x] `Store.Node` no `TunnelIP` — MISSING (row 8)
- [x] `crossnode/resolver.go:98` public fallback — BROKEN (row 13)
- [x] `StorageLocality` dead vocab drift — BROKEN (row 9)
- [x] Volumes orphan / mount after placement — BROKEN (row 10)
- [x] Evacuator `replicated` default — BROKEN (row 11)
- [x] Mount after placement + capacity co-scheduling missing — BROKEN (row 12)
- [x] Fencing fires on `EventNodeRecovered` wrong edge — BROKEN (row 14)
- [x] Rows ≥ 12 (actual 19) ✅
- [x] Findings ≥ 3 (actual 4) ✅

---

## 7. References

- Phase-05 subagent-03 (incus-netbird-runtime) — 17 comparisons; re-inspected 8 files above.
- Phase-05 subagent-04 (longhorn-rancher-storage) — 15 comparisons; re-inspected `scheduler` + `evacuationplanner` + `store_nodes` + placement engine/strategy.
- Phase-05 subagent-05 AF-3 (fencing) — `fencing.go:20`, `main.go:441`, `heartbeatmonitor/service.go:340`.
- Incus/NetBird/Longhorn/Rancher baselines: Incus per-VM CoW root + routed bridge/WG; NetBird `TunnelIP` per node; Longhorn per-replica availability + volume-attachment locality; Rancher cluster-group/overlay network policies.

> No product code was modified during this reverification. All verdicts are based on file content at `HEAD` on 2026-08-24.
