<#
.SYNOPSIS
    Forge Control Plane - Native Windows Dev Runner.
.DESCRIPTION
    Manages local native development services on Windows:
    PostgreSQL, Redis, Forge API, Beacon daemon, and Web dashboard.
.USAGE
    .\native.ps1 start              # Start all services
    .\native.ps1 start -WithWeb     # Start all services including Next.js frontend
    .\native.ps1 stop               # Stop all running services
    .\native.ps1 status             # Check service and port health
    .\native.ps1 logs [service]     # View logs for api, beacon, pgsql, or redis
#>

param(
    [Parameter(Position = 0)]
    [ValidateSet("start", "stop", "restart", "status", "logs")]
    [string]$Command = "start",

    [Parameter(Position = 1)]
    [string]$Service = "api",

    [switch]$WithWeb
)

$ErrorActionPreference = "Continue"
$ROOT = $PSScriptRoot

# --- Ports ---
$DB_PORT      = 5432
$REDIS_PORT   = 6379
$API_PORT     = 8080
$BEACON_PORT  = 9090
$WEB_PORT     = 3000

# --- Directories ---
$TOOLS_DIR   = "$ROOT\.dev-tools"
$DATA_DIR    = "$ROOT\.dev-data"
$LOG_DIR     = "$ROOT\.dev-logs"
$PID_DIR     = "$ROOT\.dev-pids"
$SECRETS     = "$ROOT\.dev-secrets.env"

New-Item -ItemType Directory -Force -Path $TOOLS_DIR, $DATA_DIR, $LOG_DIR, $PID_DIR | Out-Null

# --- PATH Configuration ---
$env:PATH = "$TOOLS_DIR\go\bin;$TOOLS_DIR\pgsql\bin;$TOOLS_DIR\redis;$env:PATH"

# --- Colors & Helpers ---
function Write-Header($msg) { Write-Host "`n=== $msg ===" -ForegroundColor Cyan }
function Write-OK($msg)     { Write-Host "  [ok]   $msg" -ForegroundColor Green }
function Write-Warn($msg)   { Write-Host "  [warn] $msg" -ForegroundColor Yellow }
function Write-Fail($msg)   { Write-Host "  [fail] $msg" -ForegroundColor Red }
function Write-Info($msg)   { Write-Host "  $msg" }

function Test-PortOpen($port) {
    try {
        $client = New-Object System.Net.Sockets.TcpClient
        $iar = $client.BeginConnect("127.0.0.1", $port, $null, $null)
        $success = $iar.AsyncWaitHandle.WaitOne(800, $false)
        if ($success -and $client.Connected) {
            $client.EndConnect($iar)
            $client.Close()
            return $true
        }
        $client.Close()
        return $false
    } catch {
        return $false
    }
}

function Wait-Port($port, $maxSeconds = 30) {
    for ($i = 0; $i -lt $maxSeconds; $i++) {
        if (Test-PortOpen $port) { return $true }
        Start-Sleep -Seconds 1
    }
    return $false
}

function Ensure-Secrets {
    if (-not (Test-Path $SECRETS)) {
        $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
        $b32 = New-Object byte[] 32
        $b24 = New-Object byte[] 24
        $b16 = New-Object byte[] 16

        $rng.GetBytes($b32)
        $authSecret = "dev-" + [System.BitConverter]::ToString($b32).Replace("-","").ToLower()

        $rng.GetBytes($b32)
        $appKey = "base64:" + [System.Convert]::ToBase64String($b32)

        $rng.GetBytes($b32)
        $masterKey = [System.Convert]::ToBase64String($b32)

        $rng.GetBytes($b24)
        $nodeToken = "devnodetoken0001." + [System.BitConverter]::ToString($b24).Replace("-","").ToLower()

        $rng.GetBytes($b16)
        $sftpPass = "dev-" + [System.BitConverter]::ToString($b16).Replace("-","").ToLower()

        @"
API_AUTH_SECRET=$authSecret
APP_KEY=$appKey
FORGE_MASTER_KEY=$masterKey
DAEMON_NODE_TOKEN=$nodeToken
DAEMON_SFTP_HOST_KEY_PASSPHRASE=$sftpPass
"@ | Set-Content -Path $SECRETS -Encoding UTF8
    }

    Get-Content $SECRETS | ForEach-Object {
        if ($_ -match '^(.*?)=(.*)$') {
            [System.Environment]::SetEnvironmentVariable($matches[1], $matches[2], "Process")
        }
    }
}

function Ensure-Env {
    Ensure-Secrets
    $env:DATABASE_URL                  = "postgres://gamepanel:gamepanel@127.0.0.1:${DB_PORT}/gamepanel?sslmode=disable"
    $env:API_ADDR                      = ":${API_PORT}"
    $env:APP_ENV                       = "development"
    $env:APP_CIPHER                    = "AES-256-GCM"
    $env:API_SEED_DEMO                 = "true"
    $env:REDIS_ADDR                    = "127.0.0.1:${REDIS_PORT}"
    $env:REDIS_PASSWORD                = ""
    $env:BEACON_BASE_URL               = "http://127.0.0.1:${BEACON_PORT}"
    $env:PANEL_URL                     = "http://localhost:${WEB_PORT}"
    $env:SESSION_COOKIE_SECURE         = "false"
    $env:FORGE_MASTER_KEY_ID           = "primary"
    $env:FORGE_ALLOW_EPHEMERAL_MASTER_KEY = "false"
    $env:DAEMON_NODE_ID                = "22222222-2222-2222-2222-222222222222"
    $env:NEXT_PUBLIC_API_URL           = "/api/v1"
    $env:API_INTERNAL_URL              = "http://127.0.0.1:${API_PORT}"
    $env:MIGRATIONS_DIR                = "$ROOT\forge\api\migrations"
    $env:DAEMON_ADDR                   = ":${BEACON_PORT}"
    $env:DAEMON_DATA_DIR               = "$DATA_DIR\beacon"
    $env:PANEL_API_URL                 = "http://127.0.0.1:${API_PORT}/api/v1"
    $env:DAEMON_ALLOW_MOCK_RUNTIME     = "true"
}

function Stop-ProcByName($name) {
    Get-Process -Name $name -ErrorAction SilentlyContinue | ForEach-Object {
        try { Stop-Process -Id $_.Id -Force -ErrorAction SilentlyContinue } catch {}
    }
}

function Stop-All {
    Write-Header "Stopping Forge Dev Services"

    Stop-ProcByName "api"
    Stop-ProcByName "daemon"
    Stop-ProcByName "postgres"
    Stop-ProcByName "redis-server"

    $webPids = netstat -ano 2>$null | Select-String ":${WEB_PORT}\s"
    if ($webPids) {
        $webPids | ForEach-Object { ($_ -split '\s+')[-1] } | Sort-Object -Unique | ForEach-Object {
            try { Stop-Process -Id $_ -Force -ErrorAction SilentlyContinue } catch {}
        }
    }

    Write-OK "All native services stopped"
}

function Show-Status {
    Write-Header "Forge Dev Services Status"

    $pgOk = Test-PortOpen $DB_PORT
    if ($pgOk) { Write-OK "PostgreSQL listening on 127.0.0.1:$DB_PORT" } else { Write-Fail "PostgreSQL NOT listening on $DB_PORT" }

    $rdOk = Test-PortOpen $REDIS_PORT
    if ($rdOk) { Write-OK "Redis listening on 127.0.0.1:$REDIS_PORT" } else { Write-Fail "Redis NOT listening on $REDIS_PORT" }

    $apiOk = Test-PortOpen $API_PORT
    if ($apiOk) { Write-OK "Forge API listening on 127.0.0.1:$API_PORT" } else { Write-Warn "Forge API NOT listening on $API_PORT" }

    $bcnOk = Test-PortOpen $BEACON_PORT
    if ($bcnOk) { Write-OK "Beacon daemon listening on 127.0.0.1:$BEACON_PORT" } else { Write-Warn "Beacon daemon NOT listening on $BEACON_PORT" }

    $webOk = Test-PortOpen $WEB_PORT
    if ($webOk) { Write-OK "Web frontend listening on 127.0.0.1:$WEB_PORT" } else { Write-Info "Web frontend not active on $WEB_PORT" }
}

function Start-All {
    Ensure-Env

    Write-Header "PostgreSQL & Redis (Native)"

    # PostgreSQL
    if (-not (Test-PortOpen $DB_PORT)) {
        Write-Info "Starting PostgreSQL on port $DB_PORT..."
        Start-Process -FilePath "$TOOLS_DIR\pgsql\bin\postgres.exe" `
            -ArgumentList "-D", "`"$DATA_DIR\pgsql`"" `
            -RedirectStandardOutput "$LOG_DIR\pgsql.log" `
            -RedirectStandardError "$LOG_DIR\pgsql.err.log" `
            -WindowStyle Hidden
        if (Wait-Port $DB_PORT 15) {
            Write-OK "PostgreSQL running on 127.0.0.1:$DB_PORT"
        } else {
            Write-Fail "Failed to start PostgreSQL; check $LOG_DIR\pgsql.err.log"
            exit 1
        }
    } else {
        Write-OK "PostgreSQL already running on port $DB_PORT"
    }

    # Redis
    if (-not (Test-PortOpen $REDIS_PORT)) {
        Write-Info "Starting Redis on port $REDIS_PORT..."
        Start-Process -FilePath "$TOOLS_DIR\redis\redis-server.exe" `
            -ArgumentList "--port", "$REDIS_PORT" `
            -RedirectStandardOutput "$LOG_DIR\redis.log" `
            -RedirectStandardError "$LOG_DIR\redis.err.log" `
            -WindowStyle Hidden
        if (Wait-Port $REDIS_PORT 10) {
            Write-OK "Redis running on 127.0.0.1:$REDIS_PORT"
        } else {
            Write-Warn "Redis did not start on $REDIS_PORT"
        }
    } else {
        Write-OK "Redis already running on port $REDIS_PORT"
    }

    Write-Header "Forge API"
    if (-not (Test-PortOpen $API_PORT)) {
        Write-Info "Starting Forge API (running migrations and demo seed)..."
        Start-Process -FilePath "$ROOT\forge\api\api.exe" `
            -WorkingDirectory "$ROOT\forge\api" `
            -RedirectStandardOutput "$LOG_DIR\api.log" `
            -RedirectStandardError "$LOG_DIR\api.err.log" `
            -WindowStyle Hidden
        if (Wait-Port $API_PORT 60) {
            Write-OK "Forge API ready on http://localhost:$API_PORT/api/v1"
        } else {
            Write-Fail "API did not open port $API_PORT within timeout; check $LOG_DIR\api.err.log"
        }
    } else {
        Write-OK "Forge API already running on port $API_PORT"
    }

    Write-Header "Beacon Daemon"
    if (-not (Test-PortOpen $BEACON_PORT)) {
        Write-Info "Starting Beacon daemon..."
        Start-Process -FilePath "$ROOT\beacon\daemon.exe" `
            -WorkingDirectory "$ROOT\beacon" `
            -RedirectStandardOutput "$LOG_DIR\beacon.log" `
            -RedirectStandardError "$LOG_DIR\beacon.err.log" `
            -WindowStyle Hidden
        if (Wait-Port $BEACON_PORT 20) {
            Write-OK "Beacon running on http://localhost:$BEACON_PORT/health"
        } else {
            Write-Warn "Beacon has not opened port $BEACON_PORT yet; check $LOG_DIR\beacon.log"
        }
    } else {
        Write-OK "Beacon daemon already running on port $BEACON_PORT"
    }

    if ($WithWeb) {
        Write-Header "Forge Web (Next.js)"
        if (-not (Test-PortOpen $WEB_PORT)) {
            Write-Info "Starting Next.js frontend..."
            Start-Process -FilePath "cmd.exe" `
                -ArgumentList "/c", "npm run dev" `
                -WorkingDirectory "$ROOT" `
                -RedirectStandardOutput "$LOG_DIR\web.log" `
                -RedirectStandardError "$LOG_DIR\web.err.log" `
                -WindowStyle Hidden
            if (Wait-Port $WEB_PORT 45) {
                Write-OK "Web frontend ready on http://localhost:$WEB_PORT"
            } else {
                Write-Info "Web frontend is still compiling; check $LOG_DIR\web.log"
            }
        } else {
            Write-OK "Web frontend already running on port $WEB_PORT"
        }
    }

    Write-Host ""
    Write-Host "=========================================" -ForegroundColor Green
    Write-Host "       All Forge Services Running!       " -ForegroundColor Green
    Write-Host "=========================================" -ForegroundColor Green
    Write-Host "  API:         http://localhost:$API_PORT/api/v1" -ForegroundColor Cyan
    Write-Host "  Beacon:      http://localhost:$BEACON_PORT/health" -ForegroundColor Cyan
    Write-Host "  PostgreSQL:  127.0.0.1:$DB_PORT" -ForegroundColor Cyan
    Write-Host "  Redis:       127.0.0.1:$REDIS_PORT" -ForegroundColor Cyan
    if ($WithWeb) {
        Write-Host "  Web UI:      http://localhost:$WEB_PORT" -ForegroundColor Cyan
    }
    Write-Host ""
    Write-Host "  Stop:   .\native.ps1 stop" -ForegroundColor Yellow
    Write-Host "  Status: .\native.ps1 status" -ForegroundColor Yellow
    Write-Host "  Logs:   .\native.ps1 logs <api|beacon|pgsql|redis>" -ForegroundColor Yellow
    Write-Host ""
}

function Show-Logs($svc) {
    $logFile = "$LOG_DIR\$svc.log"
    $errFile = "$LOG_DIR\$svc.err.log"
    if (Test-Path $logFile) {
        Write-Header "$svc Log (last 30 lines)"
        Get-Content $logFile -Tail 30
    }
    if (Test-Path $errFile) {
        $errContent = Get-Content $errFile -Tail 30
        if ($errContent) {
            Write-Header "$svc Errors (last 30 lines)"
            $errContent
        }
    }
}

switch ($Command) {
    "start"   { Start-All }
    "stop"    { Stop-All }
    "restart" { Stop-All; Start-Sleep -Seconds 2; Start-All }
    "status"  { Show-Status }
    "logs"    { Show-Logs $Service }
}
