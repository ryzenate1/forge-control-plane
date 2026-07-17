# Development Setup

## Prerequisites
- Go 1.26+
- Node.js 20+
- pnpm
- Docker (for PostgreSQL/Redis)

## Quick Start

### 1. Clone the repository
```bash
git clone https://github.com/anomalyco/gamepanel.git
cd gamepanel
```

### 2. Start infrastructure
```bash
docker compose -f infra/compose.yml up -d postgres redis
```

### 3. Configure environment
```bash
cp .env.example .env
# Edit .env with your settings
```

### 4. Start API
```bash
cd forge/api
go run ./cmd/api
```

### 5. Start Frontend
```bash
cd forge/web
pnpm install
pnpm dev
```

### 6. Open browser
Navigate to http://localhost:3000

## Project Structure
```
gamepanel/
├── forge/
│   ├── api/          # Go API (Fiber)
│   │   ├── cmd/      # Entry points
│   │   ├── internal/ # Application code
│   │   │   ├── http/       # HTTP handlers
│   │   │   ├── store/      # Database access
│   │   │   ├── services/   # Background services
│   │   │   ├── policies/   # Authorization
│   │   │   └── observers/  # Event observers
│   │   ├── migrations/     # SQL migrations
│   │   └── docs/           # API documentation
│   └── web/          # Next.js frontend
├── beacon/           # Node daemon (Go)
├── packages/         # Shared packages
│   ├── ui/           # React UI components
│   ├── sdk/          # TypeScript SDK
│   └── shared-types/ # Shared type definitions
├── infra/            # Infrastructure configs
├── lang/             # Translation files
└── docs/             # Documentation
```

## Database Migrations
```bash
# Run migrations (automatic on startup)
cd forge/api && go run ./cmd/api

# Create a new migration
touch forge/api/migrations/NNN_description.sql
```

## Testing
```bash
# Run all tests
make test

# Run API tests only
make api-test

# Run with coverage
cd forge/api && go test -cover -count=1 ./...
```
