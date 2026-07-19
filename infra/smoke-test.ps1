param(
    [string]$ApiBaseUrl = "http://127.0.0.1:8080/api/v1",
    [string]$DaemonBaseUrl = "http://127.0.0.1:9090",
    [string]$Email = "admin@example.com",
    [string]$Password = $env:GAMEPANEL_ADMIN_PASSWORD
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($Password)) {
    throw "Provide -Password or set GAMEPANEL_ADMIN_PASSWORD; no default admin password is permitted."
}

function Invoke-Json {
    param(
        [string]$Method,
        [string]$Uri,
        [hashtable]$Headers = @{},
        [object]$Body = $null
    )
    $params = @{
        Method = $Method
        Uri = $Uri
        Headers = $Headers
        UseBasicParsing = $true
    }
    if ($null -ne $Body) {
        $params.ContentType = "application/json"
        $params.Body = ($Body | ConvertTo-Json -Depth 8)
    }
    $response = Invoke-WebRequest @params
    if ($response.Content) {
        return $response.Content | ConvertFrom-Json
    }
    return $null
}

Write-Host "Checking API health"
$apiHealth = Invoke-Json -Method GET -Uri "$ApiBaseUrl/health"
if (-not $apiHealth.ok) { throw "API health check failed" }

Write-Host "Checking daemon health"
$daemonHealth = Invoke-Json -Method GET -Uri "$DaemonBaseUrl/health"
if (-not $daemonHealth.ok) { throw "Daemon health check failed" }

Write-Host "Checking metrics"
$apiMetrics = Invoke-WebRequest -UseBasicParsing "$ApiBaseUrl/metrics"
if ($apiMetrics.Content -notmatch "game_panel_api_uptime_seconds") { throw "API metrics missing expected series" }
$daemonMetrics = Invoke-WebRequest -UseBasicParsing "$DaemonBaseUrl/metrics"
if ($daemonMetrics.Content -notmatch "game_panel_daemon_uptime_seconds") { throw "Daemon metrics missing expected series" }

Write-Host "Logging in"
$login = Invoke-Json -Method POST -Uri "$ApiBaseUrl/auth/login" -Body @{ email = $Email; password = $Password }
if (-not $login.token) { throw "Login did not return a token" }
$auth = @{ Authorization = "Bearer $($login.token)" }

Write-Host "Checking seeded resources"
$nodes = Invoke-Json -Method GET -Uri "$ApiBaseUrl/nodes" -Headers $auth
$templates = Invoke-Json -Method GET -Uri "$ApiBaseUrl/templates" -Headers $auth
$servers = Invoke-Json -Method GET -Uri "$ApiBaseUrl/servers" -Headers $auth
$allocations = Invoke-Json -Method GET -Uri "$ApiBaseUrl/allocations" -Headers $auth
if ($nodes.Count -lt 1) { throw "Expected at least one node" }
if ($templates.Count -lt 1) { throw "Expected at least one template" }
if ($servers.Count -lt 1) { throw "Expected at least one server" }
if ($allocations.Count -lt 1) { throw "Expected at least one allocation" }

Write-Host "Smoke test passed"
