# ION COMMAND - developer wall launcher (source tree).
#
#   powershell -ExecutionPolicy Bypass -File .\start-wall.ps1
#
# Starts the collector (if it is not already answering) and the packaged client
# from dist\windows. Uses collector\configs\local.json when present so your
# personal setup (home QTH, RBN callsign) stays out of the repository, and
# falls back to the neutral live.json.
#
# Options:
#   -ResX / -ResY        wall resolution (default 5120x1440)
#   -Callsign/-Locator   write an own-station override for the packaged client
#                        (packaging deletes it); otherwise set them in the
#                        in-app SETTINGS panel, which persists them itself.
#   -ShowDeck            restore the diegetic console panels
param(
    [int]$ResX = 5120,
    [int]$ResY = 1440,
    [string]$Callsign = "",
    [string]$Locator = "",
    [switch]$ShowDeck
)

$ErrorActionPreference = 'Stop'
$repo      = $PSScriptRoot
$collector = Join-Path $repo 'collector\bin\ion-collector.exe'
$client    = Join-Path $repo 'dist\windows\IonCommand.exe'
$configDir = Join-Path $repo 'dist\windows\IonCommand\Saved\Config\Windows'

# Personal config first, neutral default second.
$configName = if (Test-Path (Join-Path $repo 'collector\configs\local.json')) { 'configs/local.json' } else { 'configs/live.json' }

# Only write the station override when explicitly asked; the settings panel
# persists callsign/locator on its own.
if ($Callsign -and $Locator) {
    New-Item -ItemType Directory -Path $configDir -Force | Out-Null
    [System.IO.File]::WriteAllText(
        (Join-Path $configDir 'Game.ini'),
        "[IonCommand.Station]`nCallsign=$Callsign`nLocator=$Locator`n",
        (New-Object System.Text.UTF8Encoding($false)))
    Write-Host "Own station set to $Callsign / $Locator"
}

$up = $false
try { $up = (Invoke-RestMethod -Uri 'http://127.0.0.1:7810/api/health' -TimeoutSec 3).status -eq 'ok' } catch { }
if (-not $up) {
    Write-Host "Starting collector ($configName)..."
    Start-Process -FilePath $collector -ArgumentList '-config', $configName `
        -WorkingDirectory (Join-Path $repo 'collector') -WindowStyle Hidden `
        -RedirectStandardOutput (Join-Path $repo 'collector\collector-stdout.log') `
        -RedirectStandardError  (Join-Path $repo 'collector\collector-stderr.log')
    $deadline = (Get-Date).AddSeconds(20)
    do {
        Start-Sleep -Milliseconds 500
        try { $up = (Invoke-RestMethod -Uri 'http://127.0.0.1:7810/api/health' -TimeoutSec 2).status -eq 'ok' } catch { }
    } while (-not $up -and (Get-Date) -lt $deadline)
}
Write-Host ("Collector: " + $(if ($up) { 'running' } else { 'NOT REACHABLE' }))

$clientArgs = @('-windowed', "-ResX=$ResX", "-ResY=$ResY", '-IonCollectorUrl=ws://127.0.0.1:7810/ws/live')
if ($ShowDeck) { $clientArgs += '-IonShowDeck' }
Write-Host "Starting client ($ResX x $ResY)..."
Start-Process -FilePath $client -ArgumentList $clientArgs
Write-Host 'Keys: TAB HUD / O overlays / V paths / M my RX-TX / H heatmap / wheel zoom / RMB orbit'
