param(
    [string]$UnrealRoot = "",
    [double]$CaptureAfterSeconds = 50,
    [int]$ResX = 5120,
    [int]$ResY = 1440,
    # Alternative port so verification never collides with an operator's live
    # collector/client pair on the default 7810.
    [int]$Port = 7810
)

# End-to-end First Light proof: build collector, stream live mock traffic into
# the game client, capture a native-resolution screenshot, and fail on errors.
$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path -Parent $PSScriptRoot
$engineRoot = & (Join-Path $PSScriptRoot 'find-unreal.ps1') -RequestedRoot $UnrealRoot
$editorCommand = Join-Path $engineRoot 'Engine\Binaries\Win64\UnrealEditor-Cmd.exe'
$project = Join-Path $repositoryRoot 'unreal\IonCommand.uproject'
$screenshot = Join-Path $repositoryRoot 'unreal\Saved\Screenshots\Reference\ION_COMMAND_Live.png'
$gameLog = Join-Path $repositoryRoot 'unreal\Saved\Logs\Game-live-capture.log'

# A listener already on the port would answer our health check and silently
# hijack the verification (the collector we start below cannot bind).
$existing = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
if ($existing) { throw "port $Port is already in use (PID $($existing[0].OwningProcess)) - stop it or pass -Port" }

& (Join-Path $PSScriptRoot 'build.ps1')
if ($LASTEXITCODE -ne 0) { throw 'collector build failed' }

$configPath = Join-Path $repositoryRoot 'collector\configs\development.json'
if ($Port -ne 7810) {
    $config = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
    $config.server.listenAddress = "127.0.0.1:$Port"
    $configPath = Join-Path $env:TEMP "ion-first-light-$Port.json"
    # WriteAllText with an explicit BOM-less encoding: PowerShell 5.1's
    # -Encoding UTF8 emits a BOM, which Go's JSON decoder rejects.
    [System.IO.File]::WriteAllText($configPath, ($config | ConvertTo-Json -Depth 10), (New-Object System.Text.UTF8Encoding($false)))
}

$collector = Start-Process -FilePath (Join-Path $repositoryRoot 'collector\bin\ion-collector.exe') `
    -ArgumentList @('-config', ('"{0}"' -f $configPath)) `
    -WorkingDirectory (Join-Path $repositoryRoot 'collector') -PassThru -WindowStyle Hidden
try {
    $deadline = (Get-Date).AddSeconds(15)
    do {
        Start-Sleep -Milliseconds 500
        try { $health = Invoke-RestMethod -Uri "http://127.0.0.1:$Port/api/health" -TimeoutSec 2 } catch { $health = $null }
    } while (-not $health -and (Get-Date) -lt $deadline)
    if (-not $health -or $health.status -ne 'ok') { throw 'collector did not become healthy' }
    Write-Output 'collector healthy, starting client capture...'

    & $editorCommand $project -game -windowed "-ResX=$ResX" "-ResY=$ResY" -NoSplash `
        "-IonCollectorUrl=ws://127.0.0.1:$Port/ws/live" `
        "-IonScreenshotAfter=$CaptureAfterSeconds" "-IonScreenshotFile=$screenshot" `
        -IonExitAfterScreenshot "-AbsLog=$gameLog"
    if ($LASTEXITCODE -ne 0) { throw "game client exited with $LASTEXITCODE" }
} finally {
    if (-not $collector.HasExited) { Stop-Process -Id $collector.Id -Force }
}

if (-not (Test-Path -LiteralPath $screenshot)) { throw 'live screenshot was not produced' }
$errors = Select-String -LiteralPath $gameLog -Pattern ': Error:'
if ($errors) { $errors | Select-Object -First 10 | ForEach-Object { $_.Line }; throw 'game log contains errors' }

Write-Output "First Light capture complete: $screenshot"
Get-Item -LiteralPath $screenshot | Select-Object Length, LastWriteTime
