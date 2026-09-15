param(
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]] $Arguments
)

# CLI fallback for sessions where the installed MCP tools are not exposed.
# The wrapper does not install plugins or change global Codex settings.
$cacheRoot = Join-Path $env:USERPROFILE '.codex\plugins\cache\context-mode\context-mode'
$cli = Get-ChildItem -LiteralPath $cacheRoot -Directory -ErrorAction SilentlyContinue |
  Where-Object { Test-Path -LiteralPath (Join-Path $_.FullName 'cli.bundle.mjs') } |
  Sort-Object LastWriteTime -Descending |
  Select-Object -First 1
if (-not $cli) {
  throw "Installed Context Mode CLI not found under $cacheRoot"
}

$previousDir = $env:CONTEXT_MODE_DIR
$previousPlatform = $env:CONTEXT_MODE_PLATFORM
try {
  if (-not $env:CONTEXT_MODE_DIR) {
    $env:CONTEXT_MODE_DIR = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\.dev-data\context-mode'))
  }
  $env:CONTEXT_MODE_PLATFORM = 'codex'
  & (Join-Path $PSScriptRoot 'rtk.ps1') proxy node (Join-Path $cli.FullName 'cli.bundle.mjs') @Arguments
  $result = $LASTEXITCODE
} finally {
  $env:CONTEXT_MODE_DIR = $previousDir
  $env:CONTEXT_MODE_PLATFORM = $previousPlatform
}
exit $result
