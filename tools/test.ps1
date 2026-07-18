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
$parseFailures = @()
Get-ChildItem -LiteralPath $PSScriptRoot -Filter *.ps1 -File | ForEach-Object {
    $tokens = $null
    $errors = $null
    [System.Management.Automation.Language.Parser]::ParseFile($_.FullName, [ref]$tokens, [ref]$errors) | Out-Null
    if ($errors.Count -gt 0) { $parseFailures += "$($_.Name): $($errors -join '; ')" }
}
if ($parseFailures.Count -gt 0) { throw "PowerShell syntax validation failed: $($parseFailures -join ' | ')" }
Write-Output 'PowerShell syntax validation passed.'
