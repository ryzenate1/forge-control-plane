# 09 — Production Readiness

## Scoring Methodology

Each component is scored on a **1–10 scale**, where 10 = ready for production use at scale. Assessment is based on six dimensions: correctness, completeness, reliability, security, performance, and maintainability.

---

## Component Scores

| Component | Score | Key Strengths | Key Weaknesses |
|---|---|---|---|
| Store layer (SQL/pgx) | 9/10 | Complete, production-quality SQL across 47 files | Minor: duplicate state enums |
| Auth system | 8/10 | 3 auth methods, OAuth2, node HMAC, 2FA backend, revocation list | No 2FA UI, no reCAPTCHA |
| API route surface | 7/10 | 189/222 routes functional, Wings-compatible remote API | 33 routes panic, mail test stub |
| Service layer code quality | 8/10 | Well-designed interfaces, testable, correct algorithms | None wired to binary |
| Service wiring | 2/10 | nodeRegistry and nodeProbe wired | 10/12 services nil in main.go |
| Frontend admin | 8/10 | Comprehensive, correct API calls, TanStack Query | Duplicate routes, no templates link |
| Frontend server UI | 5/10 | Databases/schedules/backups/users complete | Files broken, no dashboard, no logout |
| Beacon HTTP API | 6/10 | 40+ routes, correct Docker integration | Won't compile, delete backup missing |
| Beacon Docker | 7/10 | Full lifecycle, security profile, crash detection | Stats polling, volumes not cleaned |
| Beacon SFTP | 6/10 | Ed25519, panel auth, 5 operations | No quota, no symlinks, no session cancel |
| Beacon backup | 4/10 | Local zip works | S3 broken, panel never notified, delete route missing |
| Event system | 2/10 | Fully designed, correct implementation | Never instantiated anywhere |
| Orchestration services | 3/10 | All 12 services well-implemented | None reachable through API |
| Schema/migrations | 9/10 | 47 tables, 36 migrations, richer than Pterodactyl | 3 missing migration numbers |

---

## Overall System Score: 5.5/10

The infrastructure layer (schema, store, auth) is production-grade. The upper layers (wiring, daemon compilation, frontend completeness) bring the average down significantly. **The gap between code quality and runtime correctness is the defining characteristic of this codebase.** The bones are excellent; the connective tissue is missing.

---

## Readiness by Use Case

| Use Case | Status | Notes |
|---|---|---|
| Development/testing | ⚠️ Partially usable | After fixing beacon compile error |
| Single admin managing nodes | ✅ Usable | Admin panel is functional |
| End users managing servers | ❌ Not ready | No user dashboard, power control panics |
| Production hosting | ❌ Not ready | No TLS, services not wired, orchestration unreachable |
| Large-scale multi-node | ❌ Not ready | All orchestration services are nil |
| Pterodactyl migration | ⚠️ Partial | Remote API compatible, but panel→daemon flow broken |

---

## Immediate Blockers for Minimum Viable State

The following 10 fixes must be made before the panel is minimally usable. They are ordered by impact and dependency.

1. Add `golang.org/x/sync` to `beacon/go.mod`
2. Wire all 10 services in `main.go`
3. Wire `events.Registry`
4. Start `heartbeatMonitor` + `reconciler` + `reservations.Manager`
5. Implement `notifyPanelInstallStatus` in beacon
6. Fix `/server/[id]/files` page to use `FilesView`
7. Add logout button
8. Add user `/servers` dashboard page
9. Fix Tailwind CSS variables
10. Add `wsTicketStore` mutex
