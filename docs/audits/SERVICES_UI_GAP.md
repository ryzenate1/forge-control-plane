# Services Without Dedicated UI — Inventory

Generated 2026-08-16. Compared `forge/api/internal/http/handlers_*.go` + `forge/api/internal/services/*` against `forge/web/components/admin/admin-registry.ts` (70 admin pages) and `forge/web/components/console/console-nav.tsx` / `forge/web/app/{admin,console}/*`.

> Definitions: "has UI" = there is an admin page (`/admin/<slug>`) or console page with a dedicated component (`AdminXxx.tsx`) registered in the nav. A raw API route alone is not UI.

## Now HAS UI (after this patch)

| Feature | Runtime / API | UI |
|---------|---------------|----|
| Servers (create with runtime selector: docker / kubernetes / podman / containerd / firecracker / lxc / kvm + auto) | `POST /servers` now accepts `runtime`; scheduler filters on `nodes.runtime_provider`; beacon validates `provider` on `POST /servers` | `forge/web/components/admin/AdminServers.tsx` — Runtime select, node list filtered + provider shown |
| Apps (create with runtime) | `RUNTIME` env forwarded via `createApp` | `forge/web/components/app/app-create-form.tsx` — Runtime select + env preview |
| Kubernetes clusters | Beacon `/kubernetes/pods|deployments|services|events|scale` → API `GET /admin/kubernetes/{pods,deployments,services,events}` + `POST /admin/kubernetes/deployments/:name/scale` | NEW: `forge/web/components/admin/AdminKubernetes.tsx` + `forge/web/app/admin/kubernetes/page.tsx`, nav entry **Infrastructure → Kubernetes** (`admin-registry.ts`), client `forge/web/lib/api/kubernetes.ts` |
| LXC / KVM runtimes | New beacon `beacon/internal/runtime/lxc.go`, `kvm.go` (exec-based, no build tag), registered in `runtime/factory.go` + `cmd/daemon/main.go` via `DAEMON_LXC_*` / `DAEMON_KVM_*`; API adapters `forge/api/internal/runtime/lxcadapter.go`, `kvmadapter.go` + `MultiRuntimeAdapter` wiring in `cmd/api/main.go` | Selectable in the same Server/App runtime chooser (same path as firecracker/kubernetes); nodes report provider via heartbeat (`nodes.runtime_provider`) |

## Services that still have NO dedicated UI page

These have HTTP handlers (or internal services) but no admin. View them today only via API / direct DB / logs. Good candidates for the next batch of pages.

### Internal services with handlers but no page
| API prefix / handler file | What it is | Route examples |
|---------------------------|------------|----------------|
| `/api/reconcile`, `/api/recover` | Drift reconciliation + stuck-operation recovery | `handlers_reconcile.go` |
| `/api/orphan-remediations` | Detects and remediates orphan resources | `handlers_orphan_remediations.go` |
| `/api/queue` / async operations | Background job queue / operations | `handlers_procedures.go`, `handlers_processes.go` |
| `/api/mtls`, `/api/acme/accounts` | Per-node mTLS (ACME/Let's Encrypt) cert lifecycle | `handlers_mtls.go`, `handlers_acme_accounts.go`, `handlers_certificates_ext.go` |
| `/api/capabilities`, `/api/remote/servers` | Node remote config / capabilities reporting | `handlers_capabilities.go`, `handlers_remote*.go` |
| `/api/transfer`, `/api/migrations/:id/*` | Live server transfers between nodes | `handlers_migrations.go`, `handlers_remote_extra.go` |
| `/api/portainer` | Portainer integration proxy | `handlers_portainer.go` |
| `/api/convoy` / cross-node | Cross-node RPC for distributed ops | `handlers_crossnode.go` |
| Reservation / placement intents | Placement reservation lifecycle (`POST /servers` → reservation row) | `handlers_revisions.go` internal, not admin-visible |
| Prediction metrics / node probe | `POST /scheduler/predictive/metrics/:nodeId` (health) | `handlers_scheduler.go` predictive routes are minimal |

### Features with partial UI (API fuller than the page exposes)
- **Scheduler**: `/admin/scheduler` shows scoring + affinity, but **K3s/Nomad backend** selection (`PUT /admin/scheduler/nodes/:id/scheduler` → `scheduler_type=k3s|nomad`) has no dedicated cluster-management page — partially addressed by the new **Kubernetes** page. Nomad still has no UI.
- **Compose / Git / Build / Buildpacks**: pages exist, but **registry auth per-build**, **incremental build logs WS** (`/ws/build` etc.) and **phase-1 Git** flows are only partly surfaced.
- **Gateway / ingress / loadbalancer / traffic / domains / certificates / DNS**: rich pages exist, but **per-domain mTLS overrides** and **ACME challenge DNS-propagation diagnostics** are API-only.
- **User console**: `forge/web/app/console/*` has server/app detail tabs, but console-level **resource quotas vs. usage** (billing metering) is admin-only (`/admin/billing`).

### Suggestions (next in priority)
1. **Orphan & Reconciliation center** — unify `/reconcile` + `/orphan-remediations` + `recovery` into one admin page.
2. **Transfer / Migration cockpit** — live migrations (`/migrations`, `/transfer`) deserve a page with progress WS.
3. **Queue / workers inspector** — drain queue length, stuck operations, reservation expiry.
4. **Nomad scheduler page** — mirror the new Kubernetes page (the scheduler already supports `nomad` tipo).
5. **Node runtime health per provider** — add to **Nodes** detail tabs: `Ping()` result, tool availability (`lxc-info`, `virsh`, `firecracker`) and capability badges from `runtime/Capabilities`.

> To claim one of these, add `GET` routes to the matching `register*Routes` group, a client module in `forge/web/lib/api/<domain>.ts`, an `AdminXxx.tsx` component, a wrapper `forge/web/app/admin/<slug>/page.tsx`, and an entry in `admin-registry.ts`.
