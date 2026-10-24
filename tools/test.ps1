param([switch]$Race)

$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path -Parent $PSScriptRoot
Push-Location (Join-Path $repositoryRoot 'collector')
try {
    if ($Race) { go test -race ./... } else { go test ./... }
    if ($LASTEXITCODE -ne 0) { throw "Go tests failed with exit code $LASTEXITCODE" }
} finally { Pop-Location }
python (Join-Path $PSScriptRoot 'validate_repository.py')
if ($LASTEXITCODE -ne 0) { throw "Repository validation failed with exit code $LASTEXITCODE" }
& (Join-Path $PSScriptRoot 'check-powershell-syntax.ps1')
