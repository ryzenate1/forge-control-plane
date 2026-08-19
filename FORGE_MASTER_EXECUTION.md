# FORGE — MASTER AUTONOMOUS RECONSTRUCTION + IMPLEMENTATION MISSION

You are the primary senior engineer, systems architect, security engineer, infrastructure engineer, backend engineer, frontend engineer, database engineer, DevOps engineer, and product engineer responsible for completing the Forge platform.

This is not an audit-only task.

This is not a documentation task.

This is not a redesign-only task.

This is not a frontend-only task.

This is not a backend-only task.

This is an autonomous end-to-end reconstruction and implementation mission.

Your job is to inspect the current Forge repository, reconstruct how the entire system actually works, identify every broken or incomplete connection, repair the implementation across all necessary layers, integrate the platform into one coherent product, and verify that the resulting system works from UI to runtime and back.

The final system must be a real, integrated infrastructure control plane.

Do not merely make the UI look complete.

Do not merely make APIs compile.

Do not merely make database state change.

Do not merely mark operations successful.

Do not create mock success.

Do not create fake telemetry.

Do not create fake health.

Do not create fake deployments.

Do not create decorative controls.

Do not leave features “implemented” when they do not actually execute.

======================================================================
0. OPERATING PRINCIPLE
======================================================================

Treat Forge as ONE distributed system represented by a dependency graph.

Do not reason about files in isolation.

A feature is only considered implemented when its real end-to-end lifecycle works:

Frontend
→ API client
→ HTTP route
→ authentication
→ authorization
→ validation
→ service/domain
→ persistence
→ durable operation
→ queue/worker
→ scheduler/placement
→ Beacon
→ runtime
→ actual state
→ observation
→ persistence of observed state
→ event
→ realtime delivery
→ frontend state
→ truthful UI.

When applicable, include:

Frontend
→ API
→ scheduler
→ cloud/provider
→ host registration
→ Beacon
→ runtime
→ health
→ reconciliation
→ recovery.

When applicable, include:

User action
→ durable operation
→ retries
→ idempotency
→ restart recovery
→ reconciliation
→ actual state
→ completion.

You must not stop tracing at the first successful layer.

======================================================================
1. CURRENT TRUTH RULE
======================================================================

The current repository is the source of truth.

Historical audits, prior descriptions, previous conversations, plans, comments, TODOs, READMEs, type declarations, and assumptions are context only.

Verify every important historical finding against the current repository before modifying it.

The repository may have changed since previous audits.

Never assume a line number from an older audit is still valid.

Use current source locations.

If historical context conflicts with current code, current code wins.

Do not blindly recreate old code.

Do not preserve a broken implementation merely because it existed previously.

Do preserve working architecture and useful existing implementation.

======================================================================
2. REPOSITORY PRESERVATION
======================================================================

Before modifying anything:

- inspect current branch
- inspect git status
- inspect staged changes
- inspect unstaged changes
- inspect untracked files
- inspect ignored-but-important generated/runtime files when relevant
- inspect worktrees
- inspect stashes
- inspect recent commits
- inspect merge history
- inspect the current project layout
- determine which work is already in progress.

Never:

- git reset --hard
- git clean -fd
- delete untracked work
- discard user changes
- overwrite unexplained work
- remove a stash
- remove a worktree
- rewrite history
- force-push
- destroy databases/volumes
- run destructive production operations.

If an existing changed file is relevant to the mission, understand it and integrate with it.

Preserve valid in-flight work.

If a change appears broken, repair it rather than deleting it.

======================================================================
3. REQUIRED AGENT TOOLCHAIN
======================================================================

Use these tools as part of the engineering workflow.

----------------------------------------------------------------------
3.1 CONTEXT MODE
----------------------------------------------------------------------

Use:

https://github.com/mksglu/context-mode

Context Mode is the context-management layer.

Use it for:

- large file analysis
- large command output
- repository research
- persistent investigation notes
- retrieval of previous findings
- context-safe execution
- indexing large documents
- preserving investigation state across compaction.

Verify Context Mode is actually installed and reachable.

Verify:

- MCP availability
- session state
- storage location
- hooks if the current environment supports them
- routing behavior.

Do not assume Antigravity IDE has the same hook capabilities as Codex CLI.

If the current host is Antigravity IDE and hooks are unavailable, use the supported MCP + instruction-file workflow.

Never allow giant command output to unnecessarily flood the main reasoning context.

Use context-mode execution/index/search for large data whenever appropriate.

Store durable investigation facts in the context system where useful.

----------------------------------------------------------------------
3.2 GRAPHIFY
----------------------------------------------------------------------

Use:

https://github.com/Graphify-Labs/graphify

Graphify is the structural dependency/relationship system.

Build a graph over the actual Forge repository.

The graph should cover, where supported and useful:

- TypeScript
- TSX
- Go
- SQL
- YAML
- JSON
- Markdown
- configuration
- Docker
- migration files
- API contracts
- reference material
- relevant PDFs/documents
- package manifests
- MCP configuration
- agent configuration.

The graph must include at least:

forge/
beacon/
packages/
reference/
infra/
deploy/
scripts/
relevant root configuration.

Verify Graphify actually indexed the repository.

Do not claim Graphify is available merely because installation succeeded.

Run real queries proving it can locate Forge code.

Prove it can answer relationship questions.

Examples:

- frontend Overview → API function → backend route → service → store
- server start → service → queue → Beacon → runtime
- domain → endpoint → gateway → certificate
- backup → policy → worker → Beacon → storage adapter
- node → heartbeat → health → scheduler
- shared type → API consumer
- SDK → shared type
- UI component → frontend consumer
- migration → schema
- event → subscriber
- reference project → corresponding Forge subsystem.

Use graph queries to find:

- orphaned code
- duplicate implementations
- dead exports
- unconsumed packages
- routes with no callers
- callers of nonexistent routes
- types with no consumers
- consumers using local substitute types
- duplicated concepts
- dependency hubs
- risky central nodes
- missing edges.

Use Graphify as a living architectural map, not as decoration.

Do not read thousands of files linearly when the graph can narrow the search.

----------------------------------------------------------------------
3.3 RTK
----------------------------------------------------------------------

Use:

https://github.com/rtk-ai/rtk

Install/verify the integration for the current agent environment.

Use RTK for command-output compression where supported.

Typical high-output operations:

git
rg
grep
find
go
npm
pnpm
docker
kubectl
tests
logs
builds.

Verify:

rtk --version
rtk gain

Then verify the current agent integration.

Do not assume RTK is active merely because it is installed.

Remember that command hooks primarily affect shell/Bash-style execution.

For built-in file tools that bypass shell hooks, use compact shell/RTK equivalents when large output would otherwise flood context.

Do not sacrifice important errors for output compression.

----------------------------------------------------------------------
3.4 CAVEMAN FOR CODEX
----------------------------------------------------------------------

Use the Codex Caveman adaptation when available:

lite / full / ultra

Use ultra for progress/status communication whenever safe.

Caveman compression must NEVER remove or alter technical facts.

Preserve exactly:

- code
- paths
- filenames
- symbols
- line references
- API routes
- HTTP methods
- SQL
- IDs
- numbers
- versions
- migration numbers
- error messages
- command names
- test names
- configuration keys
- environment variables.

Compress conversational filler only.

Do not compress security warnings, irreversible actions, ambiguous instructions, or critical implementation detail.

Persisted source code and technical artifacts must remain normal readable engineering text.

======================================================================
4. FIRST MISSION — RECONSTRUCT THE REAL FORGE SYSTEM
======================================================================

Before making broad changes, understand:

/forge
/beacon
/packages
/reference
/infra
/deploy
/scripts
root package configuration
root Go workspace
Docker/Compose
Kubernetes
CI configuration
environment configuration
migration systems
agent tooling.

Build a system map.

At minimum identify:

- frontend application
- API
- database
- cache
- queue
- scheduler
- placement
- reservations
- operations
- deployment engine
- reconciliation
- health
- heartbeat
- recovery
- failover
- crash detector
- runtime abstraction
- Beacon
- realtime
- event store
- event relay
- notifications
- alerting
- audit/activity
- secrets
- cloud providers
- networking
- storage
- DNS
- certificates
- backups
- database provisioning
- workloads
- tenancy
- auth
- RBAC
- API keys
- plugins
- integrations
- frontend API client
- shared-types
- SDK
- UI package
- game templates.

======================================================================
5. FORGE PRODUCT IDENTITY
======================================================================

Forge evolved from a game-server panel into a unified infrastructure control plane.

It must remain one coherent product.

Do not turn Forge into a collection of unrelated products.

Forge should not feel like:

Coolify + Pterodactyl + Portainer + Rancher + random SaaS dashboards.

Use the useful behavior and patterns from those systems to make Forge one unified platform.

Common primitives must serve all workloads.

A game server, application, service/container, and database should share the same core workload model while exposing specialized controls where required.

The user should feel that everything is part of Forge.

======================================================================
6. CORE DOMAIN MODEL
======================================================================

Preserve or establish a coherent relationship around:

Organization
→ Project
→ Environment
→ Workload

Common workload primitives include:

- Workload
- Deployment
- Revision
- Instance
- Domain
- Endpoint
- Storage
- Operation
- Secret
- Credential
- Schedule
- Backup
- Health
- Activity
- Audit
- Event.

Specialized workload interfaces can include:

GAME SERVER
- Console
- Files
- Players
- Startup
- Variables
- Schedules
- Backups

APPLICATION
- Source
- Build
- Deployments
- Domains
- Environment
- Logs
- Runtime

DATABASE
- Connections
- Backups
- Users
- Configuration
- Runtime
- Logs

SERVICE / CONTAINER
- Runtime
- Environment
- Networking
- Storage
- Logs
- Health
- Scaling.

======================================================================
7. FUNDAMENTAL STATE MODEL
======================================================================

Never collapse these concepts:

Desired state
Actual state
Observed state
Health
Connectivity
Availability
Operation state.

Forge must explicitly distinguish them.

Example:

Desired:
running

Actual:
stopped

Observed:
stopped 8 seconds ago

Health:
unknown

Connectivity:
online

This is valid.

Do not convert it into:

status = healthy

or:

0

or:

success.

Never use false healthy defaults.

Never use zero as a fallback for unavailable telemetry.

Never claim an operation succeeded solely because the API accepted the request.

202 Accepted is not success.

Queued is not running.

Running is not succeeded.

Requested is not observed.

======================================================================
8. DURABLE OPERATION RULE
======================================================================

Every long-running or failure-prone operation must have a durable lifecycle.

Preferred state model:

queued
running
retrying
needs_attention
succeeded
failed
cancelled.

Where useful:

waiting
paused
blocked
rolling_back
rolled_back.

Operations must support:

- durable persistence
- idempotency
- retry policy
- timeout
- cancellation when safe
- progress
- heartbeat/lease
- restart recovery
- reconciliation
- failure visibility
- observability
- final evidence of completion.

Do not write:

queued

without creating a consumer.

Do not write:

success

without verifying execution.

Do not emit completion events from placeholder functions.

======================================================================
9. QUEUE / ASYNC RECONSTRUCTION
======================================================================

Audit and repair:

services/queue
services/operation
all job registrations
all operation registrations
all queue consumers
periodic schedulers
tickers
goroutines
leases
heartbeats
retry logic
recovery logic.

There are historically two overlapping asynchronous infrastructures.

Consolidate responsibilities.

Bring the reliability characteristics of a durable job framework such as River into Forge without blindly replacing architecture unless justified.

Every asynchronous operation must have:

- one authoritative durable state
- one authoritative execution path
- idempotent handler
- durable retry
- explicit failure
- restart recovery
- no duplicate writer ambiguity
- no orphan queue record
- no fake pending state.

Implement durable dead-letter / needs-attention handling.

Wire all declared job types that correspond to real product capabilities.

Historical problem areas requiring current verification include:

- server start/stop/restart/kill
- server install/uninstall
- server transfer
- backup create/restore
- deployment promotion
- source deployments
- app deployments
- buildpack builds
- compose operations
- database operations
- plugin lifecycle
- recovery
- evacuation
- scaling
- scheduled tasks.

======================================================================
10. DEPLOYMENT ENGINE
======================================================================

Deployment is one of the highest-priority areas.

Trace:

deployment request
→ authorization
→ deployment record
→ revision
→ queue
→ execution
→ build
→ artifact/image
→ scheduling
→ placement
→ runtime
→ health gate
→ promotion
→ old revision drain
→ cleanup
→ verification
→ completion.

Repair all historically stubbed/no-op deployment steps.

No function is allowed to report step completion when it did no work.

Ensure:

- build types correspond to actual executors
- cacheFrom/cacheTo survive the entire path
- platform survives the entire path
- clone depth/ref/SHA rules are safe
- deploy keys never leak
- logs are truly live where promised
- revision state matches actual deployment state
- preview environments use the intended preview subsystem
- rollback actually changes runtime state
- health gates target the correct workload/node
- cleanup is real
- promotion is real
- canary behavior is real
- scaling is real.

If a deployment is asynchronous, use durable operation state.

======================================================================
11. SCHEDULER / PLACEMENT
======================================================================

Preserve and complete:

- least-loaded
- bin-pack
- spread
- random
- affinity
- anti-affinity
- region
- node
- labels
- reservations
- capacity
- drain
- evacuation
- failover.

Node eligibility must have one coherent meaning.

Resolve historical mismatches such as:

online
active
healthy
degraded
unreachable
offline.

Make status semantics explicit.

Capacity must include:

- allocated
- reserved
- actual capacity
- overcommit policy
- requested capacity
- usable capacity
- locality
- storage constraints
- network constraints.

If over-allocation configuration exists, either implement it or remove the misleading behavior from active paths.

Do not silently ignore scheduler constraints.

======================================================================
12. NOMAD / ORCHESTRATION CAPABILITIES
======================================================================

Use the Nomad reference implementation as a major behavioral and architectural reference.

Forge should support real orchestration concepts such as:

- jobs
- allocations
- placement
- scheduling
- rescheduling
- draining
- health
- task lifecycle
- service registration
- constraints
- affinities
- resource accounting
- rolling deployment
- restart policy
- failure recovery
- region-aware placement.

Translate those capabilities into Forge's domain rather than creating an unrelated orchestration model.

Where actual Nomad interoperability/integration is appropriate, implement it truthfully.

Where Forge owns its own scheduler, implement the equivalent Forge-native behavior.

Never create a UI that claims Nomad functionality exists when no execution path exists.

======================================================================
13. WORKLOAD EXECUTION
======================================================================

Forge must actually run workloads.

Support the runtime abstractions present in the project where they are intended to be supported:

- Docker
- Containerd
- Podman
- Firecracker
- Kubernetes
- other existing runtime adapters.

Verify each provider before advertising it.

ClusterManager must never silently perform a DB-only fallback when runtime execution is required.

Repair nil-runtime paths that mutate only desired state.

Workload lifecycle:

create
provision
start
stop
restart
kill
delete
rebuild
redeploy
scale
inspect
health
logs
console
files
network
storage.

Actual runtime state must flow back into Forge.

======================================================================
14. BEACON
======================================================================

Beacon is the host execution plane.

Keep the separation:

FORGE:
- policy
- desired state
- auth
- persistence
- scheduling
- orchestration
- high-level workflow

BEACON:
- privileged host operations
- runtime lifecycle
- filesystem
- host inspection
- runtime state
- host health
- execution
- host-side reconciliation.

Audit and repair all Forge→Beacon boundaries.

Verify:

- request signing
- timestamp validation
- credential lifecycle
- mTLS
- reconnect behavior
- timeout
- retry
- host registration
- node targeting
- runtime operations
- telemetry
- actual-state reporting
- filesystem isolation
- SFTP
- backup
- upgrade
- firewall
- DNS
- cron
- resource controls.

Never allow a development fallback token into a path that can become a production authentication bypass.

Verify filesystem isolation against path traversal.

Verify DNS/network operations against rebinding and SSRF risks.

Verify upgrade download/verification/replacement is safe and atomic.

Verify backup archive extraction is guarded.

Verify SFTP exposure and chroot/path isolation.

Fix missing packages and build failures in Beacon.

======================================================================
15. CLOUD PROVIDERS
======================================================================

Audit the cloud provider abstraction.

Historically:

AWS was implemented while other provider kinds existed without full implementations.

Complete providers that Forge intends to support.

Cloud provisioning lifecycle:

request
→ provider
→ instance creation
→ bootstrap
→ Beacon enrollment
→ Forge Node registration
→ heartbeat
→ ready
→ capacity
→ scheduling eligibility.

A cloud instance must not exist invisibly from Forge because registration was skipped.

If a provider cannot be supported yet, expose truthful capability state.

Do not silently advertise it as working.

======================================================================
16. NODE ENROLLMENT / IDENTITY
======================================================================

Make node enrollment durable and secure.

Verify:

- node identity
- enrollment token lifecycle
- certificate/key handling
- credential rotation
- registration race conditions
- duplicate node behavior
- revoked nodes
- bootstrap replay
- reconnect behavior.

Do not let node identity be guessed.

Do not fall back to the first node.

Explicit targeting must remain explicit.

======================================================================
17. NETWORKING
======================================================================

Networking is a first-class Forge subsystem.

Implement and integrate:

- networks
- endpoints
- service discovery
- DNS
- domains
- TLS
- certificates
- reverse proxy
- gateway
- load balancing
- traffic routing
- firewall
- ingress
- egress
- webhooks
- health-aware routing
- network policies
- internal service communication
- node-to-node connectivity.

Where reference systems provide mesh behavior, study and implement equivalent functionality.

Use:

- NetBird
- Caddy
- Traefik
- Nginx Proxy Manager
- service-discovery patterns.

Where WireGuard/mesh is appropriate, implement a real mesh architecture.

Do not represent Docker networks as a complete multi-node network system.

Do not fake cross-node routing.

Implement:

domain
→ endpoint
→ routing rule
→ gateway
→ TLS
→ workload.

Domain management must work end-to-end.

Certificate management must work end-to-end.

Load balancer behavior must be real.

Health-aware routing should use observed workload/node health.

======================================================================
18. STORAGE
======================================================================

Implement storage as a first-class capability.

Support the relevant concepts from:

Longhorn
Restic
Kopia
Incus
Forge's own storage architecture.

Cover:

- persistent volumes
- storage classes
- storage locality
- attach/detach
- snapshots
- replication
- backup
- restore
- retention
- encryption
- quotas
- integrity
- migration.

Do not claim distributed block storage if implementation only archives and restores files.

Do not claim replication if only one copy exists.

======================================================================
19. BACKUPS
======================================================================

Complete the backup subsystem.

Existing reference behaviors include:

- local backup
- S3
- retention
- scheduling
- verification
- restore journals
- checksums
- archive safety.

Repair:

- encryption
- incremental backup
- dedupe
- chunking
- progress
- admin APIs
- restore progress
- remote storage management
- snapshot/backup browsing
- retention enforcement
- verification
- failure recovery
- quota enforcement.

If policy models contain encryption fields, actual implementation must honor them.

Do not show “encrypted” when the payload is not encrypted.

Do not report restore success until restored data is verified.

The historical `/admin/backups/*` 501 paths are high priority.

======================================================================
20. DATABASE PROVISIONING
======================================================================

Database workloads must be real.

Support:

- database hosts
- provisioning
- credentials
- users
- connections
- health
- configuration
- backup
- restore
- lifecycle
- resource management
- service discovery.

Distinguish:

database workload
database host
database server
database connection.

Never allow type names or UI labels to hide the difference.

======================================================================
21. MIGRATIONS
======================================================================

Audit all migration systems:

forge/api migrations
batch2
eventstore
Beacon migrations
dialect-specific migrations
rollback migrations
migration runners.

There are historically multiple runners and schema tracking systems.

Consolidate or clearly separate them.

Guarantee:

- deterministic ordering
- collision detection
- dependency safety
- idempotency
- destructive-operation protection
- upgrade safety
- rollback/recovery
- environment compatibility
- test compatibility.

Verify PostgreSQL/SQLite/MySQL claims against actual implementation.

Do not advertise support for a database dialect if migrations do not work for it.

Repair migration test failures caused by PostgreSQL-only SQL where SQLite support is actually intended.

======================================================================
22. DATABASE / STORE / MODEL LAYER
======================================================================

The store layer is a major contract boundary.

Audit:

forge/api/internal/store/
forge/api/internal/models/

There are historically competing representations of entities.

Examples to investigate:

- string UUID vs uint identifiers
- camelCase vs snake_case
- duplicate model structs
- divergent JSON tags
- duplicate domain representations.

Establish one canonical domain representation per entity.

Ensure audit/notification writes are not silently swallowed.

Handle JSON errors.

Remove fragile SQL string construction where safe.

Keep parameterized SQL.

Do not compromise query safety.

======================================================================
23. SHARED TYPES
======================================================================

Audit:

packages/shared-types/src/

This becomes the canonical frontend/API contract layer.

Repair:

- missing backend response types
- missing pagination type
- missing file content responses
- missing task types
- missing notification/alert types
- duplicate aliases
- duplicate fields
- naming conflicts
- snake_case exceptions
- any usage that can be strongly typed
- inconsistent request/response representation.

Historical examples requiring verification:

ApiBackup:
- uuid/id
- locked/isLocked

ApiAllocation:
- isPrimary/primary

TwoFactorSetup:
- qrCodeUrl/image_url

ApiStartupVariable:
- envVariable/env_variable
- defaultValue/default_value
- serverValue/server_value

ApiPanelMailSettings:
- mailFrom
- mailFromAddress/mailFromName
- host/smtpHost
- port/smtpPort

ApiPanelAdvancedSettings:
- reCAPTCHAEnabled/recaptchaEnabled
- reCAPTCHASiteKey/recaptchaWebsiteKey

ApiEgg:
- dockerImages
- dockerImagesList
- image

ApiMount:
- nodeIds/nodes
- serverIds/servers

ApiNodeCapacity:
snake_case inconsistency.

CrashEvent:
snake_case inconsistency.

ScheduleTask payload:
historically any.

ApiEgg.config:
historically any.

ApiActivityLog.properties:
historically any.

Do not blindly delete compatibility fields.

Determine actual API response shapes first.

Normalize contracts deliberately.

======================================================================
24. SDK
======================================================================

Audit:

packages/sdk/src/

Historically this package is mostly a type export surface and not a functional SDK.

Turn it into a real usable SDK if Forge intends to expose one.

Provide:

- HTTP client
- authentication
- configurable base URL
- request handling
- retries where safe
- timeout
- error class
- typed responses
- typed pagination
- cancellation
- endpoint modules
- streaming/realtime support where appropriate.

Export all canonical types.

Do not duplicate shared types.

The SDK must compile and work independently of accidental monorepo hoisting.

======================================================================
25. UI PACKAGE
======================================================================

Audit ALL:

packages/ui/src/

Especially:

AdminLayout.tsx
Sidebar.tsx
TopBar.tsx
DataTable.tsx
FormCard.tsx
StatsCard.tsx
EmptyState.tsx
lib/utils.ts

Fix:

- dependency declarations
- runtime dependencies
- client/server boundaries
- imports
- icon rendering
- keyboard navigation
- aria labeling
- semantic markup
- focus states
- generic typing
- key stability
- Tailwind integration
- component props.

Historical issues include:

- missing tailwind-merge
- undeclared clsx
- undeclared Next.js dependency
- missing client boundaries
- icon strings never rendered
- empty icon spans
- unlabeled search inputs
- unlabeled table
- pagination buttons lacking semantics
- unlabeled mobile menu
- trend indicators without accessible text
- generic table assuming id
- any in generic type.

Repair the package as a real design-system foundation.

======================================================================
26. GAME TEMPLATES
======================================================================

Audit:

packages/game-templates/src/

and all template files.

Make the package actually build and export meaningful TypeScript artifacts if that is the intended architecture.

Provide:

- canonical template schema
- TypeScript types
- registry
- validation
- metadata
- descriptions
- tags
- stable exports.

Ensure templates are consumable by Forge.

Do not leave package.json pointing at nonexistent dist output.

Do not declare shared-types as a dependency if it is not genuinely used.

======================================================================
27. FRONTEND API CLIENT
======================================================================

Audit:

forge/web/lib/api.ts
forge/web/lib/api/*.ts
forge/web/lib/utils.ts
forge/web/lib/use-translation.ts

Cross-reference EVERY API call against actual Go route registrations.

Do not trust the API client or backend route names independently.

Generate a route matrix during investigation.

Every frontend function must map to:

HTTP method
path
parameters
body
auth expectations
response type
error behavior.

Historical high-priority mismatches requiring current verification include:

SSH key deletion
email change
backup download/restore
migration prepare
migration execute
evacuation plan execution
evacuation plan lookup
evacuation cancellation.

Also verify the broader historically reported mismatch set covering:

- mounts
- allocations
- eggs
- roles
- ACME
- certificates
- host
- firewall
- apps
- compose
- cron jobs
- deployments
- source deployments
- builds
- tenancy
- database containers
- database services
- preview deployments
- monitoring
- alerting
- recovery
- evacuation
- migration
- certificates
- host operations.

Fix the backend or frontend side according to the real desired contract.

Do not create duplicate backend routes simply to accommodate stale frontend calls if the canonical API should be changed.

======================================================================
28. FRONTEND API FETCH INFRASTRUCTURE
======================================================================

The main API client must have one consistent request model.

Provide:

- proper ApiError
- safe JSON parsing
- timeout
- AbortSignal support
- retry on appropriate transient errors
- no retry for unsafe mutations unless idempotency makes it safe
- consistent auth behavior
- CSRF support
- authentication expiration behavior
- consistent envelopes
- pagination handling
- error codes
- request IDs
- logging/debug information where safe.

Remove hardcoded production URLs.

Historically:

getBeaconPanelURL()
had a hardcoded SSR fallback.

Verify and repair.

Remove dead API_WS_URL logic if genuinely unused, or properly integrate it.

Avoid raw fetch for paths that need common request semantics.

If text/stream responses require raw fetch, wrap them in the same error/auth/timeout model.

======================================================================
29. AUTH / CSRF / SESSION / RBAC
======================================================================

Audit every route.

Verify:

- JWT/session handling
- CSRF
- 2FA
- session version
- password changes
- account recovery
- API keys
- OAuth
- social login
- admin scope
- role checks
- tenant checks
- organization/project/environment checks
- IP restrictions
- mutation rate limiting
- read rate limiting.

Historical issues requiring verification:

- requireAuth no-op paths
- source deployment mutation routes lacking role/scope
- Git routes receiving but ignoring adminIPAccess
- admin scope routes without explicit role checks
- plugin settings mutation missing limiter.

Authorization must be checked in the backend.

Never trust hidden UI controls as authorization.

======================================================================
30. SECURITY
======================================================================

Perform a complete security pass.

Check:

- SSRF
- DNS rebinding
- path traversal
- arbitrary filesystem access
- host command execution
- WebSocket authorization
- Origin validation
- CSRF
- credential leakage
- master key lifecycle
- token handling
- HMAC replay
- timestamps
- mTLS
- SFTP exposure
- Docker socket exposure
- host networking
- webhook validation
- upgrade downloads
- plugin execution
- cloud credentials
- SSH keys
- deploy keys
- backup secrets
- secrets at rest
- plaintext environment secrets
- tenant isolation
- resource abuse
- rate limiting
- audit trails.

Historical findings include:

- subuser → privileged execution risks
- console/stats/log WebSocket authorization mismatches
- credential exposure in transfer paths
- production mTLS guard issues
- installer authorization issues
- webhook signing configuration gaps
- HMAC replay concerns
- compose quota/node authorization bypass
- SFTP binding
- host-wide file APIs
- upgrade SSRF/non-atomic replacement
- development token fallback.

Verify each against current source.

======================================================================
31. REALTIME
======================================================================

Unify:

- WebSockets
- event store
- event relay
- notification stream
- stats
- console
- logs
- health updates.

The event flow should be:

event persisted
→ relay
→ subscriber
→ frontend.

Do not rely on multiple disconnected systems without clear responsibility.

Repair historical event relay/subscriber wiring issues.

Ensure dead-lettered events are observable.

Do not silently drop events.

WebSocket connections must have:

- authorization
- tenancy checks
- resource checks
- origin checks
- reconnect handling
- backpressure
- cleanup
- heartbeat
- stale connection handling.

======================================================================
32. MONITORING / HEALTH / METRICS
======================================================================

Build one coherent observability architecture.

Distinguish:

- node connectivity
- node heartbeat
- node health
- workload health
- actual utilization
- allocation/capacity
- historical metrics
- current state.

Fix duplicated metrics systems where possible.

Metrics should not reset unnecessarily unless explicitly intended.

Avoid unbounded maps.

Use proper duration histograms where required.

Do not report fake GC or host metrics.

Heartbeat persistence failures must be visible.

Do not return 200 with "{}" when monitoring data failed.

Use explicit:

healthy
degraded
unhealthy
offline
stale
unknown
unavailable.

======================================================================
33. CRASH DETECTION
======================================================================

Repair:

- fabricated timestamps
- unbounded in-memory state
- race conditions
- restart state recovery
- durable crash-state reconstruction
- eviction
- synchronization.

Crash state must survive service restarts where product semantics require it.

======================================================================
34. ALERTING + NOTIFICATIONS
======================================================================

There are historically multiple parallel systems:

services/alerting
services/notification
services/notifications.

Consolidate responsibilities.

Implement:

event
→ rule evaluation
→ alert state
→ notification job
→ channel
→ delivery
→ retry
→ dead letter.

Repair:

- no-op email
- duplicate alerts
- acknowledgement suppression bugs
- tenant/user filtering
- N+1 queries
- webhook serialization
- Slack field naming
- blocking/leaking notification WebSockets
- non-durable dispatch.

======================================================================
35. AUDIT / ACTIVITY
======================================================================

Activity must be durable where required.

Fix:

- filters being parsed but ignored
- CSV export hard limit
- in-memory-only loss
- ring buffer truncation
- fragile dynamic SQL
- audit writes whose errors are silently ignored.

Audit records must preserve:

actor
tenant
resource
action
timestamp
request ID
result
metadata.

======================================================================
36. LOGGING
======================================================================

Consolidate logging middleware.

Use one coherent request ID.

Preserve context propagation.

Use OriginalURL where required rather than misleading route paths.

Close file handles on reconfiguration.

Do not leak secrets.

Make logs useful for distributed debugging.

======================================================================
37. I18N
======================================================================

Finish actual i18n integration.

Frontend useTranslation must be connected.

Backend translation helper must be connected.

Unify locale lists.

Resolve:

pt
vs
pt-BR.

Support interpolation consistently.

Avoid frontend `{0}` versus backend fmt.Sprintf mismatch.

Do not silently flatten arrays/numbers/booleans away.

Cache loaded translations.

Avoid duplicate i18n endpoints where unnecessary.

Replace hardcoded user-facing English progressively.

Never compress or degrade translated user-facing language.

======================================================================
38. FRONTEND DESIGN SYSTEM
======================================================================

Forge needs one unified UI.

The UI should feel premium, technical, industrial, clear and intentional.

Use:

Manrope
for interface typography.

JetBrains Mono
for:

- terminal
- logs
- IDs
- technical values
- code
- diagnostics.

Use the existing CSS/token system as the current source of truth.

Do not introduce random new palettes.

Avoid:

- generic SaaS-dashboard aesthetic
- excessive gradients
- excessive glow
- giant cards
- rainbow statuses
- random purple
- glassmorphism everywhere
- decorative badges
- fake AI visual language
- excessive pills
- unnecessary borders.

Brand red is a meaningful Forge identity/critical-action/destructive color, not a generic highlight sprayed everywhere.

Status colors should remain semantic.

Use progressive disclosure.

======================================================================
39. PAGE COMPOSITION
======================================================================

Establish unified page primitives.

Conceptually:

ForgePage
ForgePageHeader
ForgeSection
ForgeInfo
ForgeCard
ForgePanel
ForgeToolbar
ForgeTabs
ForgeTable
ForgeList
ForgeEmptyState
ForgeErrorState
ForgeLoadingState
ForgeStatus
ForgeBadge
ForgeButton
ForgeIconButton
ForgeInput
ForgeSelect
ForgeCombobox
ForgeDialog
ForgeConfirmDialog
ForgeDrawer
ForgePopover
ForgeTooltip
ForgeCommandMenu
ForgeSearch
ForgeCode
ForgeMetric
ForgeChart
ForgeTimeline
ForgeTerminal
ForgeConsole
ForgeResourcePicker.

Do not create multiple visually unrelated versions of the same primitive.

======================================================================
40. INFORMATION / "i" DISCLOSURE
======================================================================

Major pages and sections should provide concise explanations.

Use the existing page-info-disclosure pattern if it is correct, or improve it.

An information disclosure should be:

- discoverable
- keyboard accessible
- screen-reader accessible
- concise
- contextual
- progressive.

Do not replace documentation with giant blocks of prose.

Explain concepts at the point of use.

======================================================================
41. DIALOG SYSTEM
======================================================================

Dialogs are a core Forge UI primitive.

Standard pattern:

title
short explanation
content
validation
actions.

Support:

- loading
- validation
- errors
- async execution
- keyboard navigation
- accessibility
- focus management
- cancellation
- progress
- result state.

Complex workflows may become:

context
→ configuration
→ advanced
→ preview/diff
→ confirmation
→ execution
→ result.

Do not reproduce the same workflow differently in every page.

======================================================================
42. SIDEBAR / INFORMATION ARCHITECTURE
======================================================================

Use goal-oriented navigation rather than backend package names.

Target structure:

COMMAND
- Overview
- Monitoring
- Health
- Activity

BUILD
- Servers
- Applications
- Catalog

DEPLOY
- Deployments
- Pipelines
- Git
- Preview Deployments

INFRASTRUCTURE
- Nodes
- Regions
- Locations
- Hosts
- Runtime
- Storage

NETWORK
- Domains
- Endpoints
- Traffic
- Load Balancing
- Networking

DATA
- Databases
- Backups

OPERATIONS
- Migrations
- Reconciliation
- Operations
- Installation
- Cleanup
- Orphan Remediation

AUTOMATION
- Schedules
- Procedures
- Scaling
- Failover

ACCESS
- Users
- Roles
- Auth
- Identity

TENANCY
- Organizations
- Projects
- Environments

PLATFORM
- Integrations
- Plugins
- API Keys
- Notifications
- Settings.

Treat this as a conceptual information architecture.

Adjust based on real current functionality and role context.

Group technical depth under understandable parent concepts.

======================================================================
43. OVERVIEW
======================================================================

Overview is the reference implementation for truthful Forge UX.

Trace every metric individually.

Examples:

nodes
workloads
health
capacity
alerts
activity
deployments
operations.

Each metric must have a documented source in code during investigation.

Never silently turn API failures into zero.

Never turn missing telemetry into healthy.

Show stale/unknown/unavailable states.

Show divergence between desired and actual state.

Provide useful navigation from metrics into the relevant resource.

Overview should become the quality standard for the rest of Forge.

======================================================================
44. CLIENT CONSOLE
======================================================================

Forge console should be production-quality.

Support as appropriate:

- command input
- output stream
- timestamps
- connection state
- reconnecting
- offline
- clear
- scroll
- copy
- search
- fullscreen
- resize
- keyboard shortcuts
- history
- permission state
- workload state
- loading
- errors.

Never show connected when the WebSocket isn't actually connected.

Never silently drop console output.

======================================================================
45. WORKLOAD UX
======================================================================

Every workload should use a common shell.

Common tabs can include:

Overview
Deploy
Runtime
Resources
Networking
Storage
Databases
Environment
Backups
Logs
Activity
Settings

Specialized tabs:

Game server:
Console
Files
Players
Startup
Variables
Schedules

App:
Source
Build
Deployments
Domains

Database:
Connections
Backups
Users
Configuration.

The shell must expose real capabilities, not decorative navigation.

======================================================================
46. APPLICATION PLATFORM FEATURES
======================================================================

Use Coolify/Dokploy/Dokku/CapRover/Portainer/Komodo/1Panel/Uncloud as behavioral and architectural references.

Implement the meaningful platform capabilities relevant to Forge:

- source deployment
- Git
- buildpacks
- Dockerfile builds
- compose
- environment variables
- secrets
- deployments
- revisions
- rollback
- domains
- TLS
- health checks
- preview environments
- logs
- runtime
- networking
- volumes
- scheduled tasks
- scaling.

Translate the patterns into Forge.

======================================================================
47. GAME SERVER FEATURES
======================================================================

Use Pterodactyl/Pelican/PufferPanel reference behavior.

Implement:

- server lifecycle
- console
- files
- SFTP where intended
- startup configuration
- variables
- player information
- schedules
- backups
- resource allocation
- templates/eggs
- migration
- recovery
- limits
- permissions.

Actual runtime behavior is mandatory.

======================================================================
48. BACKUP REFERENCE BEHAVIOR
======================================================================

Study Restic and Kopia for:

- chunking
- deduplication
- encryption
- repository behavior
- retention
- verification
- snapshots
- restore
- integrity.

Implement the useful concepts in Forge's own architecture.

======================================================================
49. NETWORK REFERENCE BEHAVIOR
======================================================================

Study:

Caddy
Traefik
Nginx Proxy Manager
NetBird.

Use their architecture and UX patterns to implement:

- gateways
- routing
- certificates
- TLS
- service discovery
- endpoint management
- traffic
- mesh
- security
- policy.

======================================================================
50. ORCHESTRATION REFERENCE BEHAVIOR
======================================================================

Study:

Nomad
Incus
Rancher
Longhorn
River
NetBird.

Translate their useful concepts into Forge:

- job scheduling
- cluster state
- workloads
- nodes
- drain
- rescheduling
- storage locality
- replication
- reconciliation
- service discovery
- cluster health
- agent management.

======================================================================
51. REFERENCE FOLDER WORKFLOW
======================================================================

Graphify/index the reference folder.

Connect reference repositories to their relevant Forge subsystems.

Build an internal mapping like:

Coolify
→ application deployment UX
→ Forge Applications/Deployments

Pterodactyl/Pelican
→ game workload UX
→ Forge Game Server workload

Restic/Kopia
→ backup architecture
→ Forge Backup subsystem

Caddy/Traefik/NPM
→ networking/gateway
→ Forge Network subsystem

Nomad
→ scheduling/orchestration
→ Forge Scheduler

Incus
→ virtualization/container lifecycle
→ Forge runtime/infrastructure

NetBird
→ mesh/network connectivity
→ Forge networking

Longhorn
→ distributed persistent storage
→ Forge storage

Rancher
→ cluster management
→ Forge infrastructure.

Use the references to improve the real Forge implementation.

======================================================================
52. ORPHAN / DUPLICATION HUNT
======================================================================

Use Graphify and repository search to find:

- dead exports
- dead routes
- duplicate services
- duplicate models
- duplicate notification systems
- duplicate metrics systems
- duplicate migration systems
- duplicate logging middleware
- duplicate i18n systems
- unused SDK
- unused UI package
- unused game template package
- old API functions
- stale frontend routes
- obsolete feature flags
- no-op handlers
- TODO stubs
- placeholder implementations
- simulated workers
- fallback code.

Do not delete something simply because it is unused.

Determine whether it is:

dead
unfinished
future-facing
legacy
compatibility
duplicate
intentionally internal.

Then consolidate intentionally.

======================================================================
53. API CONTRACT MATRIX
======================================================================

Build a complete mapping:

Frontend function
→ HTTP method
→ URL
→ backend route
→ handler
→ auth
→ scope
→ validator
→ response type
→ shared type
→ consumer.

Find every mismatch.

Find every backend route with no frontend consumer where a UI is expected.

Find every frontend API function without a backend route.

Find every response that does not match its declared type.

Find every request body that does not match backend validation.

Fix systematically.

======================================================================
54. BUILD / COMPILE FIRST
======================================================================

Before high-level feature work, establish a clean baseline.

At minimum verify:

- root install
- package manager
- TypeScript
- frontend build
- Go build
- Beacon build
- Go vet
- lint where configured
- shared package build
- game-template build
- SDK build
- UI build
- API build.

Historical build failures include:

- incorrect registerEnhancedNotificationRoutes arguments
- unused imports in handlers_admin.go
- missing Beacon/internal/activity
- missing Beacon/internal/logo
- missing Beacon/internal/contextbag
- missing Beacon/internal/errors
- missing workerpool dependency.

Re-verify against current code.

Fix build blockers.

Do not move past broken compilation because “the UI can be tested later”.

======================================================================
55. TEST STRATEGY
======================================================================

Create confidence across layers.

Frontend:

- API client
- query functions
- hooks
- state
- important components
- dialogs
- forms
- permissions
- error states
- Overview
- Monitoring
- Health
- Activity
- workload pages.

Backend:

- handlers
- middleware
- stores
- services
- queue
- scheduler
- runtime
- Beacon
- events
- notifications
- migrations
- security boundaries.

Integration:

Frontend
→ API
→ PostgreSQL
→ queue
→ Beacon
→ runtime
→ actual state
→ event
→ frontend.

Use real integration where possible.

Do not make tests pass by weakening production code.

Do not write tests against mocks when an integration contract is what matters.

Fix broken test infrastructure too.

Historical issues include:

- PostgreSQL test authentication
- hardcoded DB credentials
- SQLite incompatible migration
- low frontend coverage
- many backend packages with zero tests.

======================================================================
56. CI/CD
======================================================================

Create reliable CI/CD.

CI should detect at minimum:

- frontend build
- frontend tests
- backend build
- Beacon build
- lint
- vet
- migrations
- package builds
- type checking
- relevant integration tests
- security checks
- Docker build.

Add image build/push only where environment supports it.

Deploy pipelines must be reproducible.

Migration validation must run automatically.

Rollback must restore both application version and infrastructure state safely.

Never destroy database volumes as a rollback mechanism.

======================================================================
57. DOCKER / COMPOSE
======================================================================

Audit and repair:

infra/compose.yml
Dockerfiles
.dockerignore
networks
healthchecks
resource limits
secrets
TLS
Redis
Postgres
Beacon
Soketi
Traefik/Caddy
backup containers.

Historical problems include:

- Beacon resource limits
- missing Soketi healthcheck
- host networking
- SFTP binding
- missing migration job
- port collisions
- Caddy/Traefik port conflicts
- Docker socket exposure
- backup container missing AWS CLI
- plaintext dev secrets
- root dockerignore omissions
- unnamespaced networks.

Verify each.

======================================================================
58. KUBERNETES
======================================================================

Audit:

- secret management
- ingress
- probes
- resources
- Redis auth
- PostgreSQL probes
- Beacon host networking
- hostPort
- configuration
- TLS
- readiness
- liveness
- persistence.

Do not keep placeholder production secrets.

Do not hardcode example domains in a production configuration.

======================================================================
59. DEPLOYMENT / UPGRADE / ROLLBACK
======================================================================

Verify scripts:

status
start-dev
deploy
deploy-prod
rollback
upgrade
diagnose
test_migrations.

Historical problems include:

- wrong deploy directory
- quote stripping
- health endpoint mismatch
- rollback restoring env but not image versions
- compose --wait assumptions
- rollback assumptions around GHCR tags
- upgrade volume destruction
- diagnostics relative-path assumptions
- self-referencing migration test loop.

Fix them.

Rollback must be safe.

======================================================================
60. SECRETS
======================================================================

Use secure secret handling.

Forge already has strong keyring foundations.

Preserve:

- AES-GCM
- envelope encryption
- rotation
- constant-time comparisons
- hashed tokens.

Fix unsafe development/production transitions.

Ephemeral master key mode must never unexpectedly destroy the ability to decrypt persisted secrets after restart.

Make production secret requirements explicit.

Never log secrets.

======================================================================
61. PRODUCT COMPLETENESS
======================================================================

Forge is intended to be a unified control plane capable of managing:

- game servers
- applications
- containers
- services
- databases
- VMs where supported
- cloud instances
- bare metal where supported
- Kubernetes
- Nomad-style orchestration
- storage
- networking
- DNS
- domains
- TLS
- load balancing
- reverse proxy
- monitoring
- backups
- migrations
- recovery
- failover
- autoscaling
- auto-healing
- Git deployments
- preview environments
- pipelines
- integrations
- plugins
- users
- teams
- organizations
- projects
- environments
- billing-related capabilities where actually implemented.

Review the reference ecosystem for feature gaps.

Implement the meaningful feature set that fits Forge's architecture.

======================================================================
62. BILLING / TENANCY / QUOTAS
======================================================================

Billing and quotas must be functional if exposed.

Do not leave:

quota
metering
allocation
usage
billing

as decorative fields.

Ensure:

tenant
→ project
→ environment
→ workload

boundaries are enforced.

Metering must derive from real observed usage where appropriate.

Billing webhooks must have secure verification.

======================================================================
63. PLUGINS / INTEGRATIONS
======================================================================

Plugin lifecycle must be real.

Integrations must be real.

Implement:

- install
- configure
- enable
- disable
- remove
- permissions
- secrets
- health
- version
- failures.

Do not show plugin controls when no execution path exists.

======================================================================
64. SETTINGS
======================================================================

Settings must reflect real behavior.

Remove redundant representations.

Panel settings must have one canonical representation.

Changes need:

- validation
- persistence
- cache invalidation
- reload behavior where needed
- audit event
- error state
- permission enforcement.

======================================================================
65. FRONTEND STATE MANAGEMENT
======================================================================

Use TanStack Query coherently.

Each query needs:

- canonical queryKey
- correct identifiers
- stale time
- invalidation
- error handling
- loading behavior
- optimistic updates only where safe
- mutation lifecycle
- realtime synchronization where appropriate.

Do not create literal duplicated query keys throughout the app.

Do not return stale desired state as actual state.

======================================================================
66. PERFORMANCE
======================================================================

Avoid:

- N+1 queries
- giant page payloads
- unbounded client state
- redundant polling
- redundant health calls
- duplicated history queries
- excessive WebSocket subscriptions
- unnecessary rerenders
- huge context dumps.

Use:

- pagination
- batching
- caching
- proper indexes
- selective queries
- streaming
- incremental loading.

======================================================================
67. ACCESSIBILITY
======================================================================

Every UI surface must be usable by keyboard and assistive technology.

Verify:

- buttons
- dialogs
- drawers
- forms
- labels
- tables
- tabs
- tooltips
- navigation
- focus
- error messages
- color contrast
- reduced motion.

Never encode essential state using color alone.

======================================================================
68. RESPONSIVE UX
======================================================================

Forge must work on:

- desktop
- laptop
- tablet
- narrow displays.

Do not merely shrink desktop dashboards.

Reflow intelligently.

Preserve critical operational controls.

Console and tables need intentional mobile behavior.

======================================================================
69. ERROR UX
======================================================================

Every major operation needs truthful:

loading
empty
zero
unavailable
stale
permission denied
validation failed
network error
backend error
timeout
retrying
success
failure
needs attention.

Do not use:

try
catch
return zero

for failed observability.

Do not hide errors.

Do not expose raw backend internals unnecessarily.

Use structured user-facing error messages while retaining diagnostics in logs.

======================================================================
70. SECURITY + AUTHORIZATION TESTS
======================================================================

For every critical route test:

- unauthenticated
- authenticated wrong role
- authenticated wrong scope
- wrong tenant
- wrong project
- wrong environment
- valid authorized user
- replay where applicable
- expired credential
- CSRF
- malformed input
- malicious path
- SSRF
- oversized input.

======================================================================
71. ZERO FALSE COMPLETION POLICY
======================================================================

This is absolute.

Forbidden patterns include:

return nil
after doing nothing

event:
deployment.completed

without execution

DB:
status = running

without runtime verification

metrics:
0

on query failure

health:
healthy

on missing data

backup:
verified

without verification

restore:
succeeded

without verification

cloud:
provisioned

without instance confirmation

node:
ready

without Beacon heartbeat

queue:
queued

without consumer.

Replace these with explicit real states.

======================================================================
72. RECONCILIATION
======================================================================

Reconciliation is the bridge between desired and actual state.

Use it intentionally.

For each supported workload:

desired
vs
actual.

Reconciler must:

- detect divergence
- produce a plan
- execute plan
- observe result
- persist actual state
- resolve divergence
- retry failures
- surface attention state.

Do not use reconciliation to mask broken direct execution.

======================================================================
73. RECOVERY / FAILOVER
======================================================================

Recovery plans must be real.

Failover must be real.

Support:

- node loss
- workload loss
- backup restore
- placement
- rehydration
- ownership transfer
- DNS/routing changes where appropriate
- verification
- cleanup.

No failover should claim completion without post-failover verification.

======================================================================
74. AUTOSCALING
======================================================================

Distinguish:

vertical scaling
horizontal scaling
scheduler placement.

Implement actual resource changes.

Implement hysteresis/cooldown where needed.

Persist scaling decisions.

Observe the runtime after scaling.

Do not emit “scaled” merely because the DB record changed.

======================================================================
75. HEALTH GATES
======================================================================

Deployments and failover may use health gates.

Health must target the actual workload/node.

Do not health-check the wrong host.

Support:

HTTP
TCP
command/runtime health where appropriate.

Persist health history.

Make stale data explicit.

======================================================================
76. FILESYSTEM / FILE MANAGER
======================================================================

File operations must be properly isolated.

Audit:

- path normalization
- rootfs jail
- openat2
- symlink escapes
- host-wide paths
- upload
- download
- rename
- delete
- archive extraction
- permissions
- SFTP.

Preserve the existing openat2/rootfs protections.

Do not weaken them for convenience.

======================================================================
77. GIT / SOURCE DEPLOYMENT
======================================================================

Source deployment must support:

- repository
- branch
- tag
- commit SHA
- authentication
- deploy key
- shallow clone
- secure checkout
- build
- artifact
- deploy
- revision
- rollback.

Never allow arbitrary SHA handling to become an authorization or repository escape mechanism.

Do not leak deploy credentials.

======================================================================
78. COMPOSE
======================================================================

Compose should not exist as a disconnected side feature.

It must integrate with:

- workload model
- deployments
- services
- networking
- volumes
- environment
- health
- operations
- rollback
- logs.

Avoid duplicate stack rows after redeploy.

Verify actual runtime.

======================================================================
79. API / OPENAPI CONTRACT
======================================================================

Where OpenAPI exists, reconcile it with actual routes.

Do not maintain three incompatible truths:

Go routes
OpenAPI
frontend API.

Use one canonical contract model where practical.

Generate types from a trusted source if architecture supports it.

======================================================================
80. REFERENCE-DRIVEN UI QUALITY
======================================================================

Do not imitate the visual identity of reference products.

Instead study:

- information density
- navigation
- lifecycle flows
- error recovery
- status presentation
- terminal UX
- files UX
- deployment flows
- networking flows
- backup flows
- orchestration flows
- scheduling flows.

Translate those ideas into Forge.

Forge must remain visually and conceptually Forge.

======================================================================
81. NO FEATURE THEATER
======================================================================

If a feature cannot actually work in the current environment:

- identify the real missing dependency
- implement the missing layer when it is part of the mission
- otherwise expose truthful capability state.

Do not create fake buttons.

Do not create disabled controls everywhere as a substitute for implementation.

Do not fake API responses.

Do not fake worker completion.

Do not fabricate sample nodes/resources as live data.

======================================================================
82. CODE QUALITY
======================================================================

Favor:

- clear ownership
- cohesive modules
- typed contracts
- context propagation
- explicit errors
- deterministic behavior
- idempotency
- testability
- observability
- minimal duplication.

Avoid:

- hidden global state
- magic constants
- silent fallback
- swallowed errors
- ignored return values
- giant handlers
- duplicate services
- package-level circular dependencies
- accidental dependency hoisting.

======================================================================
83. WORK SEQUENCING
======================================================================

Do NOT attempt random parallel changes across every subsystem.

Use controlled dependency order.

Recommended macro-sequence:

PHASE 0
Environment and tool verification

PHASE 1
Repository inventory + Graphify knowledge graph

PHASE 2
Build blockers

PHASE 3
Security-critical boundaries

PHASE 4
Canonical data models and shared contracts

PHASE 5
API route/client contract alignment

PHASE 6
Queue/operation infrastructure

PHASE 7
Scheduler/placement/reconciliation

PHASE 8
Beacon/runtime execution

PHASE 9
Events/realtime/observability

PHASE 10
Database/migrations

PHASE 11
Networking/storage/backups

PHASE 12
Cloud/infrastructure

PHASE 13
Frontend data layer

PHASE 14
Unified UI system

PHASE 15
Workload UX

PHASE 16
Product-wide feature completion

PHASE 17
Testing + CI/CD

PHASE 18
Full end-to-end verification.

Within each phase:

discover
→ map
→ implement
→ test
→ integrate
→ verify
→ continue.

======================================================================
84. DO NOT STOP AT THE FIRST WORKING LAYER
======================================================================

Example:

If API endpoint compiles:
continue.

If service works:
continue.

If queue starts:
continue.

If Beacon receives:
continue.

If runtime executes:
continue.

If actual state is updated:
continue.

If event publishes:
continue.

If frontend receives:
continue.

If error/retry/reconnect works:
continue.

Only then is the feature end-to-end complete.

======================================================================
85. CHANGE MANAGEMENT
======================================================================

For every significant change maintain an internal reasoning record containing:

- problem
- current behavior
- desired behavior
- affected layers
- files
- dependencies
- test strategy
- implementation
- verification evidence.

Use Context Mode storage for durable investigation context when useful.

Use Graphify for relationship lookup.

Do not create documentation files merely to claim progress.

Code is the deliverable.

======================================================================
86. ISSUE REGISTRY — HISTORICAL FINDINGS
======================================================================

Treat the following as historical high-value investigation anchors.

They must be re-verified against current source.

They are not permission to blindly change code.

----------------------------------------------------------------------
SECURITY / AUTH
----------------------------------------------------------------------

- subuser to privileged execution / cron immediate-run risk
- console/stats/log WebSocket auth mismatch
- legacy transfer credential exposure
- mTLS production guard/environment mismatch
- installer WebSocket authorization hole
- webhook HMAC configuration impossibility
- HMAC replay concern
- compose quota/node authorization bypass
- nonce eviction concerns
- development token fallback
- host-wide root file API
- SFTP public interface exposure
- plugin mutation authorization
- missing route scopes
- ignored IP restrictions
- tenant filter bypass/ignored filters
- upgrade SSRF concerns
- DNS rebinding concerns
- firewall authorization concerns.

----------------------------------------------------------------------
DEPLOYMENTS / EXECUTION
----------------------------------------------------------------------

- deployment step stubs
- false completion
- DB-only promotion
- wrong health target
- app desired-state-only start/stop
- RestartApp delegated incorrectly
- empty image path
- duplicate Compose stack records
- incomplete Docker admin paths
- deploy key leakage
- arbitrary SHA handling
- BuildType mismatch
- cacheFrom/cacheTo loss
- platform loss
- post-hoc build logs
- preview subsystem mismatch
- disconnected revisions/placement.

----------------------------------------------------------------------
ASYNC
----------------------------------------------------------------------

- two queue systems
- orphan queued operations
- unregistered job types
- no DLQ
- lease/retry concerns
- retry double increment
- in-memory schedulers
- reaper re-execution
- recovery attempt state
- buildpack bare goroutine
- source deployment pending record without worker
- app deployment pending path
- server installation without worker
- transfer without worker
- backup split execution
- promotion without durable worker.

----------------------------------------------------------------------
SCHEDULER / INFRASTRUCTURE
----------------------------------------------------------------------

- online vs active state mismatch
- first-node fallback
- overallocate fields ignored
- incomplete drain UI/wiring
- hidden autoscaling ledger
- horizontal vs vertical scaling separation
- weak storage locality
- migration instead of replicated storage
- no proper mesh
- partial cloud provider support
- cloud instance without Forge node.

----------------------------------------------------------------------
OBSERVABILITY
----------------------------------------------------------------------

- duplicate metrics systems
- metrics reset
- unbounded duration maps
- no histogram buckets
- GC pause issue
- alert email no-op
- duplicate alerting systems
- acknowledged critical alert suppression bug
- fire-and-forget alert dispatch
- N+1 alert rule queries
- duplicate notification systems
- tenant/user notification filtering ignored
- blocking WS notification
- crash-state fabricated timestamps
- crash-state no eviction
- crash race
- in-memory loss on restart
- heartbeat persistence failures
- "{}" 200 on monitoring failure
- duplicated heartbeat history query
- synchronous EvaluateAll
- missing retention
- audit filter ignored
- CSV hard limit
- in-memory audit loss
- duplicate request logging
- context propagation split
- event dead-letter silence
- event failure double count
- relay/subscriber disconnect.

----------------------------------------------------------------------
DATABASE
----------------------------------------------------------------------

- multiple migration runners
- primary/batch2 schema tracking collision
- migration numbering gaps
- missing rollbacks
- dialect divergence
- SQLite PostgreSQL-only migrations
- eventstore migration path
- Beacon migration separation
- competing models
- string vs numeric ID mismatch
- JSON naming mismatch
- swallowed audit writes
- ignored unmarshal errors
- fragile table-name interpolation.

----------------------------------------------------------------------
SHARED PACKAGES
----------------------------------------------------------------------

- missing shared response types
- no PaginatedResponse
- duplicate response fields
- duplicate aliases
- snake_case exceptions
- any payloads
- SDK with no runtime client
- missing SDK exports
- UI missing runtime dependency
- undeclared dependencies
- missing client directives
- icon placeholders
- accessibility gaps
- table generic assuming id
- game template package with no usable TS output
- registry metadata gaps
- build script omitting game templates
- package consumers/orphans.

----------------------------------------------------------------------
FRONTEND API
----------------------------------------------------------------------

- hardcoded SSR URL
- unused WebSocket URL
- no fetch timeout
- no retries
- inconsistent response envelope handling
- raw fetch inconsistencies
- SSH key DELETE mismatch
- email change HTTP method mismatch
- backup UUID/name mismatch
- migration endpoint mismatch
- evacuation endpoint mismatch
- frontend/backend route mismatches across modules
- i18n relative path behavior
- API client/type drift.

----------------------------------------------------------------------
I18N
----------------------------------------------------------------------

- unused useTranslation
- unused backend translation helper
- incomplete locales
- pt vs pt-BR
- multiple locale lists
- interpolation mismatch
- cookie mismatch
- panic possibility with no locale
- duplicate endpoint
- no cache
- hardcoded English
- lossy flattening.

----------------------------------------------------------------------
INFRA / CI
----------------------------------------------------------------------

- no GitHub workflows
- no registry push
- migration validation not in CI
- scripts with partial coverage
- lint tool assumptions
- resource limits
- healthchecks
- port collisions
- Caddy/Traefik conflict
- host networking
- Docker socket
- plaintext dev secrets
- Redis auth inconsistencies
- backup tool missing
- dockerignore gaps
- missing start_period
- K8s placeholder secrets
- hardcoded ingress
- no API-aware web readiness
- no resource limits
- unsafe upgrade rollback
- broken scripts.

======================================================================
87. FEATURE COMPLETION STANDARD
======================================================================

A feature is COMPLETE only if:

1. Domain model exists and is coherent.
2. Database schema exists and is migrated safely.
3. Backend service exists.
4. API route exists.
5. Authorization exists.
6. Validation exists.
7. Shared type exists.
8. Frontend API client exists.
9. Query/mutation state works.
10. UI exists.
11. Loading state exists.
12. Empty/zero state exists.
13. Error state exists.
14. Permission state exists.
15. Async operation is durable where needed.
16. Queue worker exists where needed.
17. Scheduler/placement integration exists where needed.
18. Beacon integration exists where needed.
19. Runtime execution exists.
20. Actual state is observed.
21. Actual state is persisted.
22. Events/realtime update consumers where needed.
23. Audit/activity exists where required.
24. Retry/recovery exists where required.
25. Tests cover the behavior.
26. Build/lint/typecheck pass.
27. No fake completion remains.

If any of those are missing, the feature is not fully complete.

======================================================================
88. VERIFICATION MATRIX
======================================================================

For every repaired subsystem prove:

BUILD
- compiles
- typechecks
- lints.

UNIT
- important pure behavior.

INTEGRATION
- API ↔ service ↔ DB.

EXECUTION
- queue ↔ scheduler ↔ Beacon ↔ runtime.

OBSERVATION
- actual state returned and persisted.

REALTIME
- events reach frontend.

RECOVERY
- restart does not corrupt durable state.

SECURITY
- authz/tenant boundaries hold.

UX
- loading/error/empty/stale states.

ACCESSIBILITY
- keyboard/screen reader semantics.

PERFORMANCE
- no obvious pathological polling/N+1.

======================================================================
89. PRODUCTION TRUTH
======================================================================

The application must be safe to operate.

That means:

- no destructive hidden fallbacks
- no unsafe default credentials
- no misleading “healthy”
- no silent dropped events
- no invisible failed jobs
- no irreversible rollback behavior
- no credentials leaked to clients
- no privilege escalation through UI/API gaps.

======================================================================
90. AUTONOMOUS EXECUTION
======================================================================

You are expected to execute this mission, not return an audit report as the main deliverable.

Do not respond:

“I found these problems; someone should fix them.”

Fix them.

Do not stop after discovering a problem.

Trace it.

Implement the correction.

Test it.

Continue.

Do not ask for permission to perform ordinary repository investigation or code changes that are clearly within this mission.

Ask for user input only when an external irreversible/destructive action, credential, missing external service, or genuinely ambiguous product decision makes continuation unsafe.

Otherwise continue autonomously.

======================================================================
91. HANDLING MISSING EXTERNAL DEPENDENCIES
======================================================================

If a feature requires something unavailable:

- identify the exact dependency
- determine whether a local implementation is appropriate
- determine whether an existing Forge abstraction already supports it
- implement the integration if practical
- add truthful capability detection
- avoid mocks.

Do not fake external infrastructure.

======================================================================
92. FINAL SYSTEM SHAPE
======================================================================

The target architecture is conceptually:

                        FORGE CONTROL PLANE

Frontend
  ↓
Canonical API client
  ↓
Canonical shared contracts
  ↓
Go/Fiber API
  ↓
Auth / RBAC / Tenancy
  ↓
Domain Services
  ↓
PostgreSQL
  ↓
Durable Operations
  ↓
Queue / Scheduler / Placement
  ↓
Beacon
  ↓
Runtime
  ↓
Actual State
  ↓
Observation
  ↓
Events
  ↓
Realtime
  ↓
Frontend.

Supporting planes:

- Networking
- Storage
- Backup
- Cloud
- DNS
- Certificates
- Monitoring
- Alerting
- Notifications
- Audit
- Recovery
- Failover
- Automation
- Integrations.

======================================================================
93. SUCCESS CONDITION
======================================================================

Do not define success as:

“all pages exist”

or:

“all endpoints respond”

or:

“build passes”.

Success is:

Forge can actually manage its supported infrastructure through one coherent control plane.

Users can create resources.

Users can deploy workloads.

Users can run workloads.

Users can stop/restart/migrate workloads.

Users can configure networking.

Users can configure domains and certificates.

Users can inspect health.

Users can inspect real telemetry.

Users can use console/files where supported.

Users can create and restore backups.

Users can perform migrations.

Users can recover workloads.

Users can operate nodes.

Users can scale workloads.

Users can schedule work.

Users can observe operations.

Users can understand failures.

Users can recover from failures.

All of this obeys:

auth
RBAC
tenancy
quotas
security
durability
actual-state verification.

======================================================================
94. FINAL VALIDATION GATE
======================================================================

Before declaring completion, perform a whole-system verification.

At minimum prove real end-to-end paths for:

NODE
create/register
→ heartbeat
→ healthy
→ capacity
→ scheduler visibility.

WORKLOAD
create
→ placement
→ provision
→ Beacon
→ runtime
→ actual state.

LIFECYCLE
start
→ runtime
→ actual state
→ event
→ frontend.

DEPLOYMENT
source
→ build
→ revision
→ deployment
→ health
→ promotion
→ actual runtime.

NETWORK
workload
→ endpoint
→ domain
→ certificate
→ gateway
→ reachable workload.

BACKUP
create
→ storage
→ verification
→ list
→ restore
→ verification.

MIGRATION
plan
→ reserve
→ transfer
→ restore
→ runtime
→ verify
→ ownership switch.

FAILOVER
failure
→ detection
→ plan
→ placement
→ recovery
→ verification.

MONITORING
Beacon
→ metric/heartbeat
→ persistence
→ aggregation
→ API
→ frontend.

EVENT
domain event
→ store
→ relay
→ subscriber
→ frontend.

AUTH
unauthorized
→ denied.

Wrong tenant
→ denied.

Wrong scope
→ denied.

Correct authorization
→ succeeds.

======================================================================
95. FINAL RULE
======================================================================

NEVER confuse code volume with completeness.

NEVER confuse UI presence with capability.

NEVER confuse database mutation with runtime execution.

NEVER confuse queued with successful.

NEVER confuse desired state with actual state.

NEVER confuse missing telemetry with zero.

NEVER confuse absent errors with success.

NEVER add another parallel subsystem when an existing one can be repaired.

NEVER leave a known incomplete path merely because it looks implemented.

NEVER discard existing user work.

NEVER fabricate verification.

BUILD THE REAL FORGE.

Make the entire repository coherent.

Make the control plane real.

Make Beacon real.

Make runtime execution real.

Make networking real.

Make domains real.

Make storage real.

Make backups real.

Make orchestration real.

Make Nomad-class scheduling capabilities real.

Make workloads real.

Make async execution durable.

Make state truthful.

Make the API contracts canonical.

Make the shared packages useful.

Make the UI one unified Forge system.

Make the frontend reflect actual system truth.

Make the platform testable.

Make the platform operable.

Make the platform secure.

Do the work end-to-end.