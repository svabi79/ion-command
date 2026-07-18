param(
    [switch]$Unreal,
    [string]$UnrealRoot = "",
    [ValidateRange(1, 64)]
    [int]$MaxParallelActions = 8
)

$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path -Parent $PSScriptRoot
$collectorRoot = Join-Path $repositoryRoot 'collector'
$binDirectory = Join-Path $collectorRoot 'bin'
New-Item -ItemType Directory -Path $binDirectory -Force | Out-Null
Push-Location $collectorRoot
try {
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw "Go tests failed with exit code $LASTEXITCODE" }
    go build -trimpath -o (Join-Path $binDirectory 'ion-collector.exe') ./cmd/ion-collector
    if ($LASTEXITCODE -ne 0) { throw "Collector build failed with exit code $LASTEXITCODE" }
} finally { Pop-Location }
Write-Output "Collector built: $binDirectory\ion-collector.exe"

if ($Unreal) {
    $engineRoot = & (Join-Path $PSScriptRoot 'find-unreal.ps1') -RequestedRoot $UnrealRoot
    $buildScript = Join-Path $engineRoot 'Engine\Build\BatchFiles\Build.bat'
    $project = Join-Path $repositoryRoot 'unreal\IonCommand.uproject'
    & $buildScript IonCommandEditor Win64 Development $project -WaitMutex -FromMsBuild "-MaxParallelActions=$MaxParallelActions"
    if ($LASTEXITCODE -ne 0) { throw "Unreal build failed with exit code $LASTEXITCODE" }
}
