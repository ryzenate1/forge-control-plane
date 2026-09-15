param(
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]] $Arguments
)

$exe = Join-Path $PSScriptRoot '..\..\.tools\rtk.exe'
$exe = [System.IO.Path]::GetFullPath($exe)

if (-not (Test-Path -LiteralPath $exe)) {
  throw "RTK binary missing: $exe. Install from https://github.com/rtk-ai/rtk/releases"
}

$dataDir = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\.dev-data\rtk'))
New-Item -ItemType Directory -Path $dataDir -Force | Out-Null

# Keep SQLite journals and recoverable output inside the writable workspace.
# Honor explicit caller configuration, and never change HOME or CODEX_HOME.
$defaults = @{
  RTK_DB_PATH = Join-Path $dataDir 'history.db'
  RTK_RECALL_DB = Join-Path $dataDir 'recall.db'
  CLAUDE_CONFIG_DIR = Join-Path $dataDir 'claude'
}
$changed = @()
try {
  foreach ($name in $defaults.Keys) {
    if (-not [Environment]::GetEnvironmentVariable($name, 'Process')) {
      [Environment]::SetEnvironmentVariable($name, $defaults[$name], 'Process')
      $changed += $name
    }
  }
  & $exe @Arguments
  $result = $LASTEXITCODE
} finally {
  foreach ($name in $changed) {
    [Environment]::SetEnvironmentVariable($name, $null, 'Process')
  }
}
exit $result
