# Repository Inventory

## File Counts

| Project | Backend Files | Frontend Files | SQL/Migrations | Config | Docs | Total Relevant |
|---------|:------------:|:--------------:|:--------------:|:------:|:----:|:--------------:|
| GamePanel | 196 (Go) | 445 (TS/JS) | 53 (SQL) | 141 | 127 | ~1,305 |
| Pterodactyl | 961 (PHP) | 287 (TS/JS) | ~50 (PHP migrations) | ~30 | 7 | ~1,648 |
| Pelican Panel | 1,716 (PHP) | 35 (JS) | ~50 (PHP migrations) | ~20 | 5 | ~2,108 |
| PufferPanel | 240 (Go) | 544 (JS/Vue) | ~15 (Go migrations) | ~30 | 11 | ~1,727 |
| Wings | 134 (Go) | 0 | 0 | ~10 | 3 | ~435 |

## Technology Stack

| Category | GamePanel | Pterodactyl | Pelican Panel | PufferPanel | Wings |
|----------|-----------|-------------|---------------|-------------|-------|
| Backend Language | Go | PHP (Laravel) | PHP (Laravel) | Go | Go |
| HTTP Framework | GoFiber | Laravel Router | Laravel Router | Gin | net/http |
| Frontend Framework | Next.js (React) | React+TypeScript | JavaScript | Vue.js | N/A |
| Database | PostgreSQL | MySQL | MySQL | SQLite/PostgreSQL | N/A |
| ORM | Raw SQL (pgx) | Eloquent | Eloquent | GORM | N/A |
| Package Manager | Go Modules + npm | Composer + npm | Composer + yarn | Go Modules + npm | Go Modules |
| Containerization | Docker | Docker | Docker | Docker | Docker |
| CI/CD | GitHub Actions | GitHub Actions | GitHub Actions | None | GitHub Actions |
| Monitoring | Prometheus+Grafana | None | None | None | None |
| Reverse Proxy | Nginx | N/A | N/A | N/A | N/A |
| Testing | Go testing + Vitest | PHPUnit + Jest | PHPUnit | Go testing | Go testing |
| Static Analysis | golangci-lint | PHPStan + ESLint | PHPStan + Prettier | None | None |

## Project Structure

### GamePanel
```
gamepanel/
├── beacon/          # Go daemon (Wings-equivalent)
│   ├── cmd/daemon/  # Entry point (main.go)
│   ├── config/      # Configuration
│   └── internal/    # backup, events, ignore, installer, remote, rootfs, runtime, server, sftpserver, system, transfer
├── forge/
│   ├── api/         # Go API server
│   │   ├── cmd/api/ # Entry point
│   │   ├── internal/ # daemon, domain, events, http, realtime, runtime, secrets, services, store
│   │   └── migrations/ # 53 SQL files
│   └── web/         # Next.js frontend
│       ├── app/     # Pages (account, admin, server, servers, setup)
│       ├── components/ # React components
│       ├── lib/     # Utilities, API client
│       └── stores/  # State management
├── packages/        # Shared: sdk, shared-types, ui
├── infra/           # compose.yml, nginx.conf, prometheus.yml, grafana/, alertmanager.yml
├── scripts/         # deploy, diagnose, start-dev, stop-dev, logs, status
├── docs/            # 127+ documentation files
└── .github/workflows/ # ci.yml, docker.yml, release.yml
```

### Pterodactyl
```
petrodactylpanel/
├── app/             # Laravel: Models, Services, Http/Controllers, Jobs, Notifications
├── bootstrap/       # Laravel bootstrap
├── config/          # Laravel config (auth, database, queue, etc.)
├── database/        # Migrations (195+), seeders
├── public/          # Static assets
├── resources/       # Views, JS/TS (React), CSS
├── routes/          # web.php, api.php
├── tests/           # PHPUnit tests
├── Dockerfile
└── docker-compose.example.yml
```

### Pelican Panel
```
pelicanpanel/
├── app/             # Laravel: Models, Services, Http/Controllers
├── bootstrap/       # Laravel bootstrap
├── config/          # Laravel config
├── database/        # Migrations (242+), seeders
├── plugins/         # Plugin system
├── public/          # Static assets
├── resources/       # Views, JS
├── routes/          # web.php, api.php
├── tests/           # Tests
├── Dockerfile, Dockerfile.base, Dockerfile.dev
├── compose.yml, compose-bind.yml, compose-full-stack.yml
└── README.md, security.md
```

### PufferPanel
```
pufferpanel/
├── *.go (root)      # Core types: server.go, engine.go, console.go, etc.
├── cmd/             # CLI commands
├── client/          # Vue.js frontend
├── config/          # Configuration
├── database/        # GORM migrations
├── middleware/       # HTTP middleware
├── models/          # Data models
├── servers/         # Server management
├── services/        # Business logic
├── sftp/            # SFTP server
├── web/             # HTTP router
├── Dockerfile (multiple variants)
└── go.mod, package-lock.json
```

### Wings
```
wings/
├── cmd/             # CLI (Cobra)
├── config/          # Configuration
├── environment/     # Docker environment
├── events/          # Event system
├── internal/        # Core logic
├── loggers/         # Logging
├── parser/          # Config parsing
├── remote/          # Panel communication
├── router/          # HTTP router
├── server/          # Server management (filesystem, backup, install, transfer, power)
├── sftp/            # SFTP server
├── system/          # System utilities
├── Dockerfile, Makefile
└── go.mod
```

## Exclusions

| Directory/File | Reason |
|----------------|--------|
| node_modules/ | Third-party npm dependencies |
| .next/ | Next.js build output |
| .git/ | Git metadata |
| vendor/ | PHP dependencies |
| storage/ | Runtime logs, cache |
| bootstrap/cache/ | Laravel cache |
| dist/ | Build output |
| *.gz, *.meta, *.pack | Compressed/cached/generated files |
