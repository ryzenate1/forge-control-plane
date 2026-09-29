# Subagent 08 — Uncloud WireGuard Mesh & Clusterless Orchestration vs Forge Networking + Clustering

## Scope
- Reference: `reference/app-platforms/uncloud` — `pkg/client/service.go` RunService, `internal/machine/docker/*`, `internal/ucind/cluster.go`, `internal/machine/cluster.go`, `internal/machine/cluster/cluster.go`, `internal/machine/store/*`, `internal/machine/dns/*`, `internal/machine/caddyconfig/controller.go`, `pkg/client/connector/wireguard.go`, `pkg/client/deploy/*`, `internal/machine/network/*`
- Forge: `forge/api/internal/services/clustermembership/service.go`, `services/crossnode/{resolver.go,health_filter.go,ingress_sync.go,routegroup.go}`, `services/servicediscovery/{registry.go,reachability.go,model.go}`, `services/loadbalancer/service.go`, `internal/store/store_capacity.go`, `internal/store/store_target_groups.go`, `internal/placement/{engine.go,strategy.go}`, `services/heartbeatmonitor/service.go`, `forge/web/app/admin/cloud/page.tsx:1-165`, `forge/api/internal/store/store_cloud.go:17-58`, requested `forge/api internal store cluster_membership` + `beacon/internal/remote` (both not present as distinct modules — see C17)
- Method: file:line citations, no product code modification; `forge/beacon/internal/remote` does not exist in this workspace (`find forge/beacon` returns no such directory); `store_cluster_membership` is not a file — membership persistence is via generic `store` (`store.go`/`store_cloud.go`) + `clustermembership/service.go`

---

## 1. Architecture Overview

### Uncloud: Clusterless / Decentralized (No Control Plane)

Uncloud is explicitly "Decentralized, no control plane" (`reference/app-platforms/uncloud/AGENTS.md:31`). Every machine runs an equal daemon `uncloudd` (`reference/app-platforms/uncloud/cmd/uncloudd/main.go`). State is held in **Corrosion** — a CRDT-based distributed SQLite (Fly.io project) with gossip replication (`reference/app-platforms/uncloud/internal/corrosion/client.go:1`, `reference/app-platforms/uncloud/internal/machine/store/store.go:25-31`). There is no leader, no quorum, and "partial network splits remain functional" (`AGENTS.md:33`).

Key subsystems on each node (`reference/app-platforms/uncloud/internal/machine/cluster.go:39-74` — `clusterController`):
- `network.WireGuardNetwork` (mesh)
- `corroservice.Service` (Corrosion)
- `docker.Service` + `docker.Controller` (container lifecycle)
- `dns.Server` + `dns.ClusterResolver` (service discovery)
- `caddyconfig.Controller` (ingress via Caddy)
- `metrics.Server` + embedded `unregistry`

### Forge: Centralized API + Postgres + Daemons (Beacon)

Forge has a central `forge/api` (Go, Postgres/pgx) that is the authoritative control plane. Nodes run `beacon` (TLS-authenticated) and heartbeat into Postgres (`forge/api/internal/services/heartbeatmonitor/service.go:18-47`). Membership, placement, scheduling, and ingress are all coordinated through the API database. There is no mesh — inter-node traffic traverses public IP / FQDN / AllowedIPs (`forge/api/internal/services/servicediscovery/registry.go:356-381` `SelectNodeAddress`).

---

## 2. Detailed Comparisons (16 items)

### C01 — Membership & Failure Detection: Gossip/CRDT vs Postgres Heartbeat State Machine

| | Uncloud | Forge |
|---|---|---|
| Mechanism | CRDT `cr_sqlite` membership via Corrosion admin; `ClusterMembershipStates(true)` maps Serf ALIVE/SUSPECT/DOWN to `pb.MachineMember_UP/SUSPECT/DOWN` (`reference/app-platforms/uncloud/internal/machine/cluster/cluster.go:209-238`) with local override: current machine always UP (`:232-234`) | Postgres-backed state machine in `store` + `heartbeatmonitor` with thresholds: `WarningThreshold 30s → SUSPECTED`, `OfflineThreshold 90s → UNREACHABLE`, `UnavailableAfter 300s → OFFLINE`, `RecoveryThreshold` consecutive heartbeats to flip to `RECOVERING→RECONCILING→HEALTHY` (`forge/api/internal/services/heartbeatmonitor/service.go:89-97,266-313`) |
| Quorum | None; "no quorum requirements" (`AGENTS.md:33`) | Single-writer Postgres; if API/DB is down, membership freezes |
| Who decides | Every node independently queries Corrosion locally; no central arbiter | `heartbeatmonitor.EvaluateAll` periodically calls `SetNodeHeartbeatClassification` (`:231-232`) which writes canonical state |
| Timeliness | Serf gossip is near-real-time (seconds), but converge time depends on mesh connectivity (`internal/corro*`) | Polling interval `30s` + `5s` jitter (`heartbeatmonitor/service.go:142-149`) |
| Stale-write handling | Corrosion merges via CRDT; last-write-wins per key | DB row lock `FOR UPDATE` on `nodes` (`store_capacity.go:219` pattern + heartbeat classification) |

**Implication:** Uncloud tolerates API loss but risks split-brain deploys (two partitions both accept writes that later conflict). Forge serializes membership under Postgres but becomes unavailable when the API is down.

---

### C02 — State Consistency: Eventually Consistent CRDT Store vs Strongly Consistent Postgres

| | Uncloud | Forge |
|---|---|---|
| DB | Embedded `badger` + `cr-sqlite` via Corrosion over WireGuard (`internal/machine/store/store.go:26-27`; `internal/corrosion/admin.go`) | Postgres via `pgx` (`forge/api/internal/store/driver_postgres.go`) |
| Version vector | `SELECT site_id, db_version FROM crsql_db_versions` → UUID actor version map (`internal/machine/store/store.go:64-88`). `waitStoreSync` + `laggingActors` + `waitKnownMissingChanges` poll for gaps on join (`internal/machine/cluster.go:371-483`) | No version vector; monotonic `schema_versions` + transactional row versions |
| Read semantics | Local read of `machines`/`containers` may see empty JSON during partial replication; callers must skip `mJSON == "{}"` (`store.go:198-202,288-292` and `container.go:150-154,242-246`) and re-read; F/G in `ListMachines`/`ListContainers` explicitly filter empties | Single-writer transactional snapshot; no partial-replication filtering needed |
| Write semantics | `INSERT OR REPLACE` / `ExecContext` via HTTP2 Anted Corrosion API with `RetryRoundTripper` (transport retries `net.OpError` bounded to `2s`) and `RetrySubscription` resubscribe from `lastChangeID` (`internal/corrosion/client.go:51-66,148-248`) | Synchronous SQL transactions; `store_capacity.go` uses `FOR UPDATE` and pending/active reservation accounting |
| Gaps | `__corro_bookkeeping_gaps` table tracks missing `[actor_id, start, end]` (`store.go:96-117`) | No equivalent; gaps are DB unavailability |

**Implication:** Forge comparers must not assume Postgres-strong consistency maps onto Uncloud's read path — Uncloud callers must always handle partial replication explicitly, which they do via skips, but Forge's replicated callers never do.

---

### C03 — Overlay Network: Native WireGuard Mesh vs No Mesh (Public Routing)

| | Uncloud | Forge |
|---|---|---|
| Data plane | Each machine gets `/24` subnet within `10.210.0.0/16` (`internal/machine/cluster/ipam.go:13`, `internal/machine/cluster.go:149-154`). Peers configured via `network.PeerConfig` (subnet, management IP, endpoints, public key) and `WireGuardNetwork.Configure` (`internal/machine/cluster.go:681-743`, `internal/machine/network/wireguard.go:12-26`). Keepalive `25s`, MTU clamped `1280–1420` | No overlay; service-to-service uses public hostname/FQDN or AllowedIPs directly (`servicediscovery/registry.go:356-381`) |
| Endpoint rotation | `wgnet.WatchEndpoints()` → `handleEndpointChanges` persists peer endpoint to `state.Network.Peers` and `state.Save()` (`internal/machine/cluster.go:96,339-367`) | No endpoint rotation primitive; resolver cache TTL `30s` (`crossnode/resolver.go:55`) is only freshness mechanism |
| Cross-host container path | Docker bridge `uncloud` (driver `bridge`, IPAM `Subnet: <machine subnet>`) with `trusted_host_interfaces = uncloud`, MTU matched to WG (`internal/machine/docker/controller_linux.go:76-106`), plus iptables allow `wg → br-*` and skip masquerade `--src 10.210.X → wg RETURN` before Docker MASQUERADE (`:129-168`) | No equivalent; containers typically `host` network on beacon or exposed via host port; cross-node goes over WAN |

**Implication:** Uncloud's isolation depends on correct iptables/Docker bridge plumbing on every node. Forge's design has simpler networking but no encrypted overlay and no placement-aware locality.

---

### C04 — Subnet Allocation: In-Memory Deterministic IPAM vs None

Uncloud's `internal/machine/cluster/ipam.go:21-81` (`IPAM`) allocates `DefaultSubnetBits=24` from `10.210.0.0/16` deterministically by scanning from network address, skipping overlaps using an `IPSetBuilder`. On `AddMachine` it builds `allocatedSubnets` from existing rows, constructs `NewIPAMWithAllocated`, then `AllocateSubnetLen` (`internal/machine/cluster/cluster.go:113-162`). Proto `ManagementIP` derived via `network.ManagementIP(publicKey)` if absent (`:145-148`, `pkg/client/connector/wireguard.go:53`).

Forge has no IPAM. Address assignment for allocations is port/IP pool in Postgres (`store_capacity.go:168-191` `FindAvailableAllocation` — `SELECT a WHERE server_id IS NULL AND NOT EXISTS migration_reservation ...`). Not a network-level allocation, only host-port binding.

---

### C05 — Client / API Access: Mesh-Tunneled gRPC vs Direct HTTP/TLS to Central API

Uncloud's user CLI `uc` talks either to a local socket, direct TCP, or via WireGuard tunnel: `WireGuardConnector.Connect` resolves machine host, builds `tunnel.Config { LocalAddress=user.MgmtIP, RemoteNetwork=mgmtIP/128, Endpoint=host:51820 }`, calls `tunnel.Connect`, then `grpc.NewClient(machineAPIAddr, WithContextDialer(tun.DialContext))` (`pkg/client/connector/wireguard.go:37-81`). "Try only first machine" TODO (`:41-43`). Deployment is orchestrated by the client: `RunService` → `InspectClusterState` (broadcast) → `VolumeScheduler` → per-machine `CreateVolume` → `NewDeployment` → plan → execute (`pkg/client/service.go:21-86`).

Forge's panel/API clients call central `forge/api` over HTTP/TLS; beacons are long-lived agents that poll or receive server-sent reconciliation commands via Postgres-backed queue/reconciler (`services/reconciler/*`, `queue/*`). There is no per-node tunneling for operator commands.

**Implication:** Uncloud's client is orchestrator (fat client anti-pattern for Forge's model). Forge must not replicate `RunService`-style broadcast-from-client — it breaks with offline/many nodes.

---

### C06 — Service Discovery: Per-Node Embedded DNS (internal. + locality modes) vs Central Registry + Postgres-Backed Resolution

Uncloud runs `dns.Server` (`internal/machine/dns/server.go:44-54`) listening on the machine management IP on port `53` (`internal/machine/cluster.go:222-229`), fed by `ClusterResolver` which watches `store.SubscribeContainers` and maps `serviceName → []netip.Addr` (`internal/machine/dns/resolver.go:39-48`). Entries filtered: not hook, healthy, has `UncloudNetworkIP` (`:76-88`). Extra lookups: `serviceID`, service-name, and `<machine-id>.m.<service-name>` (`:96-103`). `handleAQuery` supports `nearest.<svc>.internal.` (sort local-subnet IPs first) and `rr.<svc>.internal.` with base shuffle then locality sort (`dns/server.go:294-326`) plus forward to upstreams with `maxConcurrentForwards=1024` and `forwardingTimeout=3s` (`:26-32,259-291`).

Forge's `servicediscovery.Registry` is an in-memory map `endpoints` + `services` keyed `tenant/service` (`registry.go:32-40`) backed by `EndpointStore` Postgres (`registry.go:103-108 SaveEndpoint`, `264-277 LoadFromStore`). Resolution in `crossnode.Resolver.resolveFromDiscovery` tries discovery first, falls back to `ResolutionStore.GetServerNodeID/GetNodeHost` → public hostname/FQDN (`crossnode/resolver.go:135-166`). `ReachabilityVerifier.VerifyEndpoint/Sweep` does per-endpoint `DialContext("tcp", addr:port)` with `timeout 5s` (`servicediscovery/reachability.go:33-55,82-95`) vs Uncloud's pure IP selection.

**Key gap:** Uncloud's resolver TODO: "implement machine membership check using Corrosion Admin client to filter available containers" (`resolver.go:46,63`); similarly `caddyconfig/controller.go:133-136`. Forge's equivalent is `heartbeatmonitor` + `loadbalancer.MarkNodeTargetsUnhealthy/Healthy` (`loadbalancer/service.go:552-598,600-639`) and `TargetStatusHealthy` filtering (`service.go:407-411`).

---

### C07 — Docker Network Management: Declarative Per-Node `EnsureUncloudNetwork` vs Absence of Docker Network Coordination

Uncloud's `EnsureUncloudNetwork` (`internal/machine/docker/controller_linux.go:31-72`) does:
- Inspect `uncloud` bridge; if subnet mismatched → `NetworkRemove` then recreate.
- If MTU mismatched and no containers attached → recreate; if containers attached → **warn and defer** (avoid disrupting services).
- Create `bridge` with IPAM `Subnet`, `trusted_host_interfaces=uncloud`, `mtu` set on `com.docker.network.driver.mtu`.
- Configure iptables `Firewall.DOCKER-USER` and `UNCLoud-INPUT` chains + NAT skip.

Forge's `api` does NOT manage Docker bridge networks. Docker on beacons is managed locally or via `nodeautoscale`/`placement` choosing host; ingress is via external adapters, not Docker bridge.

**Operational risk delta:** Uncloud's deferred MTU change can leave cross-machine throughput degraded until coordinated drain. Forge has no equivalent split.

---

### C08 — Health Checking Semantics: Container-Level Health Filter vs Target/Node-Level Health

Uncloud filters routing/DNS on `Container.Healthy()` (`dns/resolver.go:80`, `caddyconfig/controller.go:142`) — Docker `Healthcheck` status passthrough + debounce `100ms` + periodic `30s` resync (`docker/controller.go:21-24,103-117`). No cross-node active probing for containers; TODO on caddy/dns to check machine membership.

Forge has two layers:
- `crossnode.HealthFilter` — per-host:port failure counter, threshold `3` → `DEGRADED` → `DOWN`, with stale reaper (`health_filter.go:42-56,98-124,203-214,268-279`).
- `servicediscovery.ReachabilityResult` / `VerifyEndpoint` TCP dial (`reachability.go:33-55`).
- `loadbalancer` target `Status` in Postgres + `connCount`/`roundRobin` maps (`loadbalancer/service.go:71-85`).
- `heartbeatmonitor` node-level classification (`heartbeatmonitor/service.go:266-313`).

Uncloud never dials backends; Forge never checks container health natively — layers are disjoint.

---

### C09 — Load Balancing & Algorithm Diversity

Uncloud: DNS shuffling + Caddy `reverse_proxy` upstream load distribution. DNS path does naive shuffle (`dns/server.go:307-309`) and locality sort; Caddy side uses `CaddyfileGenerator.Generate` + fingerprint cache to avoid reload churn (`caddyconfig/controller.go:148-200`, `caddyconfig/jsonconfig.go` for older path). No connection-weighted balancing.

Forge: Explicit algorithm selection per target group (`loadbalancer/service.go:22-29`):
- `round_robin` (`nextRoundRobin` mutex-guarded index)
- `least_connections` (`nextLeastConnections` tracks `connCount[targetID]++` on pick, `ReleaseConnection` decrements)
- `ip_hash` (`fnv32a(clientIP) % len(healthy)`)
- `weighted_round_robin` (cumulative weight scan over `totalWeight`).
- Port range clamp `30000–30100` and optional bindHost (`:104-108`), with TCP/UDP listeners per group (`service.go:308-310 summary not included`).

Uncloud lacks least-connections and weighted semantics entirely; Forge lacks "nearest" locality routing.

---

### C10 — Ingress / Reverse Proxy: Per-Node Caddy Watching Cluster State vs Central `IngressSynchronizer` + `gatewayAdapter`

Uncloud: Each node's `caddyconfig.Controller.Run` subscribes to `store.SubscribeContainers` (`caddyconfig/controller.go:92`), filters healthy (`:98,119`), generates a Caddyfile via `generator.Generate(ctx, containers, caddyAvailable)` (`:163`), checks `client.IsAvailable()` as circuit (**skip regeneration if fingerprint unchanged and Caddy available; but always regenerate if Caddy unavailable to keep disk file current** — `:153-178`), then `client.Load(ctx, caddyfile)` (which validates) before writing disk (`:183-195`). Invalid config is **never written** (`:188-189`).

Forge: `crossnode.IngressSynchronizer` (`ingress_sync.go:23-37`) holds `rules`, `policies`, `tracking` maps, periodically (`Start(..., interval)` ticker, `ticker.C` → `Sync`) does `GroupRulesByRoute` → `UniqueBackends` → `health.FilterHealthy` → drops groups with zero healthy backends entirely (`:125-145`). It expands to `primary + replica rules` (`:144-163`), then calls `adapter.UpdateRoutes(mergedRules, policies)` and records `RouteGenerationRecords` (`:173-181`). `CleanupStale`, `ReloadGateway`, `Health` delegate to adapter (`:230-246`).

Divergences:
- Forge drops *entire route* if no healthy backend (`ingress_sync.go:132-137`), causing 404 vs Uncloud's still-routable DNS (but no endpoints).
- Uncloud's write-if-body-changed skips timestamp line to avoid write churn (`controller.go:226-249`); Forge re-pushes full rule set each tick.

---

### C11 — Scheduling / Placement vs Uncloud Deployment Scheduler

Uncloud's `pkg/client/deploy/scheduler/state.go:29-59` builds `ClusterState{Machines: [{Info, Volumes, ScheduledVolumes}]}` via `ListMachines(Available:true)` + `ListVolumes`, grouped per `MachineID`. `VolumeScheduler` and `ServicePlan` strategy (`deploy/strategy.go`, `scheduler/service.go`, `scheduler/volume.go`, `scheduler/constraint.go`) run **client-side**, single-shot.

Forge's `placement.Engine` (`placement/engine.go:37-77`) is **server-side**, `sync.Mutex`-guarded (`:19,39`), with pluggable `Scorer` (`strategy.go:14-78`):
- `LeastLoadedScorer` — sum of available ratios (mem+cpu+disk)
- `BinPackScorer` — avg utilization `(memUtil+cpuUtil+diskUtil)/3`
- `SpreadScorer` — `1/(1+serverCount)`
- `RandomScorer` — `rand.Float64()` with mutex.
- Hard `ConstraintChecker.FilterByConstraints` first (`engine.go:40,82`), soft bonuses applied after (`:56-57`) with logging.

Capacity source is Postgres `NodeCapacitySnapshot` (`store_capacity.go:48-102`) including:
- `AllocatedCPU = SUM(s.cpu_shares)+SUM(i.cpu)` + similar for memory/disk, plus **pending/active `placement_reservations`** (`:67-70`) and `FOR UPDATE` path for locked snapshot (`:193-248`).

Uncloud has no `FOR UPDATE`, no soft/hard constraint split, and no reservations; Forge has no volume-attach affinity scheduling (volumes modeled as host allocations).

---

### C12 — Lifecycle & Drain/Evacuation vs Container Removal

Forge's `clustermembership.Service` (`clustermembership/service.go:102-292`):
- `Join/Leave` flip `DesiredStateActive` and `Status online/offline`, `Leave` fails if `ListServersForNode` non-empty.
- `StartDrain` sets `Draining=true, DesiredState=draining`, `WithdrawNodeTargets`, tracks `draining[nodeID]=done`, spawns evacuation via `evacuator.CreatePlan → ExecutePlan → completeDrain` (which clears draining, reinstates targets via `TrafficManager.ReinstateNodeTargets`).
- `CancelDrain` closes chan, reinstates.
- `EnableMaintenance` (`:314-338`) and `DisableMaintenance`.

Uncloud's equivalent is `Cluster.RemoveMachine` (`cluster/cluster.go:245-270`): `DeleteContainers(MachineIDs:[id])` best-effort, then `DeleteMachine(id)`; Docker `Controller.Cleanup` (`controller_linux.go:201-263`) does: `ContainerStop + ContainerRemove(RemoveVolumes:true)` on all `label=uncloud.managed`, then `cleanupIptables + NetworkRemove`. Machine removal is immediate, not staged/gated on load migration, and has no traffic withdraw step.

---

### C13 — Distribution vs Broadcast Fanout

Uncloud's `service.go` fanouts via gRPC metadata broadcast:
- `InspectService` (`pkg/client/service.go:93-115`) lists machines, builds `metadata.MD` with `md.Append("machines", m.Machine.Id)` for UP/SUSPECT, then `Docker.ListServiceContainers(listCtx, nameOrID, opts)` which proxies via `grpc-proxy` to fanout.
- `ListServices` (`:322-386`) similarly broadcasts then calls `InspectService` per discovered ID (noted `TODO optimise` `:347`).
- `Remove/Stop/StartService` fanout removal via `sync.WaitGroup` + `errCh` per container across nodes (`:221-286`).
- `ListMachines` + `Subscribe*` via Corrosion HTTP subscribe: `SubscribeContext → Changes() → chan struct{}` (`store.go:270-333`, `container.go:213-288`).

Forge's fanout is indirect: central reconciler or `crossnode` components push to per-node adapters/beacons via server-initiated queue or direct TCP dial for reachability checks (`servicediscovery/reachability.go:42-48`). No broadcast metadata header; instead `Resolver` caches per-server/node host for `IngressSynchronizer` expansion.

---

### C14 — Health-Aware Filtering & Stale Reaping Triad

| Subsystem | Reaper | Interval | Semantics |
|---|---|---|---|
| Uncloud Docker `Controller` | Docker events debounce `100ms` + periodic `SyncInterval 30s` (`docker/controller.go:21-24`) | event-driven | `WatchAndSyncContainers` re-lists and `CreateOrUpdateContainer/DeleteContainers` (`:150-194`) |
| Forge `HealthFilter` | `reaper` ticker `interval` reaps stale `BackendHealth` older than `2*interval` (`health_filter.go:217-257,268-282`) | `5m` default | Deletes `healthState[key]` entries not checked recently |
| Forge `loadbalancer` | `Handle(EventNodeOffline/Recovered)` marks targets (`loadbalancer/service.go:552-599,600-639`) | event-driven | Flips DB + memory status |
| Forge `servicediscovery.ReachabilityVerifier` | `Sweep` verifies all non-draining endpoints via dial (`reachability.go:82-95`) | caller-driven | Writes `results[ source/target/service ]` |
| Forge `heartbeatmonitor` | `Start` jitter `0-5s` + ticker `Interval 30s` (`heartbeatmonitor/service.go:142-158`) | periodic | Classifies + publishes `EventNode{Offline,Unreachable,Suspected,...}` |

Uncloud lacks a node-offline-triggered reaping of unhealthy services; Forge lacks hot-debounce on Docker events.

---

### C15 — Fault Tolerance Under Partition

- **Uncloud:** With mesh partition, both sides continue to accept writes to local Corrosion replica (CRDT LWW). Subnet allocation happening concurrently on both sides can allocate overlapping `/24` (IPAM is in-memory per-`AddMachine` call, no distributed lock — `cluster/cluster.go:150-159` → `ipam.AllocateSubnetLen` checks local snapshot only, and `TODO announce the new machine to achieve consensus. We should perhaps not proceed if this machine is in a minority partition.` comment at `:174-175` is explicit acknowledgment). Re-merge reconciles via CRDT merge, but overlapping subnets cause persistent networking conflict requiring manual remediation.
- **Forge:** Partition between partition-A containing API+DB and partition-B containing nodes → nodes in B become `OFFLINE` after `UnavailableAfter` (`heartbeatmonitor/service.go:283-287`), targets marked `unhealthy` (`loadbalancer/service.go:563-597`), ingress strips those backends while partition persists. No overlapping allocation because placement is centrally serialized. API down means no new placements, but prior state is authoritative, not divergent.

---

### C16 — Security Model: WireGuard Identity vs API Token / mTLS

Uncloud: WireGuard public key is the machine identity; `ManagementIP = f(publicKey)` (`wireguard.go:53` + `internal/machine/network/config.go` style deterministic `/128` from key prefix). API served only on management IP over WG (`cluster.go:179: apiAddr = managementIP:MachineAPIPort`). Adding a machine requires its public key + `Endpoints` out-of-band via `Token` (`internal/ucind/cluster.go:167-199` → `AddMachineRequest{ Network: {Endpoints, PublicKey} }`). No TLS: gRPC over WG uses `insecure.NewCredentials()` (`connector/wireguard.go:70`).

Forge: API auth via `auth` + `apiKeys`, beacon mTLS via `mtls_migrator.go` and `store/secrets.go`, nodes authenticate heartbeats via `node_id` + token. Ingress/gateway adapter may have TLS termination but cross-node node-to-node carries gateway encryption or none depending on deployment; no per-node deterministic IP identity.

---

## 3. Logic Findings (4)

### LF-01 — Single-Writer Broadcast Assumption vs Multi-Writer CRDT: Optimistic Exists Check Is Racy (HIGH)

**Location:** `reference/app-platforms/uncloud/pkg/client/service.go:28-37` (optimistic `InspectService` check), `pkg/client/deploy/deploy.go:341-348` (same in `Validate`).

```go
// service.go:29-37
if spec.Name != "" {
    _, err := cli.InspectService(ctx, spec.Name)
    if err == nil {
        return resp, fmt.Errorf("service with name '%s' already exists", spec.Name)
    }
    if !errors.Is(err, api.ErrNotFound) {
        return resp, fmt.Errorf("inspect service: %w", err)
    }
}
```

```go
// service.go:152-167 comment
// Containers from different services may share the same service name (distributed and eventually consistent store
// may not prevent this), or a service name might match another service's ID.
if foundByID { ... } else {
    serviceID := containers[0].Container.ServiceID()
    for _, mc := range containers[1:] {
        if mc.Container.ServiceID() != serviceID {
            return svc, fmt.Errorf("multiple services found with name '%s', use the service ID instead", nameOrID)
        }
    }
}
```

**Issue:** Comment on `:152-153` admits the store's eventual consistency cannot prevent duplicate names — the optimistic check is advisory. Two clients can pass the check concurrently and both create containers with the same `LabelServiceName` but distinct `ServiceID`s. Subsequent `InspectService(name)` then errors ("multiple services found"). This is the canonical CRDT write-write conflict with no conflict-resolution hook. Forge avoids this by making service name uniqueness a DB unique constraint guarded by transaction / advisory lock — `Validate` failing open here is a product-visible semantic difference.

**Porting risk if copied to Forge:** Must not weaken Forge's invariant by adopting "name can duplicate, filter by ID priority". Forge operators expect name uniqueness enforced; copying Uncloud's permissive path would break UI/DNS assumptions (DNS uses name as key).

---

### LF-02 — Membership-Blind Data Plane: Unfiltered Orphan/Down-Node Containers Used for Routing/DNS (MEDIUM-HIGH)

**Location:** `reference/app-platforms/uncloud/internal/machine/dns/resolver.go:46-48,63-64`, `reference/app-platforms/uncloud/internal/machine/caddyconfig/controller.go:133-147`, `reference/app-platforms/uncloud/internal/machine/store/container.go:108-111` & `213-218`.

Both `ClusterResolver.Run` and `CaddyConfig.Controller` mark themselves:
```go
// resolver.go:46-48,63
// TODO: implement machine membership check using Corrossion Admin client to filter available containers.
// TODO: left similar for filterHealthyContainers
```

```go
// container.go:108-111 / 213-218 — ListContainers & SubscribeContainers
q := sq.Select(...).From("containers c").Join("machines m ON m.id = c.machine_id") // orphan-only filter
    .Where(sq.Eq{"c.sync_status": SyncStatusSynced})
```

The SQL join excludes **orphans** (containers whose machine row was deleted) but does NOT exclude containers whose machine is `DOWN`/`SUSPECT`. `ClusterController.handleMachineChanges` reconfigures WireGuard peers on membership changes (`cluster.go:243-249`) but `dns.Server` and `caddyconfig.Controller` remain membership-blind, so down-node IPs can remain in DNS A's and Caddy upstreams until their container rows are pruned (only on `RemoveMachine` → `DeleteContainers`). This can serve stale IPs for up to full partition duration.

Forge explicitly couples heartbeat state to routing: `heartbeatmonitor → EventNodeOffline/Recovered → loadbalancer.MarkNodeTargetsUnhealthy/Healthy` (`loadbalancer/service.go:552-639`) and `crossnode.HealthFilter.FilterHealthy` drops unhealthy backends in `IngressSynchronizer.Sync` (`ingress_sync.go:130-137`). Forge must not regress to "orphan filter only" if borrowing Uncloud's `Join`-based query pattern.

---

### LF-03 — Uncloud's "No Healthy Backend = Drop Route" Is Donor-Dangerous; Forge Must Not Adopt Blindly (MEDIUM)

**Location:** `forge/api/internal/services/crossnode/ingress_sync.go:129-143` vs `reference/app-platforms/uncloud/internal/machine/caddyconfig/controller.go:142-147`, `reference/app-platforms/uncloud/internal/machine/dns/server.go:295-310`.

Uncloud generates Caddy upstreams with **all healthy containers** as backends; if `healthy == 0` the `CaddyfileGenerator` emits Caddy with zero upstreams (site still responds, likely 502). Forge's current `IngressSynchronizer.Sync` *omits the entire route* when `len(healthyBackends)==0`:

```go
healthyBackends := is.health.FilterHealthy(backends)
if len(healthyBackends) == 0 {
    slog.Warn("no healthy backends for route; route omitted", ...)
    continue // route disappears — callers get 404 rather than 502
}
```

Adopting Uncloud's "serve zero-upstream" semantics would change Forge's failure visibility (monitoring would miss route disappearance vs explicit 502). Conversely, applying Forge's filter-healthy-then-drop policy to Uncloud would cause intermittent 404s under membership flaps where health checks lag, which Uncloud tolerates by serving the stale pool. Neither policy is universally correct; Forge must keep the intentional 404-vs-502 distinction explicit and not silently mirror Uncloud.

---

### LF-04 — Volume `/24` Exhaustion + In-Memory-Only IPAM Missing Durability & Rollback (MEDIUM)

**Location:** `reference/app-platforms/uncloud/internal/machine/cluster/ipam.go:21-81`, `reference/app-platforms/uncloud/internal/machine/cluster/cluster.go:148-162`.

```go
// ipam.go:44-65 — AllocateSubnetLen scans deterministically from network base
subnet := netip.PrefixFrom(ipam.network.Addr(), bits)
for ipam.network.Contains(subnet.Addr()) {
    ipset, _ := ipam.allocated.IPSet()
    if !ipset.OverlapsPrefix(subnet) {
        ipam.allocated.AddPrefix(subnet); return subnet,nil
    }
    subnet = netip.PrefixFrom(netipx.PrefixLastIP(subnet).Next(), bits)
}
return "", errors.New("no available subnet")
```

```go
// cluster.go:149-162 — IPAM is throwaway in-memory; no persisted allocation map
allocatedSubnets := make([]netip.Prefix, len(machines))
for i, m := range machines { allocatedSubnets[i], _ = m.Network.Subnet.ToPrefix() }
ipam, _ := NewIPAMWithAllocated(clusterNetwork, allocatedSubnets)
subnet, err := ipam.AllocateSubnetLen(DefaultSubnetBits)
```

Subnet state is not durably tracked — it is reconstructed from `machines` table rows each `AddMachine`. If a node fails between IPAM allocation and successful `CreateMachine` insertion, the subnet is "lost" until manual GC; if Corrosion replicates slowly, a concurrent adder sees a stale `ListMachines` and can double-assign the same `/24` (see LF-01 partition comment). With `DefaultNetwork = 10.210.0.0/16` and `/24` per host, cap is 256 machines before `"no available subnet"` hard fail — no policy for heterogeneous prefix lengths or reclaim of departed hosts beyond physical row deletion. Forge's allocation exhaustion is similar but expressed as Postgres `no available allocation` (`store_capacity.go:181-184`) with explicit reservation tracking; copying Uncloud's throwaway IPAM into Forge would lose reservation accounting and make capacity appear higher than reality.

---

## 4. Forge → Uncloud Feature Matrix (Adoption Risks)

| Feature | Uncloud has | Forge has | Borrow risk |
|---|---|---|---|
| WireGuard mesh + encrypted overlay | ✅ | ❌ | High ops cost; requires per-node key management Forge currently externalizes |
| Per-node embedded DNS `*.internal.` with `nearest/rr` locality | ✅ | ❌ (external DNS via `services/dns`) | Low — could add as optional, but need membership filter first |
| Client-side broadcast orchestration | ✅ (fat client) | ❌ (server-driven reconciler) | Do not adopt — breaks SBA/Failover central audit trail |
| CRDT multi-writer store with gap tracking | ✅ | ❌ (Postgres) | Do not adopt for Forge control plane; useful only for disconnected edge beacons |
| Per-node Caddy ingress watching container stream | ✅ | Partial (central `IngressSynchronizer` pushes to adapter) | Medium — converging would need consensus on "who owns Caddyfile" |
| Docker bridge lifecycle (`EnsureUncloudNetwork`) | ✅ | ❌ | Not applicable — Forge not opinionated about host networking |
| Membership-aware reaping (event-driven health flip) | ❌ (TODO) | ✅ | Should *add* to Uncloud-inspired modules, not strip from Forge |
| Multi-algorithm LB + least-connections tracking | ❌ | ✅ | Don't downgrade |
| Placement reservations (`FOR UPDATE` + pending/active) | ❌ | ✅ | Don't downgrade |
| Reservation-aware capacity snapshot | ❌ | ✅ (`available = total - (allocated+reserved)`, `:98-101`) | — |
| Drain gated on workloads + traffic withdraw | ❌ | ✅ | If mirroring Uncloud's instant `RemoveMachine`, would regress UX |

---

### C17 — Requested But Absent Modules: `forge/web/app/admin/cloud`, `store cluster_membership`, `beacon/internal/remote`

- `forge/web/app/admin/cloud/page.tsx:1-165` is a TanStack Query admin panel for **cloud instance provisioning** (providers/instances/links), not a networking/clustering component. It calls `/admin/cloud/providers`, `/admin/cloud/instances?provider=`, `/admin/cloud/links` (`page.tsx:46-62`), shows `cloud_node_links` via `store_cloud.go:17-58` (`CreateCloudNodeLink`/`ListCloudNodeLinks` backed by `cloud_node_links` table). It has no WireGuard, DNS, or load-balancer logic — comparing it to Uncloud's mesh is a category error, but it is the only file at the requested path.
- `forge/api internal store cluster_membership` does not exist as a file (`find` returned no `store_cluster_membership.go`). Cluster membership persistence is fully in generic `store` (`store/store_capacity.go`, `store/store_cloud.go`) plus metrics/lifecycle via `services/clustermembership/service.go:25-29` (`membershipStore` interface `GetNode/UpdateNode/ListServersForNode`) and `services/heartbeatmonitor/service.go:50-55` (`SetNodeHeartbeatClassification`). There is no separate cluster_membership store to diff against Uncloud's `internal/machine/store/schema.sql:10-21` machines table.
- `beacon/internal/remote` does not exist in the workspace (`forge/beacon: No such file or directory`). Beacon-related files are under `infra/compose.beacon.yml`, `infra/bootstrap-beacon.sh`, `infra/ship/kubernetes/beacon-daemonset.yaml` and `forge/api/internal/placement` / `forge/api/internal/daemon` for server-side placement. Forge's remote execution is not at the requested path — comparably, Uncloud's remote execution is per-machine gRPC `ListServiceContainers` fanout (`pkg/client/service.go:99-113`).
- Practical impact: Any audit directive to compare `beacon/internal/remote` to Uncloud's `container.go`/`docker/service.go` must be redirected to `forge/api/internal/services/nodeprobe`, `daemon`, or `placement` — otherwise the comparison is against a non-existent module.

---

## 5. Recommended Attach Points if Learning From Uncloud (Without Regressing)

1. **Add membership filter to any DNS/ingress module before borrowing query patterns.** Any reuse of `Join(machines) → ListContainers` (`container.go:108-111`) must also check `Corrosion.MembershipState` / `heartbeatmonitor` state, otherwise orphan filtering alone reintroduces LF-02.
2. **Keep optimistic existence checks pessimistic in Forge.** Do not replace Forge's transactional uniqueness with Uncloud's advisory check (`pkg/client/service.go:28-37`). Consider server-side idempotency key instead.
3. **Prefer central placement reservations to in-memory IPAM** (`placement/engine.go` + `store_capacity.go:67-70,193-248`). Uncloud's IPAM determinism is nice for edge but unsafe under partitions without a distributed mutex; Forge's DB lock is the right primitive for its topology.
4. **If adding overlay, keep Forge's `HealthFilter` threshold-reaper semantics** (`health_filter.go:42-56,268-282`) rather than Uncloud's pure `Healthy()` flag, to avoid flap-amplification under Serf SUSPECT.

---

*Generated for Phase 6 audit — file:line citations point to inspected snapshot; no product code modified.*
