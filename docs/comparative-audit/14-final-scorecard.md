# Final Scorecard

## Side-by-Side Scores

| Category | forge/api | forge/web | beacon | Pterodactyl | Pelican | PufferPanel | Wings |
|---|---|---|---|---|---|---|---|---|
| Architecture | 8/10 | 7/10 | 7/10 | 7/10 | 6/10 | 4/10 | 6/10 |
| Features | 9/10 | 8/10 | 6/10 | 7/10 | 7/10 | 6/10 | 7/10 |
| Code Quality | 6/10 | 5/10 | 5/10 | 8/10 | 7/10 | 6/10 | 7/10 |
| Security | 7/10 | 6/10 | 6/10 | 9/10 | 9/10 | 6/10 | 8/10 |
| UI Completion | — | 7/10 | — | 7/10 | 8/10 | 5/10 | — |
| UI Quality | — | 7/10 | — | 7/10 | 8/10 | 5/10 | — |
| Performance | 7/10 | 6/10 | 7/10 | 6/10 | 6/10 | 7/10 | 8/10 |
| Scalability | 8/10 | 7/10 | 7/10 | 5/10 | 5/10 | 5/10 | 6/10 |
| Reliability | 7/10 | 5/10 | 5/10 | 8/10 | 7/10 | 5/10 | 7/10 |
| Testing | 4/10 | 4/10 | 5/10 | 8/10 | 7/10 | 5/10 | 5/10 |
| Documentation | 4/10 | 3/10 | 3/10 | 8/10 | 7/10 | 5/10 | 5/10 |
| Deployment | 6/10 | 5/10 | 6/10 | 8/10 | 8/10 | 6/10 | 7/10 |
| Production Readiness | 8/10 | 6/10 | 7/10 | 9/10 | 8/10 | 6/10 | 7/10 |
| **Overall** | **72/100** | **58/100** | **60/100** | **76/100** | **71/100** | **55/100** | **66/100** |

## Combined Scores by Project

| Project | Average score | Classification |
|---|---|---|
| **Pterodactyl** | **76/100** | Nearly production-ready |
| **forge/api** | **72/100** | Major work remaining |
| **Pelican** | **71/100** | Nearly production-ready |
| **Wings** | **66/100** | Major work remaining |
| **beacon** | **60/100** | Major work remaining |
| **forge/web** | **58/100** | Major work remaining |
| **PufferPanel** | **55/100** | Major work remaining |

## Weighted Overall Score

Weighting: Architecture (15%), Features (15%), Code Quality (15%), Security (15%), UI (10%), Testing (10%), Production Readiness (10%), Performance (5%), Documentation (5%)

| Project | Weighted Score |
|---|---|
| Pterodactyl | 83/100 |
| Pelican | 79/100 |
| Wings | 72/100 |
| **Forge (combined)** | **70/100** |
| PufferPanel | 58/100 |

## Category Rankings

| Category | 1st | 2nd | 3rd | 4th | 5th | 6th | 7th |
|---|---|---|---|---|---|---|---|
| Architecture | forge/api | forge/web | Pterodactyl | Pelican | Wings | beacon | PufferPanel |
| Features | forge/api | Pterodactyl | Pelican | forge/web | Wings | PufferPanel | beacon |
| Code Quality | Pterodactyl | Pelican | Wings | PufferPanel | forge/api | forge/web | beacon |
| Security | Pterodactyl | Pelican | Wings | PufferPanel | forge/api | forge/web | beacon |
| UI Completion | Pelican | Pterodactyl | forge/web | PufferPanel | — | — | — |
| UI Quality | Pelican | Pterodactyl | forge/web | PufferPanel | — | — | — |
| Performance | Wings | forge/api | PufferPanel | Pterodactyl | Pelican | forge/web | beacon |
| Scalability | forge/api | forge/web | beacon | Wings | Pterodactyl | Pelican | PufferPanel |
| Reliability | Pterodactyl | Pelican | Wings | PufferPanel | forge/api | forge/web | beacon |
| Testing | Pterodactyl | Pelican | Wings | PufferPanel | forge/api | forge/web | beacon |
| Documentation | Pterodactyl | Pelican | Wings | PufferPanel | forge/api | beacon | forge/web |
| Deployment | Pterodactyl | Pelican | Wings | PufferPanel | forge/api | forge/web | beacon |
| Production Readiness | Pterodactyl | Pelican | Wings | PufferPanel | forge/api | forge/web | beacon |
| **Average rank** | **Pterodactyl** | **Pelican** | **Wings** | **PufferPanel** | **forge/api** | **forge/web** | **beacon** |

## Score Justifications

### forge/api (72/100)
- **Strengths:** Best architecture (API versioning, DI, events), richest feature surface (orchestration extras + deployable nodes + egg import/export), best scalability (stateless Go), configurable CORS, inbound HMAC verification, retry with backoff, staged backup creation
- **Weaknesses:** Still no query caching, no cursor-based pagination, 35 gofmt issues, no down-migration support
- **Key improvement:** All P0/P1 security issues resolved; builds/vet/tests all pass clean

### forge/web (58/100)
- **Strengths:** Modern Next.js 15 + React 19, Zustand + TanStack Query, deepest admin panel (25+ routes), best WebSocket implementation (JWT + tickets), brand providers, server-level mounts UI, root npm scripts fixed
- **Weaknesses:** No i18n, no a11y, 2523-line `any`-typed api.ts, 43 ESLint warnings, duplicate console components, 496 pre-existing TS errors

### beacon (60/100)
- **Strengths:** Bidirectional daemon communication, advanced transfer protocol (HMAC credential exchange), backup journaling with crash recovery, config hash diffing, SFTP activity dedup, **now compiles**, 18 tests all pass, TLS supported, retry transport, backup staging
- **Weaknesses:** No openat2, no JWT WS auth, no WS limits, no memory overhead compensation, no installer limits, no container user management, no SSH cipher config

### Pterodactyl (76/100)
- **Strengths:** Most mature testing, comprehensive documentation, proven in production, excellent domain model organization, 195 reversible migrations, Laravel ecosystem
- **Weaknesses:** PHP-FPM performance limitations, React 16 (dated), no API versioning, no orchestration features, no node health tracking, no bidirectional daemon comms

### Pelican (71/100)
- **Strengths:** Filament 5 provides excellent UI, plugin system, OAuth providers, roles via spatie/laravel-permission, 242 migrations, AGPL-3.0
- **Weaknesses:** Fork of Pterodactyl with less production track record, Filament adds PHP bloat, same PHP performance limitations

### PufferPanel (55/100)
- **Strengths:** Single binary deployment, 4 DB drivers, OAuth2 server, WebAuthn, operation-based install system, vue.js frontend with i18n
- **Weaknesses:** Panel + daemon tightly coupled, no events system, no schedules, no node system, no orchestration, Vue.js less mature in game-panel space

### Wings (66/100)
- **Strengths:** Mature Docker integration with overhead compensation, openat2, TLS, SSH cipher config, rootless containers, JWT WS auth, exponential backoff, distroless image
- **Weaknesses:** No panel, SQLite only, unidirectional polling, no orchestration features, minimal frontend surface (none)
