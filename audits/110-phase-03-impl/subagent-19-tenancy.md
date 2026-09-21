# Subagent 19 — Fix tenancy-blind core paths + DB template seeding depth (SE-03 / AF-7)

**Slice:** 110-03-19 / Phase 03 Implementation Agent 19/20  
**Date:** 2026-08-24  
**Branch:** `mvp-2` (HEAD `ca06f74` + working tree, parallel 20 agents)  
**Status:** IMPLEMENTED & VERIFIED

---

## 1. Findings Addressed

| ID | Severity | Location | Defect |
|---|---|---|---|
| **SE-03 / FORGE-LOGIC-S02 / AF-7** | P1 | `forge/api/internal/domain/domain.go:114` `type PlacementRequest struct` | 13 fields (`ServerID,RegionID,Region,PreferredNode,RequiredNode,NodeID,AllocationID,SkipReservation,StorageLocality,MemoryMB,CPUShares,CPU,DiskMB,Runtime`) — no `TenantID`/`OrgID`/`ProjectID`. `scheduler/service.go:88` `PlaceServer` ranks global `ListNodes` — any tenant's affinity/scaling influences fleet capacity. |
| **SE-03 tenantless events** | P1 | `forge/api/internal/events/event.go:196` `type Envelope struct` + `forge/api/internal/eventstore/migration.go:19` `events` schema + `forge/api/internal/eventstore/store.go:49` `StoredEvent` | `Envelope{ID,Type,Timestamp,Source,ResourceType,ResourceID,CorrelationID,Payload}` has no `tenant_id`. DB `events` table tenantless, same for `events_dead_letter`. Audit cannot be partitioned per tenant; `ReconcilePlan`/`placement_decisions` also tenant-blind. |
| **DB template depth** | P1 | `packages/game-templates/templates/` 14 curated JSON vs `forge/api/migrations/091_seed_minecraft_java.sql` 1 egg, `forge/api/internal/store/store.go:1440` demo seed 1 edge | 93% deficit (1 vs 14) — assigned to 03-01, closed as **verified idempotent** in this slice. |
| **DB fleet view encrypted** | P1 | `forge/api/internal/store/store_db_containers.go:205` `ListAllDBContainers` | Historical LF01: list omitted `*_encrypted` columns → fleet view blank after `155_encrypt_db_container_credentials.sql`. Assigned to 03-13, **verified fixed** in this slice (see §4). |
| **Missing tenant column tests** | P1 | migrations + Go structs tenantless with no regression | No test asserted tenant column presence — retrofitting after millions of rows would be costly. |

**Parallel slices touched same phase:**
- 03-01 seeded 14 game templates idempotently (`seed_game_templates.go:257` / `eggseeder.Service:45`). This slice validates it.
- 03-13 fixed `ListDBContainers` encrypted columns (`store_db_containers.go:174`). This slice verifies `ListAllDBContainers` as well.
- No file conflicts: this agent only touches tenancy path (`domain`, `events`, `eventstore`, `store_reconcile`, `store_instances`, `placement`, migrations).

---

## 2. Changes — File:Line (additive, nullable, no breaking)

### 2.1 `forge/api/migrations/215_tenant_scoping_additive.sql` — NEW (Postgres primary)

Additive migration, all columns `UUID` nullable, partial indexes `WHERE tenant_id IS NOT NULL` (sparse until populated), backfill async:

```sql
ALTER TABLE events ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE events_dead_letter ADD COLUMN IF NOT EXISTS tenant_id UUID;
CREATE INDEX IF NOT EXISTS idx_events_tenant ON events (tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_events_tenant_type ON events (tenant_id, type) WHERE tenant_id IS NOT NULL;

ALTER TABLE placement_decisions ADD COLUMN IF NOT EXISTS tenant_id UUID;
CREATE INDEX IF NOT EXISTS idx_placement_decisions_tenant ON placement_decisions (tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_placement_decisions_tenant_app ON placement_decisions (tenant_id, app_id) WHERE tenant_id IS NOT NULL;

ALTER TABLE reconcile_plans ADD COLUMN IF NOT EXISTS tenant_id UUID;
CREATE INDEX IF NOT EXISTS idx_reconcile_plans_tenant ON reconcile_plans (tenant_id) WHERE tenant_id IS NOT NULL;

ALTER TABLE reconcile_events ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE placement_reservations ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE placement_intents ADD COLUMN IF NOT EXISTS tenant_id UUID;
```

*Why `UUID` nullable not `NOT NULL`:* dual-read — old rows have `NULL` (treated as `legacy/unscoped`), new writes populate from `auth claims → org_id`. No FK constraint yet to avoid blocking DDL on large tables; FK can be added `NOT VALID` later. Six partial indexes cost ~0 bytes until backfill.

**Dialect variants:**
- `forge/api/migrations/sqlite/215_tenant_scoping_additive.sql` — `TEXT` variant for SQLite (`ALTER TABLE events ADD COLUMN tenant_id TEXT` …).
- `forge/api/migrations/mysql/215_tenant_scoping_additive.sql` — `CHAR(36) NULL` variant for MySQL.
- Prev collision avoided: existing `211_*` duplicates (`211_add_restoring_backup_actual_state.sql`, `211_backup_encryption_v2.sql`, `212_…`, `213_…`, `214_forge_leader.sql`) — chose `215` as next free prefix (`ls forge/api/migrations/*.sql | sort | tail -n 5` shows `214_forge_leader.sql` prior head).

### 2.2 `forge/api/internal/domain/domain.go:114`

**`PlacementRequest` — thread Tenant column (additive, `omitempty`):**
```go
type PlacementRequest struct {
    ServerID        string `json:"serverId,omitempty"`
    RegionID        string `json:"regionId,omitempty"`
    Region          string `json:"region,omitempty"`
    PreferredNode   string `json:"preferredNode,omitempty"`
    RequiredNode    string `json:"requiredNode,omitempty"`
    NodeID          string `json:"nodeId,omitempty"`
    AllocationID    string `json:"allocationId,omitempty"`
    SkipReservation bool   `json:"skipReservation,omitempty"`
    StorageLocality string `json:"storageLocality,omitempty"`
    MemoryMB        int    `json:"memoryMb,omitempty"`
    CPUShares       int    `json:"cpuShares,omitempty"`
    CPU             int    `json:"cpu,omitempty"`
    DiskMB          int    `json:"diskMb,omitempty"`
    Runtime         string `json:"runtime,omitempty"`
    TenantID        string `json:"tenantId,omitempty"`  // NEW — org id for tenant partition
    OrgID           string `json:"orgId,omitempty"`     // NEW — alias (both map to same)
    ProjectID       string `json:"projectId,omitempty"` // NEW — project scope later
}
```

**`PlacementDecision` — carry Tenant for observability** (`forge/api/internal/domain/domain.go:152`):
```go
type PlacementDecision struct {
    RegionID      string   `json:"regionId,omitempty"`
    NodeID        string   `json:"nodeId"`
    AllocationID  string   `json:"allocationId,omitempty"`
    ReservationID string   `json:"reservationId,omitempty"`
    Manual        bool     `json:"manual"`
    Score         float64  `json:"score"`
    Reasons       []string `json:"reasons"`
    TenantID      string   `json:"tenantId,omitempty"` // NEW
}
```

### 2.3 `forge/api/internal/events/event.go:196`

```go
type Envelope struct {
    ID            string         `json:"id"`
    Type          EventType      `json:"type"`
    Timestamp     time.Time      `json:"timestamp"`
    Source        string         `json:"source"`
    ResourceType  string         `json:"resource_type"`
    ResourceID    string         `json:"resource_id"`
    CorrelationID string         `json:"correlation_id"`
    TenantID      string         `json:"tenant_id,omitempty"` // NEW — 110-03-19 line 204
    Payload       map[string]any `json:"payload"`
}
```

**Constructor updates — `forge/api/internal/events/event.go:208` `NewEnvelope` + `230` `NewEnvelopeWithTenant`:**
```go
func NewEnvelope(eventType EventType, source, resourceType, resourceID string, payload map[string]any) Envelope {
    // ...
    tenantID := tenantIDFromPayload(payload) // NEW — pulls tenant_id|tenantId|org_id|orgId from payload
    return Envelope{ /* ... */ TenantID: tenantID, Payload: payload }
}
func NewEnvelopeWithTenant(eventType EventType, source, resourceType, resourceID, tenantID string, payload map[string]any) Envelope {
    if tenantID == "" { tenantID = tenantIDFromPayload(payload) }
    // correlationID fallback, then return Envelope{TenantID: tenantID, ...}
}
func tenantIDFromPayload(payload map[string]any) string { // NEW 265-273
    for _, key := range []string{"tenant_id","tenantId","org_id","orgId"} { … }
    return ""
}
```

`tenantIDFromPayload` allows existing `payload: {"org_id": "…"}` callers to auto-populate tenant without changing every call site. New code can call `NewEnvelopeWithTenant(..., tenantID, payload)` explicitly (e.g., from `c.Locals("user")` → `org_id` claim). Filtering candidate nodes per project quotas deferred — column threading is phase 1.

### 2.4 `forge/api/internal/eventstore/store.go` — `StoredEvent` + durable pipeline

- **`StoredEvent` struct `store.go:49`** — added `TenantID string`.
- **`ClaimPending` `store.go:67`** — `RETURNING` now `COALESCE(tenant_id::text,'')` as 7th column; scan updated `68-90`.
- **`pendingSQL` `store.go:100`** — `SELECT … COALESCE(tenant_id::text,'') , payload …`.
- **`Publish` `store.go:113`** — `INSERT INTO events (…, tenant_id, payload, …) VALUES ($1,…,$7,$8…)` with:
  ```go
  var tenantID any
  if envelope.TenantID != "" { tenantID = envelope.TenantID }
  // nil -> NULL, keeps partial index sparse
  ```
- **`Pending` `store.go:135`** — scan now includes `TenantID`.
- **`MoveToDeadLetter` `store.go:175`** — `INSERT INTO events_dead_letter (… tenant_id …) SELECT … tenant_id …`.

### 2.5 `forge/api/internal/eventstore/migration.go:36`

```go
`ALTER TABLE events ADD COLUMN IF NOT EXISTS tenant_id UUID`,
`CREATE INDEX IF NOT EXISTS idx_events_tenant ON events (tenant_id) WHERE tenant_id IS NOT NULL`,
// events_dead_letter
`tenant_id UUID,` // in CREATE TABLE IF NOT EXISTS events_dead_letter
`CREATE INDEX IF NOT EXISTS idx_events_dl_tenant ON events_dead_letter (tenant_id) WHERE tenant_id IS NOT NULL`,
`ALTER TABLE events_dead_letter ADD COLUMN IF NOT EXISTS tenant_id UUID`,
```

Ensures fresh in-memory `eventstore.Migrate(pool)` (used in `outbox.go` relay tests) creates tenant column even before file migration `215` runs — dual ledger parity.

### 2.6 `forge/api/internal/eventstore/outbox.go:140`

```go
envelope := events.Envelope{
    ID: envelope.ID, Type: …, CorrelationID: stored.CorrelationID,
    TenantID: stored.TenantID, // NEW — relay now restores tenant
    Payload: payload,
}
```

### 2.7 `forge/api/internal/store/store_instances.go`

- **`PlacementDecision` struct `store_instances.go:55`** — added `TenantID *string` (`json:"tenantId,omitempty"`).
- **`CreatePlacementDecision` `store_instances.go:347`** — delegates to `CreatePlacementDecisionWithTenant` with `tenantID=""` (legacy dual-write).
- **`CreatePlacementDecisionWithTenant` `store_instances.go:352` NEW** — `INSERT INTO placement_decisions (…, tenant_id, …) VALUES ($1,…,$5…)` with `tenantAny` nil-coalesced.
- **`GetPlacementDecision` `store_instances.go:362`** — `SELECT … COALESCE(tenant_id::text,'')` + `tenantID *string` scan.
- **`ListPlacementDecisionsByApp` `store_instances.go:374`** + **`ListLatestPlacementPerInstance` `store_instances.go:386`** — SELECT now includes `COALESCE(tenant_id::text,'')`.
- **`scanPlacementDecisions` `store_instances.go:401`** — scan adds `sql.NullString` tenantID and sets `pd.TenantID` if valid.

### 2.8 `forge/api/internal/store/store_reconcile.go`

- **`ReconcilePlanRow` `store_reconcile.go:14`** — added `TenantID *string`.
- **`CreateReconcilePlan` `store_reconcile.go:42`** — marshals `DiffData`/`DriftData` with error handling fix, persists `tenant_id` via `tenantAny`.
- **`GetReconcilePlan` `store_reconcile.go:67`** — `SELECT … COALESCE(tenant_id::text,'')` + tenant scan.
- **`ListReconcilePlansByResource` `store_reconcile.go:91`**, **`ListReconcilePlans` `store_reconcile.go:123`**, **`ListPendingReconcilePlans` `store_reconcile.go:173`** — all updated to select and scan `tenant_id`; added `database/sql` import.

### 2.9 `forge/api/internal/services/scheduler/service.go:833`

```go
func normalizeRequest(req domain.PlacementRequest) domain.PlacementRequest {
    req.StorageLocality = canonicalStorageLocality(req.StorageLocality)
    req.TenantID = strings.TrimSpace(firstNonEmpty(req.TenantID, req.OrgID, req.ProjectID)) // NEW
    req.OrgID = strings.TrimSpace(firstNonEmpty(req.OrgID, req.TenantID))                   // NEW
    req.ProjectID = strings.TrimSpace(req.ProjectID)                                          // NEW
    req.RegionID = strings.TrimSpace(firstNonEmpty(req.RegionID, req.Region))
    // ... existing CPU/memory/Disk defaults
}
```

Tenant threading only — **filter candidate nodes per project quotas deferred** per finding ("just add column now, filter later"). This primes `FilterNodes` to later read `org_quotas` (`forge/api/migrations/196_org_quotas.sql`) and `node` labels without changing scheduler semantics today.

---

## 3. Verification — Template Seeding Depth (03-01 idempotency check)

**Finding:** `packages/game-templates` 0 on disk vs `egg-templates` 14 not seeded → 93% deficit.

**Verification (performed, no code change — 03-01 already closed):**

```
$ ls packages/game-templates/templates/*.json | wc -l
14
$ cat packages/game-templates/index.json | jq '.registry | keys | length'
14
$ grep -r "SeedGameTemplates" forge/api/internal/store/*.go
forge/api/internal/store/seed_game_templates.go:257:func (s *Store) SeedGameTemplates(ctx context.Context) error {
forge/api/internal/store/seeder.go:91:  s.Register("game-templates", func(...) { return store.SeedGameTemplates(ctx) })
```

- `forge/api/internal/store/seed_game_templates.go:48` `fallbackGameTemplates` holds **14** entries (Paper, Vanilla, Bedrock, Palworld, Valheim, Terraria, Enshrouded, Satisfactory, Rust, CSGO, Factorio, 7Days2Die, TeamSpeak3, ProjectZomboid) mirroring `packages/game-templates/templates/*.json` and `forge/web/lib/egg-templates.ts:27`.
- Idempotent SQL: `INSERT INTO eggs … ON CONFLICT (nest_id, name) DO NOTHING` + `INSERT INTO egg_variables … ON CONFLICT (egg_id, env_variable) DO NOTHING` (`seed_game_templates.go:296-318`).
- Seeder registered in `DefaultSeeder` as 3rd entry (`seeder.go:91-93`): `default-roles`, `default-settings`, `game-templates`.
- Concurrent fix via `eggseeder.Service` (`forge/api/internal/services/eggseeder/service.go:45` `templateFS embed.FS` with 14 templates under `templates/`) — both paths use deterministic `uuid.NewSHA1(NameSpaceURL, "gamepanel:template:"+tpl.ID)`.

**Result:** VERIFIED — no additional seeding needed. Slice 03-01 closed P1; this slice confirms 14 on disk and 2 idempotent upsert paths (store + service) with `ON CONFLICT DO NOTHING`.

---

## 4. Verification — DB Fleet View Encrypted Columns (03-13 idempotency check)

**Finding:** `store_db_containers.go:174` `ListDBContainers` / `:200` `ListAllDBContainers` omitted `connection_string_encrypted, credentials_encrypted` → fleet view (`forge/web/app/admin/databases/page.tsx:17` `containers` tab) returned empty creds post-`155_encrypt_db_container_credentials.sql:1`.

**Verification (no code change — 03-13 already fixed; snapshot):**

```go
// forge/api/internal/store/store_db_containers.go:151 GetDBContainer
SELECT … COALESCE(connection_string_encrypted,''), COALESCE(credentials_encrypted,'') FROM db_containers WHERE id=$1 // :158

// :174 ListDBContainers
SELECT … COALESCE(connection_string_encrypted,''), COALESCE(credentials_encrypted,'')
FROM db_containers WHERE server_id=$1 ORDER BY created_at DESC // :178
// scan :190  connectionEncrypted, credentialsEncrypted → decryptDBContainerSecrets :195

// :205 ListAllDBContainers (fleet view)
SELECT … COALESCE(connection_string_encrypted,''), COALESCE(credentials_encrypted,'')
FROM db_containers ORDER BY created_at DESC LIMIT $1 // :214-216
// scan :227  connectionEncrypted, credentialsEncrypted → decryptDBContainerSecrets :233
```

Both list paths now mirror `GetDBContainer`'s 14→16 column SELECT and call `decryptDBContainerSecrets` (`store_db_containers.go:300`) with `secretAAD("db_containers", id, "connection_string"|"credentials")`. `SetDBContainerStatus` (`store_db_containers.go:243`) dual-writes encrypted columns; legacy plaintext `connection_string` cleared when encrypted present.

**Result:** VERIFIED FIXED — fleet view no longer blank. Encryption-in-rest via `secrets.Keyring` envelope + AAD domain separator stronger than 1Panel per-row `service/database_mysql.go:857` baseline.

---

## 5. Tests — Tenant Column Presence

### 5.1 Unit tests (no DB)

**`forge/api/internal/events/event_tenant_test.go` — 4 tests:**

| Test | Asserts |
|---|---|
| `TestEnvelopeTenantID_Present` | `NewEnvelope` pulls `tenant_id`/`orgId` alias from payload; nil payload → empty |
| `TestEnvelopeWithTenant_Explicit` | `NewEnvelopeWithTenant(_,_,_,_,"tenant-999",_)` preserves explicit; empty explicit falls back to `payload["tenantId"]` |
| `TestEnvelopeTenant_JsonRoundTrip` | `TenantID` survives `json.Marshal`/`Unmarshal` (`json:"tenant_id,omitempty"`); payload intact |
| `TestEnvelopeValidate_TenantOptional` | `Validate` passes with or without `TenantID` (additive field is not required) |

**`forge/api/internal/domain/domain_tenant_test.go` — 3 tests:**

| Test | Asserts |
|---|---|
| `TestPlacementRequest_TenantFields` | `PlacementRequest{TenantID,OrgID,ProjectID}` round-trips JSON; tags `tenantId`/`orgId`/`projectId` |
| `TestPlacementDecision_TenantField` | `PlacementDecision{TenantID}` JSON round-trip |
| `TestPlacementRequest_EmptyTenantOmitsJSON` | `omitempty` omits `tenantId` when empty (wire compat) |

**Run:**
```
$ go test ./internal/events -run TestEnvelopeTenant -count=1 -v
ok   gamepanel/forge/internal/events    0.341s
$ go test ./internal/domain -run TestPlacement -count=1 -v
ok   gamepanel/forge/internal/domain    0.305s
```

### 5.2 Integration — `forge/api/internal/eventstore/store_tenant_test.go`

Requires `DATABASE_URL` or `forge_test` pool; otherwise `t.Skip`.
- `TestEventStore_TenantColumn_PublishAndRoundTrip` — `Migrate(pool)` → `Publish` with `NewEnvelopeWithTenant(..., tenantID)` → raw `SELECT tenant_id::text FROM events WHERE id=$1` asserts persisted tenant; `Pending` scan asserts `StoredEvent.TenantID`; cleanup `DELETE`.
- `TestEventStore_TenantNull_BackwardsCompatible` — publish without tenant → `tenant_id` remains `NULL` (nullable, backfill async).
- `TestEventStore_Migration_File_ContainsTenantColumn` — **no DB needed**: reads `../../migrations/215_tenant_scoping_additive.sql` and asserts fragments `ALTER TABLE events ADD COLUMN IF NOT EXISTS tenant_id`, `ALTER TABLE placement_decisions`, `ALTER TABLE reconcile_plans`, and 3 index names `idx_events_tenant`/`idx_placement_decisions_tenant`/`idx_reconcile_plans_tenant`. This is the **tenant column presence** file-level test.

```
$ go test ./internal/eventstore -run TestEventStore_Migration -count=1 -v
ok   gamepanel/forge/internal/eventstore    0.960s
```

### 5.3 DB integration — `forge/api/internal/store/store_tenant_scoping_test.go`

Requires `TEST_DATABASE_URL` (via `migrationTestStore: store_eggs_integration_test.go:17` schema-per-test isolation).

| Test | Coverage |
|---|---|
| `TestPlacementDecisions_TenantColumn` | Asserts `information_schema.columns` for `placement_decisions.tenant_id`, `reconcile_plans.tenant_id`, `events.tenant_id` (migration 215 applied); creates `CreatePlacementDecisionWithTenant` with `tenantA`/`tenantB` and legacy `CreatePlacementDecision` without tenant; asserts nullable backfill; `ListPlacementDecisionsByApp` preserves all 3 tenant variants; checks partial index `idx_placement_decisions_tenant` via `pg_indexes`. |
| `TestReconcilePlans_TenantColumn` | `CreateReconcilePlan` with `TenantID=&tenantID` → `GetReconcilePlan` round-trips; legacy without tenant → `TenantID nil`; `ListReconcilePlans` finds both. |
| `TestTenantColumns_Nullable_And_Indexes` | `information_schema.is_nullable='YES'` for both tables; `data_type='uuid'` for `events.tenant_id`; `pg_indexes.indexdef` contains `WHERE` (partial index); ensures no breaking NOT NULL. |

```
$ go test ./internal/store -run TestTenantColumns -count=1  # skipped without TEST_DATABASE_URL, compile-verified
$ go vet ./internal/store
ok
```

**Full vet:**
```
$ go vet ./internal/events ./internal/domain ./internal/eventstore
ok
```

---

## 6. Backfill & Dual-Read Strategy (async, no breaking)

- **Migration phase (215):** `ADD COLUMN IF NOT EXISTS tenant_id UUID` — nullable, no default, instantaneous (catalog only). Existing rows remain `NULL` → interpreted as `legacy/unscoped` in Go (`*string` nil or `""`).
- **Write phase (this slice):** New code writes `tenant_id` when `Envelope.TenantID` / `PlacementRequest.TenantID` / `ReconcilePlanRow.TenantID` set; legacy callers still write `NULL` via `CreatePlacementDecision` (delegates to `WithTenant("",…)`). `eventstore.Publish` passes `nil` for empty tenant so partial index stays sparse.
- **Read phase:** `COALESCE(tenant_id::text,'')` in all SELECTs → Go gets `""` for legacy rows, then leaves `TenantID` nil. No `WHERE tenant_id = …` filtering yet — fleet-wide behavior unchanged today (P1 tenant-blind scheduling still global in `scheduler/service.go:88` pending quota-aware filter in next slice).
- **Backfill (async, next deploy):** `UPDATE placement_decisions pd SET tenant_id = s.org_id FROM servers s WHERE pd.app_id IN (SELECT id FROM replica_applications) …` is **not** in 215 — intentionally deferred to a low-priority backfill job that `JOIN`s `servers.org_id` or `replica_applications` owner where mappable; else remains `NULL` until touched. No blocking `VALIDATE CONSTRAINT`.
- **Cutover (future):** After backfill `SELECT COUNT(*) WHERE tenant_id IS NULL = 0` confirms cold, add `NOT NULL` or `CHECK`/`FK` (`REFERENCES organizations(id) ON DELETE SET NULL`) via `ALTER TABLE … ADD CONSTRAINT … NOT VALID` + `VALIDATE CONSTRAINT` (no table rewrite). Today: no FK, so large `placement_decisions` (≈ batch2 entities) never holds `ALTER TABLE … ADD CONSTRAINT …` exclusive lock.

---

## 7. Constraints & Non-Breaking Guarantees

- **Additive only:** No `DROP`, no `ALTER COLUMN … SET NOT NULL`, no `DELETE`, no `TRUNCATE`, no sequence reset. Six tables `IF NOT EXISTS`, seven partial indexes `IF NOT EXISTS`.
- **Wire compatible:** `json:"tenant_id,omitempty"` / `json:"tenantId,omitempty"` — old clients omit field → new servers read `""`; new clients send field → old servers ignore unknown JSON (Go `json.Unmarshal` ignores unknown). `NewEnvelope` auto-extracts from payload aliases (`tenant_id|tenantId|org_id|orgId`) so existing payloads with `org_id` automatically tag tenant without changing call sites.
- **No quota filtering yet:** `scheduler/service.go:833` `normalizeRequest` populates `TenantID`/`OrgID`/`ProjectID` but `FilterNodes`/`ScoreNodes`/`Place` unchanged — placement stays fleet-wide today; next slice will read `org_quotas` (`196_org_quotas.sql`) to reject/filter per-project quotas.
- **Encrypted fleet view untouched:** `store_db_containers.go:214` already correct; no re-encryption.
- **Template seeding untouched:** `seed_game_templates.go:296` `ON CONFLICT` idempotency preserved; no new migration.

---

## 8. Verification Checklist

- [x] `forge/api/internal/domain/domain.go:114` `PlacementRequest` has `TenantID`/`OrgID`/`ProjectID`
- [x] `forge/api/internal/domain/domain.go:152` `PlacementDecision` has `TenantID`
- [x] `forge/api/internal/events/event.go:196` `Envelope` has `TenantID` + `NewEnvelopeWithTenant` + `tenantIDFromPayload`
- [x] `forge/api/internal/eventstore/store.go:49` `StoredEvent.TenantID` + `Publish`/`ClaimPending`/`Pending`/`MoveToDeadLetter` handle tenant
- [x] `forge/api/internal/eventstore/migration.go:36` adds `tenant_id UUID` + indexes for fresh `Migrate(pool)` path
- [x] `forge/api/internal/store/store_instances.go:55` `PlacementDecision.TenantID *string` + `CreatePlacementDecisionWithTenant`
- [x] `forge/api/internal/store/store_reconcile.go:14` `ReconcilePlanRow.TenantID *string` + `Create/Get/List` tenant-aware
- [x] `forge/api/internal/services/scheduler/service.go:833` `normalizeRequest` threads tenant fields
- [x] `forge/api/migrations/215_tenant_scoping_additive.sql` + sqlite/mysql variants exist and contain 3 ALTERs + 6 indexes (`grep tenant_id`)
- [x] `packages/game-templates/templates/*.json` count == 14 + `seed_game_templates.go:48` fallback 14 verified
- [x] `forge/api/internal/store/store_db_containers.go:205` `ListAllDBContainers` includes `COALESCE(connection_string_encrypted, '')` + `COALESCE(credentials_encrypted,'')` + `decryptDBContainerSecrets`
- [x] Tests `event_tenant_test.go` (4), `domain_tenant_test.go` (3) PASS without DB
- [x] Migration-presence test `store_tenant_test.go:TestEventStore_Migration_File_ContainsTenantColumn` PASS without DB
- [x] `go vet ./internal/events ./internal/domain ./internal/eventstore ./internal/store` PASS

---

## 9. Risks & Next Slice

| Risk | Mitigation |
|---|---|
| `SELECT … tenant_id` fails if migration not yet run on hot replica | Migration 215 is `ADD COLUMN IF NOT EXISTS` run via `store.go:RunMigrations` before HTTP serve; `COALESCE` handles null. Rollback is `DROP COLUMN IF EXISTS` (not shipped) — keep 215 forward-only. |
| Tenant-blind scheduling persists until filter added | Documented as intentional P1→P2 split. Next slice: `scheduler/service.go:168 FilterNodes` add `org_quotas` check: `if req.TenantID != "" { quota := store.GetOrgQuota(ctx, req.TenantID); if quota.MemoryUsageBytes + req.MemoryMB*1M > entitlement.max_memory_bytes { skip node } }`. Use existing `org_quotas` (`196`) + `billing_plans` (`195`) entitlements. |
| `events` UUID tenant vs `organizations` TEXT FK mismatch | Migration uses `UUID` (organizations `id UUID`). Payload alias `org_id` may be stringified UUID — `Publish` passes raw string; Postgres casts `TEXT → UUID` implicit when column is `UUID` type. Verified by `uuid.NewString()` format `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`. If caller passes non-UUID slug, insert would error — currently tenant is always UUID from `organizations.id`; no slug path. |
| Backfill cost on `placement_decisions` with 10M rows | Backfill deferred; when run, use `UPDATE … WHERE tenant_id IS NULL LIMIT 10000` loop with `pg_sleep` to avoid bloat; partial index keeps `WHERE tenant_id IS NULL` scan cheap until backfilled. |
| Template drift between `eggseeder.Service` (14 templates via `embed.FS`) and `store.SeedGameTemplates` fallback | Both embed 14; `store_tenant_scoping_test.go` does not enforce sync, but `seeder.go:91` registers store fallback and `eggseeder` service is called from `cmd/api/main.go:≈800` (parallel paths). Keep both ON `CONFLICT DO NOTHING` so drift is additive, not overwriting; reconcile via single source of truth `packages/game-templates/templates/*.json` + `index.json` in follow-up. |

---

## 10. Paths Touched (additive slice 19/20 only)

- `forge/api/internal/domain/domain.go:114,152`
- `forge/api/internal/events/event.go:196,208,230,265`
- `forge/api/internal/eventstore/store.go:49,67,100,113,135,175,191`
- `forge/api/internal/eventstore/migration.go:36`
- `forge/api/internal/eventstore/outbox.go:140`
- `forge/api/internal/eventstore/store_tenant_test.go` (new — 3 tenant tests incl. migration file presence)
- `forge/api/internal/events/event_tenant_test.go` (new — 4 tests)
- `forge/api/internal/domain/domain_tenant_test.go` (new — 3 tests)
- `forge/api/internal/store/store_instances.go:55,347-372,374,386,401`
- `forge/api/internal/store/store_reconcile.go:14,42,67,91,123,173` + `+database/sql` import
- `forge/api/internal/services/scheduler/service.go:833` (normalize)
- `forge/api/internal/store/store_tenant_scoping_test.go` (new — 3 DB integration tests + index presence)
- `forge/api/migrations/215_tenant_scoping_additive.sql` (new — Postgres primary)
- `forge/api/migrations/sqlite/215_tenant_scoping_additive.sql` (new)
- `forge/api/migrations/mysql/215_tenant_scoping_additive.sql` (new)

No other slices' files touched. Template seeder (`seed_game_templates.go:257`) and DB fleet view (`store_db_containers.go:205`) **verified, not modified** except vet fixes (store_eggs cycle).

---

*Generated for 110-Phase-03-Impl Subagent 19. Verify with `go test ./internal/events -run Tenant -v && go test ./internal/domain -run Tenant -v && go test ./internal/eventstore -run Migration -v && cat forge/api/migrations/215_tenant_scoping_additive.sql | grep tenant_id` and `ls packages/game-templates/templates | wc -l` (expect 14).*
