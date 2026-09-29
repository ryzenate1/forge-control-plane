# Phase 5 — Subagent 4: STORAGE ABSTRACTION + CLUSTER/FLEET MANAGEMENT
## Longhorn (volumes/replicas/rebuilds/snapshots/backups) + Rancher (fleet/clusters/agents) vs Forge

Reference locks (reference/REFERENCE_LOCK.txt):
- `reference/large-systems/longhorn` @ d5d522b70cbd134e35e424fea9812d1c117fd8cf (chart templates + enhancement proposals)
- `reference/large-systems/rancher` @ 76c28ec6e31441bf31528e20b297fc543d71a6d6 (management-plane source)

Forge trees inspected: `forge/api/internal/{store,placement,services/*}`, `forge/api/migrations`, `beacon/internal/server`.
No product code was modified.

---

## 0. Executive verdict

**Should Forge chase block-storage replication? No.** Longhorn's replication stack (engine process per volume,
replica processes inside instance-managers, TCP/SPDK data plane, snapshot chains, salvage) exists because
Kubernetes pods are ephemeral and need a network block device that follows them. Forge workloads are
node-pinned containers over node-local directories (`daemon_base` default `/var/lib/forge/volumes`,
forge/api/migrations/012_wings_node_parity.sql:12) plus a real backup pipeline (local/S3,
forge/api/internal/services/backup/storage.go:44). Synchronous block replication would add a distributed data
plane Forge cannot operate, to solve a problem its backup+evacuate model already bounds.

What Forge should take from Longhorn/Rancher is control-plane discipline:

1. **Locality-aware placement at schedule time** - Longhorn filters disks/nodes by tags when placing replicas
   (`diskSelector`/`nodeSelector` on the Volume spec, longhorn/chart/templates/crds.yaml:4592,4636). Forge checks
   mount eligibility only *after* placement (Finding F3).
2. **Replica-health semantics** - `failedAt`/`healthyAt` timestamps and replenishment-wait before destroying a
   possibly-recoverable replica (crds.yaml:3048-3096; enhancements/20200821-rebuild-replica-with-existing-data.md).
   Forge replaces "failed" instances with no grace window (services/replicamanager/service.go:883-910).
3. **Active connectivity proof** - Rancher pings *through* the agent tunnel every 15s instead of trusting
   last-report time (rancher/pkg/controllers/management/clusterconnected/clusterconnected.go:35-41,64-88).
4. **One vocabulary for storage locality** - Forge currently has three (Finding F1).

---

## 1. Side-by-side architecture

| | Longhorn | Rancher | Forge |
|---|---|---|---|
| Storage unit | `Volume` CRD (spec+status+robustness+Schedulable condition), crds.yaml:4424+ | n/a (CSI consumer) | `mounts` = static path map (migrations/015_a_mounts.sql:1-11); `volumes` = orphaned stub (104_a_backup_system.sql:16-20); Beacon exposes node-local docker volume CRUD (beacon/internal/server/server.go:481-488) |
| Redundancy unit | `Replica` CRD, one per data copy per disk/node, crds.yaml:2962+ | n/a | replicamanager instances = stateless compute copies only |
| Failure detection | engine monitors -> robustness degraded; node NotReady | tunnel ping every 15s -> `Connected` condition | pushed heartbeats -> 6-state machine (services/heartbeatmonitor/service.go:266-313) |
| Recovery | rebuild replica from surviving replicas/snapshots | reprovision cluster via CAPR plans | replace instance elsewhere / evacuate via backups |
| Fleet delivery | n/a | fleet Cluster objects per workspace; agent reports bundle rollout state (pkg/controllers/provisioningv2/fleetcluster/fleetcluster.go:63-104) | none at group level; per-server `config_sync_pending/error` flags |

---

## 2. Comparisons (15)

### C1. Volume as declarative object vs path-string mounts
Longhorn `Volume` carries desired state (`numberOfReplicas` crds.yaml:4640, `dataLocality`:4582,
`staleReplicaTimeout`:4732), observed state (`currentNodeID`:4806) and a `Schedulable` condition printed at
crds.yaml:4449-4452. A Forge `mounts` row is name + source/target strings + booleans
(migrations/015_a_mounts.sql:1-11): a bind-mount mapping, not a managed entity - no size, no state machine, no
health, no lifecycle owner. The `volumes` table that could have been that entity is an unused FK stub (Finding F2).

### C2. Data replicas vs compute replicas
Longhorn's Replica CRD is a data copy pinned to `nodeID`+`diskID`, with `failedAt` ("likely to have useful though
possibly stale data"), `healthyAt`, `hardNodeAffinity`, `evictionRequested`, `rebuildRetryCount`
(crds.yaml:3046-3096). Forge `replicamanager.Instance` is a compute slot (`Idx`, status, reservation) created from
app CPU/Mem/Disk (services/replicamanager/service.go:195-258). Identical names, disjoint semantics:
N Longhorn replicas = N durable copies; N Forge replicas = N interchangeable workers sharing nothing.
Assumed parity gives false durability confidence (Finding F4).

### C3. Rebuild-with-existing-data vs instant replace
Longhorn delays failed-replica replacement (`ReplicaReplenishmentWaitInterval`) so a temporarily unreachable
replica can be reused instead of fully resynced, with retry counts and backoff before giving up
(enhancements/20200821-rebuild-replica-with-existing-data.md:22-53). This avoids massive resyncs after node
reboots/network flaps. Forge's reconcile marks any instance failing Beacon verification `"failed"` and immediately
retries placement elsewhere (replicamanager/service.go:753-783, 883-910) - fine for stateless pods, but there is no
"the node may come back" wait state and no cost model distinguishing restart from resync.

### C4. Data locality modes vs dead scoring
Longhorn models locality as a volume property: `disabled` / `best-effort` / `strict-local`
(enhancements/20200819-keep-a-local-replica-to-engine.md:29-40; 20221123-local-volume.md:20-58), enforced by the
scheduler co-locating engine and replica. Forge has the concept fields (`PlacementRequest.StorageLocality`,
forge/api/internal/domain/domain.go:123; `Candidate.StorageLocality`, forge/api/internal/placement/strategy.go:42)
but the placement-side usage is dead code (Finding F1). The living implementation is the evacuation/failover
classifier (services/evacuationplanner/service.go:691-728), which infers locality by sniffing mount source strings.

### C5. Locality/tags inside scheduling vs checked after placement
Longhorn places replicas onto disks matching tag selectors during scheduling, alongside anti-affinity and reserved
capacity (crds.yaml:4592-4646; enhancements/20251001-replica-balance-scheduling.md:14-24 lists the filter chain).
Forge's scheduler filters on state/region/capacity/runtime only (services/scheduler/service.go:183-222); mounts
appear nowhere in `filterNodes` or any scorer. Eligibility surfaces later, at assignment time:
`ensureMountAvailableForServer` joins `mount_node` x `egg_mount` x servers (store/store_mounts_ext.go:297-314) and
fails with "mount is not available for this server node and egg". So Forge can place a server on node A and only
then discover its required mount lives on node B. Longhorn makes this a schedule-time rejection; Forge turns it
into a post-hoc operator error.

### C6. Replica anti-affinity and rebalancing vs spread penalty only
Longhorn forbids two copies of one volume on the same node/zone (soft/hard anti-affinity settings), supports
`replicaAutoBalance` to migrate replicas back to even distribution when nodes return
(20210510-automatic-rebalance-replica.md:12-40), and now simulates candidate placements to pick balance-maximizing
disks (20251001-replica-balance-scheduling.md:17-27). Forge's only anti-concentration mechanism is a linear spread
penalty `-0.1 x existing instances` during replica placement (placement/replica.go:170-175) plus a generic
SpreadScorer (placement/strategy.go:137-147). Nothing rebalances when nodes come back; evacuation is manual.

### C7. Snapshot chain vs backup artifacts
Longhorn snapshots are crash-consistent block deltas; each is mirrored as a `snapshots.longhorn.io` CR reconciled
against engine state, removed via finalizer only after deletion from the engine, retained/pruned by recurring jobs
(crds.yaml:3767; enhancements/20220420-longhorn-snapshot-crd.md:19-31). Forge has no snapshot concept on live data:
closest is `createPreRestoreSnapshot`, which takes a full backup artifact before restoring
(services/backup/restore.go:733-799). Cost model difference: O(delta) vs O(full copy).

### C8. Backupstore target vs panel S3 settings + local dir
Longhorn backs up incremental blocks to an off-cluster target with one `backups.longhorn.io` CR per cloud object
and per-node concurrency limits (crds.yaml:625; values.yaml:336-338; enhancements/20240801-delete-backup-in-the-
backupstore-asynchronously.md). Forge runs two parallel stacks: Pterodactyl-style `backups`
(migrations/019_backups.sql:1-15; node-local files + checksum) and the newer config/job/artifact system
(104_a_backup_system.sql:29+) whose providers are a local directory on the API host (backup/storage.go:44+) or S3 -
with panel-global credentials (048_s3_backup_config.sql:5-12). Neither records which node holds a local artifact in
a form restore planning consumes; recovery items carry only HasBackup/BackupName
(services/recovery/recovery_ops.go:27-34).

### C9. Attach/detach discipline vs drain-time protect policy
An RWO Longhorn volume attaches to exactly one node (`currentNodeID`); detach-on-down-node and force-deleting
terminating pods are explicit settings (`NodeDownPodDeletionPolicy`: do-nothing/delete-statefulset/delete-deployment,
enhancements/20200817-improve-node-failure-handling.md:24-28,52-62). Forge's equivalent guardrail is in the
evacuation planner: servers classified `StorageLocalOnly` get `ReplacementPolicyProtect`; shared/replicated get
`AutoReplace` (evacuationplanner/service.go:720-728), surfaced via orphan detection (:730-765) and consumed by
failover (services/failover/service.go:43-56) and main wiring (cmd/api/main.go:932-936). Same idea, weaker evidence
base (Finding F5).

### C10. Robustness reporting vs count-based degraded status
Longhorn reports volume robustness derived from actual replica modes. Forge marks apps `degraded` purely from
running-count vs desired (replicamanager/service.go:792-806) - a count heuristic, not an integrity signal.
Borrowable: per-instance failedAt/lastHealthyAt so operators can tell "down 5s" from "down 5h". Forge already does
exactly this for nodes (heartbeatmonitor Evaluation carries LastSeenAt/age/recovery counters,
heartbeatmonitor/service.go:213-225); the pattern just stops at the node boundary.

### C11. Agent connectivity: active round-trip proof vs passive staleness
Rancher's checker holds a remotedialer session per cluster and every 15s issues an HTTP GET *through the tunnel*;
success flips `Connected=true`, failure sets `Ready=False` reason "Disconnected"
(pkg/controllers/management/clusterconnected/clusterconnected.go:35-41,64-88,129-135). Forge trusts beacon-pushed
heartbeats and classifies age into healthy/suspected/unreachable/offline/recovering/reconciling with hysteresis
thresholds and consecutive-success counters (heartbeatmonitor/service.go:266-313). Verdict: Forge's state machine
is richer than Rancher's boolean condition, but Rancher's proof is stronger - a control-plane-initiated round trip
catches a node whose heartbeat goroutine still fires while the node is otherwise hung. Cheap borrow: periodically
request an echo over the existing beacon channel instead of only aging last-seen timestamps.

### C12. Health sync direction (push/pull hybrid)
Rancher runs a health syncer on the downstream side that pushes component/node conditions upstream every 15s
(pkg/controllers/managementuser/healthsyncer/healthsyncer.go:31,63-71), complementing the management-side
connectivity poll - push plus pull. Forge is push-only (beacon heartbeat POST) with central evaluation over a
history table feeding consecutive-success counting (heartbeatmonitor/service.go:206-212,267). Forge's
history-driven hysteresis is something Rancher lacks; Rancher's independent verification is something Forge lacks.
Complementary designs, not competing ones.

### C13. Fleet clusters/workspaces vs regions + cluster_group_id
Rancher mirrors every provisioning cluster into a fleet `Cluster` object, assigns it to a workspace namespace, and
fleet agents report bundle rollout state back (fleetcluster/fleetcluster.go:63-104; fleetworkspace/controller.go
creates namespace + RBAC per workspace). That yields per-tenant RBAC'd grouping and gitops delivery targets.
Forge's grouping primitives are regions (020_a_regions_multi_node_foundation.sql) and `nodes.cluster_group_id TEXT`
(122_node_expansion.sql:35), consumed only by autoscale policies (192_node_autoscale_policies.sql:2-7). There is no
bundle/desired-state distribution across groups; Forge's nearest analog is per-server `config_sync_pending` /
`config_sync_error` flags (store_mounts_ext.go:288) plus the beacon command log. If Forge ever needs "apply this
template set to every node in group X", copy fleet's shape (group -> manifest -> per-node apply status), not its code.

### C14. Upgrades
Rancher upgrades Kubernetes via CAPR plan objects reconciled against desired channel versions, and ships managed
charts; Longhorn gates its own engine upgrades via per-node concurrency settings
(20230420-upgrade-checker-info-collection.md:131 lists concurrent-automatic-engine-upgrade-per-node-limit).
Forge's upgrade service is a version-file checker with plan/execute steps for api/web components
(services/upgrade:120-201,263+). Nobody here coordinates upgrades with storage safety except Longhorn itself.
Actionable for Forge: gate node drains/upgrades on "no server on this node lacks a verified backup" - the planner
already computes those inputs; nothing enforces them pre-upgrade.

### C15. Orphan cleanup semantics
Longhorn tracks leaked engine/replica runtime data as `orphans.longhorn.io` CRs with auto-deletion settings
(crds.yaml:2706; enhancements/20220324-orphaned-data-cleanup.md; 20250331-orphaned-runtime-cleanup.md) - each orphan
records why it exists and requires reconciliation before deletion. Forge detects orphaned workloads on offline nodes
(DetectOrphans, evacuationplanner/service.go:730-765) and can RecordOrphanAndHardDeleteServer
(clustermanager/service.go:35), plus Beacon-side docker volume prune endpoints (beacon/internal/server/server.go:483).
Gap: Forge prune is an unscoped manual admin action with no ownership linkage back to control-plane records.

---

## 3. Logic findings (6)

### F1. StorageLocality is scored but can never fire - and three vocabularies disagree
- No production caller populates `domain.PlacementRequest.StorageLocality`. The server-create handler builds the
  request field-by-field and omits it (http/handlers_servers.go:860-874); the only JSON bindings are the debug
  endpoints `/placement/explain` and `/placement/enrich` (http/phase6_registrar.go:73-98). Therefore the
  +/-1e10/+1e8 locality scoring at services/scheduler/service.go:317-323 and the hard filter at :212 are dead code.
- Worse, if wired up later it still cannot match: candidates are labeled `"local"` or `"shared"`
  (nodeToCandidate, scheduler/service.go:714-717) while requests would carry the evacuator enum
  `local_only`/`replicated`/`shared` (evacuationplanner/service.go:28-33; failover compares these strings,
  failover/service.go:45-51). A request of `local_only` never equals candidate `local`, so every node takes the
  permanent -1e10 penalty. Three spellings of one concept across three packages.
- envaffinity's explain path builds candidates without StorageLocality entirely (envaffinity/explain.go:104-118),
  so even explanations understate locality effects.
- Longhorn contrast: one enum (`disabled/best-effort/strict-local`) defined once on the volume spec and enforced by
  the same controller that schedules (20221123-local-volume.md:48-58).

### F2. The volumes table is an orphaned FK anchor; two unrelated "volume" concepts collide
- `CREATE TABLE volumes` is explicitly a stub "(for foreign key references)" holding id/name/created_at
  (migrations/104_a_backup_system.sql:4-20). Nothing in the Go tree inserts into it (`INSERT INTO volumes` has zero
  matches); five backup tables FK `volume_id` into it (:36,:99,:175,:243,:358) and CHECK constraints advertise
  `'volume'` backup types (:39,:95,:157,:239) that cannot reference any real entity.
- Meanwhile the volume identifiers actually written at runtime are free-form docker volume names like
  `mgp-db-<id12>` (services/dbprovisioner/containers.go:216,283), stored in columns declared
  `volume_id TEXT NOT NULL DEFAULT ''` (098_app_platform_foundations.sql:60, 114_database_service_plugins.sql:29,
  116_managed_databases.sql:17) - not UUIDs from this table. So "volume backup" configs point at a table with zero
  rows while real volume names live in unrelated columns. Either drop the stub (and the volume branches of the
  CHECK constraints) or make db/dbcontainer volumes actual rows in it.

### F3. Mount eligibility is enforced after placement, not during scheduling
The scheduler filter chain (state, region, capacity, runtime - scheduler/service.go:183-221) and all scorers never
consult mounts; `AllowedMountSourcesForNode` exists (store_mounts_ext.go:367-388) but nothing in provisioning or
placement calls it. Eligibility surfaces only when assigning a mount to an already-placed server:
ensureMountAvailableForServer joins mount_node x egg_mount x servers (store_mounts_ext.go:297-314). Placement thus
ignores a hard storage constraint that is knowable upfront - exactly the error class Longhorn prevents by making
disk/node tags part of replica scheduling (crds.yaml:4592,4636).

### F4. Replacement is not rebuild: replicamanager can silently drop stateful data
ReplaceInstance re-dispatches a start command for the same app spec on a new node
(replicamanager/service.go:561-652); instances carry no data-path identity, and RetryFailedPlacements sweeps all
failed instances every minute (:883-910). This is safe only under the implicit assumption that everything placed
through replicamanager is stateless. The sole protection for anything stateful is the evacuator's string heuristic
isNetworkStorage (source containing `://` or `@`+`:` counts as shared; otherwise local_only -> Protect;
evacuationplanner/service.go:691-718) - path sniffing as a proxy for "is this data replicated", where Longhorn would
answer authoritatively from replica objects. If a stateful service type is ever routed through CreateApp/deployReplicas,
replacement destroys data with no guard.

### F5. Default storage classification overstates durability
StorageLocality() returns `StorageReplicated` whenever a server has no writable extra mounts
(evacuationplanner/service.go:707) - i.e., the default classification for an ordinary game server whose entire data
directory lives on one node is "replicated". ReplacementPolicyForServer then maps replicated to AutoReplace (:720-728),
so orphan/failover automation treats unreplicated local data as safely movable; the only thing downgrading the action
to Notify is the hasVerifiedBackup input in failover/service.go:43-56. Longhorn never infers redundancy - robustness
comes from observed replica objects. Fix direction: default to local_only unless evidence (mount source is network,
or a verified off-node backup artifact exists within RPO) says otherwise.

### F6. Two coexisting backup stacks with divergent locality assumptions
The classic `backups` table (019_backups.sql) stores checksummed archives next to the server's node; the newer
backup_configurations/jobs/artifacts stack (104_a_backup_system.sql) writes to the API host's local disk by default
(backup/storage.go:44+) with optional panel-global S3 (048_s3_backup_config.sql). Neither cross-references the other,
and restore paths differ accordingly; recovery_ops plans restores using HasBackup booleans without knowing which
node holds which artifact (recovery_ops.go:27-34). Longhorn's single backupstore abstraction (one target URI, CRs
per cloud object) is the simpler shape to converge on: artifacts should record storage location + reachability so
restore planning is locality-aware.

---

## 4. Recommendations (prioritized, control-plane only)

1. **Do not build block replication.** Keep durability = backups + evacuation. Longhorn's engine/replica data plane
   is a product-scale commitment; Forge's failure model (node loss -> restore from verified artifact) is coherent
   without it.
2. **Unify the locality enum** into one shared type (`local_only | replicated | shared`), defined once, used by
   placement candidates, requests, evacuator, and failover. Kill the `"local"` spelling in scheduler/service.go:714.
3. **Make placement locality/mount-aware at schedule time**: join `mount_node` availability into candidate
   construction and reject or heavily penalize nodes that cannot serve a requested mount (Longhorn: tag selectors
   at scheduling). This converts Finding F3 from runtime error to schedule-time rejection.
4. **Wire StorageLocality end-to-end or delete it** (Finding F1): either populate it on server-create from template
   metadata (stateful templates => local_only) or remove the dead scoring to stop implying enforcement.
5. **Borrow replica-health semantics for instances**: add failed_at / last_healthy_at per instance and an optional
   replenishment-wait before replacement (mirrors ReplicaReplenishmentWaitInterval) so transient node blips do not
   trigger mass re-placement.
6. **Resolve or delete the volumes stub table** (Finding F2); make db/dbcontainer volume names reference real
   entities if volume-scoped backups are ever advertised.
7. **Add active connectivity proof**: periodic API-initiated echo over the beacon channel alongside passive
   heartbeats (Rancher C11), keeping Forge's richer state machine.
8. **Record artifact locality in backups**: which node/provider holds each artifact + reachability check, so
   recovery plans are restore-path aware (C8/F6).
9. **If group-level config delivery is ever needed**, adopt fleet's shape: cluster_group -> manifest bundle ->
   per-node apply status object with RBAC scoping per group (C13), rather than ad-hoc flags.

## 5. Coverage checklist

- [x] longhorn controller semantics (volume/replica CRDs, scheduling conditions) - crds.yaml citations
- [x] replica rebuilding - 20200821 enhancement + FailedAt/HealthyAt/RebuildRetryCount fields
- [x] snapshot/backup CRDs - snapshots.longhorn.io:3767, backups.longhorn.io:625, recurringjobs:2825
- [x] attach/detach + node-failure policy - 20200817 enhancement, currentNodeID
- [x] topology/locality - dataLocality modes, diskSelector/nodeSelector, auto-balance docs
- [x] rancher fleet controllers - fleetcluster.go, fleetworkspace controller
- [x] rancher cluster agent heartbeat - clusterconnected checker (15s tunnel ping), healthsyncer (15s push)
- [x] forge store_mounts*.go - store_mounts_ext.go full read
- [x] forge migrations mounts/volumes - 015_a_mounts.sql, 104_a_backup_system.sql (+019, 048)
- [x] forge services/replicamanager - service.go full read; storage locality: absent by design (compute-only)
- [x] forge placement StorageLocality field - strategy.go/engine.go/replica.go traced; dead code confirmed
- [x] forge services/clustermembership vs Rancher fleet - clustermembership (drain/join/leave) vs fleet (gitops
      delivery): different problems sharing one word "cluster"; comparison C13 covers the overlap

