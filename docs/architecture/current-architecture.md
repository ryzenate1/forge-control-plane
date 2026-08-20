# Current Architecture — alias

> This file exists to satisfy historical references to `docs/architecture/current-architecture.md`.
> The canonical CURRENT architecture document is [`docs/architecture/overview.md`](./overview.md).

See [`overview.md`](./overview.md) for the verified CURRENT system overview, component table, proxy truth (Caddy CURRENT vs host Nginx example vs Traefik EXPERIMENTAL), deployment overlays, installation paths, language truth, OpenAPI/Swagger notes, and Phase 27 handler↔SDK contract.

**Status:** **CURRENT** alias — content is identical to `overview.md` as of 2026-08-23 (`ca06f741`). Do not diverge the two files; edit `overview.md` and keep this file as a pointer.

- `infra/compose.yml:602` `caddy:2.9.1-alpine` — **CURRENT**
- `infra/nginx.conf:1` — **HISTORICAL/example** (host Nginx, not deployed by Compose)
- `infra/compose.tls.yml` — **EXPERIMENTAL** (Traefik, mutually exclusive with `compose.caddy.production.yml`)
- Handoffs at `docs/implementation/50-agent-run/handoffs/` — **HISTORICAL** empty archival
