param(
    [double]$DurationSeconds = 90,
    [int]$ResX = 5120,
    [int]$ResY = 1440,
    # Alternative port so benchmarks never hijack an operator's live pair.
    [int]$Port = 7810
)

# 10k-arc benchmark: streams 600 synthetic spots/s into the PACKAGED client
# (dist\windows) and derives the average frame rate from the frame counter in
# the client log at capture time.
$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path -Parent $PSScriptRoot
$clientExe = Join-Path $repositoryRoot 'dist\windows\IonCommand.exe'
if (-not (Test-Path -LiteralPath $clientExe)) { throw 'packaged client missing - run tools\package.ps1 first' }
$screenshot = Join-Path $repositoryRoot 'unreal\Saved\Screenshots\Reference\ION_COMMAND_Benchmark.png'
$benchLog = Join-Path $repositoryRoot 'unreal\Saved\Logs\Game-benchmark.log'

# A listener already on the port would answer the health check and the bench
# would silently measure against foreign live traffic.
$existing = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
if ($existing) { throw "port $Port is already in use (PID $($existing[0].OwningProcess)) - stop it or pass -Port" }

$configPath = Join-Path $repositoryRoot 'collector\configs\benchmark.json'
if ($Port -ne 7810) {
    $config = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
    $config.server.listenAddress = "127.0.0.1:$Port"
    $configPath = Join-Path $env:TEMP "ion-bench-$Port.json"
    [System.IO.File]::WriteAllText($configPath, ($config | ConvertTo-Json -Depth 10), (New-Object System.Text.UTF8Encoding($false)))
}

$collectorLog = Join-Path $repositoryRoot 'unreal\Saved\Logs\bench-collector.log'
$collector = Start-Process -FilePath (Join-Path $repositoryRoot 'collector\bin\ion-collector.exe') `
    -ArgumentList @('-config', ('"{0}"' -f $configPath)) `
    -WorkingDirectory (Join-Path $repositoryRoot 'collector') -PassThru -WindowStyle Hidden `
    -RedirectStandardOutput $collectorLog -RedirectStandardError "$collectorLog.err"
try {
    $deadline = (Get-Date).AddSeconds(15)
    do {
        Start-Sleep -Milliseconds 500
        try { $health = Invoke-RestMethod -Uri "http://127.0.0.1:$Port/api/health" -TimeoutSec 2 } catch { $health = $null }
    } while (-not $health -and (Get-Date) -lt $deadline)
    if (-not $health) { throw 'collector did not become healthy' }

    & $clientExe -windowed "-ResX=$ResX" "-ResY=$ResY" -NoSplash `
        "-IonCollectorUrl=ws://127.0.0.1:$Port/ws/live" `
        "-IonScreenshotAfter=$DurationSeconds" "-IonScreenshotFile=$screenshot" `
        -IonExitAfterScreenshot "-AbsLog=$benchLog" | Out-Null
} finally {
    if (-not $collector.HasExited) { Stop-Process -Id $collector.Id -Force }
}

$requested = Select-String -LiteralPath $benchLog -Pattern 'automation screenshot requested' | Select-Object -First 1
if (-not $requested) { throw 'benchmark capture did not complete' }
# Explicit GFrameCounter value; the log line's own frame column wraps at 1000.
if ($requested.Line -notmatch 'at frame (\d+)') { throw "could not parse frame counter: $($requested.Line)" }
$frames = [int]$Matches[1]
$fps = [math]::Round($frames / $DurationSeconds, 1)
Write-Output ("frames={0} duration={1}s avg_fps={2}" -f $frames, $DurationSeconds, $fps)
$errors = Select-String -LiteralPath $benchLog -Pattern ': Error:'
if ($errors) { $errors | Select-Object -First 5 | ForEach-Object { $_.Line }; throw 'benchmark log contains errors' }
Write-Output "benchmark screenshot: $screenshot"
