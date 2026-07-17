# Exclusions and Limitations

## Excluded Files and Directories

| Pattern | Reason | Projects affected |
|---|---|---|
| `node_modules/` | Third-party dependencies | forge/web, pterodactyl, pelican, pufferpanel |
| `.next/` | Build output | forge/web |
| `vendor/` | Third-party dependencies (not present in Go projects) | N/A |
| `.git/` | Git metadata | All |
| `.DS_Store` | macOS metadata | All |
| `*.lock` (go.sum, package-lock.json, composer.lock) | Generated dependency locks | All |
| Compiled binaries (`forge/api/api`, `beacon/daemon`) | Build output | forge/api, beacon |
| `.tsbuildinfo` | Build cache | forge/web |
| `*.log` | Runtime logs | forge/api |
| `.dev-data/`, `.dev-logs/`, `.dev-pids/` | Dev runtime artifacts | Root |
| `.commandcode/`, `.claude/` | AI agent artifacts | Root |
| `apps/panel/.next/` | Stale build output (dead workspace) | Root |

## Excluded File Counts by Project

| Project | Excluded | Reason |
|---|---|---|
| forge/api | 3 | Compiled binary (api), api.log, go.sum |
| forge/web | ~500+ | node_modules, .next, package-lock.json |
| beacon | 2 | daemon binary, go.sum |
| pterodactyl | ~500+ | node_modules, vendor, composer.lock, yarn.lock, .git |
| pelican | ~500+ | node_modules, vendor, composer.lock, .git |
| pufferpanel | ~800+ | node_modules, lang files (JSON), .git |
| wings | ~50 | go.sum, .git |

## Analysis Limitations

### Language & Framework Differences
- **Go vs PHP:** Our Go projects cannot be directly compared line-for-line with PHP projects. Go is statically compiled with built-in concurrency; PHP is interpreted with process-per-request model. Performance comparisons are inherently biased toward Go.
- **Next.js vs React SPA vs Filament:** Different rendering strategies (SSR vs CSR vs server-rendered PHP). Feature parity was judged by functionality, not implementation approach.
- **Single binary vs multi-process:** PufferPanel's single-binary approach is architecturally simpler but less scalable. Direct comparison is valid but biased.

### Completion Status Verification
- **forge/api:** All 114 source files read and analyzed. 52 migration files manually reviewed (first 30 lines each, key files in full).
- **forge/web:** All 109 source files catalogued. Key files read in full (console.tsx, auth page, admin pages, api.ts excerpts). Remaining ~80 component files sampled.
- **beacon:** All 37 source files read in full (small project).
- **Pterodactyl:** 966 PHP files — sampled by subsystem. Routes (6 files) read in full. Controllers sampled (~20 files). Models listed but not read individually. 195 migrations listed, 5 read.
- **Pelican:** 1,720 PHP files — same sampling strategy. Filament resources and pages sampled (~15 files).
- **PufferPanel:** 240 Go files — sampled by subsystem (~30 files read).
- **Wings:** 134 Go files — sampled by subsystem (~40 files read).

### Not Verified
- **Actual production behavior** — No project was run in production. Runtime assertions about performance, reliability, and scaling are based on static analysis and architecture patterns.
- **Cross-node communication** — Bidirectional daemon communication was verified in code but not end-to-end tested.
- **Backup/restore integrity** — S3 backup code exists but was not verified against actual S3.
- **Database provisioning** — dbprovisioner code was read but not tested against MySQL/PostgreSQL.
- **Webhook delivery** — Webhook table is never created; delivery cannot work.
- **Performance benchmarks** — No load tests or profiling data collected. All performance assessments are static or inferred.

### Inferred Ratings
The following categories are **inferred** rather than measured:
- Performance (Go vs PHP throughput)
- Scalability (stateless vs stateful architecture)
- Reliability (test coverage vs production track record)
- Memory usage (compiled Go vs interpreted PHP)

### Ground-Truth Verified
The following were **directly verified** by running commands or reading code:
- go build/vet/test results (forge/api: PASS; beacon: FAIL)
- npm run typecheck/lint/build (forge/web: all PASS)
- Migration 020 stray byte (direct read: confirmed)
- Migration webhooks orphan (direct read: confirmed)
- dbprovisioner SQL injection (code review: confirmed)
- Auth token key mismatch (code review: confirmed)
- ESLint warnings count (43 measured)
- gofmt issues count (35 forge/api, 10 beacon)

## Methodology
1. **Repository inventory** — Glob-based recursive file listing
2. **Subsystem identification** — Feature areas defined by game-panel domain knowledge
3. **Code sampling** — Key files read in full; large codebases sampled per subsystem
4. **Command verification** — Build/test/typecheck commands actually run
5. **Cross-reference with docs** — Existing docs/ analysis reviewed for consistency
6. **Gap identification** — Features present in references but absent in Forge
7. **Security audit** — Manual code review focused on OWASP Top 10 patterns
8. **Scoring** — Weighted average across 13 categories with written justifications
