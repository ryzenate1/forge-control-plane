# Strengths and Weaknesses

## Forge Strengths

### Architecture & Design
1. **Modular 3-process architecture** — API, Web, Daemon are independent deployable units with clean boundaries. Far superior to PufferPanel's single-binary coupling and comparable to the Pterodactyl+Wings split.
2. **Constructor-injected service graph** — 14 services wired via explicit DI in `cmd/api/main.go:120-168`. Testable, swappable, traceable.
3. **Typed event bus** — ~40 typed event types with correlation IDs, wildcard subscriptions. More sophisticated than any reference project.
4. **Bidirectional daemon communication** — Unique among all projects. Wings only polls the panel. Beacon sends heartbeats, receives config pushes.
5. **Explicit API versioning** (`/api/v1/`) — No reference project versions their API.
6. **Per-field authorization** — Separate permission checks for detail/startup/image/allocation changes vs coarse role checks in references.

### Orchestration Features (Unique)
7. **Cluster management** — Not present in any reference.
8. **Placement reservations** — Pre-allocates capacity before server creation.
9. **Migration planner/executor** — Multi-stage state machine for server transfers.
10. **Evacuation planner** — Plans and executes node evacuation.
11. **Recovery coordinator** — Failure recovery with plan status tracking.
12. **Reconciler/drift correction** — Automatically corrects configuration drift.
13. **Heartbeat state machine** — 5-state classification (healthy/suspected/unreachable/offline/recovering).
14. **Observability timeline** — Metrics collection and correlation.

### Frontend
15. **Modern Next.js 15 App Router** — Server components, streaming, React 19. Far ahead of Pterodactyl's React 16 + Webpack.
16. **Zustand + TanStack Query** — Clean separation of UI state and server state. Excellent developer experience.
17. **25 admin routes** — Deepest admin panel of any project. Health monitoring, operations dashboard, plugin management, OAuth client management.
18. **Short-lived WebSocket tickets** — JWT + short-lived ticket pattern. More secure than Wings' plain JWT-in-URL.

### Technology Choices
19. **Go for API and daemon** — Type safety, fast compilation, excellent concurrency. Single binary deployment.
20. **TypeScript throughout frontend** — Better developer experience than plain JavaScript in PufferPanel.
21. **No PHP dependency** — Avoids Laravel/FPM complexity entirely.
22. **PostgreSQL with pgx** — Excellent driver, connection pooling, prepared statements.

### Security (Relative)
23. **Full security headers** — CSP, HSTS, Permissions-Policy, X-Frame-Options.
24. **API key IP whitelisting** — Unique to Forge.
25. **Container hardening** — CapDrop ALL, ReadonlyRootfs, no-new-privileges, Init.
26. **Login rate limiting** — Per-IP and per-email (SHA-256 hashed).
27. **Audit logging** — Comprehensive, computed via `safeAuditMeta` to prevent injection.
28. **Production refuses to start with default secrets** — `main.go:44-51` check.

## Forge Weaknesses

### Compilation & Runtime
1. **Beacon does not compile** — Missing `gopkg.in/yaml.v2` in go.mod. Multiple compile errors in `installer.go`.
2. **Migration 020 has stray `p` byte** — All migrations ≥020 silently fail.
3. **Webhook migration orphaned** — In wrong directory, never applied.
4. **10/12 services nil in dev mode** — ~30 routes lack nil-guards and will panic.
5. **dbprovisioner SQL injection** — `fmt.Sprintf` identifier interpolation.

### Testing
6. **Beacon: zero tests** — No test files exist. Critical quality gap.
7. **forge/api: only 2 packages tested** — `internal/http` and `internal/store` only. All 14 service packages have no tests.
8. **forge/web: minimal test coverage** — Coverage thresholds at 25% lines, 20% functions.
9. **35 files not gofmt-clean** — forge/api code hygiene issue.

### Missing Features vs References
10. **No file chmod UI** — Backend supports it, frontend doesn't expose.
11. **No file copy UI** — Backend supports it, frontend doesn't expose.
12. **No backup rename** — Not in backend or frontend.
13. **No backup lock UI integration** — Backend has it, frontend wiring unclear.
14. **No server-level mounts tab** — API exists, no route or nav entry.
15. **No egg import/export** — Pterodactyl has this.
16. **No `/nodes/deployable` endpoint** — Pelican has this.
17. **No OAuth social login** — Not in any project but common feature request.
18. **No i18n** — Pterodactyl has full i18next.

### Security Gaps
19. **No inbound HMAC verification** — Panel signs outbound calls but doesn't verify inbound.
20. **No `openat2` syscall** — Wings has kernel-level path safety.
21. **No rootless containers** — Wings supports this.
22. **No SSH cipher lock-down** — Wings has modern cipher configuration.
23. **No TLS on beacon** — HTTP only.
24. **No JWT for WebSocket auth on beacon** — Uses bearer token.
25. **Rate limiter fails open** — Redis down = no rate limiting.
26. **Default credentials in production** — Seed runs unconditionally.
27. **Shared daemon token** — Single secret for all daemons.
28. **Auth token key mismatch** — Frontend localStorage key mismatch.
29. **CORS not configurable** — Hard-coded to localhost URLs.

### Code Quality
30. **`store.go` is 1311 lines** — Monolithic file mixing types, DTOs, connection, migration, seed.
31. **`lib/api.ts` is 2523 lines with `any` types** — eslint-ignored, high maintenance burden.
32. **Duplicate migration prefixes** — 015, 018, 023 duplicated.
33. **Duplicate console components** — `console.tsx` and `console-view.tsx` are near-identical.
34. **Empty `components/pterodactyl/` directory** — Dead code.
35. **No shared types between frontend and backend** — Types drift risk.

### Production Readiness
36. **No upgrade/rollback path for database** — SQL-only migrations, no down-migrations.
37. **No deployment documentation** — README covers dev setup only.
38. **No operational runbooks** — Recovery, backup, monitoring procedures undocumented.
39. **No tracing** — OpenTelemetry or similar not present.
40. **No backup/restore from S3 tested** — Code exists but unproven.
41. **Root npm scripts broken** — `npm run dev/build/lint/typecheck` all fail from repo root.
42. **Build artifact committed** — `forge/api/api` binary checked in.

### Daemon-Specific
43. **No memory overhead compensation** — JVM OOM risk (Wings adds 5-15%).
44. **No installer resource limits** — Wings has configurable installer CPU/memory caps.
45. **No per-server WebSocket limit** — Wings limits to 30 connections.
46. **No WS message rate limiting** — Wings has 10msg/200ms limit.
47. **No transfer progress events** — Wings publishes progress on WebSocket.
48. **No log rotation config** — Wings auto-configures logrotate.
49. **No per-server context/cancellation** — Wings has `Server.Context()` for graceful shutdown.
50. **No Egg-level file denylist** — Wings has this at configuration level.

## Comparison Summary

| Dimension | Forge vs Pterodactyl | Forge vs Pelican | Forge vs PufferPanel | Forge vs Wings |
|---|---|---|---|---|
| Architecture | **Better** (modular) | **Better** (modular) | **Much better** (separated) | **Better** (bidirectional) |
| Features | **More** (orchestration) | **More** (orchestration) | **Comparable** | **More** (panel features) |
| Code Quality | **Worse** (less tested) | **Worse** (less tested) | **Comparable** | **Worse** (zero daemon tests) |
| Security | **Worse** (several criticals) | **Worse** | **Comparable** | **Worse** (no openat2/TLS) |
| UI | **Better** (modern stack) | **Comparable** (less a11y) | **Better** | N/A |
| Performance | **Better** (Go vs PHP) | **Better** | **Comparable** | **Comparable** |
| Scalability | **Better** (stateless Go) | **Better** | **Better** | **Comparable** |
| Testing | **Much worse** | **Much worse** | **Comparable** | **Much worse** |
| Production readiness | **Much worse** | **Much worse** | Worse | Worse |
| Daemon maturity | Worse | Worse | Worse | **Much worse** |
