# Upgrading Guide — Forge Control Plane + Beacon

> Upgrade with backup, migration, verification, and rollback. Primary source: `forge/api/internal/services/upgrade/service.go` (653 lines). Related: `infra/compose.yml` (`TAG`-pinned images), `infra/postgres-backup.sh`, `infra/gen-env.sh`.

## 0. How Upgrading Works in This Repo

The in-app upgrade orchestrator is `forge/api/internal/services/upgrade/service.go:87-116`:

```go
type Service struct {
    store       Store
    logger      *slog.Logger
    installDir  string // default "/opt/gamepanel"
    backupDir   string // default "/var/backups/gamepanel"
    versionFile string // default "$installDir/version.txt"
}
```

**Upgrade model:**
- `UpgradeType`: `api` | `web` | `beacon` | `full` | `database` (`service.go:30-39`)
- `UpgradeStatus`: `pending` → `downloading` → `backing_up` → `upgrading` → `completed` | `failed` → `rolled_back` (`service.go:17-28`)
- `UpgradePlan`: `ID` (uuid), `FromVersion`/`ToVersion` (joined per component), `Components[]`, `Status`, `Progress`/`TotalSteps` (`TotalSteps = 3 + len(components)*2` via `calculateTotalSteps:252`), `CurrentStep`, `Error`, `BackupPath`, timestamps (`service.go:50-66`)
- `VersionInfo`: `Component`/`Current`/`Latest`/`Upgradable` (`service.go:42-47`)
- Store interface: `CreateUpgradePlan`/`GetUpgradePlan`/`ListUpgradePlans`/`UpdateUpgradePlan`/`DeleteUpgradePlan`/`GetLatestUpgrade` (`service.go:77-85`)

**Key flows:**
- `CheckForUpgrades:120` iterates `api, web, beacon, database`, calls `GetCurrentVersion` (env `<COMPONENT>_VERSION` or `$versionFile`/`<component>.version`, with `api` → `version.txt`) and `GetLatestVersion` (`<COMPONENT>_LATEST_VERSION` or `<component>.latest.version`), then compares strings.
- `CreateUpgradePlan:201` validates `len(components)>0`, collects from/to versions, creates `pending` plan with `uuid.NewString()`, persists via `store`.
- `ExecuteUpgradePlan:263` → `createBackup` → loop `upgradeComponent` per entry in `Components` → on any failure `rollbackUpgrade` → mark `completed`/`failed` (`service.go:283-351`).
- `createBackup:418` → `mkdir -p $backupDir/upgrade-$planID-$timestamp` → copy `docker-compose.yml`, `nginx.conf`, `.env` → `backupDatabase:455`.
- `backupDatabase:455` requires `DATABASE_URL`, parses it via `postgresCommandEnvironment:537` (`postgres`/`postgresql` URL, extracts `PGHOST/PGPORT/PGUSER/PGPASSWORD/PGSSLMODE/PGDATABASE`), then `pg_dump --format=custom --no-owner --no-acl --file $backupPath/database.dump $dbname` (`exec.CommandContext`), chmod `0600`, size check.
- `rollbackUpgrade:488` → sets `rolled_back`, calls `restoreFromBackup:514` → validates `database.dump` exists & non-empty → `pg_restore --clean --if-exists --no-owner --no-acl --dbname $dbname $dump` with same env.
- Helpers: `RunDatabaseMigrations:607` (stub, real migrations via `RunMigrations` in `forge/api/cmd/api/main.go:189`), `VerifyUpgrade:617` → `verifyComponentHealth:631` per component, `GetSystemStatus:638`, `CancelUpgrade:574`.

> The service is intentionally image-agnostic: concrete `upgradeAPI/upgradeWeb/upgradeBeacon/upgradeDatabase:373-415` are simulated (log-only) in this revision — the production path is **Compose image pull + container recreate** (§3 below). The DB + config backup/restore path is fully implemented.

## 1. Pre-Upgrade Checklist

- [ ] `git status` clean, `.env` backed up securely (outside version control, `chmod 600`).
- [ ] Note current `TAG` in `infra/.env` (`grep ^TAG infra/.env`) and `git describe --tags --always` / `git rev-parse --short HEAD`.
- [ ] Choose maintenance window (migrations hold `10m` startup context, failover actions `2h` — schedule accordingly).
- [ ] Verify monitoring green: `curl -s http://127.0.0.1:8080/api/v1/health/ready | jq` and `curl -s http://127.0.0.1:9090/health`.
- [ ] Confirm Beacon compatibility target (see §7) and `DAEMON_UPGRADE_PUBLIC_KEY` verification key material if using signed upgrades.
- [ ] Notify users if game workloads will restart.

## 2. Backup (mandatory — automated + manual)

### 2.1 Automated pre-upgrade backup via Upgrade Service

If driving upgrades through the API (`/api/v1/upgrade` → `CreateUpgradePlan` → `ExecuteUpgradePlan`):

```bash
# Example API flow (requires admin auth):
curl -s -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/api/v1/upgrade/check | jq
# → [{component, current, latest, upgradable}]
curl -s -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"type":"full","components":["api","web","beacon","database"]}' \
  http://127.0.0.1:8080/api/v1/upgrade/plans | jq
# → {id, status:"pending", totalSteps: 11 ...}  (3 base + 4*2)
curl -s -X POST -H "Authorization: Bearer $TOKEN" \
  http://127.0.0.1:8080/api/v1/upgrade/plans/$PLAN_ID/execute | jq
# Service then:
# - createBackup: $backupDir/upgrade-$PLAN_ID-YYYYMMDD-HHMMSS/  (default /var/backups/gamepanel)
#   - copies $installDir/docker-compose.yml, nginx.conf, .env  (copyFile:598)
#   - pg_dump $DATABASE_URL → database.dump (0600, non-empty verified)
# - upgrades each component, updating plan progress/currentStep
```

Poll status:

```bash
curl -s -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/api/v1/upgrade/plans/$PLAN_ID | jq '.status,.currentStep,.progress,.error'
# pending|downloading|backing_up|upgrading|completed|failed|rolled_back
curl -s -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:8080/api/v1/upgrade/history?limit=20" | jq
```

### 2.2 Manual backup (Compose path — recommended for operators)

Do this even if using the API flow — it is the source of truth for `restoreFromBackup`:

```bash
cd infra

# 1. Host-level DB dump (matches backupDatabase:467 pg_dump path):
set -a; . ./.env; set +a
BACKUP_ROOT="${POSTGRES_BACKUP_HOST_DIR:-/var/backups/gamepanel/postgres}"
BACKUP_DIR="$BACKUP_ROOT/manual-$(date -u +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP_DIR"
# DATABASE_URL is postgres://user:pass@postgres:5432/db?sslmode=require
# For host pg_dump, parse host/port/user/db from DATABASE_URL or use compose exec:
docker compose -f compose.yml -f compose.production.yml --env-file .env exec -T postgres \
  pg_dump --format=custom --no-owner --no-acl --file /tmp/manual.dump gamepanel
docker compose -f compose.yml -f compose.production.yml --env-file .env cp postgres:/tmp/manual.dump "$BACKUP_DIR/database.dump"
chmod 600 "$BACKUP_DIR/database.dump"
ls -lh "$BACKUP_DIR/database.dump"   # must be >0 bytes (size check at service.go:477)

# Alternative: use the dedicated postgres-backup service (infra/compose.yml:137):
# It runs postgres-backup.sh every $POSTGRES_BACKUP_INTERVAL_SECONDS (default 86400) with retention $POSTGRES_BACKUP_RETENTION_DAYS (14),
# optionally syncing to S3 ($S3_BACKUP_BUCKET/PREFIX). Check its volume:
ls -lh "${POSTGRES_BACKUP_HOST_DIR:-/var/backups/gamepanel/postgres}/"

# 2. Config snapshot (matches createBackup:430 configFiles):
cp -a infra/.env               "$BACKUP_DIR/env"
cp -a infra/compose.yml        "$BACKUP_DIR/compose.yml" 2>/dev/null || true
cp -a /opt/gamepanel/.env      "$BACKUP_DIR/opt-env"     2>/dev/null || true
cp -a /opt/gamepanel/docker-compose.yml "$BACKUP_DIR/docker-compose.yml" 2>/dev/null || true
cp -a /opt/gamepanel/nginx.conf     "$BACKUP_DIR/nginx.conf" 2>/dev/null || true

# 3. Also snapshot images/tags for rollback:
grep -E '^(TAG|API_IMAGE|WEB_IMAGE|DAEMON_IMAGE)=' infra/.env | tee "$BACKUP_DIR/image-tags.txt"
docker images --format "{{.Repository}}:{{.Tag}} {{.ID}}" | grep -E 'gamepanel|forge|beacon' | tee "$BACKUP_DIR/docker-images.txt"

# 4. Off-host copy (required for prod):
# aws s3 cp --recursive "$BACKUP_DIR" "s3://$S3_BACKUP_BUCKET/$S3_BACKUP_PREFIX/manual-$(date -u +%Y%m%d)/"
# or scp/rsync to backup host
tar -czf "/tmp/gamepanel-backup-$(date -u +%Y%m%d-%H%M%S).tgz" -C "$(dirname "$BACKUP_DIR")" "$(basename "$BACKUP_DIR")"
echo "Backup at $BACKUP_DIR — KEEP $FORGE_MASTER_KEY separately; loss = permanent data loss."
```

> Never store `FORGE_MASTER_KEY` alongside DB dumps without encryption. Rotate via `FORGE_PREVIOUS_MASTER_KEYS` + `go run ./forge/api rotate-master-key` per `infra/.env.example:77-80`.

## 3. Pull & Stage New Release

Images are **TAG-pinned** (`infra/compose.yml:192,268,644,694`):

```
image: ${API_IMAGE:-ghcr.io/gamepanel/forge-api:${TAG:?Set TAG in .env to a specific release}}
image: ${DAEMON_IMAGE:-ghcr.io/gamepanel/beacon:${TAG:?...}}
image: ${WEB_IMAGE:-ghcr.io/gamepanel/forge-web:${TAG:?...}}
```

### 3.1 Choose the target TAG

```bash
cd infra
# Option A — nearest git tag (what gen-env.sh does at 122-134):
git -C .. describe --tags --always  # sanitize: s/[^A-Za-z0-9._-]/-/g
# Option B — explicit release tag from registry:
# ghcr.io/gamepanel/forge-api:v1.2.3  etc.

# Pin it:
sed -i '' 's/^TAG=.*/TAG=v1.2.3/' .env   # macOS BSD sed; on Linux: sed -i 's/^TAG=.*/TAG=v1.2.3/' .env
grep ^TAG .env
```

### 3.2 Pull (no restart yet)

```bash
cd infra
docker compose -f compose.yml -f compose.production.yml --env-file .env pull
# Pulls api, daemon, web, docs, postgres:16-alpine, redis:7-alpine, caddy, prometheus, grafana, etc.
# Verify:
docker images | grep -E 'forge-api|forge-web|beacon'
```

### 3.3 Diff the config

```bash
docker compose -f compose.yml -f compose.production.yml --env-file .env config --quiet
# Validates merged YAML after TAG change — must exit 0 (same check in bootstrap-control-plane.sh:25)
docker compose -f compose.yml -f compose.production.yml --env-file .env config | grep -E 'image:|TAG'
```

## 4. Apply Upgrade (Migrations Run Automatically)

### 4.1 Production Compose (recommended)

```bash
cd infra
# Staged: recreate only changed services first (API → Web → Beacon):
docker compose -f compose.yml -f compose.production.yml --env-file .env up -d --build api
# API startup: RunMigrations + eventstore.Migrate + MigrateOperationalSecrets within 10m context
docker compose -f compose.yml -f compose.production.yml --env-file .env logs api --tail 100 -f
# Wait for healthy before proceeding:
until curl -sf http://127.0.0.1:8080/api/v1/health/ready >/dev/null; do sleep 2; done
echo "API healthy"

docker compose -f compose.yml -f compose.production.yml --env-file .env up -d --build web daemon
# Full stack (if Caddy/TLS overlays in use, include them):
# docker compose -f compose.yml -f compose.production.yml -f compose.caddy.production.yml --env-file .env up -d --build

# Or single-shot full upgrade:
# docker compose -f compose.yml -f compose.production.yml --env-file .env up -d --build

docker compose -f compose.yml -f compose.production.yml --env-file .env ps
# Expect all services (healthy) — api start_period 60s, web 60s, daemon 30s
```

**Migrations detail:**

- API container entry runs `RunMigrations(MIGRATIONS_DIR=/migrations)` (`main.go:189`). In Docker, `/migrations` is baked from `forge/api/migrations/` (001_init.sql … 052_...). Batch-2 migrations at `BATCH2_MIGRATIONS_DIR=/batch2-migrations` if set.
- Also `river_schema_migrations` for the queue (`river.go:runRiverMigrations`).
- Rerun-safe: fresh → all, existing → delta. No manual `migrate` command needed. Check `migrationCount` in health details:

```bash
curl -s http://127.0.0.1:8080/api/v1/health/ready | jq '.details.migrationCount'
# or
docker compose -f compose.yml -f compose.production.yml --env-file .env exec postgres \
  psql -U "${POSTGRES_USER:-gamepanel}" -d "${POSTGRES_DB:-gamepanel}" -c "SELECT count(*) FROM river_schema_migrations;"
```

### 4.2 Installer path (`/opt/gamepanel`)

```bash
cd /opt/gamepanel
sudo docker compose pull
sudo docker compose up -d   # recreates api/database/web/proxy
sudo docker compose ps
curl -s http://localhost:8080/api/health | jq
```

## 5. Verify Upgrade

```bash
# 1. Container health
docker compose -f compose.yml -f compose.production.yml --env-file .env ps
# All (healthy) or (running) with correct image tags

# 2. API + Beacon health (matches VerifyUpgrade:617 verifyComponentHealth loop)
curl -sf http://127.0.0.1:8080/api/v1/health/ready | jq .
curl -sf http://127.0.0.1:9090/health | jq .
curl -sf http://127.0.0.1:3000/ >/dev/null && echo "web ok"

# 3. Version check (UpgradeService GetSystemStatus:638 / VersionInfo:42)
curl -s -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/api/v1/upgrade/check | jq
curl -s http://127.0.0.1:8080/api/v1/health/ready | jq '.version, .details'
# Also compare TAG:
grep ^TAG infra/.env
docker inspect --format '{{.Config.Image}}' $(docker ps -q --filter name=api) | head -1

# 4. Functional smoke (see docs/audits/docker-compose-audit.md for full matrix):
# - Login → Admin → Nodes (heartbeat healthy)
# - Create/start a test server (egg itzg/minecraft-server:java21, allocation 25565)
# - docker ps on beacon host shows game container
# - Backups: trigger manual backup → verify S3/local artifact
```

If using the Upgrade API, also:

```bash
curl -s -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/api/v1/upgrade/plans/$PLAN_ID | jq '{status,currentStep,progress,totalSteps,error,backupPath}'
# Must be completed with progress == totalSteps
```

## 6. Rollback

Two rollback paths — **in-app** (Upgrade Service) and **manual** (Compose + pg_restore). Both restore `database.dump` with `pg_restore --clean --if-exists --no-owner --no-acl` (`service.go:529`) and caller-supplied `DATABASE_URL` env parsed by `postgresCommandEnvironment:537`.

### 6.1 In-app rollback (automatic on ExecuteUpgradePlan failure, or manual)

The service auto-calls `rollbackUpgrade:488` on any `upgradeComponent` error (`ExecuteUpgradePlan:314`). To trigger manually:

```bash
# Cancel a pending/upgrading plan (CancelUpgrade:574 — only pending|upgrading allowed):
curl -s -X POST -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/api/v1/upgrade/plans/$PLAN_ID/cancel | jq

# The plan moves to failed with error "Upgrade cancelled by user"; restoreFromBackup runs via rollbackUpgrade
# Check:
curl -s -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/api/v1/upgrade/plans/$PLAN_ID | jq '{status,error,backupPath}'

# Internally: restoreFromBackup validates $backupPath/database.dump (regular file, size>0) then:
#   pg_restore --clean --if-exists --no-owner --no-acl --dbname $PGDATABASE $dump
# with PGHOST/PGPORT/PGUSER/PGPASSWORD/PGSSLMODE from DATABASE_URL
```

**Preconditions for restore (`service.go:518-523`):**
- `DATABASE_URL` must be set (error otherwise)
- `$backupPath/database.dump` must exist, be a regular file, size >0 (else `valid database backup is required for rollback`)

### 6.2 Manual rollback (operator — when API is down or TAG needs revert)

This mirrors `restoreFromBackup:514` and `createBackup:430` file restores:

```bash
cd infra

# 1. Identify backup dir:
BACKUP_DIR="/var/backups/gamepanel/upgrade-$PLAN_ID-YYYYMMDD-HHMMSS"
# or your manual backup: /var/backups/gamepanel/postgres/manual-YYYYMMDD-HHMMSS
ls -lh "$BACKUP_DIR/database.dump"  # must exist, >0

# 2. Revert TAG to previous release:
PREV_TAG="v1.2.2"  # from infra/.env backup or image-tags.txt
sed -i '' "s/^TAG=.*/TAG=$PREV_TAG/" .env   # Linux: sed -i "s/^TAG=.*/TAG=$PREV_TAG/" .env
grep ^TAG .env

# 3. Restore configs (from $BACKUP_DIR):
cp -a "$BACKUP_DIR/env" infra/.env  # if you snapshotted infra/.env there
# or for /opt/gamepanel path:
# sudo cp -a "$BACKUP_DIR/docker-compose.yml" /opt/gamepanel/docker-compose.yml
# sudo cp -a "$BACKUP_DIR/nginx.conf" /opt/gamepanel/nginx.conf
# sudo cp -a "$BACKUP_DIR/.env" /opt/gamepanel/.env

# 4. Stop API/daemon to avoid writes during restore:
docker compose -f compose.yml -f compose.production.yml --env-file .env stop api daemon web

# 5. Restore DB (host must have pg_restore 16+ and DATABASE_URL access):
set -a; . ./.env; set +a
# Option A — via compose postgres container (recommended):
docker compose -f compose.yml -f compose.production.yml --env-file .env cp "$BACKUP_DIR/database.dump" postgres:/tmp/restore.dump
docker compose -f compose.yml -f compose.production.yml --env-file .env exec -T postgres \
  pg_restore --clean --if-exists --no-owner --no-acl --dbname "${POSTGRES_DB:-gamepanel}" /tmp/restore.dump
# Option B — from host (requires psql client + DATABASE_URL parsing like postgresCommandEnvironment):
# PGHOST=... PGPORT=... PGUSER=... PGPASSWORD=... PGDATABASE=gamepanel pg_restore --clean --if-exists --no-owner --no-acl --dbname gamepanel "$BACKUP_DIR/database.dump"

# 6. Restart stack with previous images:
docker compose -f compose.yml -f compose.production.yml --env-file .env pull  # pulls PREV_TAG images
docker compose -f compose.yml -f compose.production.yml --env-file .env up -d --build

# 7. Verify (see §5):
docker compose -f compose.yml -f compose.production.yml --env-file .env ps
curl -sf http://127.0.0.1:8080/api/v1/health/ready | jq .version
grep ^TAG infra/.env
```

> The upgrade service sets `plan.Status = rolled_back, CurrentStep = Rolling back` during `rollbackUpgrade:496` and logs `Rollback completed`. Manual rollback achieves the same DB + config state but does not update the `UpgradePlan` row — reconcile via `GET /upgrade/plans/$PLAN_ID` after API returns.

## 7. Beacon Compatibility Check

Beacon (daemon) and Forge API are version-coupled via `DAEMON_NODE_TOKEN` auth and optional Ed25519 signature verification.

**Before upgrading either side:**

| Check | Where | What to do |
|---|---|---|
| **API ↔ Beacon wire compatibility** | `infra/.env` `PANEL_API_URL` (must be `https://…/api/v1` in prod, `infra/compose.yml:285`), `DAEMON_NODE_ID/TOKEN`, `BEACON_BASE_URL` | Keep `TAG` in sync for `API_IMAGE` + `DAEMON_IMAGE` when doing `full` upgrades. If upgrading one at a time, use `UpgradeType` `api` or `beacon` and verify `GET /upgrade/check` shows the other side not `upgradable` unexpectedly. |
| **Upgrade signing key** | `DAEMON_UPGRADE_PUBLIC_KEY` (`infra/.env.example:90`, `gen-env.sh:220`, `compose.yml:284`) base64 Ed25519 public key | If the new Beacon image requires signed upgrade metadata, ensure `DAEMON_UPGRADE_PUBLIC_KEY` matches the release signing key for `TAG`. Mismatch → Beacon rejects upgrade (`upgrade/service.go` beacon path + `beacon/internal/` verifier). Rotate by updating `.env` + `docker compose up -d daemon`. |
| **DB + Beacon DB** | `DAEMON_DATA_DIR=/srv/game-panel/servers`, `BEACON_DATABASE_PATH=/srv/game-panel/servers/.beacon/beacon.db` (`compose.yml:278-280`) | Never downgrade Beacon without restoring `beacon.db` from the same backup window if the schema migrated. Beacon's embedded SQLite migrations are forward-only. |
| **Docker API** | `DOCKER_HOST=tcp://docker-proxy:2375` (`compose.yml:275`), `docker-proxy` caps (`CONTAINERS/IMAGES/NETWORKS/VOLUMES=1`, `BUILD/EXEC=0`) | Old Docker (<24) may break `tecnativa/docker-socket-proxy:v0.4.2` or Beacon's `no-new-privileges` + `cap_drop: ALL` hardening. Verify `docker info` on game hosts before beacon upgrade. |
| **Load-balancer / Caddy** | `LOAD_BALANCER_PORT_MIN/MAX=30000-30100`, `CADDY_ADMIN_ADDR=caddy:2019` (`compose.yml:218-222`) | Beacon upgrade should not recycle `caddy` unless `Caddyfile` changed. Check `docker compose logs caddy` for `admin.api` bind errors after. |
| **Version files** | `Service.versionFile` (`/opt/gamepanel/version.txt`, per-component `<component>.version` / `<component>.latest.version`, `service.go:158-176`) or env `API_VERSION`/`BEACON_VERSION` vs `<COMP>_LATEST_VERSION` | `GetCurrentVersion` prefers env `<COMPONENT>_VERSION` else file; `GetLatestVersion` prefers `<COMPONENT>_LATEST_VERSION` else `<component>.latest.version` file. Ensure CI publishes `.latest.version` artifacts or set the env override in `.env`. Invalid files (`len>128` or `\r\n\x00`) are rejected. |

**Compatibility probe:**

```bash
# From infra host after upgrade:
curl -s http://127.0.0.1:9090/health | jq '.version // .status'
curl -s http://127.0.0.1:8080/api/v1/health/ready | jq '{apiVersion: .version, beaconNodes: .details.beacon}'

# From Forge API perspective (UpgradeService.GetSystemStatus:638):
curl -s -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/api/v1/upgrade/check | jq
# Each entry: {component:"api"|"web"|"beacon"|"database", current, latest, upgradable}

# If beacon reports unhealthy after API upgrade:
docker compose -f compose.yml -f compose.production.yml --env-file .env logs daemon --tail 100
# Check for DAEMON_NODE_TOKEN mismatch, PANEL_API_URL TLS errors, or DAEMON_UPGRADE_PUBLIC_KEY signature failures
```

**Rolling multi-node Beacon upgrades:** upgrade one node at a time (`UpgradeType=beacon`, `components=["beacon"]`), keep at least one healthy Beacon online for game workloads, verify `GET /health` and `ListUpgradeHistory` before proceeding to next host. Evacuation/migration (`services/evacuationplanner`, `migration`) must be idle.

## 8. Post-Upgrade Tasks

- `docker system prune` (optional, after verification window).
- Re-enable cron/schedules if paused.
- Re-run `scripts/cleanup/production-guard.sh` against live `.env`.
- Update runbooks and `docs/audits/MASTER_TRACKING.md` entry for the release.
- Rotate `FORGE_PREVIOUS_MASTER_KEYS` if rotating `FORGE_MASTER_KEY`.

## 9. Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `Upgrade failed: backup failed` | `DATABASE_URL` missing or `pg_dump` not in `PATH` (container) / not installed (host) | `docker compose exec postgres pg_dump --version`; ensure `DATABASE_URL` is `postgres://…` with db name (`postgresCommandEnvironment:538`) |
| `pg_dump produced an empty backup` | DB unreachable or permission | `pg_isready -U $POSTGRES_USER -d $POSTGRES_DB`; check `postgres` healthcheck `pg_isready` (`compose.yml:49`) |
| `valid database backup is required for rollback` | `database.dump` missing/empty/permission | `ls -l $backupPath/database.dump`; `chmod 600` and size >0 |
| `DATABASE_URL is not a valid PostgreSQL URL` | Scheme not `postgres`/`postgresql` or missing host/db | `echo $DATABASE_URL | grep -E '^postgres'`; fix `infra/.env` |
| `cannot cancel upgrade with status: completed` | `CancelUpgrade:585` only allows `pending|upgrading` | Use manual rollback §6.2 |
| API stuck `upgrading` >10m | Migration timeout (`10*time.Minute` startup context) or deadlock | `docker compose logs api --tail 300`; `docker compose restart api`; if persistent, manual restore + revert TAG |
| Beacon offline after API upgrade | `PANEL_API_URL` HTTPS mismatch, token, or `DAEMON_UPGRADE_PUBLIC_KEY` | `docker compose logs daemon`; verify `PANEL_API_URL=https://…`, `DAEMON_NODE_TOKEN`, and signing key |
| Web shows old version | Browser cache or `WEB_IMAGE` not updated | Hard refresh, `docker compose pull web && up -d web` |

## 10. Reference

- Service: `forge/api/internal/services/upgrade/service.go` — `CheckForUpgrades`, `CreateUpgradePlan`, `ExecuteUpgradePlan`, `createBackup`/`backupDatabase`, `rollbackUpgrade`/`restoreFromBackup`, `postgresCommandEnvironment`, `copyFile`, `RunDatabaseMigrations`, `VerifyUpgrade`, `GetSystemStatus`, `CancelUpgrade`.
- Compose: `infra/compose.yml` + `infra/compose.production.yml` (`!override` ports, healthchecks).
- Env generator: `infra/gen-env.sh` (validates 13 required vars).
- Bootstrap: `infra/bootstrap-control-plane.sh` (`config --quiet` + `up -d postgres redis api web`).
```

