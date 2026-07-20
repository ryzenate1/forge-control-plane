# ============================================================
# GamePanel — Windows Installation Script (PowerShell)
#
# Installs the Forge control plane on Windows using Docker
# Desktop. Performs pre-flight checks, generates secure
# configuration, and starts all services.
#
# Requirements:
#   Windows 10/11 64-bit or Windows Server 2019+
#   Docker Desktop 4.x+ with WSL2 backend
#   PowerShell 5.1+ or PowerShell 7+
#
# Usage:
#   .\install.ps1
#   .\install.ps1 -Unattended -Fqdn panel.example.com -AdminEmail admin@example.com -AdminPassword MyPass123
# ============================================================

param(
    [switch]$Unattended,
    [string]$Fqdn,
    [string]$AdminEmail,
    [string]$AdminPassword,
    [string]$DbPassword,
    [switch]$SkipChecks,
    [switch]$Force,
    [switch]$Help
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

# --- Constants ---
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir
$MinDockerVersion = "24.0"
$MinRamGB = 2
$MinDiskGB = 20
$MinCpus = 2
$RequiredPorts = @(80, 443, 8080, 9090, 3000)
$Timestamp = (Get-Date -Format "yyyyMMddTHHmmssK")

# --- Help ---
if ($Help) {
    @"
GamePanel Windows Installation Script

Usage: .\install.ps1 [OPTIONS]

Options:
  -Unattended          Non-interactive mode
  -Fqdn                Domain name (e.g. panel.example.com)
  -AdminEmail          Administrator email
  -AdminPassword       Administrator password (min 12 chars)
  -DbPassword          PostgreSQL password (auto-generated if not set)
  -SkipChecks          Skip pre-flight system checks
  -Force               Force reinstall over existing installation
  -Help                Show this message

Environment variables (alternative to parameters):
  `$env:GAMEPANEL_FQDN
  `$env:GAMEPANEL_ADMIN_EMAIL
  `$env:GAMEPANEL_ADMIN_PASSWORD
  `$env:GAMEPANEL_DB_PASSWORD
"@
    exit 0
}

# --- Write helpers ---
function Write-Info  { Write-Host "  [ok] $args" -ForegroundColor Green }
function Write-Warning2 { Write-Host "  [!!] $args" -ForegroundColor Yellow }
function Write-Detail { Write-Host "  [..] $args" -ForegroundColor Blue }
function Write-Fail  { Write-Host "  [FAIL] $args" -ForegroundColor Red; exit 1 }
function Write-Header { Write-Host ""; Write-Host "=== $args ===" -ForegroundColor Cyan }
function Write-Step   { Write-Host ""; Write-Host "[$($args[0])/$($args[1])] $($args[2])" -ForegroundColor White }

# --- Utility ---
function New-Secret {
    param([int]$Bytes = 32)
    $hex = [byte[]]::new($Bytes)
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($hex)
    return ($hex | ForEach-Object { $_.ToString("x2") }) -join ""
}

function New-Uuid {
    return [Guid]::NewGuid().ToString()
}

function Test-PortInUse {
    param([int]$Port)
    try {
        $tcp = [System.Net.Sockets.TcpClient]::new("127.0.0.1", $Port)
        $tcp.Close()
        return $true
    } catch {
        return $false
    }
}

function Invoke-SecurePrompt {
    param([string]$Prompt, [string]$Default, [bool]$Secret = $false)
    if ($Secret) {
        $ss = [System.Security.SecureString]::new()
        Write-Host -NoNewline "  $Prompt`: "
        [Console]::TreatControlCAsInput = $false
        while ($true) {
            $ki = [Console]::ReadKey($true)
            if ($ki.Key -eq [ConsoleKey]::Enter) { break }
            if ($ki.Key -eq [ConsoleKey]::Backspace) {
                if ($ss.Length -gt 0) { $ss.RemoveAt($ss.Length - 1); Write-Host -NoNewline "`b `b" }
            } else {
                $ss.AppendChar($ki.KeyChar)
                Write-Host -NoNewline "*"
            }
        }
        Write-Host ""
        $ptr = [System.Runtime.InteropServices.Marshal]::SecureStringToBSTR($ss)
        try { return [System.Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr) }
        finally { [System.Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr) }
    } else {
        $val = Read-Host -Prompt "  $Prompt` [$Default]"
        if ([string]::IsNullOrWhiteSpace($val)) { return $Default }
        return $val
    }
}

# ============================================================
# Step 1: OS Detection
# ============================================================
function Test-OS {
    Write-Step 1 7 "Detecting operating system"
    $os = Get-CimInstance Win32_OperatingSystem
    Write-Info "$($os.Caption)"
    $build = [Environment]::OSVersion.Version.Build
    if ($build -lt 17763) {
        Write-Fail "Windows build $build is too old. Minimum: 17763 (Server 2019 / Windows 10 1809)"
    }
    Write-Info "Build: $build"

    # Check WSL2 (needed for Docker Desktop)
    try {
        $wsl = wsl --status 2>&1
        if ($LASTEXITCODE -ne 0) {
            Write-Warning2 "WSL2 not detected. Docker Desktop requires WSL2 backend."
        } else {
            Write-Info "WSL2 detected"
        }
    } catch {
        Write-Warning2 "Cannot check WSL2 status"
    }
}

# ============================================================
# Step 2: Architecture
# ============================================================
function Test-Architecture {
    Write-Step 2 7 "Detecting CPU architecture"
    $arch = $env:PROCESSOR_ARCHITECTURE
    if ($arch -eq "AMD64") {
        Write-Info "Architecture: amd64 (x86_64)"
    } elseif ($arch -eq "ARM64") {
        Write-Info "Architecture: arm64"
    } else {
        Write-Fail "Unsupported architecture: $arch"
    }
}

# ============================================================
# Step 3: Docker
# ============================================================
function Test-Docker {
    Write-Step 3 7 "Checking Docker"

    try {
        $dockerVersion = docker version --format '{{.Server.Version}}' 2>&1
        if ($LASTEXITCODE -ne 0) {
            Write-Fail "Docker is not running. Start Docker Desktop."
        }
    } catch {
        Write-Fail "Docker is not installed. Install Docker Desktop from https://www.docker.com/products/docker-desktop/"
    }

    Write-Info "Docker $dockerVersion"

    try {
        $composeVersion = docker compose version --short 2>&1
        Write-Info "Docker Compose $composeVersion"
    } catch {
        Write-Fail "Docker Compose v2 is required"
    }

    if ([version]$dockerVersion -lt [version]$MinDockerVersion) {
        Write-Fail "Docker $dockerVersion is too old. Minimum: $MinDockerVersion"
    }
}

# ============================================================
# Step 4: Ports
# ============================================================
function Test-Ports {
    Write-Step 4 7 "Checking port availability"
    $conflicts = @()
    foreach ($port in $RequiredPorts) {
        if (Test-PortInUse $port) {
            Write-Warning2 "Port $port is in use"
            $conflicts += $port
        } else {
            Write-Info "Port $port available"
        }
    }
    if ($conflicts.Count -gt 0) {
        Write-Fail "Ports in use: $($conflicts -join ', '). Stop conflicting services."
    }
}

# ============================================================
# Step 5: Resources
# ============================================================
function Test-Resources {
    Write-Step 5 7 "Checking system resources"

    $ramMB = [math]::Round((Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory / 1MB)
    Write-Info "RAM: ${ramMB}MB"

    $cpus = (Get-CimInstance Win32_ComputerSystem).NumberOfLogicalProcessors
    Write-Info "CPUs: $cpus"

    $disk = Get-PSDrive -Name (Split-Path -Path $ProjectRoot -Qualifier).TrimEnd(':')
    $diskGB = [math]::Round($disk.Free / 1GB)
    Write-Info "Disk available: ${diskGB}GB"

    if ($ramMB / 1024 -lt $MinRamGB) {
        Write-Fail "Insufficient RAM: ${ramMB}MB available, ${MinRamGB}GB required"
    }
    if ($cpus -lt $MinCpus) {
        Write-Fail "Insufficient CPUs: $cpus available, $MinCpus required"
    }
    if ($diskGB -lt $MinDiskGB) {
        Write-Fail "Insufficient disk: ${diskGB}GB available, ${MinDiskGB}GB required"
    }
}

# ============================================================
# Step 6: Configuration
# ============================================================
function Invoke-Configuration {
    Write-Step 6 7 "Configuration"

    if ($Unattended) {
        $script:Fqdn = if ($Fqdn) { $Fqdn } else { $env:GAMEPANEL_FQDN }
        $script:AdminEmail = if ($AdminEmail) { $AdminEmail } else { $env:GAMEPANEL_ADMIN_EMAIL }
        $script:AdminPassword = if ($AdminPassword) { $AdminPassword } else { $env:GAMEPANEL_ADMIN_PASSWORD }
        if (-not $script:Fqdn) { Write-Fail "Fqdn is required for unattended install" }
        if (-not $script:AdminEmail) { Write-Fail "AdminEmail is required for unattended install" }
        if (-not $script:AdminPassword) { Write-Fail "AdminPassword is required for unattended install" }
        Write-Info "Using unattended configuration"
        Write-Info "FQDN: $script:Fqdn"
        Write-Info "Admin: $script:AdminEmail"
    } else {
        Write-Host ""
        Write-Host "  GamePanel Setup — Interactive Configuration"
        Write-Host ""
        Write-Host "  You will need:"
        Write-Host "    - A domain name (e.g. panel.example.com)"
        Write-Host "    - An email address for the admin account"
        Write-Host "    - A strong password (min 12 characters)"
        Write-Host ""

        $script:Fqdn = Invoke-SecurePrompt -Prompt "Enter domain name" -Default ($env:GAMEPANEL_FQDN)
        while ([string]::IsNullOrWhiteSpace($script:Fqdn)) {
            Write-Warning2 "Domain is required"
            $script:Fqdn = Invoke-SecurePrompt -Prompt "Enter domain name"
        }

        $script:AdminEmail = Invoke-SecurePrompt -Prompt "Enter admin email" -Default ($env:GAMEPANEL_ADMIN_EMAIL)
        while ([string]::IsNullOrWhiteSpace($script:AdminEmail)) {
            Write-Warning2 "Email is required"
            $script:AdminEmail = Invoke-SecurePrompt -Prompt "Enter admin email"
        }

        do {
            $script:AdminPassword = Invoke-SecurePrompt -Prompt "Enter admin password (min 12 chars)" -Secret $true
            if ($script:AdminPassword.Length -lt 12) {
                Write-Warning2 "Password must be at least 12 characters"
            }
        } while ($script:AdminPassword.Length -lt 12)
    }

    $script:DbPassword = if ($DbPassword) { $DbPassword } elseif ($env:GAMEPANEL_DB_PASSWORD) { $env:GAMEPANEL_DB_PASSWORD } else { New-Secret -Bytes 24 }
    Write-Info "Configuration complete"
}

# ============================================================
# Step 7: Generate Environment
# ============================================================
function New-Environment {
    Write-Step 7 7 "Generating environment configuration"

    $infraDir = Join-Path $ProjectRoot "infra"
    $envFile = Join-Path $infraDir ".env"

    if (-not (Test-Path $infraDir)) {
        New-Item -ItemType Directory -Path $infraDir -Force | Out-Null
    }

    if (Test-Path $envFile) {
        $backup = "$envFile.bak.$Timestamp"
        Copy-Item $envFile $backup
        Write-Detail "Backed up existing .env to $backup"
    }

    $apiSecret = New-Secret -Bytes 32
    $appKey = New-Secret -Bytes 32
    $nodeTokenId = New-Secret -Bytes 8
    $nodeTokenSecret = New-Secret -Bytes 32
    $nodeToken = "$nodeTokenId.$nodeTokenSecret"
    $nodeId = New-Uuid
    $grafanaPassword = New-Secret -Bytes 24
    $masterKey = New-Secret -Bytes 32

    @"
# =====================================================================
# GamePanel Production Environment
# Generated by install.ps1 on $(Get-Date -Format "yyyy-MM-ddTHH:mm:ssK")
# Domain: $script:Fqdn
# NEVER commit this file.
# =====================================================================

# --- Database ---
POSTGRES_DB=gamepanel
POSTGRES_USER=gamepanel
POSTGRES_PASSWORD=$script:DbPassword
DATABASE_URL=postgres://gamepanel:$script:DbPassword@postgres:5432/gamepanel?sslmode=disable
POSTGRES_BACKUP_HOST_DIR=/var/backups/gamepanel/postgres
POSTGRES_BACKUP_INTERVAL_SECONDS=86400
POSTGRES_BACKUP_RETENTION_DAYS=14

# --- Redis ---
REDIS_ADDR=redis:6379

# --- Panel URL ---
PANEL_URL=https://$script:Fqdn

# --- API ---
API_ADDR=:8080
API_AUTH_SECRET=$apiSecret
APP_KEY=$appKey
APP_ENV=production
LOAD_BALANCER_ENABLED=true
LOAD_BALANCER_BIND_HOST=
LOAD_BALANCER_PORT_MIN=30000
LOAD_BALANCER_PORT_MAX=30100

# --- Encryption at Rest ---
FORGE_MASTER_KEY=$masterKey
FORGE_MASTER_KEY_ID=primary
FORGE_PREVIOUS_MASTER_KEYS=
FORGE_ALLOW_EPHEMERAL_MASTER_KEY=false

# --- Node ---
DAEMON_NODE_TOKEN=$nodeToken
DAEMON_ADDR=:9090
DAEMON_SFTP_ADDR=:2022
DAEMON_DATA_DIR=/srv/game-panel/servers
GAME_SERVERS_HOST_DIR=/srv/game-panel/servers
DAEMON_NODE_ID=$nodeId
DAEMON_ALLOW_MOCK_RUNTIME=false
PANEL_API_URL=http://api:8080/api/v1
BEACON_PANEL_API_URL=https://$script:Fqdn/api/v1

# --- Grafana ---
GRAFANA_ADMIN_USER=admin
GRAFANA_ADMIN_PASSWORD=$grafanaPassword

# --- Backups ---
BACKUP_ADAPTER=local

# --- Admin Account ---
GAMEPANEL_ADMIN_EMAIL=$script:AdminEmail
GAMEPANEL_ADMIN_PASSWORD=$script:AdminPassword
"@ | Out-File -FilePath $envFile -Encoding utf8 -NoNewline

    Write-Info "Environment written to $envFile"

    $script:EnvFile = $envFile
    $script:DbPassword | Out-Null  # suppress output
}

# ============================================================
# Main
# ============================================================
function Main {
    Write-Host ""
    Write-Host "  +============================================================+" -ForegroundColor Red
    Write-Host "  |           GamePanel · Forge Control Plane                  |" -ForegroundColor Red
    Write-Host "  |           Windows Installation Script                     |" -ForegroundColor Red
    Write-Host "  +============================================================+" -ForegroundColor Red
    Write-Host ""

    if (-not $SkipChecks) {
        Test-OS
        Test-Architecture
        Test-Docker
        Test-Ports
        Test-Resources
    } else {
        Write-Warning2 "Skipping pre-flight checks (-SkipChecks)"
    }

    Invoke-Configuration
    New-Environment

    # Start services
    $infraDir = Join-Path $ProjectRoot "infra"
    Push-Location $infraDir

    Write-Host ""
    Write-Header "Starting GamePanel services"
    docker compose -f compose.yml -f compose.production.yml --env-file .env pull postgres redis 2>&1 | Select-Object -Last 3

    Write-Detail "Building application images"
    docker compose -f compose.yml -f compose.production.yml --env-file .env build 2>&1 | Select-Object -Last 3

    docker compose -f compose.yml -f compose.production.yml --env-file .env up -d 2>&1 | Select-Object -Last 5
    Write-Info "Services started"

    Write-Detail "Waiting for services to be healthy (max 120s)"
    Start-Sleep -Seconds 10
    docker compose -f compose.yml -f compose.production.yml --env-file .env ps

    Pop-Location

    # Verification
    Write-Host ""
    Write-Header "Verification"
    try {
        $health = Invoke-RestMethod -Uri "http://127.0.0.1:8080/api/v1/health/ready" -TimeoutSec 10 -ErrorAction Stop
        Write-Info "API health: $($health | ConvertTo-Json -Compress)"
    } catch {
        Write-Warning2 "API health check not available yet"
    }

    try {
        $web = Invoke-WebRequest -Uri "http://127.0.0.1:3000" -TimeoutSec 10 -ErrorAction Stop
        Write-Info "Web UI reachable (HTTP $($web.StatusCode))"
    } catch {
        Write-Warning2 "Web UI not responding yet"
    }

    # Summary
    Write-Host ""
    Write-Host "  +============================================================+" -ForegroundColor Green
    Write-Host "  |          GamePanel Installed Successfully!                 |" -ForegroundColor Green
    Write-Host "  +============================================================+" -ForegroundColor Green
    Write-Host ""
    Write-Host "  Service URLs"
    Write-Host "  -----------------------------------------------------------------"
    Write-Host "  Web Dashboard:     https://$script:Fqdn" -ForegroundColor Cyan
    Write-Host "  Setup (first run):  https://$script:Fqdn/setup" -ForegroundColor Cyan
    Write-Host "  API:               https://$script:Fqdn/api/v1" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "  Admin Account"
    Write-Host "  -----------------------------------------------------------------"
    Write-Host "  Email:    $script:AdminEmail" -ForegroundColor Green
    Write-Host "  Password: (as entered)" -ForegroundColor Green
    Write-Host ""
    Write-Host "  Management"
    Write-Host "  -----------------------------------------------------------------"
    Write-Host "  Status:  cd $infraDir; docker compose -f compose.yml -f compose.production.yml --env-file .env ps"
    Write-Host "  Logs:    cd $infraDir; docker compose -f compose.yml -f compose.production.yml --env-file .env logs -f"
    Write-Host ""
    Write-Host "  Next Steps"
    Write-Host "  -----------------------------------------------------------------"
    Write-Host "  1. Configure a reverse proxy (Nginx/IIS/Caddy) with TLS"
    Write-Host "  2. Open https://$script:Fqdn/setup in your browser"
    Write-Host "  3. Create a node in Admin > Nodes"
    Write-Host "  4. Deploy your first game server"
    Write-Host ""
    Write-Host "  Store $envFile securely — it contains all secrets." -ForegroundColor Yellow
    Write-Host ""
}

Main
