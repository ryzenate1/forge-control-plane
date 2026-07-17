# Reference Architecture Analysis

**Date:** 2026-06-16  
**Purpose:** Deep analysis of Pterodactyl, Pelican, and PufferPanel project structures  
**Primary Reference:** Pterodactyl Panel + Wings

---

## Executive Summary

This document analyzes three mature game server management platforms to understand their architectural decisions, project organization, and design patterns.

**Key Finding:** These projects use **separate repositories** for Panel (control plane) and Wings (daemon), not a monorepo approach.

---

## 1. Pterodactyl Architecture (Primary Reference)

### Repository Structure

**Two Separate Repositories:**
1. **pterodactyl/panel** - Control plane (PHP Laravel + React)
2. **pterodactyl/wings** - Daemon agent (Go)

### Pterodactyl Panel Structure

```
pterodactyl/panel/
├── app/                    # Laravel application core
│   ├── Console/           # CLI commands
│   ├── Contracts/         # Interfaces
│   ├── Events/            # Event definitions
│   ├── Exceptions/        # Custom exceptions
│   ├── Extensions/        # Framework extensions
│   ├── Facades/           # Laravel facades
│   ├── Http/              # HTTP layer
│   │   ├── Controllers/
│   │   │   ├── Api/
│   │   │   │   ├── Application/    # Admin API
│   │   │   │   ├── Client/         # User/Server API
│   │   │   │   └── Remote/         # Wings API
│   │   │   ├── Admin/              # Admin web UI
│   │   │   ├── Auth/               # Authentication
│   │   │   └── Base/               # Base controllers
│   │   ├── Middleware/             # Request middleware
│   │   ├── Requests/               # Form requests
│   │   └── ViewComposers/          # View composers
│   ├── Jobs/              # Queue jobs
│   ├── Models/            # Eloquent models
│   ├── Notifications/     # Email/notification templates
│   ├── Observers/         # Model observers
│   ├── Policies/          # Authorization policies
│   ├── Providers/         # Service providers
│   ├── Repositories/      # Data repositories
│   ├── Services/          # Business logic services
│   ├── Traits/            # Reusable traits
│   └── Transformers/      # API transformers
│
├── bootstrap/             # Application bootstrap
├── config/                # Configuration files
│   ├── activity.php
│   ├── auth.php
│   ├── backups.php
│   ├── database.php
│   └── pterodactyl.php    # Core config
│
├── database/              # Database layer
│   ├── Factories/         # Model factories
│   ├── migrations/        # Schema migrations
│   └── Seeders/           # Data seeders
│
├── public/                # Web root
│   ├── assets/            # Compiled assets
│   ├── themes/            # UI themes
│   └── index.php          # Entry point
│
├── resources/             # Raw resources
│   ├── lang/              # Translations
│   ├── scripts/           # React/TypeScript frontend
│   │   ├── api/           # API client
│   │   ├── components/    # React components
│   │   │   ├── auth/
│   │   │   ├── dashboard/
│   │   │   ├── elements/  # Reusable UI components
│   │   │   └── server/    # Server management
│   │   ├── state/         # State management
│   │   └── routers/       # React router
│   └── views/             # Blade templates
│
├── routes/                # Route definitions
│   ├── admin.php          # Admin web routes
│   ├── auth.php           # Auth routes
│   ├── base.php           # Base web routes
│   ├── api-application.php # Admin API
│   ├── api-client.php      # Client API
│   └── api-remote.php      # Wings communication
│
├── storage/               # Runtime storage
│   ├── app/
│   ├── framework/
│   └── logs/
│
├── tests/                 # Test suites
│   ├── Integration/
│   └── Unit/
│
├── .env.example
├── composer.json          # PHP dependencies
├── package.json           # Node dependencies
└── webpack.config.js      # Frontend build
```

### Pterodactyl Wings Structure

```
pterodactyl/wings/
├── cmd/                   # CLI commands
│   ├── configure.go       # Configuration wizard
│   ├── diagnostics.go     # System diagnostics
│   └── root.go            # Root command
│
├── config/                # Configuration management
│   ├── config.go          # Config structure
│   └── config_docker.go   # Docker config
│
├── environment/           # Runtime environment
│   ├── docker/            # Docker implementation
│   ├── allocations.go     # Port allocations
│   ├── environment.go     # Interface
│   ├── settings.go        # Container settings
│   └── stats.go           # Resource stats
│
├── events/                # Event system
│   └── events.go
│
├── internal/              # Internal packages
│   ├── cron/              # Scheduled tasks
│   ├── database/          # Local SQLite
│   ├── models/            # Data models
│   ├── progress/          # Progress tracking
│   └── ufs/               # Filesystem utilities
│
├── parser/                # Config parser
│   └── parser.go
│
├── remote/                # Panel communication
│   ├── http.go            # HTTP client
│   ├── servers.go         # Server sync
│   └── types.go           # API types
│
├── router/                # HTTP API
│   ├── middleware/        # Auth middleware
│   ├── tokens/            # JWT tokens
│   ├── websocket/         # WebSocket server
│   ├── router.go          # Route setup
│   ├── router_server.go   # Server endpoints
│   ├── router_server_files.go
│   ├── router_server_backup.go
│   └── router_system.go   # System endpoints
│
├── server/                # Server management
│   ├── backup/            # Backup system
│   ├── filesystem/        # File operations
│   ├── installer/         # Server installation
│   ├── transfer/          # Server transfers
│   ├── server.go          # Server struct
│   ├── power.go           # Power management
│   ├── console.go         # Console handling
│   ├── install.go         # Installation
│   └── manager.go         # Server manager
│
├── sftp/                  # SFTP server
│   ├── server.go
│   └── handler.go
│
├── system/                # System utilities
│   ├── locker.go          # Resource locking
│   ├── rate.go            # Rate limiting
│   └── utils.go
│
├── go.mod                 # Go dependencies
├── Makefile               # Build automation
└── wings.go               # Main entry point
```

### Key Architectural Patterns (Pterodactyl)

1. **Separation of Concerns**
   - Panel: PHP Laravel (control plane, web UI, admin functions)
   - Wings: Go (node agent, container management, file operations)

2. **API Layer Organization**
   ```
   /api/application/*  → Admin API (manage everything)
   /api/client/*       → Client API (user/server operations)
   /api/remote/*       → Wings communication (internal)
   ```

3. **Service-Repository Pattern (Panel)**
   - Controllers → Services → Repositories → Models
   - Business logic in Services
   - Data access in Repositories

4. **React Frontend Integration**
   - Separate `resources/scripts/` directory
   - API client in `api/`
   - Component organization mirrors features
   - Webpack builds to `public/assets/`

5. **Wings Communication**
   - REST API for all operations
   - WebSocket for console streaming
   - JWT tokens for authentication
   - Wings polls Panel for configuration updates

---

## 2. Pelican Architecture (Modernized Pterodactyl)

### Repository Structure

**Same Pattern as Pterodactyl:**
1. **pelican-dev/panel** - Control plane (PHP Laravel + Filament)
2. **pelican-dev/wings** - Daemon agent (Go, forked from Pterodactyl)

### Key Differences from Pterodactyl

```
pelican/panel/
├── app/
│   ├── Filament/          # NEW: Filament admin UI
│   │   ├── Admin/         # Admin panel resources
│   │   │   ├── Pages/
│   │   │   ├── Resources/ # CRUD resources
│   │   │   └── Widgets/   # Dashboard widgets
│   │   ├── Server/        # Server panel
│   │   │   ├── Pages/
│   │   │   └── Resources/
│   │   └── Components/    # Shared Filament components
│   │
│   ├── Livewire/          # NEW: Livewire components
│   └── Enums/             # NEW: PHP 8.1 enums
│
├── lang/                  # NEW: 32 languages
│   ├── en/
│   ├── es/
│   ├── fr/
│   └── ...
│
├── plugins/               # NEW: Plugin system
│
└── resources/
    ├── views/
    │   ├── filament/      # Filament views
    │   └── livewire/      # Livewire views
    └── js/                # Minimal JS (Filament handles most)
```

### Pelican Improvements

1. **Modern PHP Stack**
   - PHP 8.1+ with enums, attributes
   - Filament for admin UI (replaces custom React admin)
   - Livewire for reactive components

2. **Enhanced API**
   - Resource-based API design
   - Better pagination
   - Improved filtering
   - Enhanced permissions

3. **Plugin System**
   - Pluggable architecture
   - Hook system
   - Event-driven extensions

4. **Internationalization**
   - 32 languages out of box
   - Crowdin integration

---

## 3. PufferPanel Architecture (Monolithic Go)

### Repository Structure

**Single Monorepo:**
```
PufferPanel/PufferPanel/
├── cmd/                   # CLI entry points
│   ├── main.go
│   ├── run.go
│   └── user.go
│
├── client/                # Frontend (Vue.js)
│   ├── frontend/
│   │   ├── src/
│   │   │   ├── components/
│   │   │   │   ├── server/  # Server components
│   │   │   │   ├── template/ # Template editor
│   │   │   │   └── ui/       # UI components
│   │   │   ├── views/        # Pages
│   │   │   ├── router/       # Vue router
│   │   │   ├── lang/         # 35 languages
│   │   │   └── plugins/      # Vue plugins
│   │   └── public/
│   └── api/               # Generated API client
│
├── config/                # Configuration
├── database/              # Database layer
├── email/                 # Email providers
├── middleware/            # HTTP middleware
├── models/                # Data models
├── operations/            # Server operations
│   ├── command/
│   ├── download/
│   ├── mkdir/
│   └── ...
│
├── servers/               # Server management
│   ├── docker/            # Docker runtime
│   └── tty/               # TTY runtime
│
├── services/              # Business services
│   ├── backup.go
│   ├── node.go
│   ├── permission.go
│   ├── server.go
│   ├── sftp.go
│   └── user.go
│
├── sftp/                  # SFTP server
├── web/                   # Web layer
│   ├── api/               # API routes
│   ├── auth/              # Auth routes
│   ├── daemon/            # Daemon API (like Wings)
│   └── oauth2/            # OAuth2
│
└── go.mod
```

### PufferPanel Differences

1. **All-in-One Architecture**
   - Panel + Daemon in same binary
   - Can run as panel-only, daemon-only, or both
   - Simpler deployment

2. **Vue.js Frontend**
   - Complete Vue 3 SPA
   - 35 languages built-in
   - Theme system
   - Minimal backend rendering

3. **Operations Pattern**
   - Extensible operation system
   - Each operation is a plugin
   - Template-driven server configs

4. **Multi-Node Support**
   - Node table for distributed setup
   - Panel can manage remote daemons
   - RPC-style communication

---

## Comparative Analysis

| Aspect | Pterodactyl | Pelican | PufferPanel |
|--------|-------------|---------|-------------|
| **Repos** | 2 separate | 2 separate | 1 monorepo |
| **Panel Language** | PHP (Laravel) | PHP (Laravel) | Go |
| **Panel Frontend** | React | Filament + Livewire | Vue.js |
| **Daemon Language** | Go | Go | Go (embedded) |
| **Admin UI** | Custom React | Filament | Vue.js |
| **Client UI** | Custom React | Filament | Vue.js |
| **API Design** | RESTful | RESTful (improved) | RESTful |
| **Auth** | Session + JWT | Session + JWT | Session + JWT |
| **i18n** | Limited | 32 languages | 35 languages |
| **Plugin System** | No | Yes | No |
| **Deployment** | Panel + Wings | Panel + Wings | Single binary |

---

## Architectural Patterns Common to All

### 1. **Separation of Control and Execution**
- Control Plane (Panel): User management, server definitions, orchestration
- Execution Plane (Wings/Daemon): Container lifecycle, file operations, console

### 2. **API-First Design**
```
Admin API    → Full control
Client API   → User/server operations
Daemon API   → Node-local operations
```

### 3. **Docker-Based Isolation**
- All use Docker for server isolation
- Similar container configuration patterns
- Resource limits (CPU, RAM, disk, network)

### 4. **Port Allocation System**
- Allocations table (IP:Port combinations)
- Primary + additional allocations
- Automatic assignment

### 5. **Egg/Template System**
- Nests (categories) → Eggs (templates)
- Variables with validation
- Install scripts
- Docker images
- Startup commands

### 6. **Permission Model**
- Root admin flag
- Per-server permissions
- Subuser system
- Scoped API keys

### 7. **File Management**
- Chroot per server
- SFTP access
- Archive/extract operations
- Remote URL pull

### 8. **Backup System**
- Local + S3 storage
- Scheduled backups
- Restore functionality
- Size limits

---

## Best Practices Observed

### Project Organization

1. **Clear Layer Separation**
   ```
   Routes → Controllers → Services → Repositories → Models
   ```

2. **Feature-Based Structure**
   ```
   /components/server/console/
   /components/server/files/
   /components/server/backups/
   ```

3. **Shared Components**
   ```
   /components/elements/     (UI primitives)
   /components/dashboard/    (Account features)
   ```

4. **API Client Abstraction**
   ```typescript
   /api/server/getServer.ts
   /api/server/updateServer.ts
   /api/server/deleteServer.ts
   ```

### Configuration Management

1. **Environment-Based**
   - `.env.example` as template
   - Separate configs per environment
   - Validation on startup

2. **Type-Safe Configs**
   - PHP config files return arrays
   - Go structs with tags
   - JSON schemas for validation

### Database Design

1. **Migration-Driven**
   - Sequential migrations
   - Up/down methods
   - Foreign key constraints
   - Indexes on lookups

2. **Soft Deletes**
   - `deleted_at` column
   - Audit trail preservation
   - Cascade delete handling

### API Design

1. **RESTful Conventions**
   ```
   GET    /servers        → list
   POST   /servers        → create
   GET    /servers/:id    → show
   PATCH  /servers/:id    → update
   DELETE /servers/:id    → delete
   ```

2. **Nested Resources**
   ```
   /servers/:id/files
   /servers/:id/backups
   /servers/:id/databases
   ```

3. **Action Endpoints**
   ```
   POST /servers/:id/power        (start/stop/restart)
   POST /servers/:id/reinstall
   POST /servers/:id/suspend
   ```

---

## Recommendations for GamePanel

### Keep Current Strengths

1. **Monorepo Approach** ✅
   - Easier development
   - Atomic commits
   - Shared types via contracts package

2. **Go API** ✅
   - Better performance than PHP
   - Modern concurrency
   - Type safety

3. **Next.js Frontend** ✅
   - Modern React framework
   - SSR capabilities
   - Better DX than custom React setup

4. **Services Architecture** ✅
   - Scheduler, Reconciler, Registry
   - More advanced than references

### Adopt Reference Patterns

1. **Route Organization**
   ```
   /api/v1/admin/*    (like Pterodactyl Application API)
   /api/v1/client/*   (like Pterodactyl Client API)
   /api/v1/nodes/*    (like Wings API)
   ```

2. **Component Structure**
   ```
   apps/frontend/components/
   ├── elements/      (UI primitives)
   ├── dashboard/     (Account features)
   ├── server/        (Server features)
   │   ├── console/
   │   ├── files/
   │   ├── backups/
   │   └── ...
   └── admin/         (Admin features)
   ```

3. **Service Layer**
   ```
   apps/api/internal/services/
   ├── server/
   │   ├── creation.go
   │   ├── suspension.go
   │   └── deletion.go
   ├── allocation/
   ├── backup/
   └── ...
   ```

4. **Clear API Contracts**
   ```
   packages/contracts/
   ├── api/           (API DTOs)
   ├── events/        (Event types)
   └── models/        (Shared models)
   ```

### Don't Copy Blindly

1. **Don't Switch to PHP** ❌
   - GamePanel's Go API is better
   - Modern, performant, type-safe

2. **Don't Merge Panel + Daemon** ❌
   - Separation is correct
   - Clear boundaries
   - Independent scaling

3. **Don't Replace Next.js** ❌
   - Already modern
   - Better than Blade templates
   - Better than pure React setup

### Focus on Gaps

1. **Complete Feature Parity**
   - Database provisioning
   - S3 backups
   - Server transfers
   - Subuser invitations

2. **Polish Existing Features**
   - Consolidate duplicate code
   - Remove mock data
   - Complete workflows

3. **Production Hardening**
   - Rate limiting
   - Audit logging
   - Error handling
   - Documentation

---

## Conclusion

**GamePanel already follows modern best practices** with its monorepo, Go backend, and Next.js frontend. The architecture is sound and in many ways more advanced than the references (scheduler, reconciler, orchestration).

The **primary gap is feature completeness**, not architecture. Focus on:
1. ✅ Completing half-implemented features
2. ✅ Removing mock/demo data
3. ✅ End-to-end testing
4. ✅ Production deployment guides
5. ✅ User documentation

**Do NOT restructure** the project. The current organization is production-ready. Just finish what's already started.
