# Subagent 07 — Database Services & Template Catalog: Centralization Seam Fix

> **Scope:** `forge/api/internal/store/store_db_containers.go`, `forge/api/internal/services/dbprovisioner/*`, `forge/api/internal/services/database_service_provisioner.go`, `forge/api/internal/store/store_databases.go`, `forge/api/internal/store/store_catalog.go:84`, `forge/api/internal/store/store_app_store.go:6`, `forge/api/internal/store/store_nests.go:20`, `forge/api/internal/store/store_templates.go`, `forge/api/internal/store/seeder.go:38`, `forge/api/cmd/api/main.go:529`, `forge/web/lib/app-templates-data.ts:3`, `packages/game-templates` (FS), `scripts/validate-templates.mjs`, `forge/api/migrations/*`
> **Findings addressed:** DB-01 (fleet view blank), DB-02 (mysql TLS global leak/race), DB-03 (deprovision swallowing + Restart no-op), password divergence, status-before-delete race, string-sniff fallback, TMPL-01 (93–100% seeding deficit), `validate-templates.mjs` blind spots, catalog fragmentation
> **Backward-compat:** additive migrations + idempotent upserts; no column drops until N+1 release; plaintext fallback retained in decrypt path

---

## 1. Problem Statement & Evidence

### 1.1 DB-01 — List omits encrypted columns → blank fleet view

| Evidence | Detail |
|---|---|
| `forge/api/internal/store/store_db_containers.go:174` | `ListDBContainers(ctx, serverID)` selects 14 cols, **excludes** `connection_string_encrypted, credentials_encrypted` |
| `forge/api/internal/store/store_db_containers.go:200` | `ListAllDBContainers(ctx, limit…)` same omission |
| `forge/api/internal/store/store_db_containers.go:151` | `GetDBContainer(ctx, id)` **correctly** selects `COALESCE(connection_string_encrypted,''), COALESCE(credentials_encrypted,'')` at `:158` and calls `decryptDBContainerSecrets:295` |
| `forge/api/internal/store/store_db_containers.go:233` | `SetDBContainerStatus` encrypts and nulls plaintext: `connection_string=''`, `credentials='{}'` when encrypted present |
| `forge/api/migrations/155_encrypt_db_container_credentials.sql` (inferred) | Plaintext columns cleared post-encryption |

**Impact:** After encryption migration, `GET /api/v1/databases/containers?serverId=X` (`forge/api/internal/http/handlers_db_containers.go:67`) calls `ListDBContainers` → scans empty `connection_string/credentials` → `decryptDBContainerSecrets` receives empty ciphertext → returns `""` / `"{}"` → fleet view shows blank credentials; single-detail `GetDBContainer` still works, so bug is list-only. Caller confusion: admin sees 0 credentials until they open each row individually via `GET /databases/containers/:id` `:87` or `GET /:id/credentials` `:139`.

**Repro:** `Provision` → wait `running` → `ListAllDBContainers` → `len(Credentials)==2 ("{}")` while `GetDBContainer(id).Credentials` is populated.

### 1.2 DB-02 — `mysql.RegisterTLSConfig` global leak/race

| Evidence | Detail |
|---|---|
| `forge/api/internal/services/dbprovisioner/service.go:373` | `connectorForHost` per MySQL/MariaDB host: `name := mysqlTLSConfigName(host)` (`:417` `sha256(ID+host+mode+CA+serverName)[:12]` → `gamepanel-<hex>`) then `mysql.RegisterTLSConfig(name, tlsConfig)` |
| `github.com/go-sql-driver/mysql` | `RegisterTLSConfig` mutates `global *map[string]*tls.Config` protected by no mutex in older deps; concurrent `Provision`/`TestConnection:187` race |
| `forge/api/internal/services/dbprovisioner/service.go:378` | Guard is `if err != nil && !strings.Contains(err.Error(), "already registered")` — swallows re-registration but **leaks** CA when a host's `TLSCA` rotates (stale `*tls.Config` pinned under same derived name if CA unchanged? Actually name includes CA, so rotation yields new name; old entry never deregistered). Growth is O(#distinct host×CA combos). |

**Impact:** (a) Data race under `go test -race`; (b) unbounded global map growth; (c) stale `RootCAs` if `TLSCA` updated but name collision on same `ID+host+mode+serverName` without CA (not the case here — CA is hashed in, but spec says CA rotation creates new entry, old leaked); (d) not required — `mysql.Connector` can carry `TLSConfig *tls.Config` directly since [go-sql-driver/mysql v1.5+] without global registry when using `NewConnector` with `cfg.TLSConfig` as `*tls.Config` (not name). However current code passes `name string`.

### 1.3 DB-03 — Deprovision swallows error; Restart is status-only no-op

| Evidence | Detail |
|---|---|
| `forge/api/internal/services/dbprovisioner/containers.go:289` | `Deprovision(ctx, containerID)` — `if s.daemon != nil && db.ContainerID != "" { _ = s.daemon.DeProvisionDatabase(...) }` — **discards** daemon error, then `return s.store.DeleteDBContainer(ctx, containerID)` unconditionally hard-deletes row |
| `forge/api/internal/services/dbprovisioner/containers.go:300` | `Restart(ctx, containerID)` — `return s.store.SetDBContainerStatus(ctx, containerID, "", "running", 0, "", "", nil)` — only flips status column, never calls `daemon.AdminContainerRestart/Start/Stop` |
| `forge/api/internal/services/database_service_provisioner.go:259` | `DeleteService(ctx, id)` sets `status=deleting` (`UpdateDatabaseServiceStatus:264`) **before** daemon call, then on daemon error resets to `failed` (`:270`) — but container may already be gone if daemon partially succeeded; status-before-delete race |
| `forge/api/internal/http/handlers_db_containers.go:100` | `DELETE /databases/containers/:id` returns `{"ok":true}` even if daemon deletion failed (row already hard-deleted) |
| `forge/api/internal/http/handlers_db_containers.go:127` | `POST /databases/containers/:id/restart` similarly returns ok while container unchanged |

**Impact:** Orphaned Docker volumes/containers when daemon fails; admin thinks delete succeeded; `Restart` silently lies — alerting/recovery (`service/recovery`) that depends on restart semantics is broken. Contrast correct handling in `database_service_provisioner.go:264` (status mutation before call) vs `containers.go:289` (hard delete regardless).

### 1.4 Password generation divergence

| Location | Impl | Entropy | Output |
|---|---|---|---|
| `forge/api/internal/services/dbprovisioner/containers.go:44` | `b := make([]byte,(length+1)/2); rand.Read(b); hex.EncodeToString(b)[:length]` | `(length+1)/2` random bytes → hex truncated | hex alphabet only `[0-9a-f]` |
| `forge/api/internal/services/database_service_provisioner.go:63` | `b := make([]byte,length); rand.Read(b); hex.EncodeToString(b)[:length]` | `length` random bytes → hex truncated (allocates 2×) | same alphabet, wastes entropy |
| `forge/api/internal/store/store_databases.go:194` | `newDaemonToken()[:32]` → `store/store.go` `newDaemonToken` is `base64.RawURLEncoding.EncodeToString(rand 32 bytes)` truncated | 32 random bytes → base64url | `[A-Za-z0-9_-]` |
| `forge/api/internal/services/catalog/catalog.go:241` | `randomPassword(24)` — `catalog/providers.go` likely distinct again | check | ? |
| `forge/api/internal/services/dbprovisioner/service.go:260` | `candidate := s.store.NewServerDatabasePasswordCandidate()` delegates to `store:194` | base64url | differs from containers path |

**Impact:** Two DB service paths use hex (low alphabet, 4 bits/char) vs base64url (6 bits/char). `generatePassword(32)` hex produces 16 bytes entropy (128 bits) but `hex.EncodeToString(16) = 32` correct; `database_service_provisioner.generatePassword(32)` allocates 32 bytes but still only 16 bytes effective after truncation — misleading. No single `secrets.GeneratePassword` source; rotation tests vs provision tests diverge; password strength audits inconsistent.

**String-sniff fallback fragility:** `forge/api/internal/services/dbprovisioner/service.go:377` `strings.Contains(err.Error(), "already registered")` relies on English error string; breakage on driver message change. Similar `store/db_containers.go:93` `CanonicalDBEngine` string checks fragility if engine aliases expand.

### 1.5 Status-before-delete race

`database_service_provisioner.go:264` `UpdateDatabaseServiceStatus(ctx,id,"deleting",…) `before` `daemon.DeProvisionDatabase`. If process crashes between status write and daemon success, row stuck in `deleting` forever (no reaper). Conversely `containers.go:289` hard-deletes even on daemon failure (orphans). Neither is transactional with external side effect — but status mutation must be **after** success or be recoverable.

### 1.6 TMPL-01 — Template seeding deficit

| Source | Count | Status |
|---|---|---|
| PufferPanel upstream `reference/game-hosting/pufferpanel-templates` | 42 files, 36 game types | spec + data.json + 24 ops |
| `packages/game-templates` HEAD (`git show HEAD:packages/game-templates/templates`) | 14 (`7days2die, csgo, enshrouded, factorio, minecraft-bedrock, minecraft-paper, minecraft-vanilla, palworld, rust, satisfactory, teamspeak3, terraria, valheim, zomboid`) | **deleted on disk** at audit time (uncommitted `D  packages/game-templates/`) → **0 reachable** on fresh checkout |
| `forge/web/lib/egg-templates.ts:27` (mirrors FS) | 14 constant `EGG_TEMPLATES` | frontend-only, never seeded to DB |
| `forge/api/migrations/091_seed_minecraft_java.sql:3` | 1 egg `Minecraft Java` (`itzg/minecraft-server:java21`, empty startup) | only seeded row; `ListEggs(ctx,"")` (`store_nests.go:190`) returns 1 |
| `forge/api/internal/services/appstore/seed.go:27` `seedApps` | 7 `app_store_apps` (`nginx, postgres, redis, mongo, mariadb, portainer, traefik`) via `SeedDefaultApps:215` called every boot `cmd/api/main.go:529` | healthy pattern to copy |
| `forge/web/lib/app-templates-data.ts:3` | 5 `DEFAULT_APP_TEMPLATES` + `localStorage:forge.app-templates.v1` | browser-only, never DB-backed |

**Deficit math:** If canonical = Puffer 42 → Forge seeds 1 → 97.6% deficit; if canonical = 14 FS → seeds 1 → 92.8% deficit; if checkout deleted → 0 FS reachable → 100% deficit. Doc claims 14 OOTB false.

**Root causes:**
- No `DefaultSeeder` entry for eggs (`forge/api/internal/store/seeder.go:38` only registers `default-roles`, `default-settings:64`)
- No FS→DB bridge (unlike `SeedDefaultApps` which upserts 7 compose apps via `UpsertAppStoreApp:92`)
- `packages/game-templates/src/index.ts:8` `templateToApiTemplate` transform never invoked at boot
- `store_templates.go:1` compat shim delegates to `ListEggs` but `eggs` table empty
- `catalogEntries` (`store_catalog.go:84` 11 rows from `migrations/175_catalog_entries.sql`) disjoint from eggs/app_store/localStorage

### 1.7 `validate-templates.mjs` blind to `install_script`

From worktree copy `validate-templates.mjs:46-54`: `collectPlaceholders` only scans `content.startup` + `content.config.files`; ignores `content.install_script.script` — the most dangerous interpolation site (contains `curl -L {{DL_PATH}}` style, now `sed s/{{/${/g` transform at `minecraft-*.json:114`). An undefined `{{VAR}}` there becomes empty URL at deploy.

Also blind to:
- `appId` numeric validity (Steam `258550` etc. are unquoted strings in script)
- `groups` / conditional `if:` (Puffer parity)
- `stdin` (`rconws`/`telnet`) / `stdout` / `stopCode`
- `ports[]` ↔ `startup` Beacon/RCON gap (e.g. `satisfactory` BEACON_PORT in startup but no ports entry)

### 1.8 Catalog fragmentation

| Layer | Table/Constant | Rows/Provider | Provision Path |
|---|---|---|---|
| A. FS game-templates | `packages/game-templates/templates/*.json` | 14 JSON → `index.json` registry | FS only, not DB |
| B. Frontend egg-templates | `forge/web/lib/egg-templates.ts:27` `EGG_TEMPLATES` | 14 constants | `app-templates-data` localStorage fallback |
| C. DB eggs/nests | `store_nests.go:20` `eggs` + `egg_variables` | 1 row seeded | `store/nests.go` CRUD, UI `AdminNestsEggs.tsx` |
| D. Catalog entries | `store_catalog.go:84` `catalog_entries` + `catalog_instances` | 11 rows (`postgres, mysql, mariadb, redis, mongodb, valkey, rabbitmq, clickhouse, nats, memcached, …`) | `catalog.Service.Provision:119` → `DBProvider` or `ComposeProvider` |
| E. App Store | `store_app_store.go:6` `app_store_apps` | 7 rows seeded | `appstore.Service.SeedDefaultApps:215` → `UpsertAppStoreApp:92` |
| F. localStorage | `forge/web/lib/app-templates-data.ts:3` | 5 defaults | `loadUserTemplates:56`/`saveUserTemplates:66` |

Admin has two disjoint UIs (`nests` vs `app-templates` page `forge/web/app/admin/app-templates/page.tsx:1`), `fetchAppTemplates()` (`forge/web/lib/api/apps.ts:369`) falls back to localStorage, never hits `GET /admin/nests` or `GET /catalog/entries`.

---

## 2. Target Architecture (Single Source of Truth)

```
                    ┌──────────────────────────────────────────────────────────┐
                    │           Authoring Source (pick ONE)                     │
                    │  Option A: FS  packages/game-templates/templates/*.json  │
                    │  Option B: TS  forge/web/lib/egg-templates.ts            │
                    │  ─────────────────────────────────────────────────────── │
                    │  DECISION: FS as canonical (richer JSON, jq, schema)     │
                    │  TS generated from FS at build (or vice versa).          │
                    └──────────────────────┬───────────────────────────────────┘
                                           │  DefaultSeeder upsert  (idempotent, like SeedDefaultApps)
                                           │  forge/api/internal/services/eggseeder.Service
                                           ▼
                    ┌──────────────────────────────────────────────────────────┐
                    │           Canonical Store (DB)                           │
                    │  nests  (Games, Legacy Templates)                        │
                    │  eggs + egg_variables  ← 14–42 rows, install_script,     │
                    │                         startup, config, docker_images  │
                    │  catalog_entries  ── derived OR manually curated ──────┤
                    │  app_store_apps   ── compose marketplace (keep separate)│
                    └──────────────────────┬───────────────────────────────────┘
                                           │  REST
                                           ▼
                    ┌──────────────────────────────────────────────────────────┐
                    │           Frontend Galleries (DB-backed)                 │
                    │  /admin/nests/:nestId/eggs  (egg gallery)               │
                    │  /admin/databases       (DB fleet: masked creds)        │
                    │  /catalog               (unified Gallery)               │
                    │  localStorage path deprecated → import on boot or        │
                    │  explicit "Migrate my templates" CTA                    │
                    └──────────────────────────────────────────────────────────┘

     DB fleet view: List* queries include encrypted cols + decrypt loop
     TLS:           per-connector *tls.Config (no global map)
     Deprovision:   daemon error propagated; soft-delete then reaper
     Passwords:     single secrets.GenerateDatabasePassword
     Validation:    validate-templates.mjs covers install_script + startup + config.files + ports vs startup
```

**Non-goals:** Keep `app_store_apps` (compose marketplace) separate from game eggs; unify only game eggs + FS source. `app-templates-data.ts` localStorage retained only as import buffer, not primary store.

---

## 3. Detailed Implementation Plan

### Phase 0 — Baseline & Guardrails (0.5 day)

**Why first:** Prevent regressions while fixing seam; lock existing encrypted data contract.

**Tasks:**

1. Add integration test `forge/api/internal/store/store_db_containers_encrypted_test.go`:
   ```go
   func TestListDBContainersDecryptsEncrypted(t *testing.T) {
       // CreateDBContainer → SetDBContainerStatus with connStr+creds (encrypts)
       // ListDBContainers(serverID) must return non-empty ConnectionString/Credentials
       // ListAllDBContainers must also
       // GetDBContainerCredentials must match List result
   }
   ```
   Run with `-race`.

2. Add race test for `connectorForHost`:
   ```go
   func TestConnectorForHostConcurrent(t *testing.T) {
       hosts := []store.DatabaseHost{{ID: uuid.New(), Host:"db1", TLSMode:"verify-full", TLSCA: caPEM}, ...}
       var wg sync.WaitGroup
       for i:=0;i<50;i++{ wg.Add(1); go func(){ defer wg.Done(); _, _ = connectorForHost(hosts[i%len(hosts)], "pw") }() }
       wg.Wait()
   }
   ```

3. Snapshot `eggs` count test: `TestSeededEggCountIsOne` (documents T-0 baseline; will flip to 14 after Phase 5).

4. Snapshot `validate-templates.mjs` run in CI: `node packages/game-templates/scripts/validate-templates.mjs` must fail when `packages/game-templates` missing (assert phantom).

**Exit criteria:** Tests fail as expected (red), documenting current deficit.

---

### Phase 1 — DB-01 Fleet View Fix (0.5 day)

**File:** `forge/api/internal/store/store_db_containers.go:174,200`

**Current (buggy):**
```go
// store_db_containers.go:174 — ListDBContainers
rows, err := s.db.Query(ctx, `
    SELECT id, server_id, engine, version, container_id, connection_string,
           credentials, status, port, volume_id, memory_mb, cpu_shares, created_at, updated_at
    FROM db_containers WHERE server_id = $1 ORDER BY created_at DESC
`, serverID)
// scans: &db.ConnectionString, &db.Credentials — plaintext columns (empty post-encryption)
// never reads connection_string_encrypted / credentials_encrypted
```

**Required:**

```go
func (s *Store) ListDBContainers(ctx context.Context, serverID string) ([]DBContainer, error) {
    rows, err := s.db.Query(ctx, `
        SELECT id, server_id, engine, version, container_id, connection_string,
               credentials, status, port, volume_id, memory_mb, cpu_shares, created_at, updated_at,
               COALESCE(connection_string_encrypted, ''), COALESCE(credentials_encrypted, '')
        FROM db_containers WHERE server_id = $1 ORDER BY created_at DESC
    `, serverID)
    if err != nil { return nil, err }
    defer rows.Close()
    var dbs []DBContainer
    for rows.Next() {
        var db DBContainer
        var createdAt, updatedAt any
        var connEnc, credEnc string
        if err := rows.Scan(&db.ID, &db.ServerID, &db.Engine, &db.Version, &db.ContainerID,
            &db.ConnectionString, &db.Credentials, &db.Status, &db.Port, &db.VolumeID,
            &db.MemoryMB, &db.CPUShares, &createdAt, &updatedAt, &connEnc, &credEnc); err != nil {
            return nil, err
        }
        if err := s.decryptDBContainerSecrets(&db, connEnc, credEnc); err != nil {
            return nil, err
        }
        db.CreatedAt = formatDBContainerTime(createdAt)
        db.UpdatedAt = formatDBContainerTime(updatedAt)
        dbs = append(dbs, db)
    }
    return dbs, rows.Err()
}

func (s *Store) ListAllDBContainers(ctx context.Context, limit ...int) ([]DBContainer, error) {
    // same SELECT addition + decrypt call
}
```

**Also add projection helper to avoid drift:**

```go
const dbContainerColumns = `id, server_id, engine, version, container_id, connection_string, credentials, status, port, volume_id, memory_mb, cpu_shares, created_at, updated_at, COALESCE(connection_string_encrypted,''), COALESCE(credentials_encrypted,'')`
// reuse in GetDBContainer:151, ListDBContainers:174, ListAllDBContainers:200, GetDBContainerBackupTarget if extended
```

**Alternative (if want to keep conn string filtered from fleet view):** Provide masked view: `GET /databases/containers` returns `connectionStringMasked` via `maskConnectionString` (replace password segment with `***`) and `GET /:id/credentials` remains full. Fleet view should still decrypt internally then mask before JSON. Decision: decrypt in store but mask in HTTP handler.

**HTTP masking (optional fleet hardening):**

`forge/api/internal/http/handlers_db_containers.go:67`:
```go
// AFTER fix: dbs already decrypted, mask before returning fleet
for i := range dbs {
    dbs[i].ConnectionString = "" // or masked; credentials stripped
    // Credentials is json:"-" so not serialized anyway; but be explicit
}
return c.JSON(fiber.Map{"items": dbs, "credentialsHint": "use /databases/containers/:id/credentials"})
```
Or expose `maskedConnectionString` field via DTO. The `store.DBContainer` currently has `json:"-"` on `ConnectionString/Credentials` (`store_db_containers.go:44`) — check if fleet view ever relied on those fields being empty. If `"-"` then fleet blank is intentional security posture — but task says fleet view with credentials (masked) is desired. Change struct tags or introduce DTO.

**Recommendation:** Keep `DBContainer` internal; introduce `DBContainerFleetDTO`:
```go
type DBContainerFleetDTO struct {
    DBContainer
    ConnectionStringMasked string `json:"connectionStringMasked"`
    HasCredentials bool `json:"hasCredentials"`
}
```

**Tests:**
- `TestListDBContainersAfterEncryption` — as Phase 0 test, must pass after fix.
- Verify `GetDBContainerCredentials:275` still consistent (should share `decryptDBContainerSecrets`).

**Backward compat:** SELECT is additive (`COALESCE(...,'')` yields `""` for pre-encryption rows); `decryptSecret("", plaintext, aad)` already handles fallback to plaintext — existing rows unaffected.

**DB-02 side fix — List vs Get parity for `managed_databases` / `database_services`:** Audit those list queries for same omission (quick grep: `SELECT.*password_encrypted`).

---

### Phase 2 — DB-02 TLS Per-Dial (No Global Registry) (1 day)

**File:** `forge/api/internal/services/dbprovisioner/service.go:366,373,417,422`

**Current:**
```go
// service.go:373-383
if host.TLSMode != "disable" {
    name := mysqlTLSConfigName(host) // :417 sha256(ID+host+mode+CA+serverName)[:12]
    if err := mysql.RegisterTLSConfig(name, tlsConfig); err != nil && !strings.Contains(err.Error(), "already registered") {
        return nil, err
    }
    cfg.TLSConfig = name // string ref to global map
}
return mysql.NewConnector(cfg)
```

**Target — per-connector TLS (no global map):**

`go-sql-driver/mysql` `Config.TLSConfig` type is `string` (name) **or** `*tls.Config`? Check driver: In `mysql.Config`, field is `TLSConfig string` historically, but since `v1.7` it supports `TLS *tls.Config` via `Config.Apply`? Actually `mysql.Config.TLSConfig` is `string`. The per-connector TLS is via `mysql.NewConnectorWithDialContext` + custom `DialContext` that wraps `tls.DialWithDialer`. Simpler: keep global but mutex-guarded + `DeregisterTLSConfig` on host update, OR switch to `registerTLSConfigOnce`.

**Option A (preferred, minimal change) — sync-guarded global with dedup key including CA hash:**

```go
var (
    mysqlTLSMu sync.Mutex
    // optional: keep set to allow Deregister on rotation
)

func connectorForHost(host store.DatabaseHost, password string) (driver.Connector, error) {
    tlsCfg, err := hostTLSConfig(host) // :422
    if err != nil { return nil, err }
    address := net.JoinHostPort(host.Host, fmt.Sprintf("%d", host.Port))
    switch host.Engine {
    case "mysql", "mariadb":
        cfg := mysql.NewConfig()
        cfg.User, cfg.Passwd, cfg.Net, cfg.Addr = host.Username, password, "tcp", address
        if host.TLSMode != "disable" {
            // FIX: set TLSConfig via custom dialer instead of global registry
            // Use mysql.NewConnector with TLSConfig as *tls.Config by using DialTLS
            // If driver still expects string, guard global registry:
            name := mysqlTLSConfigName(host)
            mysqlTLSMu.Lock()
            // Deregister stale if CA rotated but name same? Name includes CA so not needed,
            // but to prevent leak, check if name already registered with different *tls.Config
            // and overwrite via Deregister+Register.
            _ = mysql.DeregisterTLSConfig(name) // idempotent since v1.6; ignore error if not exists
            if err := mysql.RegisterTLSConfig(name, tlsCfg); err != nil {
                mysqlTLSMu.Unlock()
                return nil, err
            }
            mysqlTLSMu.Unlock()
            cfg.TLSConfig = name
        }
        return mysql.NewConnector(cfg)
    case "postgresql":
        cfg, err := pgx.ParseConfig(connectionDSN(host, password))
        if err != nil { return nil, err }
        cfg.TLSConfig = tlsCfg // pgx uses *tls.Config directly — no global leak (already correct)
        return stdlib.GetConnector(*cfg), nil
    default:
        return nil, errors.New("unsupported database engine")
    }
}
```

**Better Option B — eliminate global entirely via `DialContext` hook (if driver version supports `TLS *tls.Config` field):**
Check `go.mod` driver version. If `github.com/go-sql-driver/mysql v1.8+`, `mysql.Config` has `TLSConfig` string only; no `*tls.Config`. So Option A is required. Document as tech debt to upgrade to driver that supports `RegisterTLSConfig` with `AllowCleartextPasswords` etc. Alternative is to use `mysql.NewConnectorWithDialContext`:
```go
dialer := &net.Dialer{Timeout: 5*time.Second}
connector, err := mysql.NewConnectorWithDialContext(cfg, func(ctx context.Context, addr string) (net.Conn, error) {
    conn, err := dialer.DialContext(ctx, "tcp", addr)
    if err != nil { return nil, err }
    if tlsCfg != nil {
        tlsConn := tls.Client(conn, tlsCfg)
        if err := tlsConn.HandshakeContext(ctx); err != nil { conn.Close(); return nil, err }
        return tlsConn, nil
    }
    return conn, nil
})
```
But this bypasses `cfg.TLSConfig` handling; verify `mysql` skips TLS then. Simpler to keep mutex-guarded global.

**Also fix:**
- `hostTLSConfig:422` for `verify-ca` uses `InsecureSkipVerify: true` + `VerifyConnection` — valid but document why.
- `connectionDSN:396` for MySQL re-derives `mysqlTLSConfigName` without ensuring TLSConfig registered — must call `hostTLSConfig` before `FormatDSN` or use same mutex path. The `connectionDSN` is used only for `pgx.ParseConfig`? Actually it handles MySQL branch too but not via connector path. Ensure `TestConnection:186` reuses `s.open` (which goes through `connectorForHost`) so consistent. Deprecate `connectionDSN` MySQL branch or make it call `connectorForHost`.

**Tests:**
- `go test -race ./forge/api/internal/services/dbprovisioner -run TestConnectorForHostConcurrent`
- Verify re-registration with same CA does not error; rotation yields new name, old leaked entry remains bounded (acceptable) vs Deregister+Register keeps bounded.
- `TestMySQLTLSRotation` — create host, call connector, rotate CA, call again, dial succeeds with new CA.

**Backward compat:** Names remain `gamepanel-<hex12>`; existing DSNs in logs remain valid.

---

### Phase 3 — DB-03 Deprovision & Restart Correctness (1 day)

#### 3A. Deprovision — propagate daemon error, soft-delete + reaper

**File:** `forge/api/internal/services/dbprovisioner/containers.go:289`

**Current (buggy):**
```go
func (s *DBContainerService) Deprovision(ctx context.Context, containerID string) error {
    db, err := s.store.GetDBContainer(ctx, containerID)
    if err != nil { return err }
    if s.daemon != nil && db.ContainerID != "" {
        _ = s.daemon.DeProvisionDatabase(ctx, s.beaconBaseURL, s.nodeToken, db.ContainerID, db.VolumeID)
    }
    return s.store.DeleteDBContainer(ctx, containerID)
}
```

**Required:**

```go
func (s *DBContainerService) Deprovision(ctx context.Context, containerID string) error {
    db, err := s.store.GetDBContainer(ctx, containerID)
    if err != nil { return err }
    // 1. Attempt remote teardown first; fail open (panel record retained) on error.
    if s.daemon != nil && db.ContainerID != "" {
        if err := s.daemon.DeProvisionDatabase(ctx, s.beaconBaseURL, s.nodeToken, db.ContainerID, db.VolumeID); err != nil {
            // Mark failed so fleet shows error, but retain row for operator retry/force.
            _ = s.store.SetDBContainerStatus(ctx, containerID, db.ContainerID, "deprovision_failed", db.Port, db.VolumeID, "", nil)
            return fmt.Errorf("deprovision container via beacon: %w", err)
        }
    } else if db.ContainerID == "" {
        // Nothing to teardown remotely; just delete panel record.
    } else {
        // Daemon unavailable but container existed — do not hard-delete; leave for manual remediation.
        return errors.New("beacon client unavailable: container record retained for manual cleanup")
    }
    // 2. Only after remote success, delete panel row.
    if err := s.store.DeleteDBContainer(ctx, containerID); err != nil {
        return fmt.Errorf("delete db container record: %w", err)
    }
    return nil
}

// Optional: ForceDeprovision for admin override (like ForceDeleteServerDatabase:470)
func (s *DBContainerService) ForceDeprovision(ctx context.Context, containerID, reason string, actorID *string) error {
    db, err := s.store.GetDBContainer(ctx, containerID)
    if err != nil { return err }
    // Record orphan remediation if desired (similar to database_orphan_remediations)
    if s.daemon != nil && db.ContainerID != "" {
        _ = s.daemon.DeProvisionDatabase(context.Background(), s.beaconBaseURL, s.nodeToken, db.ContainerID, db.VolumeID)
        // ignore error — force path
    }
    return s.store.DeleteDBContainer(ctx, containerID)
}
```

**HTTP handler update `handlers_db_containers.go:100`:**
```go
protected.Delete("/databases/containers/:id", ..., func(c *fiber.Ctx) error {
    ctx, cancel := requestContext(); defer cancel()
    force := c.Query("force") == "true"
    var err error
    if force {
        err = dbProvisioner.ForceDeprovision(ctx, c.Params("id"), "admin force delete", actorID)
    } else {
        err = dbProvisioner.Deprovision(ctx, c.Params("id"))
    }
    if err != nil {
        // Distinguish remote vs panel errors for correct status code
        return fiber.NewError(fiber.StatusBadGateway, "failed to deprovision database; panel record retained: "+err.Error())
    }
    return c.JSON(fiber.Map{"ok": true})
})
```

**Store — consider soft-delete for deprovision_failed:**
Add column `status` already holds `deprovision_failed` sentinel; no schema change. Alternative is to reuse `managed_databases` soft-delete pattern (`deleted_at` + `208_managed_database_deletion_protection.sql:2`). For `db_containers`, keep hard delete on success, status sentinel on failure.

**Race safety:** Do not set `status=deleting` before daemon call unless you also have a reaper that retries `deprovision_failed` / `deleting`. Simpler: only mutate status on failure (as above). If you must show `deleting` UX, set it **inside** a transaction that is rolled back on daemon error, or set `deprovision_failed` on error and require manual retry.

#### 3B. Restart — real daemon restart

**Current:** `containers.go:300` only `SetDBContainerStatus(...,"running",...)`

**Required:**

```go
func (s *DBContainerService) Restart(ctx context.Context, containerID string) error {
    db, err := s.store.GetDBContainer(ctx, containerID)
    if err != nil { return err }
    if db.ContainerID == "" {
        return errors.New("container not yet provisioned")
    }
    if s.daemon == nil {
        return errors.New("beacon client is not available for container restart")
    }
    // Mark restarting (optional UX)
    _ = s.store.SetDBContainerStatus(ctx, containerID, db.ContainerID, "restarting", db.Port, db.VolumeID, "", nil)
    if err := s.daemon.AdminContainerRestart(ctx, s.beaconBaseURL, s.nodeToken, db.ContainerID); err != nil {
        // Alt: try Stop+Start if Restart not supported for DB images
        // if err := s.daemon.AdminContainerStop(...); err != nil { ... }
        // if err := s.daemon.AdminContainerStart(...); err != nil { ... }
        _ = s.store.SetDBContainerStatus(ctx, containerID, db.ContainerID, "error", db.Port, db.VolumeID, "", nil)
        return fmt.Errorf("restart container via beacon: %w", err)
    }
    // Re-mark running after successful daemon signal; ideally verify via beacon status poll
    return s.store.SetDBContainerStatus(ctx, containerID, db.ContainerID, "running", db.Port, db.VolumeID, "", nil)
}
```

**Daemon method availability:** Verify `daemon.Client.AdminContainerRestart` exists (`forge/api/internal/daemon`); if only `AdminContainerStop/Start`, compose restart via stop+start. Add `RestartDatabase` RPC if needed.

**Check `database_service_provisioner.go:231` `StopService` / `:245` `StartService`:** Those already correctly delegate to daemon (`AdminContainerStop/Start`). `Restart` should mirror that pattern — extract shared `daemonRestart` helper.

**Tests:**
- `TestDeprovisionPropagatesBeaconError` — mock daemon returning error, assert `DeleteDBContainer` not called, status set to `deprovision_failed`, error propagated.
- `TestRestartCallsDaemon` — mock daemon, assert `AdminContainerRestart` called, status transitions `restarting → running` on success, `restarting → error` on failure.
- `TestDeprovisionForceIgnoresError` — force path deletes even when daemon fails.

#### 3C. Status-before-delete race (database services)

`database_service_provisioner.go:259` `DeleteService` sets `deleting` before daemon call. Fix options:

1. **Optimistic (recommended for beacon):** Remove pre-status write; only write `failed` on error, nothing on pending. Client polls `status` via `GetDatabaseService`; UX remains `running` until delete succeeds → row disappears. No `deleting` flicker.
2. **Pessimistic with reaper:** Keep `deleting` but add `reaper` goroutine (like `recovery.Coordinator` or `evacuationplanner`) that retries `DeleteService` for rows stuck in `deleting` > N minutes. Add `deleted_at` column + `WHERE status='deleting' AND updated_at < now() - interval '1 hour'` sweep.

Implementation for option 1 (simpler):
```go
func (p *DatabaseServiceProvisioner) DeleteService(ctx context.Context, id string) error {
    svc, err := p.store.GetDatabaseService(ctx, id)
    if err != nil { return err }
    if svc.ContainerID != "" {
        if p.daemon == nil { return errors.New("daemon not available for database deletion") }
        if err := p.daemon.DeProvisionDatabase(ctx, p.beaconBaseURL, p.nodeToken, svc.ContainerID, svc.VolumeID); err != nil {
            _ = p.store.UpdateDatabaseServiceStatus(ctx, id, "failed", "", 0, "", "", "", "", "", "", nil)
            return fmt.Errorf("delete database container: %w", err)
        }
    }
    return p.store.DeleteDatabaseService(ctx, id)
}
```
Remove the `UpdateDatabaseServiceStatus(...,"deleting",...)` line.

**Backward compat:** `status=deleting` may be observed by frontend polling (`forge/web/lib/api/database-services.ts`?). If frontend relies on `deleting` spinner, keep it but add `FOR UPDATE` lock around the transition or use outbox pattern. Document in CHANGELOG.

---

### Phase 4 — Password Generation Unified (0.5 day)

**Goal:** Single `secrets.GenerateDatabasePassword` / `store.newDaemonToken` source; eliminate hex vs base64url divergence.

**Audit divergence:**

| File:Line | Function | Callers |
|---|---|---|
| `store/store_databases.go:194` | `NewServerDatabasePasswordCandidate() string { return newDaemonToken()[:32] }` | `dbprovisioner/service.go:260` RotatePassword |
| `dbprovisioner/containers.go:44` | `generatePassword(length int)` hex truncated | `Provision:212`, `ProvisionDevFallback:271`, `envVarsForDB:114` root PW64 |
| `services/database_service_provisioner.go:63` | `generatePassword(length int)` hex alloc `length` | `ProvisionService:196`, `CreateUser` not (caller-supplied) |

**Unify:**

**1. Create `forge/api/internal/secrets/password.go`:**
```go
package secrets

import (
    "crypto/rand"
    "encoding/base64"
    "math/big"
)

// GenerateDatabasePassword returns a 32-char, URL-safe, high-entropy password.
// Use for all database credential paths (provision, rotate, root).
func GenerateDatabasePassword(length int) string {
    if length <= 0 { length = 32 }
    // base64url: 4 chars per 3 bytes. Generate enough bytes then truncate.
    // For length 32, need ceil(32*3/4)=24 bytes → 32 chars base64url without padding.
    byteLen := (length*3 + 3) / 4
    b := make([]byte, byteLen)
    if _, err := rand.Read(b); err != nil { panic("crypto/rand failed: " + err.Error()) }
    s := base64.RawURLEncoding.EncodeToString(b)
    if len(s) > length { s = s[:length] }
    // Ensure at least one of each class if length>=12? Optional policy.
    return s
}

// GenerateHexPassword for cases where hex alphabet required (e.g. legacy compat)
// Deprecated: prefer GenerateDatabasePassword.
func GenerateHexPassword(length int) string { /* keep for backward compat, delegate */ }

// Legacy: newDaemonToken already exists in store/store.go — re-export or delegate
func NewDaemonToken() string { /* existing impl */ }
```

**2. Refactor callers:**

```go
// store/store_databases.go:194
func (s *Store) NewServerDatabasePasswordCandidate() string { return secrets.GenerateDatabasePassword(32) }

// dbprovisioner/containers.go:44 — delete local generatePassword, import secrets
password := secrets.GenerateDatabasePassword(32)
rootPassword := secrets.GenerateDatabasePassword(64) // was generatePassword(64) hex

// services/database_service_provisioner.go:63 — delete local, use secrets
password := secrets.GenerateDatabasePassword(32)

// services/catalog/catalog.go:241 — randomPassword(24) → secrets.GenerateDatabasePassword(24)
```

**3. Deprecate `generateDBName/generateUsername`:** Keep but ensure they remain hex (DB name constraints). No change needed.

**4. Entropy note:** 32-char base64url = 192 bits entropy (6 bits/char × 32). Previous hex 32-char = 128 bits (4 bits/char × 32). Upgrade is intentional and compatible (DB `password_encrypted` column is opaque).

**Backward compat:** Existing stored passwords remain valid (encrypted). Only new provisions/rotations use new alphabet — acceptable. Document in migration notes.

**Test:**
```go
func TestGenerateDatabasePasswordLengthAndAlphabet(t *testing.T) {
    for _, l := range []int{24,32,64} {
        pw := secrets.GenerateDatabasePassword(l)
        if len(pw)!=l { t.Fatalf(...) }
        if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(pw) { t.Fatalf(...) }
    }
    // uniqueness
    pw1 := secrets.GenerateDatabasePassword(32)
    pw2 := secrets.GenerateDatabasePassword(32)
    if pw1==pw2 { t.Fatal("collision") }
}
```

**Also fix string-sniff fallback:** Replace `strings.Contains(err.Error(), "already registered")` (`service.go:378`) with sentinel error check or typed `mysql.MySQLError`. With mutex-guarded Deregister+Register pattern, the sniff becomes unnecessary — remove it.

---

### Phase 5 — Catalog Single Source of Truth & Seeding (2 days)

This is the largest seam. Strategy: **FS `packages/game-templates/templates/*.json` is canonical**, `eggs` is runtime store, `DefaultSeeder`-style idempotent upsert bridges them, `egg-templates.ts` becomes generated artifact or deprecated, `localStorage` path deprecated with migration.

#### 5.1 Decision: Canonical source

| Option | Pros | Cons |
|---|---|---|
| **A. FS JSON canonical** | Richer `template-schema.json:3` (config.files, resources, install_script), `jq` friendly, `index.json` registry, close to Puffer spec | Requires FS at build + seed at boot; deleted on disk now |
| B. `egg-templates.ts` canonical | Single TS import, no FS I/O | Loses `resources/disk/cpu_shares`, `file_denylist`, `install_script` richness; stringly-typed |
| C. DB canonical only | No seeding needed | No version-controlled definitions; drift |

**Choose A** (FS canonical), with TS as generated fallback for dev without FS.

**Restore `packages/game-templates` from HEAD:**
```bash
git checkout HEAD -- packages/game-templates/
# verify 14 JSON + index.json + scripts/validate-templates.mjs
ls packages/game-templates/templates/*.json | wc -l # expect 14
```

If 14→44 expansion desired (Puffer parity), incrementally curate additional templates (see 5.4).

#### 5.2 Seeder service: `eggseeder`

**Pattern to copy:** `forge/api/internal/services/appstore/seed.go:215` `SeedDefaultApps` called at `forge/api/cmd/api/main.go:529`:
```go
if err := appStoreSvc.SeedDefaultApps(appCtx); err != nil {
    slogLogger.Warn("seed default app store apps", slog.String("error", err.Error()))
}
```

**Create `forge/api/internal/services/eggseeder/service.go`:**

```go
package eggseeder

import (
    "context"
    "encoding/json"
    "log/slog"
    "os"
    "path/filepath"
    "strings"

    "gamepanel/forge/internal/store"
)

//go:embed templates/*.json  // alternative: read from disk via env var GAME_TEMPLATES_DIR
// Use disk path for flexibility; embed for single-binary deploys.

type Service struct {
    store   *store.Store
    dir     string // e.g. "packages/game-templates/templates" or "/app/game-templates/templates"
    logger  *slog.Logger
}

func New(s *store.Store, dir string, logger *slog.Logger) *Service {
    if logger==nil { logger=slog.Default() }
    if dir=="" { dir="packages/game-templates/templates" }
    return &Service{store:s, dir:dir, logger:logger}
}

// SeedEggs is idempotent: upserts eggs + egg_variables from FS JSON.
// Semantics match appstore.UpsertAppStoreApp: ON CONFLICT (nest_id,name) DO UPDATE
// but via store.CreateEgg/UpdateEgg path to preserve normalization.
func (s *Service) SeedEggs(ctx context.Context) error { ... }
```

**Upsert key:** `(nest_id, name)` unique (see `043_unify_eggs_templates_mounts.sql:43` `ON CONFLICT (nest_id, name)`). Use nest `"Games"` (or create `games` nest if missing).

**FS JSON → Egg mapping:**

FS `template-schema.json:3` fields:
```
id, name, description, version, game, image, images{}, startup, config{files,startup,stop,logs},
ports[], env[{name, env_variable, default_value, user_viewable, user_editable, rules}], resources{cpu,memory_mb, ...},
install_script{container, entrypoint, script}
```

To `store.CreateEggRequest:59`:
```go
CreateEggRequest{
    NestID:            gamesNestID,
    Name:              fs.Name, // or fs.ID for stability? Use fs.Name for display, but ensure uniqueness via Name
    Description:       fs.Description,
    DockerImages:      marshalImages(fs.Image, fs.Images), // map[string]string
    Startup:           fs.Startup,
    Config:            marshalConfig(fs.Config),
    DefaultMemoryMB:   fs.Resources.MemoryMB, // fallback 1024
    InstallScript:     fs.InstallScript.Script,
    InstallContainer:  fs.InstallScript.Container, // default alpine:3.21
    InstallEntrypoint: fs.InstallScript.Entrypoint, // default sh
    FileDenylist:      json.RawMessage(`[]`),
    // Author, UpdateURL, Features, StartupCommands left empty or from fs if present
}
```
Then for each `fs.Env` → `store.CreateEggVariable` or bulk upsert via `store.UpdateEgg`? Currently `egg_variables` are separate (`store_egg_variables.go`). Seeder should upsert variables after egg.

**Egg variable upsert:**

FS `env` element: `{name, env_variable, default_value, user_viewable, user_editable, rules}` → `store.EggVariable` fields `Name, EnvVariable, DefaultValue, UserViewable, UserEditable, Rules, Sort`.

Implement `UpsertEggVariables` helper that:
- Lists existing variables for egg (`ListEggVariables`)
- For each FS entry, `CreateEggVariable` or `UpdateEggVariable` by `EnvVariable` key
- Deletes stale vars no longer in FS (optional, but needed for deficit closure)

**Idempotence guarantee:** `SeedEggs` must be safe to run on every boot (like `SeedDefaultApps`). It must not overwrite user-edited eggs if admin customized a template? Options:

- **Overwrite always** (appstore behavior: `UpsertAppStoreApp:92` always overwrites on conflict). Simple, ensures FS fixes propagate.
- **Overwrite only if not user_edited** — check `updated_at` vs seed timestamp? More complex.

Choose **overwrite always** for now (consistent with appstore). Add `seeded_by` marker column if fine-grained control needed later.

**Wire in `main.go`:**

```go
// After appStoreSvc.SeedDefaultApps
eggSeeder := eggseeder.New(db, env("GAME_TEMPLATES_DIR", "packages/game-templates/templates"), slogLogger)
if err := eggSeeder.SeedEggs(appCtx); err != nil {
    slogLogger.Warn("seed game eggs from templates", slog.String("error", err.Error()))
}
```

**Alternative embed:** For Docker images where `packages/game-templates` not on disk, embed templates via `go:embed`:

```go
//go:embed all:templates
var embeddedTemplates embed.FS
// fallback if dir not found
```

#### 5.3 Migration to seed 14 templates (DB migration, not just runtime seeder)

Provide both: **runtime seeder** (every boot) + **migration** that seeds eggs for existing installations that may not reboot through `main.go` seeder path during migration run.

**New migration:** `forge/api/migrations/XXX_seed_game_eggs.sql` **or** better, `XXX_seed_game_templates_via_seeder.go` migration is hard to maintain as JSON evolves. Prefer **SQL migration that inserts placeholder eggs** then runtime seeder reconciles on next boot.

**Simpler: No SQL migration; rely solely on `SeedEggs` at boot.** Migration `091_seed_minecraft_java.sql:3` already seeds 1; `SeedEggs` will upsert to 14 (expand to 15 with Minecraft Java replaced by 3 variants). This is additive and idempotent — no backfill SQL needed. However for environments where `main.go` seed is skipped (e.g., `SeedDemo` false?), ensure `SeedEggs` always runs (not gated by `seedDemo:139`).

**Nest handling:**

```sql
-- In seeder Go code, ensure nests exist:
INSERT INTO nests (id, name, description) VALUES
  (gen_random_uuid(), 'Games', 'Game server templates')
ON CONFLICT (name) DO NOTHING;
```

Ensure `seeder.go:38` `DefaultSeeder` also registers `game-eggs` entry so `RunMigrations` path seeds even if `main.go` seeder not invoked during tests.

**Seeder registration alternative — in `store/seeder.go`:**

```go
func DefaultSeeder(store *Store) *Seeder {
    s := NewSeeder(store)
    s.Register("default-roles", ...)
    s.Register("default-settings", ...)
    s.Register("game-eggs", func(ctx context.Context, st *Store) error {
        // Embed or read FS; fallback to hardcoded 14 if FS missing
        svc := eggseeder.New(st, "", nil)
        return svc.SeedEggs(ctx)
    })
    return s
}
```
But `eggseeder` depends on FS I/O; for pure SQL migration tests (sqlite driver), FS may not exist. Keep `DefaultSeeder` lean; keep runtime `SeedEggs` in `main.go` as primary.

#### 5.4 14→44 expansion (optional, P2)

If product wants Puffer parity (36 types), curate incrementally:

| Phase | Templates | Justification |
|---|---|---|
| **5.4a Immediate** | 14 FS → 14 eggs (1→14) | Close 93% deficit; ship what already exists |
| 5.4b Near-term | +10 (ark, arma3, gmod, eco, squad, minecraft-curseforge/fabric/forge, etc.) | High demand; transliterate from Puffer `*.json` + 24 ops → shell (requires `javadl`/`cur seforge` handling) |
| 5.4c Full | +22 → 36+ | Complete Puffer parity; steward as community PRs |

Add donut chart to plan appendix: Puffer 36 vs Forge 14 vs DB 1 vs disk 0.

**Interop note:** Puffer's 24 ops (`fabricdl`, `mojangdl`, `steamgamedl`, etc.) transliterate to shell `curl+jq` as in `minecraft-paper.json:114` already — validate pattern scales.

#### 5.5 Deprecate `localStorage` / unify `app-templates-data.ts`

**File:** `forge/web/lib/app-templates-data.ts:3`

**Current:** `STORAGE_KEY="forge.app-templates.v1"` + `DEFAULT_APP_TEMPLATES:5` (`nginx, node, python, postgres-compose, redis`) + `loadUserTemplates:56` / `saveUserTemplates:66` / `getAllTemplates:71` — never hits DB.

**Plan:**

1. **Keep `DEFAULT_APP_TEMPLATES` as offline fallback only** (if API unreachable). Add comment deprecation.

2. **Add DB-backed helpers:**
```ts
// forge/web/lib/api/eggs.ts (new)
export type Egg = { id: string; nestId: string; name: string; startup: string; config: Record<string,any>; dockerImages: Record<string,string>; installScript: string; ... }
export async function fetchEggs(nestId?: string): Promise<Egg[]> {
  const qs = nestId ? `?nestId=${nestId}` : ""
  return apiFetch(`/api/v1/nests/eggs${qs}`) // or /templates delegated to eggs via store_templates.go:13
}
export async function fetchAppStoreApps(): Promise<AppStoreApp[]> { ... }
```

3. **Migration CTA for existing localStorage users:**
```ts
export function hasLocalTemplates(): boolean {
  if (typeof window==="undefined") return false
  return JSON.parse(window.localStorage.getItem(STORAGE_KEY) ?? "[]").length>0
}
// In admin UI:
if (hasLocalTemplates()) showBanner("You have 3 local templates — Import to DB?", onImport: async()=>{
  const locals = loadUserTemplates()
  await Promise.all(locals.map(t=> importTemplateToEgg(t))) // map AppTemplate → CreateEggRequest
  window.localStorage.removeItem(STORAGE_KEY)
})
```

4. **Remove dual UI split:** Merge `AdminNestsEggs.tsx` (eggs) + `app-templates/page.tsx:1` (localStorage) into single `TemplateGallery` component (see Phase 7). `app-templates/page.tsx` should delegate to eggs API by default, localStorage only when `?source=local`.

5. **Backward compat:** `fetchAppTemplates()` (`lib/api/apps.ts:369`) currently falls back to `getAllTemplates()`. Change to: try `fetchEggs()` first, fallback to `getAllTemplates()` only if API 404/503 (offline). Document deprecation timeline: localStorage read-only after v1.1, removed v2.0.

#### 5.6 `store_catalog.go` vs eggs unification note

Do **not** merge `catalog_entries` (11 service kinds like `valkey, rabbitmq`) with game eggs — they serve distinct personas (infra services vs game servers). However ensure doc clarity:

| Table | Persona | Provision Runtime |
|---|---|---|
| `eggs` | Gamer / Host admin | `daemon.CreateServer` (wings) |
| `catalog_entries` | Dev / Infra admin | `DBProvider` / `ComposeProvider` (`catalog.Service`) |
| `app_store_apps` | Marketplace browser | `compose.Service` |

If unified gallery desired, add `kind` discriminator column to gallery API: `GET /api/v1/templates?kind=game|service|app`.

---

### Phase 6 — Template Validation Hardening (0.5 day)

**File:** `packages/game-templates/scripts/validate-templates.mjs:46` (from worktree, to be restored)

**Current stub (reconstructed from worktree):**
```js
// validate-templates.mjs:46 — only scans startup + config.files
collectPlaceholders(content.startup, placeholders);
collectPlaceholders(content.config.files, placeholders);
// missing: install_script.script
```

**Required extensions:**

```js
const REQUIRED = ['id','name','description','version','game','image','startup','config','ports','env','resources','install_script','supported_platforms','categories']; // keep

const BUILTIN_VARIABLES = new Set([
  'SERVER_PORT','SERVER_IP','SERVER_MEMORY','SERVER_UUID','P_SERVER_UUID','STARTUP',
  'server.build.default.port','server.build.default.ip','server.build.default.ip_alias',
  // add if used in templates: expose list from forge/api startup variable injection
]);

// 1. Expand placeholder collection to install_script
const placeholders = [];
collectPlaceholders(content.startup, placeholders);
if (content.config && content.config.files) collectPlaceholders(content.config.files, placeholders);
if (content.install_script && content.install_script.script) {
  collectPlaceholders(content.install_script.script, placeholders);
}
// Optional: also scan content.ports (if ports contain {{VAR}}), but usually static.

// 2. Validate every placeholder resolves
for (const placeholder of new Set(placeholders)) {
  if (!BUILTIN_VARIABLES.has(placeholder) && !envVariables.has(placeholder)) {
    errors.push(`${file}: placeholder "{{${placeholder}}}" in startup/config/install_script is not a defined env variable or built-in`);
  }
}

// 3. New: Validate install_script required fields
if (!content.install_script || typeof content.install_script.script !== 'string' || !content.install_script.script.trim()) {
  errors.push(`${file}: install_script.script is required and must be non-empty`);
}
if (content.install_script) {
  if (!content.install_script.container) errors.push(`${file}: install_script.container is required (e.g. alpine:3.21)`);
  if (!content.install_script.entrypoint) errors.push(`${file}: install_script.entrypoint is required (e.g. sh)`);
  // 3b. Heuristic: install_script should not use string-sniff fallback "{{" literal without env
  if (/\{\{\s*\}\}/.test(content.install_script.script)) {
    errors.push(`${file}: install_script contains empty placeholder "{{}}"`);
  }
}

// 4. New: Validate Steam appId numeric literal in script (fragile magic)
// Heuristic: find occurrences of "steamcmd.*+app_update" or "+app_update"
const steamAppIds = [...content.install_script.script.matchAll(/\+app_update\s+(\S+)/g)].map(m=>m[1]);
for (const rawId of steamAppIds) {
  const id = rawId.replace(/["']/g,'').trim();
  if (!/^\d{4,7}$/.test(id) && !id.startsWith('{{') && !id.startsWith('${')) {
    // Allow variable-interpolated appIds, but flag unquoted non-numeric literals
    errors.push(`${file}: install_script steam appId "${rawId}" should be numeric 4-7 digits or {{VAR}}`);
  }
}

// 5. New: Validate groups / conditional vars if present (future Puffer parity)
// If template.groups exists, ensure each group.variables subset of envVariables
if (content.groups) {
  for (const [gi, g] of content.groups.entries()) {
    for (const v of (g.variables||[])) {
      if (!envVariables.has(v)) errors.push(`${file}: groups[${gi}].variables "${v}" not in env[]`);
    }
    // if: string is freeform — no semantic check yet
  }
}

// 6. New: Validate stdin/stdout if present
if (content.stdin && !['rconws','telnet','file','none'].includes(content.stdin.type)) {
  errors.push(`${file}: stdin.type must be one of rconws|telnet|file|none`);
}

// 7. New: Validate ports ↔ startup gap (Beacon/RCON port present in startup but missing in ports[])
const startupStr = typeof content.startup==='string' ? content.startup : JSON.stringify(content.startup);
const envPortVars = new Set([...startupStr.matchAll(/\{\{\s*([A-Z_]+PORT)\s*\}\}/g)].map(m=>m[1]));
// Map PORT var → ports[].port resolution is indirect; heuristic: if startup uses {{BEACON_PORT}} ensure ports contains beacon description or env defines it
for (const pv of envPortVars) {
  // check if pv defined in env[]
  if (!envVariables.has(pv)) {
    errors.push(`${file}: startup uses "{{${pv}}}" but no env variable "${pv}" defined`);
  }
}
// Also ensure ports[].port are within range already at :56-60 — extend to check public/protocol
if (content.ports) {
  for (const [i,p] of content.ports.entries()) {
    if (typeof p.port!=='number' || p.port<1 || p.port>65535) errors.push(`${file}: ports[${i}].port out of range`);
    if (!['tcp','udp'].includes(p.protocol)) errors.push(`${file}: ports[${i}].protocol must be tcp|udp`);
    if (p.public!==undefined && typeof p.public!=='boolean') errors.push(`${file}: ports[${i}].public must be boolean`);
  }
}

// 8. New: Validate config.files and startup.done+stop semantics
if (content.config) {
  if (content.config.startup && !content.config.startup.done) {
    errors.push(`${file}: config.startup.done is required (regex for server ready)`);
  }
  if (content.config.stop && typeof content.config.stop!=='string') {
    errors.push(`${file}: config.stop must be string`);
  }
  // config.files values should be objects with parser/file keys — validate shape
  if (content.config.files) {
    for (const [fname, fdef] of Object.entries(content.config.files)) {
      if (!fdef || typeof fdef!=='object') errors.push(`${file}: config.files["${fname}"] must be object`);
      // ensure parser is known: properties|yaml|json|ini|xml etc.
    }
  }
}

// 9. New: Validate appId collision with BUILTIN_VARIABLES (should not overlap user env)
for (const v of (content.env||[])) {
  if (BUILTIN_VARIABLES.has(v.env_variable)) {
    errors.push(`${file}: env_variable "${v.env_variable}" collides with built-in`);
  }
}
```

**DB-side validation mirror:**

`forge/api/internal/store/store_nests.go:232` `CreateEgg` calls `normalizeJSONObject` but does not validate `Startup` placeholders — add server-side `validateEggPlaceholders` that reuses same `BUILTIN_VARIABLES` set (define in `store_egg_variables.go` or `services/validation`). This ensures manual `AdminNestsEggs.tsx:124` paste-JSON is validated server-side even if FS validator bypassed.

```go
var BuiltinVariables = map[string]struct{}{
    "SERVER_PORT": {}, "SERVER_IP": {}, "SERVER_MEMORY": {}, "SERVER_UUID": {},
    "P_SERVER_UUID": {}, "STARTUP": {},
    "server.build.default.port": {}, "server.build.default.ip": {}, "server.build.default.ip_alias": {},
}

func validateEggPlaceholders(startup string, config json.RawMessage, installScript string, envVars []EggVariable) error {
    // extract {{VAR}} via regexp, check each in BuiltinVariables or envMap
}
```

**CI integration:**

`package.json` already has `prebuild: node scripts/validate-templates.mjs` (inferred). Ensure:

```json
{
  "scripts": {
    "validate": "node scripts/validate-templates.mjs",
    "prebuild": "npm run validate",
    "test": "npm run validate && vitest run"
  }
}
```

Add to `forge/api` CI: `go vet` + `npm run validate` in same pipeline; fail build if templates missing.

**Backward compat:** New checks are additive; existing 14 templates should pass (verify `minecraft-paper.json:114` install_script with `{{DL_PATH}}` style — if it uses `sed s/{{/${/g`, the placeholder extraction will see `{{DL_PATH}}` before transform — ensure test covers). If any existing template fails, fix template not validator.

---

### Phase 7 — Frontend: DB Fleet + Template Gallery (1.5 days)

#### 7A. DB Services Fleet View with Masked Credentials

**Current:** `forge/web/lib/api/database-containers.ts` (and `database-services.ts`) likely already have `listDatabaseContainers()`. `forge/web/app/admin/databases/page.tsx:7` has tabs `hosts/containers/managed` — containers tab calls `ListAllDBContainers`.

**Target:**

**1. API contract (no backend change except Phase 1 decrypt):**

| Endpoint | Method | Response | Note |
|---|---|---|---|
| `GET /api/v1/databases/containers` | GET | `DBContainer[]` (without plaintext) + `maskedConnectionString` | Fleet view — masked |
| `GET /api/v1/databases/containers/:id/credentials` | GET | `{credentials: {username,password,database}, connectionString: string}` | Existing `:139` — full, gated by `databases.read` |

If `store.DBContainer` still `json:"-"` on credentials, fleet DTO must explicitly include masked form (see Phase 1). Frontend shows `***` password.

**2. Frontend component `forge/web/components/admin/DatabaseFleet.tsx` (or extend existing `page.tsx:7`):**

```tsx
"use client"
import { useEffect, useState } from "react"
import { listDatabaseContainers, getDatabaseCredentials, restartDatabaseContainer, deleteDatabaseContainer } from "@/lib/api/database-containers"

type DBContainer = { id:string; engine:string; version:string; status:string; port:number; connectionStringMasked?:string; hasCredentials:boolean; serverId:string; createdAt:string }

export function DatabaseFleet({ serverId }: { serverId?: string }) {
  const [containers, setContainers] = useState<DBContainer[]>([])
  const [creds, setCreds] = useState<Record<string,{username:string,password:string}>>({})
  const [revealed, setRevealed] = useState<Record<string,boolean>>({})

  useEffect(()=>{ listDatabaseContainers(serverId).then(setContainers) },[serverId])

  async function onReveal(id:string){
    const { credentials, connectionString } = await getDatabaseCredentials(id)
    setCreds(c=>({...c,[id]:credentials}))
    setRevealed(r=>({...r,[id]:true}))
  }
  async function onCopy(id:string){
    await navigator.clipboard.writeText(creds[id]?.password ?? "")
  }
  // render table: Engine | Version | Status (badge running/failed/deprovision_failed) | Port | Connection (masked, Reveal button) | Actions (Restart, Delete with confirm, Force checkbox)
}
```

**UX details:**
- Status badges: `running` green, `failed`/`error` red, `restarting` yellow, `deprovision_failed` orange with tooltip `Delete failed — manual cleanup required. Retry or Force delete.`
- Restart button calls `POST /databases/containers/:id/restart` `:127` — now real daemon restart (Phase 3B). Show spinner until `running`.
- Delete button calls `DELETE /:id` — on `502 BadGateway` (`failed to deprovision; panel record retained`) keep row, show error toast with retry/force.
- Port column shows `port` from `DBContainer`.
- Credentials not auto-fetched; require explicit Reveal (audited via `database_host.deleted` audit?).
- Masking helper `maskPassword(connStr)` in `lib/utils.ts` — replace `://[^:]+:([^@]+)@` with `://user:***@`.

**3. Permissions:** Fleet view requires `databases.read` (`handlers_db_containers.go:67` already `requireAdminScope("databases.read")`). Credentials reveal also same scope; consider stricter `databases.credentials.read` if sensitivity high.

**4. Backward compat:** If backend still returns empty plaintext, frontend degrades to "No credentials — use detail view" hint.

#### 7B. Template Gallery — DB-Backed, Unified

**Goal:** Replace `localStorage` + `EGG_TEMPLATES` split with DB `eggs` gallery, import/export PTDL_v2.

**1. Backend API (existing, just wire):**

| Endpoint | Store | Notes |
|---|---|---|
| `GET /api/v1/nests` | `store_nests.go:100` ListNests | Nest gallery |
| `GET /api/v1/nests/:nestId/eggs` | `store_nests.go:190` ListEggs | Egg list (gallery) |
| `GET /api/v1/eggs/:id` | `store_nests.go:224` GetEgg | Detail |
| `POST /api/v1/nests/:nestId/eggs/import` | New: bulk import from JSON | PTDL import |
| `GET /api/v1/eggs/:id/export` | New: export to PTDL_v2 JSON | PTDL export |
| `GET /api/v1/templates` | `store_templates.go:13` ListTemplates (compat over eggs) | Legacy alias — keep |

Add bulk import endpoint:

```go
// handlers_nests.go (new or existing)
protected.Post("/nests/:nestId/eggs/import", requireRole("admin"), func(c *fiber.Ctx) error {
    var req struct {
        Eggs []json.RawMessage `json:"eggs"`
        Overwrite bool `json:"overwrite"`
    }
    if err := c.BodyParser(&req); err != nil { return fiber.NewError(400, "invalid body") }
    // For each raw, validate via validateEggPlaceholders, then CreateEgg or UpdateEgg on conflict
    // Return {imported: n, skipped: m, errors: []}
})
```

**2. Frontend `TemplateGallery` component:**

`forge/web/components/admin/TemplateGallery.tsx`:

```tsx
"use client"
import { useEffect, useState } from "react"
import { fetchEggs, fetchNests, importEggs, exportEgg } from "@/lib/api/eggs"
import { downloadJSON, readJSONFile } from "@/lib/utils"

export function TemplateGallery(){
  const [nests, setNests] = useState<Nest[]>([])
  const [eggs, setEggs] = useState<Egg[]>([])
  const [filter, setFilter] = useState({ search:"", game:"", nestId:"" })
  const [selected, setSelected] = useState<Set<string>>(new Set())

  useEffect(()=>{ fetchNests().then(setNests); fetchEggs(filter.nestId).then(setEggs) },[filter.nestId])

  const filtered = eggs.filter(e=> !filter.search || e.name.toLowerCase().includes(filter.search.toLowerCase()) || e.description.includes(filter.search))

  return (
    <div>
      {/* Header: Search, Nest dropdown, Game filter, Import/Export buttons */}
      <div className="flex gap-2 mb-4">
        <input placeholder="Search templates..." value={filter.search} onChange={e=>setFilter({...filter, search:e.target.value})} />
        <select value={filter.nestId} onChange={e=>setFilter({...filter, nestId:e.target.value})}>
          <option value="">All nests</option>
          {nests.map(n=> <option key={n.id} value={n.id}>{n.name} ({n.eggCount})</option>)}
        </select>
        <label className="btn"><input type="file" hidden accept=".json" onChange={async e=> {
          const raw = await readJSONFile(e.target.files![0])
          const eggs = Array.isArray(raw) ? raw : [raw]
          await importEggs(filter.nestId || nests[0]?.id, eggs, true)
          setEggs(await fetchEggs(filter.nestId))
        }} />Import PTDL_v2</label>
        <button onClick={async()=>{
          const payload = await Promise.all([...selected].map(id=> exportEgg(id)))
          downloadJSON(payload, "templates-ptdl-v2.json")
        }}>Export selected</button>
      </div>
      {/* Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {filtered.map(egg=> (
          <EggCard key={egg.id} egg={egg} selected={selected.has(egg.id)} onToggle={()=> setSelected(s=>{
            const n=new Set(s); n.has(egg.id)?n.delete(egg.id):n.add(egg.id); return n
          })} />
        ))}
      </div>
      {filtered.length===0 && <EmptyState> No templates match. {eggs.length===0 && "Run seeder or import PTDL_v2."} </EmptyState>}
    </div>
  )
}

function EggCard({ egg, selected, onToggle }:{ egg:Egg; selected:boolean; onToggle:()=>void }){
  const images = Object.values(egg.dockerImages ?? {}).join(", ")
  return (
    <div className="border rounded-lg p-4">
      <div className="flex justify-between"><h3>{egg.name}</h3><input type="checkbox" checked={selected} onChange={onToggle}/></div>
      <p className="text-sm text-muted">{egg.description}</p>
      <div className="text-xs">Images: {images || "—"}</div>
      <div className="text-xs">Memory: {egg.defaultMemoryMb} MB</div>
      <div className="text-xs">Startup: <code className="truncate">{egg.startup.slice(0,80)}…</code></div>
      <div className="flex gap-2 mt-2">
        <button onClick={()=> exportEgg(egg.id).then(j=> downloadJSON(j, `${egg.name}.json`))}>Export</button>
        <a href={`/admin/nests/${egg.nestId}/eggs/${egg.id}`}>Edit</a>
      </div>
    </div>
  )
}
```

**3. PTDL_v2 format (PufferPTDL v2):** Define JSON envelope for import/export:

```json
{
  "version": "ptdl_v2",
  "exportedAt": "2026-05-10T00:00:00Z",
  "templates": [
    {
      "id": "minecraft-paper",
      "name": "Minecraft Paper",
      "description": "...",
      "dockerImages": {"Paper":"ghcr.io/papermc/paper:1.21.1-..."},
      "startup": "java -Xms128M -Xmx{{SERVER_MEMORY}}M -jar {{SERVER_JARFILE}}",
      "config": {"startup":{"done":"Done ("}, "stop":"stop", "logs":{"custom":false}, "files":{}},
      "installScript": "#!/bin/sh\ncurl ...",
      "installContainer": "alpine:3.21",
      "installEntrypoint": "sh",
      "variables": [{"envVariable":"SERVER_JARFILE","defaultValue":"server.jar","userViewable":true,"userEditable":true,"rules":"required|string|max:20"}],
      "nestName": "Games"
    }
  ]
}
```

Implement `eggToPTDL(Egg) → PTDLTemplate` and `ptdlToCreateEggRequest(PTDLTemplate) → CreateEggRequest`.

**4. Unify `app-templates/page.tsx:1`:**

Current `page.tsx:1` has freeform `host:container` and `KEY=value` CSV `formToTemplate:39`. Refactor to:

```tsx
// forge/web/app/admin/app-templates/page.tsx
export default function AdminTemplatesPage(){
  const [source, setSource] = useState<"eggs"|"local">("eggs")
  if (source==="eggs") return <TemplateGallery />
  return <LegacyLocalTemplates /> // existing localStorage UI, with deprecation banner
}
```

Or simpler: redirect `/admin/app-templates` → `/admin/nests` and keep legacy at `/admin/app-templates?legacy=1`.

**5. `egg-templates.ts` deprecation:**

Add header:
```ts
/**
 * @deprecated — canonical source is FS packages/game-templates/templates/*.json
 * and DB eggs (seeded via eggseeder). This file is generated from FS at build.
 * Do not edit manually. Run `npm run generate:egg-templates` to regenerate.
 */
// GENERATED — DO NOT EDIT — source: packages/game-templates/templates/*.json
export const EGG_TEMPLATES = [...] as const
```

Generate via `packages/game-templates/scripts/generate-egg-templates.ts` (new) that reads `index.json` + templates and emits `forge/web/lib/egg-templates.generated.ts`.

**6. Styling / UX polish (per `frontend-design` skill):**

- DB fleet: dark table with monospace `connectionStringMasked`, status pills (`running` emerald, `failed` red, `restarting` amber), reveal eye icon.
- Gallery: card grid with image tags as pills, search debounced, nest filter as segmented control.

---

### Phase 8 — Migration SQL & Rollout (0.5 day)

#### 8.1 Migration index allocation

Next available migration is `MAX(existing)+1`. Check `ls forge/api/migrations/*.sql` tail — currently `052_schedule_timezone.sql` (from Phase 0 baseline; worktree adds `091…` but current checkout only `001–052`). Reserve `053_seed_game_eggs_bootstrap.sql` if SQL seed needed, or `053_egg_import_bulk_support.sql` if adding helper.

**Preferred: No mandatory SQL migration.** Runtime `SeedEggs` covers seeding. If a SQL migration is desired for tooling that only runs `RunMigrations` without `main.go` boot, add idempotent stub:

`forge/api/migrations/053_seed_game_eggs_placeholder.sql`:
```sql
-- Placeholder: game eggs are seeded at boot via eggseeder.Service (see forge/api/internal/services/eggseeder).
-- This migration ensures the Games nest exists for early installers that never boot through the seeder.
INSERT INTO nests (id, name, description)
VALUES (gen_random_uuid(), 'Games', 'Game server templates')
ON CONFLICT (name) DO NOTHING;

-- Existing 091_seed_minecraft_java.sql row remains; additional eggs are reconciled by the seeder.
-- Intentionally no egg inserts here — see packages/game-templates/templates/*.json as source.
```

#### 8.2 Full migration SQL if choosing DB seed (alternative)

`053_seed_game_eggs.sql` (14 inserts, illustrative — generated from FS via `scripts/sqlgen.mjs`):

```sql
-- GENERATED from packages/game-templates/templates/*.json — DO NOT EDIT MANUALLY
-- Idempotent upsert of 14 game eggs into Games nest.

WITH games_nest AS (
  SELECT id FROM nests WHERE name='Games' LIMIT 1
),
new_eggs(id, nest_id, name, description, docker_images, startup, config,
         default_memory_mb, install_script, install_container, install_entrypoint, file_denylist, author) AS (VALUES
  (gen_random_uuid(), (SELECT id FROM games_nest), '7 Days To Die', '7 Days To Die dedicated server', '{"7 Days To Die":"didstopia/7dtd-server:latest"}'::jsonb, './7DaysToDieServer.x86_64 -configfile=serverconfig.xml -port={{SERVER_PORT}} ...', '{"startup":{"done":"INF startGame done"},"stop":"shutdown","logs":{}}'::jsonb, 4096, '#!/bin/sh\nsteamcmd ... +app_update 294420 ...', 'alpine:3.21','sh','[]'::jsonb,'Forge Team'),
  -- ... 13 more rows
  (gen_random_uuid(), (SELECT id FROM games_nest), 'Minecraft Paper', 'Paper MC server', '{"Java 21":"ghcr.io/papermc/paper:1.21.1-123"}'::jsonb, 'java -Xms128M -Xmx{{SERVER_MEMORY}}M -jar {{SERVER_JARFILE}}', '{"startup":{"done":"Done ("},"stop":"stop","logs":{"custom":false}}'::jsonb, 1024, '#!/bin/sh\ncurl ...papermc.io...', 'alpine:3.21','sh','[]'::jsonb,'Forge Team')
)
INSERT INTO eggs (id, nest_id, name, description, docker_images, startup, config, default_memory_mb, install_script, install_container, install_entrypoint, file_denylist, author)
SELECT id, nest_id, name, description, docker_images, startup, config, default_memory_mb, install_script, install_container, install_entrypoint, file_denylist, author FROM new_eggs
ON CONFLICT (nest_id, name) DO UPDATE SET
  description=EXCLUDED.description,
  docker_images=EXCLUDED.docker_images,
  startup=EXCLUDED.startup,
  config=EXCLUDED.config,
  default_memory_mb=EXCLUDED.default_memory_mb,
  install_script=EXCLUDED.install_script,
  updated_at=NOW();
```

**Variable inserts:** Follow with `INSERT INTO egg_variables (id, egg_id, name, env_variable, default_value, user_viewable, user_editable, rules, sort)` per `env[]`, with `ON CONFLICT (egg_id, env_variable) DO UPDATE`.

**Recommendation:** Do not hand-maintain SQL for 14×~6 vars = 84 rows. Use Go seeder to generate SQL or rely solely on Go seeder.

#### 8.3 Backward compat table

| Change | Compat | Migration | Rollback |
|---|---|---|---|
| `List*` adds encrypted cols | ✓ additive `COALESCE(...,'')`; old rows fallback to plaintext | none | revert SELECT |
| TLS mutex guard | ✓ names stable | none | revert file |
| `Deprovision` error propagation | ⚠️ behavior change: previously always deleted, now retains on daemon failure | document; add `?force=true` for old behavior | keep `ForceDeprovision` |
| `Restart` daemon call | ⚠️ previously no-op, now real restart | announce; may surface latent beacon errors | revert |
| `GenerateDatabasePassword` alphabet | ✓ encrypted column opaque; no wire format change | none | revert |
| `SeedEggs` 1→14 | ✓ additive `ON CONFLICT DO UPDATE`; existing `Minecraft Java` variant matched by `(nest_id,name)` — if name differs (`Minecraft Java` vs `Minecraft Paper/Vanilla`) both coexist (14+1=15) — dedup by normalizing | ensure nest `Games` exists | `DELETE FROM eggs WHERE name IN (...)` |
| `validate-templates.mjs` new checks | ✓ additive; may fail previously-passing templates | fix templates | relax checks |
| `app-templates-data.ts` deprecate | ✓ localStorage retained as fallback; import CTA | `localStorage.removeItem` only on user action | restore |
| Fleet masked DTO | ✓ new field `connectionStringMasked`; old clients ignore | none | remove field |

---

## 4. Testing Strategy

| Layer | Test | Command | Expect |
|---|---|---|---|
| Store unit | `TestListDBContainersDecryptsEncrypted` | `go test ./forge/api/internal/store -run TestList -count=1 -race` | List returns non-empty after `SetDBContainerStatus` |
| Store unit | `TestListAllDBContainersDecryptsEncrypted` | same | same for fleet limit |
| Provisioner race | `TestConnectorForHostConcurrent` | `go test ./forge/api/internal/services/dbprovisioner -run Concurrent -race` | no race, no panic |
| Provisioner unit | `TestDeprovisionPropagatesBeaconError` | `go test ./forge/api/internal/services/dbprovisioner -run Deprovision` | status `deprovision_failed`, row retained |
| Provisioner unit | `TestRestartCallsDaemon` | same | daemon method called, status `running` |
| Password unit | `TestGenerateDatabasePasswordLengthAndAlphabet` | `go test ./forge/api/internal/secrets` | length & charset |
| Seeder unit | `TestSeedEggsIdempotent` | `go test ./forge/api/internal/services/eggseeder` | run twice, count stays 14 |
| Seeder integration | `TestSeededEggCountIsFourteen` | `go test ./... -tags=integration` with temp DB | `SELECT count(*) FROM eggs WHERE nest_id=games` =14 |
| Validation | `npm run validate` | `node packages/game-templates/scripts/validate-templates.mjs` | 0 errors; add negative fixture `__fixtures/bad-install-script.json` |
| Frontend e2e | Playwright `database-fleet.spec.ts` | `pnpm e2e -- database-fleet` | Reveal copies, Delete retains on mocked 502, Restart spinner |
| Frontend e2e | Playwright `template-gallery.spec.ts` | `pnpm e2e -- template-gallery` | Gallery shows 14, search filters, import/export round-trip |

**Coverage target:** `store_db_containers` + `dbprovisioner` + `eggseeder` ≥85%; `validate-templates.mjs` branch coverage for new checks.

---

## 5. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| `go-sql-driver/mysql` version lacks `DeregisterTLSConfig` or race remains | Low | High | Pin to `v1.7+`; mutex guard; add `go test -race` to CI gate |
| Existing `Minecraft Java` egg name collision with new `Minecraft Paper/Vanilla` | Med | Low | Treat `091` seed as legacy; seeder uses distinct names (`Minecraft Java` kept, `Minecraft Paper`/`Vanilla` added) — 15 rows; or rename+delete old in seeder |
| `packages/game-templates` still deleted on disk in some checkouts → CI fails | High | High | Restore from HEAD in Phase 5.1; add CI assertion `test -d packages/game-templates/templates` |
| Frontend reveals credentials logged to console | Med | High | Mask in store already; ensure `GetDBContainerCredentials` audit logs actor; add `console.log` lint |
| `install_script` validator false positives on `sed s/{{/${/g` transform | Med | Med | `collectPlaceholders` should run **before** transform? Actually script contains literal `{{VAR}}`; the `sed` is post-processing in shell — validator correctly expects `{{VAR}}` defined; whitelist that pattern |
| `Deprovision` change breaks `DELETE /databases/containers/:id` callers expecting always-200 | Med | Med | Keep `?force=true` escape hatch; document in CHANGELOG; return `502` only on daemon error (new) vs `200` previously — clients must handle |

---

## 6. Rollout Plan

| Step | Env | Action | Verify |
|---|---|---|---|
| 1 | dev | Apply Phase 1+2+4 (store + TLS + password) | `go test -race ./...`, fleet list shows credentials |
| 2 | dev | Apply Phase 3 (deprovision/restart) | Mock beacon failure → row retained; restart spins container |
| 3 | dev | Restore `packages/game-templates` + Phase 6 validator | `npm run validate` passes; bad fixture fails |
| 4 | dev | Deploy seeder `SeedEggs` via `main.go` | `SELECT count(*) FROM eggs` →14; `GET /api/v1/nests/:id/eggs` →14 |
| 5 | staging | Deploy all + run `go test -tags=integration` against staging DB | E2E fleet + gallery pass |
| 6 | prod | Deploy with feature flag `SEED_EGGS=true` (default on) | Monitor `SeedEggs` logs; alert if count ≠14 |
| 7 | prod | Announce `localStorage` deprecation; show migration banner for 1 release | Track banner dismissal % |
| 8 | next release | Drop plaintext `connection_string/credentials` columns (if policy) + remove `localStorage` fallback | Migration `054_drop_plaintext_db_container_columns.sql` |

---

## 7. Open Questions

1. **Should `ListDBContainers` return masked `connectionString` or omit entirely?** Task says fleet view with credentials (masked) — propose masked DTO. Confirm with security review.
2. **Does `egg_templates.ts` need to remain hand-edited or generated?** Recommend generated (`egg-templates.generated.ts`) to avoid 14×2 source drift.
3. **Should `Deprovision` hard-delete on daemon `NotFound` (already absent)?** Yes — treat `container not found` from beacon as success (idempotent). Add `errors.Is(err, daemon.ErrNotFound)` branch.
4. **Do we need to seed Puffer 44 vs 14?** Recommend 14 now, 44 as follow-up epic with ops transliteration spec (24 ops → shell).
5. **PTDL_v2 export — include `egg_variables`?** Yes — round-trip must preserve `rules`/`userViewable`.

---

## 8. File Checklist (What Changes Where)

```
forge/api/internal/store/store_db_containers.go:174,200   FIX List* to include encrypted cols + decrypt loop
forge/api/internal/store/store_db_containers.go:44         OPTIONAL add DTO/masked field
forge/api/internal/services/dbprovisioner/service.go:373   FIX TLS per-dial (mutex + Deregister+Register, or DialContext)
forge/api/internal/services/dbprovisioner/service.go:417   KEEP mysqlTLSConfigName (used for name)
forge/api/internal/services/dbprovisioner/service.go:422   REVIEW hostTLSConfig verify-ca branch
forge/api/internal/services/dbprovisioner/containers.go:44  REPLACE generatePassword with secrets.GenerateDatabasePassword
forge/api/internal/services/dbprovisioner/containers.go:289 FIX Deprovision propagate error, soft-fail status
forge/api/internal/services/dbprovisioner/containers.go:300 FIX Restart via daemon
forge/api/internal/services/database_service_provisioner.go:63 REPLACE generatePassword
forge/api/internal/services/database_service_provisioner.go:259 FIX DeleteService status-before-delete race
forge/api/internal/store/store_databases.go:194            REPLACE NewServerDatabasePasswordCandidate impl to delegate to secrets
forge/api/internal/secrets/password.go                    NEW single password generator
forge/api/internal/services/eggseeder/service.go          NEW seeder (FS→DB eggs, idempotent)
forge/api/internal/store/seeder.go:38                     OPTIONALLY register game-eggs (or keep main.go only)
forge/api/cmd/api/main.go:529                             ADD eggSeeder.SeedEggs after SeedDefaultApps
forge/api/internal/store/store_nests.go:232                ADD validateEggPlaceholders server-side
forge/api/internal/store/store_egg_variables.go            ADD BuiltinVariables + validation
forge/api/migrations/053_*.sql                            NEW nest existence + (optional) egg seed
packages/game-templates/                                  RESTORE from HEAD (14 JSON + index.json)
packages/game-templates/scripts/validate-templates.mjs:46  EXTEND to install_script, appId, ports vs startup, BUILTIN_VARIABLES:12
packages/game-templates/scripts/generate-egg-templates.ts NEW generator for forge/web/lib/egg-templates.generated.ts
forge/web/lib/app-templates-data.ts:3                     DEPRECATE localStorage, add import CTA
forge/web/lib/api/eggs.ts                                 NEW DB-backed egg API client
forge/web/lib/api/database-containers.ts                  ENSURE masked fleet + reveal
forge/web/components/admin/DatabaseFleet.tsx              NEW or extend page.tsx:7
forge/web/components/admin/TemplateGallery.tsx             NEW gallery (DB-backed, search, PTDL import/export)
forge/web/app/admin/app-templates/page.tsx:1              REFACTOR to delegate to TemplateGallery
forge/web/lib/egg-templates.ts:27                         ADD @deprecated header, eventually generated
forge/api/internal/http/handlers_db_containers.go:67,100,127 UPDATE fleet masking, delete force, restart status
forge/api/internal/http/handlers_nests.go                 ADD /nests/:nestId/eggs/import + /eggs/:id/export
```

---

## 9. References

- `store_db_containers.go:151` `GetDBContainer` — correct encrypted SELECT to mirror
- `store_db_containers.go:233` `SetDBContainerStatus` — encrypts + clears plaintext
- `store_databases.go:194` `NewServerDatabasePasswordCandidate` — current base64url source
- `dbprovisioner/containers.go:44,55` `generatePassword/generateDBName` — hex divergence
- `database_service_provisioner.go:63,69,95,105,132` `generatePassword/envVarsForDB/REDIS_PASSWORD` — divergence + non-standard env
- `dbprovisioner/service.go:373,378,417,422,486,537,625,661` `RegisterTLSConfig` leak + redis `CONFIG REWRITE` ignore + MySQL cleanup
- `store_catalog.go:84` `CatalogEntry` + `store_app_store.go:6` `AppStoreApp` + `store_nests.go:20` `Egg` — fragmentation layers
- `store_templates.go:13` `ListTemplates` delegates to `ListEggs` — shim correct but under-seeded
- `seeder.go:38` `DefaultSeeder` — only `default-roles/settings`, pattern for new `game-eggs`
- `cmd/api/main.go:529` `SeedDefaultApps` — idempotent upsert precedent
- `app-templates-data.ts:3` `STORAGE_KEY="forge.app-templates.v1"` + `DEFAULT_APP_TEMPLATES:5` — localStorage fragmentation
- `migrations/043_unify_eggs_templates_mounts.sql:20` legacy backfill + `ON CONFLICT (nest_id,name)` — upsert key
- `migrations/091_seed_minecraft_java.sql:3` — only seeded egg (baseline)
- Worktree `validate-templates.mjs:17,46,89` `BUILTIN_VARIABLES:12` + placeholder scan limited to `startup+config.files`
- `audit/phase-02/subagent-02` and `final-parity/subagent-06` parity matrices — 93–100% deficit quantification
- Reference: `spec.json:4` `$id .../v3/spec.json` + 24 ops + 42 files — Puffer parity baseline
