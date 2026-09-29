# Scope 10 — Rebrand Incus / Nomad / NetBird to Forge naming (PRODUCT LAYER) + audit those subsystems

Read `.audit-reports/briefs/fleet-common.md` first. You are scope 10; report to `.audit-reports/reports/fleet-10.md`.

## The rebrand decision (already made by the user — implement exactly this)

**Product-layer rename only.** These three backends talk to REAL upstream HTTP APIs (Incus `/1.0/...`, the Nomad HTTP
API and its JSON payloads, the NetBird Management API). Renaming wire-level identifiers would break the integrations,
and that is not a cosmetic tradeoff — it is a functional break.

RENAME (user-visible only): UI labels, page titles and headings, nav labels and descriptions, search keywords, i18n
keys and catalog strings, docs prose, human-readable log messages, table and column captions, empty-state copy.

Canonical Forge names:
- **Incus -> "Forge Virtualization"** (system containers and virtual machines)
- **Nomad -> "Forge Orchestration"** (jobs, allocations, deployments)
- **NetBird VPN -> "Forge Mesh"** (WireGuard mesh VPN control plane)

Pick consistent derived forms for sub-surfaces (e.g. "Virtualization Instances", "Orchestration Jobs", "Mesh Peers"),
and keep the three consistent across nav, page, table headers and i18n.

MUST NOT CHANGE — functional identifiers, do not rename any of these:
- Route paths and API URLs: `/admin/incus`, `/admin/nomad`, `/admin/netbird` stay exactly as they are.
- Go package, directory, type and function names: `incussvc`, `nomadsvc`, `netbirdsvc`, `NomadConfig`, `K3sConfig`,
  `SchedulerTypeNomad`, `NewNomadScheduler`, `registerIncusRoutes`, file names, import paths.
- JSON field names and anything crossing the wire, including upstream payload structs and `scheduler_config` keys
  (`addr`, `region`, `datacenter`, `namespace`, `kubeconfigPath`, `kubeApi`).
- Env vars: `INCUS_CA_CERT`, `INCUS_TLS_CERT`, `INCUS_TLS_KEY`, `INCUS_TRUST_TOKEN`, `NOMAD_ADDR`, `NOMAD_REGION`,
  `NOMAD_TOKEN`, `NETBIRD_API_URL`, `NETBIRD_API_TOKEN`.
- Persisted DB strings: the admin API-key scopes `netbird.read` / `netbird.write`
  (`forge/api/internal/store/store_apikeys.go:100-101`) — those are values stored on existing keys, so renaming them
  silently revokes live credentials. And `nodes.scheduler_type` values (`docker`, `k3s`, `nomad`) plus the switch
  cases in `internal/scheduler/scheduler_factory.go`.
- `reference/` — a 1.9 GB vendored upstream checkout that includes netbird source. Never edit it. Also leave
  `scripts/dev/clone-references.sh` alone (it legitimately clones those upstream sources) and do not rewrite the dated
  historical report `docs/audits/FORGE_CONTROL_PLANE_FORENSIC_AUDIT_2026-08-23.md`.

HONESTY REQUIREMENT: these pages drive real vendor products, so an operator must still be able to tell what is
actually running. Make the Forge name the primary label and keep the vendor as a clearly secondary technical detail —
a `provider`/`driver` value, a subtitle, or a tooltip, e.g. label `Forge Mesh`, description
`WireGuard mesh VPN control plane (driver: NetBird)`. Never rename a driver's config field into a Forge word.

## You own (edit only these)

API: `forge/api/internal/http/handlers_incus.go` (332 lines, ~13 routes, 70 vendor matches), `handlers_nomad.go`
(217, ~11 routes), `handlers_netbird.go` (272, ~22 routes), `handlers_scheduler.go`;
`internal/services/incus/service.go` (574), `internal/services/nomad/service.go` (416),
`internal/services/netbird/service.go` (572); `internal/scheduler/scheduler_nomad.go` (388), `scheduler.go`,
`scheduler_factory.go`; `internal/services/replicamanager/blocked_eval.go`, `reschedule.go`;
`forge/api/config/runtime.go`; `forge/api/cmd/api/main.go` (your ~13 lines only — surgical);
`forge/api/internal/store/store_apikeys.go` (your scope DESCRIPTION strings only — the keys must not change).
Beacon: `beacon/internal/runtime/lxc.go` (user-visible strings only; do not break Incus/LXC protocol handling).
Web: `forge/web/app/admin/incus/page.tsx`, `nomad/page.tsx`, `netbird/page.tsx`, `scheduler/page.tsx`;
`components/admin/admin-registry.ts` (your five entries around lines 164-167, surgical); `lib/api/incus.ts`,
`nomad.ts`, `netbird.ts`, `status.ts`; `lib/api.ts` (your export lines only); `components/admin/AdminNodes.tsx`
(scheduler/backend display strings only — `AdminNodes.tsx` is otherwise scope 2's file, so touch only the display
strings and note it); `forge/web/test/app-ux-18.test.tsx` (update expected labels to match the rename).
Lang: `admin.nav.incus|nomad|netbird` and matching `admin.navDesc.*` in the `en` catalog under `lang/`, plus the
equivalent keys in the other seven locale catalogs — change only the brand word and keep it consistent; do not leave
an English brand string sitting in a catalog that has a key for it.
Docs: `docs/audits/SERVICES_UI_GAP.md` only where it names these three surfaces.
App-store assets: `forge/api/internal/services/appstore/assets/appstore-templates/netbird-client.yaml`,
`service-templates.json`, `service-templates-latest.json` — rename display titles only; keep image names, container
names, service keys and any `netbird`/`nomad`/`incus` identifier a deployment depends on.

## Second job: audit these three subsystems file by file

Read each owned file completely and fix real defects under the shared invariants, with particular attention to:

1. **Fail-closed on unconfigured** — each of these services has a documented nil-safe path
   (`netbirdsvc.New(db, slogLogger)` at `main.go:1166-1168` is nil-safe when `NETBIRD_API_URL`/`NETBIRD_API_TOKEN`
   are unset). Verify a nil or absent client produces an explicit "not configured" error rather than a 200 with an
   empty list that reads as "no peers / no instances / no jobs". Unknown is not empty.
2. **Never report success for work not performed** — an unreachable Incus/Nomad/NetBird endpoint returns an error,
   not `nil`. A stale inventory reading is not a healthy one; carry and surface the as-of time.
3. **Never resolve an ambiguous target** — reject a request that omits the node/instance/peer/job it refers to rather
   than picking the first one that has credentials.
4. **Authz asymmetry you must FIX**: NetBird is the only one of the three with API-key scopes. Add `incus.read`,
   `incus.write`, `nomad.read`, `nomad.write` to the same admin scope catalog with accurate descriptions, and enforce
   them on the corresponding routes exactly the way the NetBird handlers guard their routes (read that guard pattern
   first and match it). Adding new scope names is backward compatible; renaming or removing existing ones is not.
   Verify whether scopes are stored as a JSON array column (so no migration is needed) before writing one.
5. **Hunt**: JSON decoded with wrong casing; outbound HTTP without a timeout; missing context cancellation; error
   bodies or logs leaking API keys/tokens (redact); pagination ignored so a truncated first page is presented as the
   complete fleet; scheduler type switches silently falling through to docker for an unsupported backend;
   `scheduler_factory.go` `NodeScheduler` accepting unknown types without error.

## Report requirements

Also include: (a) the exact label and description strings you introduced for the three surfaces; (b) the new scope
names and which routes now enforce them; (c) which locale files you touched and whether non-English catalogs still
need a sync; (d) a `[DELIBERATELY-UNCHANGED]` list of identifier categories you left vendor-named because they cross
the wire or are persisted — that list is the proof you respected the boundary. End with `## Cannot verify`.
