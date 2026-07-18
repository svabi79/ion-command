param(
    [string]$UnrealRoot = "",
    [double]$CaptureAfterSeconds = 50,
    [int]$ResX = 5120,
    [int]$ResY = 1440
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

& (Join-Path $PSScriptRoot 'build.ps1')
if ($LASTEXITCODE -ne 0) { throw 'collector build failed' }

$collector = Start-Process -FilePath (Join-Path $repositoryRoot 'collector\bin\ion-collector.exe') `
    -ArgumentList @('-config', (Join-Path $repositoryRoot 'collector\configs\development.json')) `
    -WorkingDirectory (Join-Path $repositoryRoot 'collector') -PassThru -WindowStyle Hidden
try {
    $deadline = (Get-Date).AddSeconds(15)
    do {
        Start-Sleep -Milliseconds 500
        try { $health = Invoke-RestMethod -Uri 'http://127.0.0.1:7810/api/health' -TimeoutSec 2 } catch { $health = $null }
    } while (-not $health -and (Get-Date) -lt $deadline)
    if (-not $health -or $health.status -ne 'ok') { throw 'collector did not become healthy' }
    Write-Output 'collector healthy, starting client capture...'

    & $editorCommand $project -game -windowed "-ResX=$ResX" "-ResY=$ResY" -NoSplash `
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
