param(
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]] $Arguments
)

$python = Join-Path $PSScriptRoot '..\..\.venv_graphify\Scripts\python.exe'
$python = [System.IO.Path]::GetFullPath($python)

if (-not (Test-Path -LiteralPath $python)) {
  throw "Graphify Python environment missing: $python"
}

& $python -m graphify @Arguments
exit $LASTEXITCODE
