# AI Agent Instructions — READ THIS FIRST

## Step 1: Read These Files Before Doing ANYTHING
1. `docs/MASTER_REFERENCE.md` — Complete project state, file map, all endpoints, database schema
2. `docs/MASTER_REFERENCE_PART2.md` — 1:1 Pterodactyl conversion map, what's built vs not, DevOps guide

## Step 2: Reference Pterodactyl Source (LOCAL — Never GitHub)
- Panel: `refs/pterodactyl-panel/` (PHP Laravel)
- Wings: `refs/pterodactyl-wings/` (Go daemon)
- When implementing ANY feature, first read the equivalent Pterodactyl source to understand behavior
- Key paths listed in MASTER_REFERENCE_PART2.md Section 11

## Step 3: Follow Conversion Pattern
1. Read Pterodactyl controller/model/service for the feature
2. Understand validation, permissions, events, data flow
3. Implement in our Go/TypeScript stack following existing patterns
4. Update docs/MASTER_REFERENCE files

## Rules
- NEVER copy Pterodactyl source code — only copy BEHAVIOR
- NEVER hallucinate Pterodactyl features — read the actual local source
- NEVER create new monolithic files — split by domain
- API handlers → `apps/api/internal/http/handlers_<domain>.go`
- Store queries → `apps/api/internal/store/store_<domain>.go`
- Frontend components → `apps/frontend/components/<ComponentName>.tsx`

## Stack: Go Fiber API, Go stdlib Daemon, Next.js 15, PostgreSQL 16, Redis 7, Docker
## Login: admin@example.com / admin123
## Ports: Frontend 3002, API 8080, Daemon 9090, Postgres 5432, Redis 6379
