# Architecture Comparison

## Monolith vs Modular

| Project | Architecture | Description |
|---------|-------------|-------------|
| GamePanel | Modular monolith | Separate Go services: Forge API, Beacon daemon, Next.js frontend |
| Pterodactyl | Monolith | Single Laravel app + Wings daemon |
| Pelican Panel | Monolith | Single Laravel app + Wings daemon |
| PufferPanel | Modular monolith | Go backend + Vue.js frontend + daemon (same binary) |
| Wings | Single daemon | Standalone agent process |

## Service Boundaries

| Aspect | GamePanel | Pterodactyl | Pelican | PufferPanel | Wings |
|--------|-----------|-------------|---------|-------------|-------|
| API-Daemon separation | ✅ Forge API ↔ Beacon | ✅ Panel ↔ Wings | ✅ Panel ↔ Wings | ⚠️ Same binary | N/A |
| Frontend-Backend separation | ✅ Next.js ↔ GoFiber | ✅ React ↔ Laravel | ✅ JS ↔ Laravel | ⚠️ Embedded Vue | N/A |
| Event-driven | ✅ Events package | ⚠️ Queue-based (Redis) | ⚠️ Queue-based | ❌ Synchronous | ⚠️ Event bus |
| Multi-node support | ✅ Regions, placement | ✅ Multiple nodes | ✅ Multiple nodes | ❌ Single node | N/A |

## Separation of Concerns

| Layer | GamePanel | Pterodactyl | Pelican | PufferPanel | Wings |
|-------|-----------|-------------|---------|-------------|-------|
| Domain models | ✅ internal/domain/ | ✅ app/Models | ✅ app/Models | ✅ models/ | ❌ |
| HTTP handlers | ✅ internal/http/ | ✅ Controllers | ✅ Controllers | ✅ web/ | ✅ router/ |
| Services | ✅ internal/services/ | ✅ app/Services | ✅ app/Services | ✅ services/ | ❌ |
| Data access | ✅ internal/store/ | ✅ Eloquent ORM | ✅ Eloquent ORM | ✅ database/ | ❌ |
| Events | ✅ internal/events/ | ⚠️ Laravel Events | ⚠️ Laravel Events | ❌ | ✅ events/ |
| Runtime | ✅ internal/runtime/ | ⚠️ Wings handles | ⚠️ Wings handles | ✅ engine/ | ✅ environment/ |

## Domain Modelling

| Aspect | GamePanel | Pterodactyl | Pelican | PufferPanel | Wings |
|--------|-----------|-------------|---------|-------------|-------|
| Bounded contexts | ✅ Clear (server, node, user, backup, schedule, allocation, region, template) | ⚠️ Flat Laravel models | ⚠️ Flat Laravel models | ✅ Package-based | N/A |
| Aggregate roots | ✅ Server, Node, User | ⚠️ Via Eloquent | ⚠️ Via Eloquent | ⚠️ Server | N/A |
| Value objects | ⚠️ Limited | ❌ | ❌ | ❌ | N/A |
| Domain events | ✅ internal/events/ | ⚠️ Laravel queue jobs | ⚠️ Laravel queue jobs | ❌ | ✅ events/ |

## Dependency Direction

| Aspect | GamePanel | Pterodactyl | Pelican | PufferPanel | Wings |
|--------|-----------|-------------|---------|-------------|-------|
| Inward deps (Clean Arch) | ✅ Domain ← HTTP ← Store | ⚠️ Controller → Service → Model | ⚠️ Controller → Service → Model | ✅ Service → Database | N/A |
| Circular dependencies | ❌ None found | ⚠️ Possible in large Laravel apps | ⚠️ Possible | ❌ None found | N/A |

## Extensibility

| Feature | GamePanel | Pterodactyl | Pelican | PufferPanel | Wings |
|---------|-----------|-------------|---------|-------------|-------|
| Plugin system | ✅ plugins/ (metadata) | ✅ Plugin API | ✅ Full runtime | ❌ | ❌ |
| Webhook system | ✅ Migrations + handlers | ✅ Notifications | ⚠️ Basic | ❌ | ❌ |
| API extensibility | ✅ OAuth2 + API keys | ✅ API keys | ✅ API keys | ✅ OAuth2 | N/A |
| Template/customization | ✅ server_templates | ✅ Nests/Eggs | ✅ Nests/Eggs | ✅ Eggs | N/A |

## Scalability

| Aspect | GamePanel | Pterodactyl | Pelican | PufferPanel | Wings |
|--------|-----------|-------------|---------|-------------|-------|
| Horizontal API | ✅ Stateless + DB | ⚠️ Session-dependent | ⚠️ Session-dependent | ✅ Stateless | N/A |
| Horizontal daemon | ✅ Multiple nodes, regions | ✅ Multiple nodes | ✅ Multiple nodes | ❌ Single node | N/A |
| Queue workers | ✅ async_delivery, webhook worker | ✅ Laravel queue | ✅ Laravel queue | ❌ | N/A |
| Database connection pooling | ⚠️ Not explicit | ⚠️ Laravel default | ⚠️ Laravel default | ⚠️ GORM pool | N/A |

## Ranking

| Rank | Project | Justification |
|------|---------|---------------|
| 1 | GamePanel | Cleanest separation, event-driven, multi-region, modular monolith |
| 2 | PufferPanel | Good package separation, OAuth2, but single-binary limits scaling |
| 3 | Wings | Well-structured daemon, good event system, but no API layer |
| 4 | Pterodactyl | Mature but monolithic Laravel, session-dependent |
| 5 | Pelican | Similar to Pterodactyl, slightly more features but same architectural limitations |
