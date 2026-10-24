param(
    [switch]$Unreal,
    [string]$UnrealRoot = "",
    [ValidateRange(1, 64)]
    [int]$MaxParallelActions = 8,
    [ValidateSet('Development','Shipping')]
    [string]$Config = 'Development'
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
    $commonArguments = @($project, '-WaitMutex', '-FromMsBuild', "-MaxParallelActions=$MaxParallelActions")
    # Game target first: it is what ships and has a different module set than the editor.
    & $buildScript IonCommand Win64 $Config @commonArguments
    if ($LASTEXITCODE -ne 0) { throw "Unreal build of target IonCommand (Win64 $Config) failed with exit code $LASTEXITCODE" }
    # An editor target has no Shipping configuration.
    & $buildScript IonCommandEditor Win64 Development @commonArguments
    if ($LASTEXITCODE -ne 0) { throw "Unreal build of target IonCommandEditor (Win64 Development) failed with exit code $LASTEXITCODE" }
}
