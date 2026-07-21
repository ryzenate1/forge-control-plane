# GamePanel Shipping Guide

## Architecture Overview

GamePanel is a multi-node game server management platform composed of the following services:

| Component | Role | Port(s) | Tech |
|-----------|------|---------|------|
| **Forge API** | REST API control plane (authentication, server CRUD, node management) | 8080 | Go 1.26 / Fiber |
| **Forge Web** | Next.js frontend (panel UI, terminal, file manager) | 3000 | Next.js 15 / React 19 |
| **Beacon / Daemon** | Game server node agent (container lifecycle, SFTP, metrics) | 9090 (API), 2022 (SFTP) | Go 1.26 |
| **PostgreSQL** | Primary database | 5432 | PostgreSQL 16 |
| **Redis** | Cache, queue, pub/sub | 6379 | Redis 7 |
| **Prometheus** | Metrics collection + alerting | 9091 | Prometheus v2.55 |
| **Alertmanager** | Alert routing (email/Slack/PagerDuty-ready) | 9093 | Alertmanager v0.27 |
| **Grafana** | Dashboards and visualisation | 3001 | Grafana 11.3 |
| **Traefik** | TLS termination + ACME (optional) | 80, 443 | Traefik v3.3 |

### Network Layout

```
Internet
    |
    |-- :443 (TLS) -- Traefik / nginx (optional)
    |                    |-- /api/*  -> api:8080
    |                    |-- /*       -> web:3000
    |
    |-- :2022 (SFTP) -> Beacon (per node)
    |
    [frontend bridge]
       |-- web:3000
       |-- api:8080
       |-- grafana:3001
    [backend bridge]
       |-- api:8080
       |-- postgres:5432
       |-- redis:6379
       |-- prometheus:9090
       |-- alertmanager:9093
       |-- grafana:3001
       |-- daemon:9090 (per node)
```

## Prerequisites

- **Docker** 24+ and **Docker Compose** v2.24.4+ (required for `!override` syntax)
- **Linux x86_64 host** (Ubuntu 22.04+ or Debian 12+ recommended)
- **Domain name** with DNS A/AAAA record pointing to the control plane IP
- **SMTP credentials** (for transactional email) — optional but recommended
- **OpenSSL** (for secret generation)
- Minimum **2 GB RAM**, **2 vCPUs**, **20 GB disk** for the control plane
- Each **beacon node**: minimum **4 GB RAM**, **2 vCPUs**, **50 GB disk** (game servers vary)

## Quick Start (5-Minute Deploy)

```bash
# 1. Clone and enter the infra directory
cd infra

# 2. Generate environment file with secure secrets
export PANEL_DOMAIN=panel.yourdomain.com
bash gen-env.sh .env

# 3. Review and edit .env if needed (at minimum set POSTGRES_PASSWORD, API_AUTH_SECRET)
#    If you have SMTP, add MAIL_* variables.

# 4. Deploy the control plane
bash ship/ship.sh

# 5. Open https://panel.yourdomain.com/setup and create the admin account
#    (or http://YOUR_IP:3000/setup behind local-only nginx)

# 6. In the panel, navigate to Nodes -> Create Node. Copy the generated
#    DAEMON_NODE_ID and DAEMON_NODE_TOKEN into infra/.env, then re-run:
bash ship/ship.sh
```

## Production Deployment Guide

### 1. DNS & TLS

Two TLS options are available:

**Option A: Traefik (auto, recommended)**
Set in `.env`:
```
PANEL_DOMAIN=panel.yourdomain.com
TRAEFIK_ACME_EMAIL=admin@yourdomain.com
TRAEFIK_CERTIFICATE_RESOLVER=letsencrypt
```
Then deploy with:
```bash
docker compose -f compose.yml -f compose.production.yml -f compose.tls.yml --env-file .env up -d
```

**Option B: Host nginx (manual)**
Edit `infra/nginx.conf`, replace `__PANEL_FQDN__` with your domain, obtain TLS certs via certbot, then:
```bash
sudo cp infra/nginx.conf /etc/nginx/sites-available/gamepanel
sudo ln -s /etc/nginx/sites-available/gamepanel /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
```

### 2. Staged Control Plane Deployment

```bash
cd infra

# Stage 1 — database and cache
docker compose -f compose.yml -f compose.production.yml --env-file .env up -d postgres redis

# Stage 2 — API and web (after DB is healthy)
docker compose -f compose.yml -f compose.production.yml --env-file .env up -d api web

# Stage 3 — complete setup in the panel, then deploy monitoring
docker compose -f compose.yml -f compose.production.yml --env-file .env up -d daemon prometheus alertmanager grafana
```

### 3. Health Verification

```bash
# Check all services
docker compose -f compose.yml -f compose.production.yml --env-file .env ps

# Test API health
curl -sf http://127.0.0.1:8080/health | jq .

# Test web health
curl -sf -o /dev/null -w "%{http_code}" http://127.0.0.1:3000/

# Check logs
docker compose logs --tail=50 -f api web
```

## Multi-Node Deployment

### Control Plane Node
Runs PostgreSQL, Redis, API, Web, Prometheus, Alertmanager, Grafana, and optionally one Beacon daemon.

### Beacon Nodes
Each game server node runs only the Beacon daemon (`compose.beacon.yml`).

**Bootstrapping a Beacon Node:**

```bash
# On the beacon node
git clone <repo> /opt/gamepanel
cd /opt/gamepanel/infra

# Create .env with the credentials from the panel's Nodes -> Create Node page
cat > .env << EOF
DAEMON_NODE_ID=<uuid-from-panel>
DAEMON_NODE_TOKEN=<token-from-panel>
PANEL_API_URL=https://panel.yourdomain.com/api/v1
GAME_SERVERS_HOST_DIR=/srv/game-panel/servers
BACKUP_ADAPTER=local
EOF

# Deploy
bash bootstrap-beacon.sh
```

**Firewall Rules for Beacon Nodes:**
```
Inbound:
  TCP 2022  — SFTP (game file uploads) — allow only game panel users' IPs
  TCP 9090  — Beacon API — allow only control plane IP
Outbound:
  TCP 443   — Panel API communication
  TCP 80    — ACME HTTP-01 challenges
  UDP 30000-30100 — Game server ports (if load balancer enabled)
```

## Monitoring Stack Setup

The monitoring stack (Prometheus + Alertmanager + Grafana) starts automatically with the control plane.

### Accessing Grafana

| Field | Default Value |
|-------|---------------|
| URL   | `https://panel.yourdomain.com:3001` or `http://YOUR_IP:3001` |
| User  | `admin` (set via `GRAFANA_ADMIN_USER`) |
| Pass  | Value set in `.env` |

Grafana is pre-provisioned with:
- **Prometheus datasource** pointing at `prometheus:9090`
- **GamePanel overview dashboard** with API and daemon metrics

### Prometheus Alerting

Default alerts (defined in `infra/prometheus/alerts.yml`):
| Alert | Severity | Condition |
|-------|----------|-----------|
| `GamePanelAPIDown` | Critical | API not scrapable for 2m |
| `GamePanelDaemonDown` | Critical | Daemon not scrapable for 2m |
| `GamePanelHighMemory` | Warning | API heap > 1 GB for 10m |
| `GamePanelHighGoroutines` | Warning | API goroutines > 10k for 10m |
| `GamePanelDaemonRuntimeDisabled` | Critical | Docker runtime unavailable for 5m |

To enable notifications, edit `infra/alertmanager.yml` and add a receiver (email, Slack, PagerDuty), then restart:
```bash
docker compose restart alertmanager
```

## Backup and Restore

### Automated PostgreSQL Backups

The `postgres-backup` container runs automatically. Configuration:
- `POSTGRES_BACKUP_INTERVAL_SECONDS` (default: 86400 = daily)
- `POSTGRES_BACKUP_RETENTION_DAYS` (default: 14)
- `POSTGRES_BACKUP_HOST_DIR` (default: `/var/backups/gamepanel/postgres`)

Backups use `pg_dump --format=custom`. Each file is named `gamepanel-<timestamp>.dump`.

### Manual Backup

```bash
# Database
docker compose exec postgres pg_dump -U gamepanel --format=custom -f /tmp/manual-backup.dump gamepanel
docker compose cp postgres:/tmp/manual-backup.dump ./manual-backup.dump

# Redis
docker compose exec redis redis-cli SAVE
docker compose cp redis:/data/dump.rdb ./redis-backup.rdb

# Configuration
cp infra/.env .env.backup
```

### Restore

```bash
# Stop API to prevent writes
docker compose stop api web

# Restore database
docker compose exec -T postgres pg_restore -U gamepanel -d gamepanel --clean < ./manual-backup.dump

# Restore Redis
docker compose cp ./redis-backup.rdb redis:/data/dump.rdb
docker compose restart redis

# Restart services
docker compose start api web
```

## Security Hardening Checklist

- [ ] **All secrets generated** — run `gen-env.sh` (do not reuse `.env.example` values)
- [ ] **`.env` file permissions** — `chmod 600 infra/.env`
- [ ] **PostgreSQL binds to `postgres:5432` only** — never expose on `0.0.0.0`
- [ ] **Redis binds to `redis:6379` only** — never expose on `0.0.0.0`
- [ ] **Production port overrides** — API/Web/Grafana/Alertmanager bind to `127.0.0.1` in `compose.production.yml`
- [ ] **Beacon mTLS** — enable in beacon config for daemon-to-daemon transfers
- [ ] **Firewall** — block all inbound ports except 443 (TLS) and 2022 (SFTP, restricted)
- [ ] **TLS** — use Let's Encrypt (Traefik auto) or certbot (nginx manual); enable HSTS
- [ ] **SSH** — disable password auth, use SSH keys only
- [ ] **Docker socket** — only Beacon needs access; do not mount `/var/run/docker.sock` into other containers
- [ ] **Forge Master Key** — set `FORGE_MASTER_KEY` for encryption at rest; store backup separately
- [ ] **Regular updates** — pull latest images and rebuild weekly
- [ ] **Audit logging** — enabled by default in Forge API; monitor `audit_logs` table
- [ ] **Rate limiting** — nginx config pre-configured for API (10 req/s) and auth (5 req/m)
- [ ] **Auto-heal** — all services use `restart: unless-stopped` and Docker healthchecks

## Upgrade Procedures

### General Upgrade (Docker Images)

```bash
cd infra
docker compose -f compose.yml -f compose.production.yml --env-file .env pull
docker compose -f compose.yml -f compose.production.yml --env-file .env up -d --build
```

### Database Migrations

Migrations run automatically on API startup. To verify:
```bash
docker compose logs api | grep migration
```

### Breaking Changes

Before upgrading between major versions:
1. Read the [changelog](https://github.com/anomalyco/gamepanel/releases)
2. Backup the database (see Backup section)
3. Drain beacon nodes in the panel (Servers -> Maintenance Mode)
4. Upgrade control plane first, then each beacon node

## Troubleshooting Guide

### API fails to start

```
Set POSTGRES_PASSWORD in infra/.env
```
→ Run `bash gen-env.sh .env` or manually set the variable.

```
Set FORGE_MASTER_KEY in infra/.env
```
→ Add `FORGE_MASTER_KEY=$(openssl rand -hex 32)` and `FORGE_MASTER_KEY_ID=primary` to `.env`.

### Beacon fails to connect

1. Verify `DAEMON_NODE_ID` and `DAEMON_NODE_TOKEN` match the panel's Node record
2. Verify `PANEL_API_URL` is reachable from the beacon node (`curl https://panel.yourdomain.com/api/v1`)
3. Check beacon logs: `docker compose -f compose.beacon.yml logs`

### Empty white page on Web UI

1. Check browser console for API connection errors
2. Verify `API_INTERNAL_URL` in the web Docker build arg (default: `http://api:8080`)
3. Ensure the API is healthy: `curl http://127.0.0.1:8080/health`

### Prometheus cannot scrape

1. Depends on API and daemon being healthy (see healthcheck conditions in `compose.yml`)
2. Verify `depends_on` conditions are satisfied
3. Check Prometheus logs: `docker compose logs prometheus`

### Port conflicts (EADDRINUSE)

In production, services bind to `127.0.0.1` (via `compose.production.yml`). If a port is in use:
```bash
lsof -i :PORT
# Kill the conflicting process or change the port in .env
```

### Reset everything

```bash
cd infra
docker compose -f compose.yml -f compose.production.yml --env-file .env down -v
# Warning: -v destroys all volumes including database data
```

## Appendix: Version Reference

| Component | Version |
|-----------|---------|
| GamePanel (project) | 0.1.0 |
| Forge API | 0.1.0 |
| Forge Web | 0.1.0 |
| Beacon Daemon | 0.1.0 |
| PostgreSQL | 16 |
| Redis | 7 |
| Prometheus | 2.55.1 |
| Alertmanager | 0.27.0 |
| Grafana | 11.3.0 |
| Traefik (optional) | 3.3 |
