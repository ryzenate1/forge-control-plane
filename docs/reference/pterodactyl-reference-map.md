# Pterodactyl Reference Map

This project uses Pterodactyl as a product and workflow reference, then rebuilds the concepts in an original modern stack. Do not copy Pterodactyl source code, assets, UI text, migrations, or proprietary implementation details.

## Concept Mapping

| Pterodactyl concept | Modern Game Panel equivalent | Status |
| --- | --- | --- |
| Panel | Next.js frontend + Go Fiber API | In progress |
| Wings daemon | Go daemon using Docker SDK | In progress |
| Nodes | `nodes` table and daemon base URL/token records | Basic listing |
| Servers | `servers` table, API orchestration, daemon runtime container | Lifecycle in progress |
| Eggs | `server_templates` table | Minecraft Java seed |
| Allocations | `allocations` table and `/allocations` API | Basic listing |
| Console | API WebSocket gateway to daemon Docker attach | Basic interactive console |
| Files | API-to-daemon jailed file operations | Basic file API |
| Schedules | Future scheduler owned by API | Not started |
| Backups | Future daemon-managed backup jobs | Not started |
| Users/Roles | API auth now, RBAC later | Partial |
| Audit logs | `audit_events` table | Basic events |

## Implementation Rule

When using Pterodactyl as a reference, describe the feature behavior first, then implement the smallest original version that matches this stack:

- Frontend: Next.js + TypeScript
- API: Go + Fiber
- Daemon: Go standard library + Docker SDK
- Data: PostgreSQL + Redis
- Realtime: WebSockets through the API gateway
