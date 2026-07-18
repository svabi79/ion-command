param([string]$Config = "")

$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path -Parent $PSScriptRoot
$collectorRoot = Join-Path $repositoryRoot 'collector'
if (-not $Config) { $Config = Join-Path $collectorRoot 'configs\development.json' }
$binary = Join-Path $collectorRoot 'bin\ion-collector.exe'
if (-not (Test-Path -LiteralPath $binary)) { & (Join-Path $PSScriptRoot 'build.ps1') }
& $binary -config $Config

