# ION COMMAND launcher (installed copy).
# Starts the data collector if it is not already answering, then the client.
# Everything is resolved relative to this script, so the install location and
# drive do not matter.
param(
    [int]$ResX = 0,
    [int]$ResY = 0,
    [switch]$Fullscreen,
    [switch]$ShowDeck
)

$ErrorActionPreference = 'Stop'
$root      = Split-Path -Parent $MyInvocation.MyCommand.Path
$collector = Join-Path $root 'collector\ion-collector.exe'
$config    = Join-Path $root 'collector\configs\live.json'
$client    = Join-Path $root 'client\IonCommand.exe'

if (-not (Test-Path $client))    { throw "Client not found: $client" }
if (-not (Test-Path $collector)) { throw "Collector not found: $collector" }

# Collector: only start it when nothing is already serving on the port, so a
# second launch attaches to the running one instead of fighting over it.
$up = $false
try { $up = (Invoke-RestMethod -Uri 'http://127.0.0.1:7810/api/health' -TimeoutSec 3).status -eq 'ok' } catch { }
if (-not $up) {
    Write-Host 'Starting data collector...'
    $logDir = Join-Path $env:LOCALAPPDATA 'IonCommand\logs'
    New-Item -ItemType Directory -Path $logDir -Force | Out-Null
    Start-Process -FilePath $collector -ArgumentList '-config', "`"$config`"" `
        -WorkingDirectory (Join-Path $root 'collector') -WindowStyle Hidden `
        -RedirectStandardOutput (Join-Path $logDir 'collector-out.log') `
        -RedirectStandardError  (Join-Path $logDir 'collector-err.log')
    $deadline = (Get-Date).AddSeconds(20)
    do {
        Start-Sleep -Milliseconds 500
        try { $up = (Invoke-RestMethod -Uri 'http://127.0.0.1:7810/api/health' -TimeoutSec 2).status -eq 'ok' } catch { }
    } while (-not $up -and (Get-Date) -lt $deadline)
}
if (-not $up) {
    Write-Warning 'Collector did not come up - the globe will start empty. See %LOCALAPPDATA%\IonCommand\logs.'
}

$clientArgs = @()
if ($Fullscreen) { $clientArgs += '-fullscreen' } else { $clientArgs += '-windowed' }
if ($ResX -gt 0) { $clientArgs += "-ResX=$ResX" }
if ($ResY -gt 0) { $clientArgs += "-ResY=$ResY" }
if ($ShowDeck)   { $clientArgs += '-IonShowDeck' }

Write-Host 'Starting ION COMMAND...'
Start-Process -FilePath $client -ArgumentList $clientArgs
