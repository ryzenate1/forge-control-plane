# Integration Runbook

This runbook verifies the full local stack: PostgreSQL, Redis, Forge API, Beacon daemon, Prometheus, Grafana, and SFTP.

## Start

```bash
docker compose -f deploy/compose.yml up -d --build
```

## Check Service State

```bash
docker compose -f deploy/compose.yml ps
```

Expected exposed services:

- Plane (frontend dev server): `http://127.0.0.1:3000` when started separately with `npm run dev`
- Forge API: `http://127.0.0.1:8080/api/v1/health`
- Beacon daemon: `http://127.0.0.1:9090/health`
- Prometheus: `http://127.0.0.1:9091`
- Grafana: `http://127.0.0.1:3001`
- SFTP: `127.0.0.1:2222`

## Smoke Test

```powershell
powershell -ExecutionPolicy Bypass -File deploy/smoke-test.ps1
```

The smoke test checks:

- API health
- Daemon health
- API and daemon metrics
- Admin login
- Seeded nodes, templates, servers, and allocations

## Frontend

```bash
npm run dev
```

Open `http://127.0.0.1:3000` and log in with the seeded local admin:

- Email: `admin@example.com`
- Password: `admin123`

## Transfer

Development SFTP uses:

- Host: `127.0.0.1`
- Port: `2222`
- User: `game`
- Password: `gamepass`
