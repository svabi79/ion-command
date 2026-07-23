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

# Collector: attach to a running one first (it may sit on a fallback port from
# an earlier launch), so a second launch never fights over the port.
$activePort = 0
foreach ($port in $candidatePorts) {
    if (Test-CollectorHealth $port) { $activePort = $port; break }
}

if (-not $activePort) {
    $logDir = Join-Path $env:LOCALAPPDATA 'IonCommand\logs'
    New-Item -ItemType Directory -Path $logDir -Force | Out-Null
    foreach ($port in $candidatePorts) {
        if (-not (Test-PortBindable $port)) {
            Write-Host "Port $port is unavailable (in use or reserved by Windows) - trying the next one."
            continue
        }
        Write-Host "Starting data collector on 127.0.0.1:$port..."
        $process = Start-Process -FilePath $collector `
            -ArgumentList '-config', "`"$config`"", '-listen', "127.0.0.1:$port" `
            -WorkingDirectory (Join-Path $root 'collector') -WindowStyle Hidden -PassThru `
            -RedirectStandardOutput (Join-Path $logDir 'collector-out.log') `
            -RedirectStandardError  (Join-Path $logDir 'collector-err.log')
        $deadline = (Get-Date).AddSeconds(20)
        do {
            Start-Sleep -Milliseconds 500
            if (Test-CollectorHealth $port) { $activePort = $port; break }
        } while (-not $process.HasExited -and (Get-Date) -lt $deadline)
        if ($activePort) { break }
        if ($process.HasExited) {
            Write-Host "Collector exited immediately (code $($process.ExitCode)) - see $logDir\collector-err.log."
        }
    }
}

if (-not $activePort) {
    # The console is hidden, so a Write-Warning would vanish - tell the user
    # in a way they can actually see, then start the client anyway.
    Add-Type -AssemblyName System.Windows.Forms
    [void][System.Windows.Forms.MessageBox]::Show(
        "The data collector could not be started - the globe will stay empty.`n`nSee $env:LOCALAPPDATA\IonCommand\logs\collector-err.log and docs\TROUBLESHOOTING.md (`"The globe is empty`").",
        'ION COMMAND', 'OK', 'Warning')
}

$clientArgs = @()
if ($Fullscreen) { $clientArgs += '-fullscreen' } else { $clientArgs += '-windowed' }
if ($ResX -gt 0) { $clientArgs += "-ResX=$ResX" }
if ($ResY -gt 0) { $clientArgs += "-ResY=$ResY" }
if ($ShowDeck)   { $clientArgs += '-IonShowDeck' }
if ($activePort) { $clientArgs += "-IonCollectorUrl=ws://127.0.0.1:$activePort/ws/live" }

Write-Host 'Starting ION COMMAND...'
Start-Process -FilePath $client -ArgumentList $clientArgs
