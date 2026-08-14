export const meta = {
  name: 'analyse-forge',
  description: 'Map the Forge monorepo across 10 subsystems, adversarially verify load-bearing claims, then synthesize a grounded analysis',
  phases: [
    { title: 'Map', detail: '10 parallel readers, one per subsystem' },
    { title: 'Verify', detail: 'adversarial refuters on each map\u2019s load-bearing claims' },
    { title: 'Synthesize', detail: 'single analysis, then a completeness critic' },
  ],
}

const SANDBOX = `
ENVIRONMENT RULES (this repo, this sandbox) \u2014 follow exactly:
- Prefix every shell command with \`rtk\` (e.g. \`rtk git status\`, \`rtk go build ./...\`). Keep the prefix inside chains. Output is condensed on purpose; treat it as the complete result.
- Before ANY go build/vet/test run: \`export GOCACHE="$TMPDIR/go-build" GOTMPDIR="$TMPDIR"; mkdir -p "$GOCACHE"\`. Without it the build prints only cache-write errors and hides real diagnostics. Treat \`~/Library/Caches/go-build\` and \`~/go/pkg/mod/cache/download\` errors as sandbox artifacts, NEVER as code defects.
- \`go test ./internal/store/\` cannot run here (module sources missing, network denied) and anything using \`httptest\` panics on port bind. Report these as environment limits; do not work around them or call them bugs.
- \`forge/web/vitest-*.txt\` are large captured test logs, not source. Never grep them for code.
- READ ONLY. Do not edit, create, or delete any file. No git write commands.
- Working dir is /Users/muni/forge/forge-test on branch mvp-4.
`

const CONTRACT = `
OUTPUT CONTRACT:
Be concrete and quantitative. Every claim needs evidence: \`path/to/file.go:123\` or a command plus what it actually printed.
Never blur these three categories \u2014 label which one you mean every time:
  (a) wired and exercised: reachable at runtime from a route, boot loop, or UI path;
  (b) exists and compiles but nothing reaches it at runtime;
  (c) present only in tests, docs, or a dead abstraction.
Do not report success for work you did not perform. Unknown is not zero. If you could not determine something, put it in openQuestions instead of guessing.
Set claim.loadBearing=true on exactly the 3 claims a reader would most rely on and that would most change the picture if they turned out false.
Prefer counting over adjectives: "41 of 97" beats "most".
`

const MAP_SCHEMA = {
  type: 'object',
  properties: {
    dimension: { type: 'string' },
    summary: { type: 'string', description: '5-10 sentences: what this subsystem is, how mature it is, what surprised you' },
    keyFacts: {
      type: 'array',
      description: '8-20 concrete quantitative findings',
      items: {
        type: 'object',
        properties: {
          fact: { type: 'string' },
          evidence: { type: 'string' },
        },
        required: ['fact', 'evidence'],
      },
    },
    claims: {
      type: 'array',
      description: 'Interpretive judgements that could be wrong. 4-8 of them.',
      items: {
        type: 'object',
        properties: {
          claim: { type: 'string' },
          evidence: { type: 'string' },
          loadBearing: { type: 'boolean' },
        },
        required: ['claim', 'evidence', 'loadBearing'],
      },
    },
    risks: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          risk: { type: 'string' },
          severity: { type: 'string', enum: ['critical', 'high', 'medium', 'low'] },
          evidence: { type: 'string' },
        },
        required: ['risk', 'severity', 'evidence'],
      },
    },
    realVsScaffold: { type: 'string', description: 'Explicit verdict on how much of this subsystem is category (a) vs (b) vs (c), with numbers' },
    openQuestions: { type: 'array', items: { type: 'string' } },
  },
  required: ['dimension', 'summary', 'keyFacts', 'claims', 'risks', 'realVsScaffold', 'openQuestions'],
}

const VERDICT_SCHEMA = {
  type: 'object',
  properties: {
    refuted: { type: 'boolean' },
    corrected: { type: 'string', description: 'The accurate version of the claim, with numbers. If fully confirmed, restate it as confirmed.' },
    evidence: { type: 'string', description: 'What you actually read or ran that settles it' },
    confidence: { type: 'string', enum: ['high', 'medium', 'low'] },
  },
  required: ['refuted', 'corrected', 'evidence', 'confidence'],
}

const DIMENSIONS = [
  {
    key: 'api-architecture',
    prompt: `Map the control-plane API architecture in \`forge/api\` (~318k LOC Go across both modules, 1208 Go files).

Read end to end: \`cmd/api/main.go\` \`run()\` (the composition root \u2014 there is no DI container), \`internal/http/server.go\`, \`internal/http/phase_registry.go\`, \`internal/http/auth.go\` (structure only \u2014 another agent audits its security).

Answer with numbers:
1. How many \`register*Routes(...)\` calls exist in NewServer, and how many distinct HTTP routes get registered at boot? Count them properly (grep route verbs across handler files; check whether Fiber's route table is dumped anywhere, e.g. an /api/docs or openapi generator).
2. There are 222 files in internal/http, 161 of them handlers_*.go. How many are referenced from server.go's registration path, and how many are orphaned?
3. Does the documented layering (handlers \u2192 services \u2192 store) actually hold? Grep for direct \`store\` usage inside internal/http and count violations.
4. The \`RegisterPhaseRegistrar(name, priority, fn)\` hook: how many \`init()\` functions actually register one, which are they, and are there registrar functions defined but never registered (AGENTS.md warns defining is not enough)?
5. The handler \`Config\` struct: how many service pointers does it hold, is it nil-safe, and what happens if a service is nil at request time?
6. What is the boot sequence (New \u2192 InitDB \u2192 InitStores \u2192 InitServices \u2192 BuildHTTP) actually doing, what can fail, and does a partial failure leave a half-serving API?
7. Where do WebSockets, events, eventstore, realtime, observers and policies fit \u2014 are they load-bearing or vestigial?`,
  },
  {
    key: 'api-services-wiring',
    prompt: `There are 97 entries under \`forge/api/internal/services\`. Determine, for EACH package, whether it is:
  (a) constructed in \`cmd/api/main.go\` \`run()\` AND reachable from at least one HTTP route or background loop,
  (b) constructed but never actually invoked,
  (c) never constructed outside its own tests.

Be systematic, not impressionistic: for each package grep its import path across non-test files, and grep its constructor (New*) for callers. A cheap loop over the directory list is the right approach \u2014 build the table mechanically, then spot-check the interesting rows by reading code.

Report:
1. The full classification table (package \u2192 a/b/c \u2192 evidence), and the headline count: how many of 97 are live.
2. Background workers / goroutine loops started at boot (autoscaler, reconciler, heartbeatmonitor, crashdetector, cleanup, queue, scheduler, failover, drain, healthcheckrunner, etc.): which actually start, on what interval, and does each have shutdown/context cancellation handling? Name any goroutine leaked at shutdown.
3. The 10 largest services by LOC, what each does, and whether its size is justified or duplicated elsewhere.
4. Obvious duplication in the service list itself \u2014 e.g. \`notification\` vs \`notifications\`, \`dbbackup\` vs \`backup\` vs \`backupengine\`, \`domains\` vs \`domainsenv\`, \`compose\` vs \`composetemplates\`, \`envgroups\` vs \`envvars\` vs \`envmanifest\`, \`nodeautoscale\` vs \`autoscaler\`. For each suspicious pair say which one is live and whether the other is dead.
5. Which multi-cloud / multi-orchestrator integrations (nomad, incus, netbird, cloud, clustermanager, crossnode, fencing, evacuationplanner) are real vs aspirational.`,
  },
  {
    key: 'data-layer',
    prompt: `Map the data layer: \`forge/api/internal/store\` plus the 243 SQL migrations in \`forge/api/migrations\`.

1. How does production actually connect? Compare \`ConnectWithKeyring\` against the \`driver_postgres.go\`/\`driver_mysql.go\`/\`driver_sqlite.go\` abstraction, \`MigrationRunner\`, and \`store_pool.go\`. For each, grep for non-test callers and classify as live or test-only dead code. State plainly which code path ships.
2. Read \`internal/store/migration.go\`: how are migrations ordered and validated, what exactly does \`validateNoDuplicatePrefixes\` allow (the grandfathered list), how are applied migrations recorded in \`schema_migrations\`, and what happens if an already-applied migration's file is renamed or its content changes? Is there checksum enforcement, and is there any down/rollback capability at all?
3. Count the tables created across all migrations and list the ~20 core entities with their rough column counts. What is the data model, in one paragraph?
4. Hunt for schema drift, with samples rather than exhaustively: migrations adding columns/tables that no Go code ever reads, and Go queries referencing columns no migration creates. Report what you sampled and what you found, and say clearly that it was a sample.
5. What SQL dialect are the migrations written in? Could the mysql/sqlite drivers ever actually apply them? Give evidence (Postgres-specific syntax: JSONB, SERIAL, ON CONFLICT, gen_random_uuid, DO blocks, etc.).
6. Transaction discipline: are multi-statement writes wrapped in transactions, and are there read-modify-write races? Sample the hottest write paths.
Note: \`go test ./internal/store/\` is unrunnable in this sandbox \u2014 audit statically and say so.`,
  },
  {
    key: 'beacon',
    prompt: `Map \`beacon\`, the per-host agent (37 packages under beacon/internal).

1. \`beacon/internal/runtime\`: list EVERY workload runtime adapter (docker, containerd, podman, kubernetes, firecracker, kvm, lxc, others). For each: is it registered in the registry, is it reachable, and classify production-real / partial / stub. Quantify \u2014 count methods that return \`nil\` without doing work, return "not implemented", or are empty. AGENTS.md claims Docker is the only verified production path: confirm or refute it with evidence.
2. How does Beacon authenticate to the panel over \`/api/remote\`, and how does the panel call Beacon (AGENTS.md: HTTP via \`internal/daemon.Client\`, WebSockets only for console/stats/logs \u2014 verify both halves)? What are the credentials, how are they provisioned and rotated, and what happens on mismatch?
3. What does Beacon expose and how real is each: health, metrics, sftpserver, transfer, backup, quota, cron, activity, database, installer, rootfs, system, logrotate, pprof, throttle, ratelimit, websocketlimiter, shutdown.
4. Apply the repo's hardest rule \u2014 "Never report success for work not performed; unknown is not zero, not-reported is not zero, a stale reading is not a healthy one." Find concrete violations in Beacon: functions returning \`nil\` on a failed step, health checks that default to healthy, metrics that report 0 for unmeasured values, stale caches served as fresh. Cite file:line for each.
5. Also apply "Never resolve an ambiguous target silently" \u2014 look for node/server resolution that picks a first match when the request was ambiguous.
6. Does Beacon degrade correctly when the container runtime is unavailable (Docker/Colima down)? What does it report then?`,
  },
  {
    key: 'web-frontend',
    prompt: `Map \`forge/web\` \u2014 Next.js 15 App Router, 151 \`page.tsx\` routes, 730 TS/TSX files across web+packages.

1. Classify all 151 pages into: fully wired to the API, partially wired, or placeholder. Detect placeholders mechanically \u2014 grep for TODO/FIXME, "Coming soon", "Not implemented", hardcoded mock data arrays, pages with no react-query hook or API import at all. Give the three bucket counts and name examples in each. This headline number matters, so make it defensible.
2. Verify the single-HTTP-primitive rule: is every network call routed through \`requestJSON\`/\`fetchJSON\` in \`lib/api/http.ts\`? Count bare \`fetch(\`/\`axios\` calls outside \`lib/api/*\` (excluding tests and e2e mocks) and name the offenders.
3. Cross-check \`components/admin/admin-registry.ts\` against the actual \`app/admin/*\` directories: pages with no registry entry (unreachable from nav) and registry entries with no page (dead links). List both sets.
4. Design-token discipline: count raw hex colors / rgb() outside \`app/globals.css\` and \`DESIGN_TOKENS.md\`. Confirm there really is no Radix/shadcn dependency, and check whether \`components/ui/\` duplicates \`packages/ui\`.
5. State management: react-query vs zustand (\`stores/\`) \u2014 is server state ever mirrored into zustand, are there duplicate sources of truth, and are query keys consistent?
6. Honesty in the UI: per the repo rule that the UI must not present unknown as zero, find places rendering 0/"healthy"/"\u2014" for values the API never measured. Note there is a known prior fix in this area (commit dac73f1 "stop the admin UI inventing resource numbers it never measured") \u2014 check whether the class of bug recurs elsewhere, e.g. in monitoring/metrics/allocation displays.
7. Loading/error/empty states: sample 10 pages and report how many handle all three.`,
  },
  {
    key: 'auth-security',
    prompt: `Analyse Forge's auth, authorization and secret-handling posture. This is a defensive code analysis of the user's own repo \u2014 report findings with file:line, do not attempt exploitation.

1. Read \`forge/api/internal/http/auth.go\`. How does \`authMiddleware\` work, what is the session lifecycle, and what are the \`__Host-forge_session\` cookie's flags (Secure/HttpOnly/SameSite/Path)? Is there session rotation, expiry, revocation?
2. The three authorization layers: admin scopes (\`requireAdminScope\`), per-server RBAC (\`requireServerPermission\`), org tenancy. How are they composed? Then the important part: enumerate route groups registered WITHOUT any of the three, and judge whether each is legitimately public (login, setup, health, webhooks) or an accidental hole. Be exhaustive about this enumeration.
3. Multi-tenancy: is org scoping enforced at the query layer or only in handlers? Look for store queries that take no org/tenant filter on tenant-owned tables \u2014 sample the hottest ones and report cross-tenant read risk with evidence.
4. \`/api/remote\`: how are node credentials verified, can a user session authenticate to it, can node credentials reach user routes, and are node tokens scoped per node (tying back to the "never resolve an ambiguous target silently" rule)?
5. Secrets and crypto: \`internal/crypto\`, \`internal/secrets\`, beacon's \`tls\`/\`tokens\`/\`crypto\`. What is encrypted at rest, with which primitives, where does the key material live (keyring? env? DB?), and is anything sensitive logged or returned by an API response?
6. Input handling: rate limiting, CSRF posture, request size limits, SQL construction (parameterized vs string-built \u2014 grep for fmt.Sprintf into queries), path traversal in the file/SFTP paths, command injection in installer/script execution paths.
7. Setup flow: \`/setup\` on first run \u2014 can it be re-triggered or hijacked after install?`,
  },
  {
    key: 'build-test-health',
    prompt: `Establish EMPIRICAL ground truth on build, typecheck, lint and test health. Actually run the commands; do not infer.

Run and report exact results (remember the GOCACHE export before every Go command):
1. \`cd forge/api && go build ./...\`
2. \`cd beacon && go build ./...\`
3. \`go vet ./...\` in both modules. AGENTS.md claims vet reports failures only in TEST files referencing symbols production code no longer exports \u2014 quantify: how many packages fail, how many failures are test drift vs real, and are production packages genuinely clean? This is the key claim to settle.
4. \`cd forge/web && npx tsc --noEmit\`
5. \`cd forge/web && npx eslint .\` \u2014 report error/warning counts and the top rules violated, not the full log.
6. \`cd forge/web && npx vitest run\` \u2014 pass/fail counts and which suites fail.
7. Go tests: run \`go test\` per module but EXPECT failures from the sandbox (httptest port binding panics; \`./internal/store/\` is unrunnable). Separate three buckets cleanly: genuine test failures, sandbox-limited tests, and passes. Give counts per bucket. Do not retry the known-impossible ones.

Then measure coverage BREADTH statically:
8. 370 \`*_test.go\` files \u2014 across how many distinct Go packages, out of how many packages exist? List the top 15 largest untested packages by LOC (zero test files).
9. 35 web test files + \`e2e/\` playwright specs against 151 pages and the lib/api modules \u2014 what is actually covered, what major surface has zero tests?
10. Is \`make build\`/\`make test\`/\`make lint\` coherent (read the Makefile and scripts/dev/lint.sh) \u2014 would CI pass today? Is there CI config at all (.github/workflows)? Report honestly if a gate is red.`,
  },
  {
    key: 'infra-ops-release',
    prompt: `Map how Forge is built, shipped and operated.

Read: \`infra/\` (compose stacks, Caddy/Nginx, Prometheus/Grafana/Alertmanager, bootstrap and backup scripts), \`forge/install/install.sh\`+\`uninstall.sh\`+\`install-dependencies.sh\`, \`scripts/\` (dev, deploy, release, diagnostics, cleanup), \`native.sh\`, \`dev.sh\`, \`start-dev.sh\`, the \`Makefile\`, all Dockerfiles, \`mise.toml\`, and docs: \`installation.md\`, \`upgrading.md\`, \`releasing.md\`, \`restore-runbook.md\`, \`migration-rollback-policy.md\`, \`firewall.md\`, \`host-tool.md\`, \`docs/operations*\`.

Answer:
1. What does a production install actually do, step by step, and what are its failure modes? Is it idempotent? Does it verify what it installed, or report success regardless (apply the repo's "never report success for work not performed" rule to the shell scripts \u2014 look for missing \`set -euo pipefail\`, unchecked exit codes, \`|| true\` swallowing real failures).
2. Upgrade and rollback: is there a real path? Cross-check \`docs/migration-rollback-policy.md\` against what the migration code can actually do (another agent is auditing migrations; here judge the POLICY vs the tooling). If migrations are forward-only, does the documented rollback story hold?
3. Observability: what is actually scraped, what dashboards and alert rules exist, and do the alerts reference metrics the code actually emits? Spot-check 5-8 alert expressions against real metric names in the Go code. Report any alert that can never fire.
4. Backup/restore: is it genuinely round-trippable? Trace the restore runbook against the backup scripts and the backup services. Name the gaps.
5. Secrets in infra: hardcoded credentials, default passwords, committed .env files, permissive file modes.
6. Inventory the scripts: which are referenced by something (Makefile, docs, package.json, CI), which are orphaned, and which are dangerous (rm -rf with interpolated paths, docker prune, destructive DB ops without confirmation).
7. VERSION is 0.1.0 on branch mvp-4. Judge whether the ops surface matches that maturity or massively overshoots it \u2014 and say which specific pieces are premature.`,
  },
  {
    key: 'docs-audits-drift',
    prompt: `Assess Forge's documentation and its own self-assessment against the code, and judge the development process visible in the repo.

Read: \`AGENTS.md\`, \`CLAUDE.md\`, \`README.md\`, \`CHANGELOG.md\`, \`CONTRIBUTING.md\`, \`FORGE_MASTER_EXECUTION.md\`, \`docs/README.md\`, \`docs/architecture/*\`, \`docs/ai-guidance.md\`, \`docs/PHASE9_API_SEMANTICS_AUDIT.md\`, and the \`audits/\` tree \u2014 especially \`audits/MASTER_FINDING_INDEX.md\`, \`audits/FINAL_PARITY_AUDIT.md\`, \`audits/FINAL_REFERENCE_ECOSYSTEM_REPORT.md\`, \`audits/FORGE_IMPLEMENTATION_PLAN.md\`, and the \`110-phase-01..09\` and \`phase-01..06\` directories.

Answer:
1. What does the project claim about its own completeness, phase, and parity with whatever reference product it is chasing? Quote the claims.
2. How many findings do the audit indexes raise, and how many are marked resolved/open? Give the tally.
3. The real work: spot-check 8-12 specific findings that are marked resolved or done, and verify against CURRENT code whether they actually are. Report each as confirmed-fixed, partially-fixed, or still-broken with file:line. This is the highest-value part of your task \u2014 spend your effort here.
4. Quantify documentation drift: statements in AGENTS.md / README / docs that are now false or stale, with evidence. (E.g. AGENTS.md says ~246 migrations \u2014 there are 243 files; check that kind of thing, and check the bigger structural claims too.)
5. Judge the documentation's usefulness: is it load-bearing for a new contributor, or mostly retrospective audit narrative? How much of \`audits/\` and \`docs/\` is dead weight? Note AGENTS.md itself says audits/ is read-only history, not spec.
6. What does \`git log\` (recent ~60 commits) reveal about how this project is actually being built, and does the CHANGELOG reflect it?`,
  },
  {
    key: 'packages-sdk-i18n',
    prompt: `Map the npm workspace packages and the i18n layer.

1. \`packages/shared-types\`: is it generated from the Go models or hand-maintained? Find any generator. Then measure drift: take 8-10 core entities (server, node, deployment, app, user, org, backup, domain) and diff their fields against \`forge/api/internal/models\`. Report per-entity: fields missing in TS, fields in TS that no longer exist in Go, type mismatches. Quantify.
2. \`packages/sdk\`: what fraction of the API's route surface does it cover (compare its methods against the registered route count)? Is it imported by \`forge/web\` at all, or does the web app use \`lib/api/*\` instead \u2014 making the SDK category (b) or (c) code? Who is the intended consumer, and is it published?
3. \`packages/ui\`: list its primitives. Does \`forge/web\` import them, or does \`forge/web/components/ui/\` duplicate them? Count overlapping component names. AGENTS.md says UI primitives are hand-rolled in \`forge/web/components/ui/\` and there is no Radix/shadcn \u2014 reconcile that with packages/ui existing.
4. \`packages/game-templates\`: how many templates, what schema, are they validated anywhere at build or runtime, and does the API/UI actually consume them?
5. \`lang/\`: eight locales. Measure key counts per locale against the reference (English) catalog: missing keys, extra keys, and values identical to English (untranslated). Give a per-locale completeness table with real numbers.
6. Verify the sync mechanism: read the \`sync:locales\` script and determine whether \`forge/web\`'s in-tree catalogs are currently IN SYNC with \`lang/\` \u2014 diff them. If they have drifted, say by how much.
7. Does \`npm run build:packages\` actually need to precede web typecheck (AGENTS.md says so)? Verify by checking whether web imports built dist output or source.`,
  },
]

const CLAIM_CAP = 3

function pickClaims(map) {
  if (!map || !Array.isArray(map.claims)) return []
  const lb = map.claims.filter(c => c && c.loadBearing)
  const rest = map.claims.filter(c => c && !c.loadBearing)
  return lb.concat(rest).slice(0, CLAIM_CAP)
}

phase('Map')
log(`Mapping ${DIMENSIONS.length} subsystems; each map's top ${CLAIM_CAP} load-bearing claims go straight to an adversarial refuter.`)

const results = await pipeline(
  DIMENSIONS,
  d => agent(`You are analysing the Forge monorepo at /Users/muni/forge/forge-test. Your assigned subsystem: **${d.key}**.\n\n${d.prompt}\n\n${CONTRACT}\n${SANDBOX}`, {
    label: `map:${d.key}`,
    phase: 'Map',
    schema: MAP_SCHEMA,
  }),
  (map, d) => {
    if (!map) return { dim: d.key, map: null, verdicts: [] }
    const picked = pickClaims(map)
    const dropped = (map.claims || []).length - picked.length
    if (dropped > 0) log(`${d.key}: verifying ${picked.length} of ${map.claims.length} claims; ${dropped} lower-priority claims NOT independently verified.`)
    return parallel(picked.map((c, i) => () =>
      agent(`You are an adversarial verifier working in the Forge monorepo at /Users/muni/forge/forge-test. A prior analysis of the **${d.key}** subsystem asserts:\n\nCLAIM: ${c.claim}\nSTATED EVIDENCE: ${c.evidence}\n\nYour job is to REFUTE this claim. Go read the actual code and run the actual commands \u2014 do not reason from the claim's own framing.\nCheck that the stated evidence exists and actually says what the claim says it says. Then hunt for: counterexamples, a second code path that contradicts it, off-by-N counts, and the classic failure \u2014 something true of one file generalized to the whole codebase.\n\nDefault to refuted=true when you are uncertain or the evidence does not check out. Set refuted=false ONLY if you positively confirmed it. If the claim is directionally right but wrong in its numbers or scope, set refuted=false and put the precise correction in \`corrected\`.\n${SANDBOX}`, {
        label: `verify:${d.key}#${i + 1}`,
        phase: 'Verify',
        schema: VERDICT_SCHEMA,
      }).then(v => (v ? { claim: c.claim, statedEvidence: c.evidence, verdict: v } : null))
    )).then(vs => ({ dim: d.key, map, verdicts: vs.filter(Boolean) }))
  }
)

const good = results.filter(Boolean).filter(r => r.map)
const failedDims = DIMENSIONS.map(d => d.key).filter(k => !good.some(g => g.dim === k))
if (failedDims.length) log(`WARNING: these dimensions produced no map and are missing from the analysis: ${failedDims.join(', ')}`)

const totalVerdicts = good.reduce((n, r) => n + r.verdicts.length, 0)
const refutedCount = good.reduce((n, r) => n + r.verdicts.filter(v => v.verdict.refuted).length, 0)
log(`Maps: ${good.length}/${DIMENSIONS.length}. Verified ${totalVerdicts} load-bearing claims; ${refutedCount} refuted.`)

phase('Synthesize')

const dossier = JSON.stringify(good, null, 1)

const synthesis = await agent(`You are writing the definitive analysis of the **Forge** monorepo (/Users/muni/forge/forge-test, branch mvp-4, VERSION 0.1.0) for its own developer, who asked simply "analyse this project".

Forge is a control plane for deploying and operating game servers and app workloads across distributed infrastructure; **Beacon** is its per-host agent. Scale: ~318k LOC Go in 1208 files across two modules, 97 service packages, 161 API handler files, 243 SQL migrations, 151 Next.js pages, 730 TS/TSX files, 370 Go test files, 35 web test files, 8 locales.

Below is the dossier: ${good.length} independent subsystem maps, each with an adversarial verifier's verdict on its load-bearing claims. ${refutedCount} of ${totalVerdicts} verified claims were REFUTED.

<dossier>
${dossier}
</dossier>

Write the analysis. Rules:
- When a verdict refuted a claim, USE THE CORRECTION, not the original claim. Never repeat a refuted claim as fact. Where a correction changed a number, use the corrected number.
- Every non-obvious statement carries evidence (file:line or a command result). Keep numbers; drop adjectives.
- Where two maps disagree, say so explicitly rather than averaging them.
- Distinguish throughout: wired-and-exercised vs compiles-but-unreachable vs test-and-docs-only.
- Do not soften. This developer has explicitly rejected work that reported success it had not performed. If a subsystem is scaffolding, say it is scaffolding, with the count that shows it.
- Do not recommend a rewrite. Prior feedback in this project: refine, don't rewrite \u2014 the existing layout is good.

Structure (markdown, no top-level title):
1. **What Forge is** \u2014 2-3 sentences, then the honest one-line verdict on its actual state.
2. **Architecture as built** \u2014 the real request path, the real boot sequence, where authority lives, how panel\u2194Beacon works.
3. **The ratio that defines this project** \u2014 quantify surface area vs working functionality across API services, Beacon runtimes, web pages, packages, SDK, locales. This is the core of the analysis; make it a table with real counts.
4. **What is genuinely solid** \u2014 be specific and fair; there is real engineering here.
5. **Risks, ranked** \u2014 severity, what breaks, evidence. Correctness and security first, then operability. Merge duplicates across maps.
6. **Health gates right now** \u2014 build/vet/typecheck/lint/test empirical results, separating sandbox limits from real failures.
7. **Drift** \u2014 docs and self-audits vs code; shared-types vs Go models; locales; admin registry vs pages.
8. **What I would do next** \u2014 5-8 concrete, ordered actions, each naming files and the smallest change that helps. No rewrites.
9. **What this analysis did not cover** \u2014 unverified claims, dimensions that failed, sandbox-blocked checks, sampled-not-exhausted areas. Be honest here; it is load-bearing.

Aim for density over length: a senior engineer's memo, roughly 1400-2200 words. Return only the memo \u2014 your output IS the deliverable.`, {
  label: 'synthesize',
  phase: 'Synthesize',
})

const critic = await agent(`You are a completeness critic. Below is an analysis memo about the Forge monorepo (/Users/muni/forge/forge-test), synthesized from ${good.length} subsystem maps with adversarial verification.

<memo>
${synthesis}
</memo>

The dimensions that were mapped: ${good.map(r => r.dim).join(', ')}.${failedDims.length ? ` Dimensions that FAILED to produce a map: ${failedDims.join(', ')}.` : ''}

Your job: find what is MISSING or WRONG in this memo. Specifically hunt for:
1. Claims in the memo that are stated as fact but were never verified, or that contradict a verifier's correction.
2. Whole areas of the repo no dimension examined. Check the actual tree for subsystems that went unmapped \u2014 you have file access, so go look (e.g. forge/api/internal/{placement,scheduler,runtime,policies,observers,eventstore,cloud,domain}, beacon's system/rootfs/installer, infra/, e2e/, graphify-out/, FORGE_MASTER_EXECUTION.md).
3. Numbers that look wrong \u2014 spot-check 4-6 of the memo's headline counts yourself with your own commands and report any that are off.
4. Questions the developer would obviously ask that the memo does not answer.
Verify with real commands before asserting a gap. Do not invent gaps to seem thorough; an empty list is a valid answer for a category.
${SANDBOX}`, {
  label: 'completeness-critic',
  phase: 'Synthesize',
  schema: {
    type: 'object',
    properties: {
      unverifiedOrContradicted: { type: 'array', items: { type: 'string' } },
      unmappedAreas: { type: 'array', items: { type: 'string' } },
      numberChecks: {
        type: 'array',
        items: {
          type: 'object',
          properties: { memoClaim: { type: 'string' }, myResult: { type: 'string' }, correct: { type: 'boolean' } },
          required: ['memoClaim', 'myResult', 'correct'],
        },
      },
      unansweredQuestions: { type: 'array', items: { type: 'string' } },
    },
    required: ['unverifiedOrContradicted', 'unmappedAreas', 'numberChecks', 'unansweredQuestions'],
  },
})

return {
  memo: synthesis,
  critic,
  coverage: {
    dimensionsMapped: good.map(r => r.dim),
    dimensionsFailed: failedDims,
    claimsVerified: totalVerdicts,
    claimsRefuted: refutedCount,
    claimCapPerDimension: CLAIM_CAP,
  },
  refutations: good.flatMap(r => r.verdicts.filter(v => v.verdict.refuted).map(v => ({ dim: r.dim, claim: v.claim, correction: v.verdict.corrected }))),
  risks: good.flatMap(r => (r.map.risks || []).map(x => ({ dim: r.dim, ...x }))),
}
