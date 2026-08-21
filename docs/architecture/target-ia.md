# Target Information Architecture — Forge Control Plane

**Status:** PLANNED / MODEL phase — awaiting Phase 3 FOUNDATION implementation
**Source:** MASTER brief §§1-4, 6-16, inventory 147 page.tsx + 77 registry hrefs
**Current IA:** `forge/web/components/admin/admin-registry.ts:24` 8 groups 77 visible +2 aliases, `admin-shell.tsx:29` SUB_GROUPS 4-group nesting

## 1. Canonical Product Model

```
Forge
 ├─ Build
 │   └─ Workloads → Application / Game Server / Service / Database / Compose Stack
 │       └─ Catalog (universal marketplace)
 ├─ Run — Deploy / Pipelines / Compose / Git, placement (constraints, env affinity, predictive scoring)
 ├─ Operate — Backups / Migrations / Recovery / Reconciliation / Cleanup / Orphan remediation / Scheduled jobs
 ├─ Infrastructure — Beacons / Networking / Storage / Cloud + Placement/Scheduling
 └─ Automation — Policies / Scheduling / Autoscaling / Failover / Procedures / Zero-downtime
```

Infrastructure underneath: Beacons (machine + agent `beacon/`, product term for Node `store.go:129`), capacity/runtime/networking/storage/capabilities `handlers_capabilities.go:18`/placement `replicamanager/service.go:67`/health `heartbeatmonitor`/telemetry.

## 2. Vocabulary Normalization

| Use | Not | Notes |
|-----|-----|-------|
| Beacon | Node (user-facing) | `store.go:129 Node` retained internally, alias `ADMIN_ALIAS_ROUTES` `nodes→beacons`, API compat `GET /api/v1/nodes` → `GET /api/v1/beacons` bridge |
| Workload | Server (generic) | Game Server retained for Pterodactyl-native flow, but shares status/resources/networking model |
| Database | Database Hosts/Services/DB Containers/Managed DBs | Unify `handlers_database_services.go:44` + `handlers_db_containers.go:19` + `handlers_managed_databases.go:28` → `Databases` |
| Catalog | App Store/Service Templates/Nests/Eggs | One `store_catalog.go:14` marketplace |

## 3. Consolidated Sidebar (32 top-level vs 77 today)

```
FORGE
 COMMAND  Overview / Monitoring / Health / Activity          (4 — keep as is, distinction §6 below)
 BUILD    Catalog · Applications · Game Servers · Services   (4 — merges Workloads 10 + Platform Automation 3 + current Build)
 DEPLOY   Deployments · Pipelines · Compose · Git           (4 — merges Workloads deploy 4 + git 2)
 INFRA    Beacons · Networking · Storage · Cloud             (4 — Networking Overview→Domains/Services/Traffic/LB [+Advanced: discovery/gateways/cross-node/DNS/ACME/certs/firewall/mTLS]; Storage → Volumes/Mounts/Backups [+providers]; Beacon detail 8 tabs)
 OPERATIONS Backups · Migrations · Recovery · Automation    (4 — Automation → Policies/Scheduling/Autoscaling/Failover/Procedures/Placement/Zero-downtime)
 ACCESS   Users · Organizations · Projects                   (3 — environments via project, roles/OAuth/social under Users; tenant add `store_tenant_scoping_test.go:65` TenantID)
 PLATFORM Security · Integrations · Settings                  (3 — Security → certs/mTLS/headers/firewall unified; Integrations → plugins/webhooks; Settings → panel)
```

**Top-level removed → contextual:** `Allocations` `handlers_admin.go:1335` → `Networking` Ports, `Mounts` `store_mounts_ext.go:13` → `Storage` Volumes, `Capabilities` `handlers_capabilities.go:18` + `Onboarding Tokens` `:303` → `Beacons/[id]` Capabilities, `SFTP` `handlers_sftp.go:9` → `Beacons/[id]` or `Storage`, `DNS Providers` `handlers_dns.go:9` + `ACME` `handlers_acme_accounts.go:11` + `mTLS` `handlers_mtls.go:10` + `Security Headers` `store_security_headers.go` + `Service Discovery` `handlers_servicediscovery.go:10` + `Traffic` `handlers_trafficmanager.go:8` + `Gateways` + `Cross-node` `handlers_crossnode.go:12` → `Networking` advanced, `Env Affinity` + `Node Autoscaler` + `Procedures` + `Zero Downtime` → `Automation` advanced.

**Compat:** Keep 77 old `href: "/admin/*"` in `admin-registry.ts` with `ADMIN_ALIAS_ROUTES` redirect until `findAdminPage` shows 0 legacy hits.

## 4. Key Experiences

### Command vs Monitoring vs Health
- Overview: "what to know" — Forge state, attention, fleet, workloads, capacity, recent `timeline_events` `024:1`
- Monitoring: "what happens over time" — charts 1h–30d `store_node_metrics.go:50`, unavailable if no telemetry (no fake)
- Health: "what's wrong" — failures `store_alerts.go` — keep distinct names

### Beacon Detail (first-class, not modal)
`app/admin/nodes/[id]/page.tsx` → `Beacons/[id]` 8 tabs: Overview(Metrics/Workloads/Networking/Storage/Capabilities/Placement/Config) showing health/OS/arch/CPU/mem/disk/net/runtime/heartbeat/workload count, real-time CPU/mem/disk/RX/TX/load, identity, capacity total/allocated/available, health states `NodeActualState` `store.go:1842`.

### Workload Shared Model
Every workload: identity/status/location/env/owner/resources/networking/storage/deployment history/activity/logs/console/config/health/backups. Tabs differ by type but mental model consistent. Server detail alive: Online/Offline/Degraded + CPU/mem/disk/net/uptime/Beacon/Env/players, primary `Start/Stop/Restart/Console/Deploy/Backup`.

### Catalog
Universal `store_catalog.go:14` — Search→Filter→Open→Version→Env→Placement→Resources→Network→Storage→Review→Deploy. Create DB: Engine→Version→Name→Env→Placement→Resources→Storage→Network→Backup policy→Review→Deploy. Remove isolated App Store / Service Templates / Nests separation.

### Networking
`Networking Overview → Domains → Services → Traffic → LB → Security` + advanced discovery/gateways/cross-node/DNS/ACME. Domain→DNS→Cert/TLS→Route→Service chain clear, contextual `Add domain` from workload.

### Operations
Distinguish proactive (backups `104_a_backup_system.sql:5`, scheduled `cronjob/service.go:44`) vs reactive (recovery `027`, incidents) vs maintenance vs recovery. Reconciliation: Desired vs Observed vs Diff vs Plan vs Result `store_reconcile.go:13` — human diff, not `diff_data` raw.

## 5. Relationships Navigable

Beacon → Workloads/Region/Location/Allocations/Storage/Placement/Health. Workload → Environment/Project/Beacon/Network/Storage/Deployment/Backups. Domain → Workload/Route/Cert/DNS. Database → Workload/Beacon/Storage/Backup/Network. Search `Find a control...` extended to workloads/beacons/apps/domains `GET /api/v1/servers?search` `handlers_servers.go:229`.

## 6. Visual & Interaction

Tokens single source `DESIGN_TOKENS.md` `--brand #dc2626` etc, Manrope/JetBrains Mono, semantic green/amber/red, progressive disclosure `Choose→Configure→Connect→Review→Create` with `Explain Placement` why `✓ mem avail ✓ runtime compatible` score 91. Modals short, drawers contextual, pages complex. No gradients/ai-slop.

## 7. Migration Safety

Additive migrations only, compat bridges `Node→Beacon`, preserve old endpoints, deprecate after `findAdminPage` confirms no dependency.
