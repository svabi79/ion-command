# Parses every tools/*.ps1 and fails on any syntax error.
# Shared by tools/test.ps1 (local) and .github/workflows/ci.yml (hosted CI).
$ErrorActionPreference = 'Stop'
$parseFailures = @()
Get-ChildItem -LiteralPath $PSScriptRoot -Filter *.ps1 -File | ForEach-Object {
    $tokens = $null
    $errors = $null
    [System.Management.Automation.Language.Parser]::ParseFile($_.FullName, [ref]$tokens, [ref]$errors) | Out-Null
    if ($errors.Count -gt 0) { $parseFailures += "$($_.Name): $($errors -join '; ')" }
}
if ($parseFailures.Count -gt 0) { throw "PowerShell syntax validation failed: $($parseFailures -join ' | ')" }
Write-Output 'PowerShell syntax validation passed.'
