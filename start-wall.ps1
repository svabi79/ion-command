# ION COMMAND — Wall-Starter
# Startet den Daten-Collector (falls noch nicht laufend) und den Wall-Client.
# Aufruf:  powershell -ExecutionPolicy Bypass -File "D:\Code\ION COMMAND\start-wall.ps1"
# Optionen:
#   -ResX / -ResY   Wandaufloesung (Default 5120x1440)
#   -ShowDeck       diegetische Deck-Panels wieder einblenden
param(
    [int]$ResX = 5120,
    [int]$ResY = 1440,
    [switch]$ShowDeck
)

$ErrorActionPreference = 'Stop'
$repo      = 'D:\Code\ION COMMAND'
$collector = Join-Path $repo 'collector\bin\ion-collector.exe'
$client    = Join-Path $repo 'dist\windows\IonCommand.exe'
$configDir = Join-Path $repo 'dist\windows\IonCommand\Saved\Config\Windows'

# Eigene Station (wird vom Packaging geloescht, daher hier sicherstellen).
New-Item -ItemType Directory -Path $configDir -Force | Out-Null
[System.IO.File]::WriteAllText(
    (Join-Path $configDir 'Game.ini'),
    "[IonCommand.Station]`nCallsign=HB9HSJ`nLocator=JN47om`n",
    (New-Object System.Text.UTF8Encoding($false)))

# Collector nur starten, wenn er nicht schon antwortet.
$collectorUp = $false
try { $collectorUp = (Invoke-RestMethod -Uri 'http://127.0.0.1:7810/api/health' -TimeoutSec 3).status -eq 'ok' } catch { }
if (-not $collectorUp) {
    Write-Host 'Starte Collector...'
    Start-Process -FilePath $collector -ArgumentList '-config', 'configs/live.json' `
        -WorkingDirectory (Join-Path $repo 'collector') -WindowStyle Hidden `
        -RedirectStandardOutput (Join-Path $repo 'collector\collector-stdout.log') `
        -RedirectStandardError  (Join-Path $repo 'collector\collector-stderr.log')
    $deadline = (Get-Date).AddSeconds(20)
    do {
        Start-Sleep -Milliseconds 500
        try { $collectorUp = (Invoke-RestMethod -Uri 'http://127.0.0.1:7810/api/health' -TimeoutSec 2).status -eq 'ok' } catch { }
    } while (-not $collectorUp -and (Get-Date) -lt $deadline)
}
Write-Host ("Collector: " + $(if ($collectorUp) { 'laeuft' } else { 'NICHT ERREICHBAR' }))

# Wall-Client starten.
$args = @('-windowed', "-ResX=$ResX", "-ResY=$ResY", '-IonCollectorUrl=ws://127.0.0.1:7810/ws/live')
if ($ShowDeck) { $args += '-IonShowDeck' }
Write-Host ("Starte Wall-Client (" + $ResX + "x" + $ResY + ")...")
Start-Process -FilePath $client -ArgumentList $args
Write-Host 'Fertig. Tasten: TAB HUD  /  O Overlays  /  V Pfade  /  H Heatmap  /  Mausrad Zoom  /  RMB Orbit'
