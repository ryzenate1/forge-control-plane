<#
.SYNOPSIS
    GamePanel - Single-command dev environment launcher.

.USAGE
    .\start-dev.ps1                # Start with Docker PostgreSQL + Redis (default)
    .\start-dev.ps1 -Native        # Start with locally installed PostgreSQL + Redis
    .\start-dev.ps1 -Stop          # Stop everything
#>

param(
    [switch]$Stop,
    [switch]$Native
)

$ErrorActionPreference = "Continue"
$ROOT = $PSScriptRoot

# --- Configuration ---
$DB_USER       = "gamepanel"
$DB_PASS       = "gamepanel"
$DB_NAME       = "gamepanel"
$DB_PORT       = 5432
$REDIS_PORT    = 6379
$API_PORT      = 8080
$FRONTEND_PORT = 3000

$env:DATABASE_URL      = "postgres://${DB_USER}:${DB_PASS}@localhost:${DB_PORT}/${DB_NAME}?sslmode=disable"
$env:API_ADDR           = ":${API_PORT}"
$env:API_AUTH_SECRET     = "dev-api-secret"
$env:APP_ENV             = "development"
$env:DAEMON_NODE_TOKEN   = "dev-node-token"
$env:API_DEMO_MODE       = "false"
$env:REDIS_ADDR          = "localhost:${REDIS_PORT}"
$env:MIGRATIONS_DIR      = "$ROOT\forge\api\migrations"
$env:NEXT_PUBLIC_API_URL = "http://localhost:${API_PORT}/api/v1"

# --- Helpers ---
function Write-Status($icon, $msg, $color) { Write-Host "  $icon " -NoNewline -ForegroundColor $color; Write-Host $msg }
function Write-Header($msg) { Write-Host "`n=== $msg ===" -ForegroundColor Cyan }

function Test-Port($port, $maxRetries = 20) {
    for ($i = 0; $i -lt $maxRetries; $i++) {
        try {
            $tcp = New-Object System.Net.Sockets.TcpClient
            $tcp.Connect("127.0.0.1", $port)
            $tcp.Close()
            return $true
        } catch {
            Start-Sleep -Seconds 1
            Write-Host "." -NoNewline
        }
    }
    return $false
}

# --- Stop Mode ---
if ($Stop) {
    Write-Header "Stopping GamePanel Dev Environment"

    Get-Process -Name "api" -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
    Write-Status "[x]" "API stopped" Yellow

    $netstat = netstat -ano 2>$null | Select-String ":${FRONTEND_PORT}\s"
    if ($netstat) {
        $pids = $netstat | ForEach-Object { ($_ -split '\s+')[-1] } | Sort-Object -Unique
        foreach ($procId in $pids) {
            try { Stop-Process -Id $procId -Force -ErrorAction SilentlyContinue } catch {}
        }
    }
    Write-Status "[x]" "Frontend stopped" Yellow

    if (-not $Native) {
        Push-Location "$ROOT\deploy"
        docker compose down 2>$null
        Pop-Location
        Write-Status "[x]" "Docker services stopped" Yellow
    }

    Write-Host "`n  All services stopped.`n" -ForegroundColor Green
    exit 0
}

# --- Banner ---
Write-Host ""
Write-Host "  +==========================================+" -ForegroundColor Red
Write-Host "  |       GamePanel Dev Environment          |" -ForegroundColor Red
Write-Host "  +==========================================+" -ForegroundColor Red
if ($Native) {
    Write-Host "  |       Mode: NATIVE (no Docker)           |" -ForegroundColor Yellow
} else {
    Write-Host "  |       Mode: DOCKER (default)             |" -ForegroundColor DarkGray
}
Write-Host ""

# --- Step 1: Database + Redis ---
Write-Header "Starting PostgreSQL and Redis"

if ($Native) {
    # Native mode: expect PostgreSQL and Redis already installed and running
    Write-Host "  Checking local PostgreSQL..." -NoNewline
    if (Test-Port $DB_PORT 5) {
        Write-Host " Found!" -ForegroundColor Green
        Write-Status "[ok]" "PostgreSQL on port $DB_PORT (native)" Green
    } else {
        Write-Host " NOT RUNNING" -ForegroundColor Red
        Write-Host ""
        Write-Host "  PostgreSQL is not running on port $DB_PORT." -ForegroundColor Red
        Write-Host "  Install it natively:" -ForegroundColor Yellow
        Write-Host "    Windows:  https://www.postgresql.org/download/windows/" -ForegroundColor DarkGray
        Write-Host "    Linux:    sudo apt install postgresql-16" -ForegroundColor DarkGray
        Write-Host ""
        Write-Host "  Then create the database:" -ForegroundColor Yellow
        Write-Host "    sudo -u postgres createuser -s gamepanel" -ForegroundColor DarkGray
        Write-Host "    sudo -u postgres createdb -O gamepanel gamepanel" -ForegroundColor DarkGray
        Write-Host "    sudo -u postgres psql -c ""ALTER USER gamepanel PASSWORD 'gamepanel';""" -ForegroundColor DarkGray
        exit 1
    }

    Write-Host "  Checking local Redis..." -NoNewline
    if (Test-Port $REDIS_PORT 5) {
        Write-Host " Found!" -ForegroundColor Green
        Write-Status "[ok]" "Redis on port $REDIS_PORT (native)" Green
    } else {
        Write-Host " NOT RUNNING (optional, continuing without)" -ForegroundColor Yellow
        $env:REDIS_ADDR = ""
    }
} else {
    # Docker mode
    Push-Location "$ROOT\deploy"
    docker compose up -d postgres redis 2>&1 | Out-Null
    Pop-Location

    Write-Host "  Waiting for PostgreSQL..." -NoNewline
    if (Test-Port $DB_PORT 30) {
        Write-Host " Ready!" -ForegroundColor Green
        Write-Status "[ok]" "PostgreSQL on port $DB_PORT (Docker)" Green
    } else {
        Write-Host " FAILED" -ForegroundColor Red
        exit 1
    }

    Write-Host "  Waiting for Redis..." -NoNewline
    if (Test-Port $REDIS_PORT 15) {
        Write-Host " Ready!" -ForegroundColor Green
        Write-Status "[ok]" "Redis on port $REDIS_PORT (Docker)" Green
    } else {
        Write-Host " TIMEOUT" -ForegroundColor Yellow
    }
}

# --- Step 2: Go API ---
Write-Header "Starting Go API"

Write-Host "  Building API..."
Push-Location "$ROOT\apps\api"
go build -o "$ROOT\apps\api\api.exe" ./cmd/api 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "  API build failed!" -ForegroundColor Red
    Pop-Location
    exit 1
}
Pop-Location

$apiLog = "$ROOT\api-dev.log"
$apiErr = "$ROOT\api-dev.err.log"
$apiProc = Start-Process -FilePath "$ROOT\apps\api\api.exe" `
    -WorkingDirectory "$ROOT\apps\api" `
    -RedirectStandardOutput $apiLog `
    -RedirectStandardError $apiErr `
    -PassThru -WindowStyle Hidden

Write-Host "  Waiting for API..." -NoNewline
if (Test-Port $API_PORT 20) {
    Write-Host " Ready!" -ForegroundColor Green
    Write-Status "[ok]" "API on http://localhost:${API_PORT} (PID: $($apiProc.Id))" Green
} else {
    Write-Host " FAILED" -ForegroundColor Red
    Write-Host "  Check: $apiErr" -ForegroundColor Red
    Get-Content $apiErr -ErrorAction SilentlyContinue | Select-Object -Last 5
    exit 1
}

# --- Step 3: Next.js Frontend ---
Write-Header "Starting Next.js Frontend"

$feLog = "$ROOT\frontend-dev.log"
$feErr = "$ROOT\frontend-dev.err.log"
$feProc = Start-Process -FilePath "cmd.exe" `
    -ArgumentList "/c", "npx next dev -p $FRONTEND_PORT" `
    -WorkingDirectory "$ROOT\apps\frontend" `
    -RedirectStandardOutput $feLog `
    -RedirectStandardError $feErr `
    -PassThru -WindowStyle Hidden

Write-Host "  Waiting for Frontend..." -NoNewline
if (Test-Port $FRONTEND_PORT 30) {
    Write-Host " Ready!" -ForegroundColor Green
} else {
    Write-Host " (still compiling)" -ForegroundColor Yellow
}
Write-Status "[ok]" "Frontend on http://localhost:${FRONTEND_PORT} (PID: $($feProc.Id))" Green

# --- Summary ---
Write-Host ""
Write-Host "  +==========================================+" -ForegroundColor Green
Write-Host "  |       All Services Running!              |" -ForegroundColor Green
Write-Host "  +==========================================+" -ForegroundColor Green
Write-Host ""
Write-Host "  Service          URL" -ForegroundColor White
Write-Host "  ---------------  ----------------------------" -ForegroundColor DarkGray
Write-Status "FE " "Frontend        http://localhost:${FRONTEND_PORT}" Cyan
Write-Status "API" "API             http://localhost:${API_PORT}" Cyan
Write-Status "DB " "PostgreSQL      localhost:${DB_PORT}" Cyan
Write-Status "RDS" "Redis           localhost:${REDIS_PORT}" Cyan
Write-Host ""
Write-Host "  Setup: open http://localhost:${FRONTEND_PORT}/setup on first run" -ForegroundColor DarkGray
Write-Host ""
Write-Host "  Logs:" -ForegroundColor DarkGray
Write-Host "    API:      Get-Content $apiLog -Wait" -ForegroundColor DarkGray
Write-Host "    Frontend: Get-Content $feLog -Wait" -ForegroundColor DarkGray
Write-Host ""
Write-Host "  Stop:  .\start-dev.ps1 -Stop" -ForegroundColor Yellow
Write-Host ""
""
