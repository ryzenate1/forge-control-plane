# Hosting websites with Forge Control Plane

Minimal supported path for hosting simple/static websites, verified against the
running stack on 2026-09-30. Two example sites (`site1-website`,
`site2-website`) were created this way; see §5 for their live record IDs.

## 1. How website hosting works here

There is no separate "website" primitive. A website is a composition of three
existing primitives:

| Layer | Primitive | API | Runs on |
|---|---|---|---|
| Runtime | **Compose stack** (e.g. `nginx:1.27-alpine` + inline HTML) | `POST /api/v1/compose` → Beacon → Docker | prod Ubuntu (Docker daemon) |
| Gateway record | **Proxy domain** (`hostname` + backend `port`) | `POST /api/v1/domains` | both (DB row; Caddy TLS hook on `letsencrypt`/`custom`) |
| Optional wrapper | **COMPOSE-type App** + `POST /api/v1/apps/:id/domains` | Apps UI | both (deploy needs a bound server, see §6) |

Edge traffic in production terminates at Caddy (`infra/Caddyfile.production`,
`infra/compose.caddy.production.yml`). The panel's `/admin/domains` list page
covers **game-server custom domains** (`/servers/:id/domains`, auto-synced to
Caddy on verification); gateway proxy domains (`/domains`) are viewed at
`/admin/domains/[id]` and `/console/domains`. Both UIs are wired to the real
API — no mocks were found in `app/admin/domains*`, `app/admin/apps*` or
`app/admin/compose*` (audit 2026-09-30; only input placeholders and the
offline template fallback in `lib/api/apps.ts:fetchAppTemplates` remain, the
latter mirroring the server catalog at `GET /admin/app-templates`).

## 2. Prerequisites

- Admin session. All mutating calls need the session cookie **and**
  `X-CSRF-Token: <forge_csrf cookie>` plus `Origin: <panel URL>`.
- Native mock dev: `./native.sh start` with `DAEMON_ALLOW_MOCK_RUNTIME=true`.
  Containers **cannot** run (no Docker daemon) — see §6.
- Production Ubuntu: Docker daemon reachable by Beacon; DNS pointing at the
  host; Caddy on 80/443.

```bash
API=http://127.0.0.1:8080/api/v1   # prod: https://panel.example.com/api/v1
curl -c cookies.txt -H 'Content-Type: application/json' \
  -X POST $API/auth/login -d '{"email":"admin@example.com","password":"admin123"}'
CSRF=$(awk '/forge_csrf/{print $NF}' cookies.txt)
NODE=22222222-2222-2222-2222-222222222222   # from GET $API/nodes (dev demo node)
```

## 3. Step-by-step: host 2 websites

### Step 1 — validate each compose document (works everywhere, no runtime needed)

`POST /api/v1/compose/validate` with `{"content": "<yaml>"}`.

### Step 2 — create each stack (deploys on prod; fail-closed record on mock)

`POST /api/v1/compose`. `nodeId` is **required** — the scheduler does not
auto-place (`scheduler node selection failed` otherwise; on the degraded macOS
node auto-placement is refused, pass the node explicitly).

```json
{
  "name": "site1-website",
  "nodeId": "22222222-2222-2222-2222-222222222222",
  "composeYaml": "<site1 yaml below>",
  "memoryMb": 256, "cpuShares": 512, "diskMb": 1024,
  "composeType": "docker-compose", "sourceType": "raw"
}
```

### Step 3 — register each gateway domain

`POST /api/v1/domains` (`certType: "none"` in dev; `"letsencrypt"` in prod,
which provisions TLS via Caddy before persisting).

```json
{ "hostname": "site1.forge.local", "serviceId": "<stack id>",
  "serviceType": "compose", "port": 18081, "path": "/",
  "certType": "none", "websocket": false }
```

### Step 4 — verify in the panel

- Compose stacks list: `/admin/compose` (status, error detail per stack)
- Domain detail: `/admin/domains/<domain id>` (+ security headers, redirects)
- Customer view: `/console/domains`
- Prod serving check: `curl -H 'Host: site1.example.com' http://<host>/`
  or direct `http://<host>:18081/`; dev mock: see §6.

### Site compose documents (distinct content, one image each, no volumes)

Site 1 — `site1-website`, host port **18081**:

```yaml
services:
  web:
    image: nginx:1.27-alpine
    ports:
      - "18081:80"
    command: ["/bin/sh", "-c", "printf '%s' '<!doctype html><html><head><title>Site One</title></head><body style=\"font-family:sans-serif;text-align:center;margin-top:10%\"><h1>Site One - Acme Landing</h1><p>Served by Forge Control Plane (nginx).</p></body></html>' > /usr/share/nginx/html/index.html && exec nginx -g 'daemon off;'"]
    restart: unless-stopped
```

Site 2 — `site2-website`, host port **18082**: same document with
`18082:80` and `Site Two - Beta Blog` content. Keep host ports distinct per
site; container port stays 80. For real content, replace the `command` hack
with a bind mount (`volumes: ["./site1:/usr/share/nginx/html:ro"]`, files
present on the Beacon host) — `env_file` is rejected by the validator, inline
`environment:` instead.

### Full curl transcript (dev)

```bash
B=http://127.0.0.1:8080/api/v1
H=(-b cookies.txt -H 'Content-Type: application/json' -H 'Origin: http://localhost:3000' -H "X-CSRF-Token: $CSRF")
# validate
curl "${H[@]}" -X POST $B/compose/validate -d @site1-validate.json
# create (repeat for site2 with its yaml/name)
curl "${H[@]}" -X POST $B/compose -d @site1-stack.json
# domain (repeat for site2: hostname site2.forge.local, port 18082)
curl "${H[@]}" -X POST $B/domains -d @site1-domain.json
# verify
curl -b cookies.txt "$B/compose" | python3 -m json.tool
curl -b cookies.txt "$B/domains?limit=10" | python3 -m json.tool
```

Panel equivalents: Compose → New Stack (paste YAML → Validate → Deploy),
then domain via API/`/console/domains` (there is no admin create-form for
proxy domains; `/admin/domains` manages game-server domains).

## 4. App-based alternative (equivalent wrapper)

Instead of raw stacks: Apps → Create App → type **Docker Compose** (paste the
same YAML) → `POST /api/v1/apps` with
`{"name":"site1-website","type":"compose","composeContent":"<yaml>", ...}` →
attach hostname via `POST /api/v1/apps/:id/domains {"domain":"..."}` →
`POST /api/v1/apps/:id/deploy`. Note `DOCKER_IMAGE`-only apps have **no
compose document**, so app-deploy rejects them (`application has no compose
document to deploy`); use type `compose`. App deploy also requires the app to
be bound to a server (`serverId`); unbound apps fail closed with 501.

## 5. Records created by the verified run (dev DB)

- Stacks: `site1-website` (`cps-9596a7cf-241`), `site2-website`
  (`cps-54e08d6d-39f`) — status `failed`, see §6.
- Proxy domains: `site1.forge.local` (`a9f30fc6-…`, port 18081),
  `site2.forge.local` (`789fd8ed-…`, port 18082), `certType: none`.

## 6. macOS mock vs production Ubuntu

| | macOS native mock (this run) | Production Ubuntu |
|---|---|---|
| `POST /compose/validate` | works | works |
| `POST /compose` record | created, then `failed`: `compose deploy: … "exec: \"docker\": executable file not found in $PATH"` (beacon 409, fail-closed, reservation cancelled) | pulls image, runs, health-checked to `running` |
| Node auto-placement | refused (`scheduler node selection failed`); pass `nodeId` explicitly | scheduler places automatically |
| `POST /domains` (`certType: none`) | 201, listed, visible in panel | 201; use `certType: "letsencrypt"` + real DNS for TLS |
| Serving traffic | **not possible**: no Docker daemon, no Caddy in `native.sh`; ports 18081/18082 never open | nginx serves on host ports; Caddy routes hostnames |
| Beacon health | `{"ok":false,"reason":"container runtime unavailable"}` | `{"ok":true}` |

To re-run the deploy leg on prod, `POST /api/v1/compose/<id>/deploy` the same
stack rows (or recreate per §3) — no config changes needed. Cosmetic note: the
dev stack rows embed a typo'd inline style (`margin-top:10% obstetric`,
harmless — invalid CSS is dropped); the canonical documents are the ones in §3.
