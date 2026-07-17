# Mock Removal Tracker

Mocks are allowed only for local demo fallback while the prototype is being wired. Before any production deployment, every item here must be removed or moved behind an explicit development mode.

## Current Mock/Fallback Areas

- Frontend `lib/mock-data.ts` fallback for nodes, servers, allocations, users, stats, and audit. Status: gated by `NEXT_PUBLIC_DEMO_MODE=true`.
- API fallback responses when PostgreSQL or daemon clients are not configured. Status: gated by `API_DEMO_MODE=true`.
- Daemon mock mode when Docker is unavailable. Status: gated by `DAEMON_ALLOW_MOCK_RUNTIME=true`.
- Dev credentials and secrets in `.env.example` and Docker Compose. Status: production rejects default secrets when `APP_ENV=production`; generated production template exists at `infra/scripts/generate-production-env.ps1`.
- Dashboard create form fixed resource defaults. Status: resource inputs are editable in the create form.

## Production Rule

Production must fail closed when infrastructure is missing:

- No silent mock API data.
- No default auth or daemon tokens.
- No daemon mock runtime.
- No frontend fallback data unless explicitly running a demo build.
- No default `dev-api-secret` or `dev-node-token` when `APP_ENV=production`.
