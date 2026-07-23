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

# 7810 is the canonical port, but Windows reserves whole port ranges for
# Hyper-V/WSL NAT ("excluded port ranges"), and on some machines 7810 falls
# inside one - the collector then dies instantly with WSAEACCES. The fallbacks
# are far apart so at least one lands outside whatever block was reserved.
$candidatePorts = @(7810, 17810, 27810)

function Test-CollectorHealth([int]$Port) {
    try { return (Invoke-RestMethod -Uri "http://127.0.0.1:$Port/api/health" -TimeoutSec 2).status -eq 'ok' } catch { return $false }
}
function Test-PortBindable([int]$Port) {
    # A bind probe catches both "in use" and "reserved range" (WSAEACCES).
    $listener = New-Object System.Net.Sockets.TcpListener([System.Net.IPAddress]::Loopback, $Port)
    try { $listener.Start(); $listener.Stop(); return $true } catch { return $false }
}

# Attach to a running collector first (it may sit on a fallback port from an
# earlier launch), so a second launch never fights over the port.
$activePort = 0
foreach ($port in $candidatePorts) {
    if (Test-CollectorHealth $port) { $activePort = $port; break }
}
if (-not $activePort) {
    foreach ($port in $candidatePorts) {
        if (-not (Test-PortBindable $port)) {
            Write-Host "Port $port is unavailable (in use or reserved by Windows) - trying the next one."
            continue
        }
        Write-Host "Starting collector ($configName) on 127.0.0.1:$port..."
        $process = Start-Process -FilePath $collector -ArgumentList '-config', $configName, '-listen', "127.0.0.1:$port" `
            -WorkingDirectory (Join-Path $repo 'collector') -WindowStyle Hidden -PassThru `
            -RedirectStandardOutput (Join-Path $repo 'collector\collector-stdout.log') `
            -RedirectStandardError  (Join-Path $repo 'collector\collector-stderr.log')
        $deadline = (Get-Date).AddSeconds(20)
        do {
            Start-Sleep -Milliseconds 500
            if (Test-CollectorHealth $port) { $activePort = $port; break }
        } while (-not $process.HasExited -and (Get-Date) -lt $deadline)
        if ($activePort) { break }
        if ($process.HasExited) {
            Write-Host "Collector exited immediately (code $($process.ExitCode)) - see collector\collector-stderr.log."
        }
    }
}
Write-Host ("Collector: " + $(if ($activePort) { "running on port $activePort" } else { 'NOT REACHABLE' }))

$collectorPort = if ($activePort) { $activePort } else { 7810 }
$clientArgs = @('-windowed', "-ResX=$ResX", "-ResY=$ResY", "-IonCollectorUrl=ws://127.0.0.1:$collectorPort/ws/live")
if ($ShowDeck) { $clientArgs += '-IonShowDeck' }
Write-Host "Starting client ($ResX x $ResY)..."
Start-Process -FilePath $client -ArgumentList $clientArgs
Write-Host 'Keys: TAB HUD / O overlays / V paths / M my RX-TX / H heatmap / wheel zoom / RMB orbit'
