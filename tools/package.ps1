param([string]$UnrealRoot = "")

$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path -Parent $PSScriptRoot
$project = Join-Path $repositoryRoot 'unreal\IonCommand.uproject'
$engineRoot = & (Join-Path $PSScriptRoot 'find-unreal.ps1') -RequestedRoot $UnrealRoot
$runUat = Join-Path $engineRoot 'Engine\Build\BatchFiles\RunUAT.bat'
$archive = Join-Path $repositoryRoot 'dist\windows'
& $runUat BuildCookRun -project=$project -noP4 -platform=Win64 -clientconfig=Shipping -build -cook -stage -pak -archive -archivedirectory=$archive
if ($LASTEXITCODE -ne 0) { throw "Packaging failed with exit code $LASTEXITCODE" }

