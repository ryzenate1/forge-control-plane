# Target API Domain Map — Forge Control Plane

**Status:** PLANNED — Phase 2 MODEL, handler inventory `forge/api/internal/http/handlers_*.go` 79 modules ~380 unique verb+path, `server.go:95` Config 76 fields, `main.go:114` 1,573-line run()
**Current truth:** `forge/api/docs/openapi.json` 256 paths is lagging handlers (phantom `/roles`, spec `/files/list` vs handler `/files?path=` `handlers_servers.go:2333` — see `docs/architecture/overview.md:103`)

## 1. Existing Domains (grouped)

| Domain | Representative handlers | Count | Issues |
|--------|-------------------------|-------|--------|
| `auth` | `handlers_auth.go:12` `handlers_social_auth.go:56` `handlers_webauthn.go:9` | 18+ | duplicates `account/*` vs `auth/*` `handlers_auth.go:317` same `handlePasswordChange:553` `PUT` vs `POST` |
| `organizations/projects/environments` | `handlers_tenancy.go:15` 32 routes `100_team_tenancy.sql:2` + `phase2_env.go:26` `phase2-environment-engine:2000` | 32 | `GET /organizations/:slug` `tenancyOrgAccess:767` param mismatch (`:slug`→`c.Params("id")` fix pending) |
| `workloads/gameServers` | `handlers_servers.go:116` ~90 routes `store.go:443` Server `store_state.go:11` desired/actual | 90 | `GET /servers/:id/files` 18 variants `handlers_servers.go:2333` |
| `applications/services` | `handlers_apphosting.go:16` `handlers_appstore.go:9` | 22+4 |  |
| `deployments` | `handlers_deployment.go:8` `handlers_deployment_history.go:11` `handlers_zerodowntime.go:10` `deployment/service.go:23` Status pending→completed | 6+5+9 |  |
| `beacons/nodes` | `handlers_admin.go:37` ~148 `handlers_capabilities.go:18` `handlers_sftp.go:9` | ~30 | vocab Node vs Beacon |
| `placement` | `handlers_scheduler.go:15` `placement/` engine `replicamanager/service.go:67` | 6 | advanced `Env Affinity` `phase8` |
| `networking` | `handlers_domains.go:19` `handlers_proxy_domains.go:9` `handlers_dns.go:9` `handlers_trafficmanager.go:8` `handlers_loadbalancer.go:24` `handlers_crossnode.go:12` `handlers_servicediscovery.go:10` | ~40 | `/domains` name collision proxy vs server |
| `storage/backups` | `handlers_backup_extended.go:35` `handlers_servers.go:1718` `104_a_backup_system.sql:5` `backup/encryption.go:121` | ~15 |  |
| `operations` | `handlers_operations_timeline.go:156` `handlers_reconcile.go:13` `handlers_orphan_remediations.go:13` `handlers_admin.go:445` evacuation | ~10 |  |
| `monitoring/health` | `handlers_observability.go:12` `handlers_alerts.go:14` `beacon/internal/metrics/metrics.go:70` | 6 | distinct Overview vs Health vs Monitoring |
| `security` | `handlers_mtls.go:10` `handlers_certificates.go:10` `middleware_security_headers.go:34` | ~10 | `infra/ship/kubernetes/secret.yaml:10` CHANGE_ME |
| `billing/catalog` | `phase3_registrar.go:15` `175_catalog_entries.sql:5` `195_billing_plans.sql:4` | 6+5 |  |
| `system` | `server.go:1107` health `/metrics:1159` `/api/docs` `swagger.go:22` | 6 |  |

## 2. Target Domain Model

```
auth / organizations / projects / environments
workloads / applications / gameServers / services / databases
deployments / beacons / placement / networking / storage / backups
operations / monitoring / health / activity / automation / security
```

Target: resource-oriented `GET /api/v1/beacons` + `GET /api/v1/beacons/:id` + `PATCH` / `POST /api/v1/workloads/:id/actions/start` explicit transitions `pending→provisioning→starting→running→stopping→stopped→failed` `store.go:1810`.

## 3. Response Envelope

```json
{"data": ..., "meta": {"pagination": ...}, "error": {"code":"BEACON_UNAVAILABLE","message":"Beacon offline","requestId":"..."}}
```
No raw DB errors. Request/operation/job IDs traceable `operations` `092` + `timeline_events` `024:1`.

## 4. API Refactor Sequence

For each domain: inventory → document (purpose/resource/method/request/response/authorization/state/error/idempotency/async) → normalize only when causing product confusion/duplication/debt → compose debug/internal endpoints behind `Advanced` → decompose `internal/http/server.go`'s `NewServer` and the `cmd/api/main.go` `run()` wiring into focused, named init steps.

`run()` is the canonical composition root and there is no DI container. An earlier revision of this section named `internal/app/container.go` as the target for a staged `InitDB/Stores/Services/HTTP` refactor; that file was a never-imported no-op and has been deleted. Decompose the composition root in place rather than staging the graph in a parallel type — see the backend conventions in `AGENTS.md`.

## 5. Known Inconsistencies to Normalize

- `account/*` vs `auth/*` sessions/password/email — unify to `account` (compat POST alias kept)
- `Node` externally → `Beacon` via alias `GET /api/v1/beacons/:id` compat to `nodes` table
- Duplicate `mounts` `store_mounts_ext.go:13` + `store_mounts_ext_reverification_test.go` allowlist `MOUNTS_ALLOWED_PREFIX` already
- OpenAPI phantom paths `openapi.json` 26 entries without `admin/` prefix → remove, add missing handler routes `GET /setup/status` `POST /auth/session/refresh`
- `fmt.Sprintf SELECT` `store_secrets.go:81` allowlisted `fieldSpec` `store_compose.go:346` identifier — add quoting guard future

## 6. Testing

Per refactor: API contract + authz + state transitions `store_state.go:121` + persistence + jobs `pipeline/service.go:95` `nodeautoscale/worker.go:15` + placement `replicamanager` + reconciliation `reconciler/service.go:153` + resource CRUD + failure recovery. Frontend: route/loading/empty/error/mutation/nav/responsive/a11y.

**Next:** Phase 3 FOUNDATION implements tokens + `AppShell` 8-group + 16 primitives before migrating pages.
