param([string]$UnrealRoot = "", [string]$Config = "Development")

$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path -Parent $PSScriptRoot
$project = Join-Path $repositoryRoot 'unreal\IonCommand.uproject'
$engineRoot = & (Join-Path $PSScriptRoot 'find-unreal.ps1') -RequestedRoot $UnrealRoot
$runUat = Join-Path $engineRoot 'Engine\Build\BatchFiles\RunUAT.bat'
$archive = Join-Path $repositoryRoot 'dist\windows'
# BuildCookRun adds to the archive rather than replacing it, so binaries from
# earlier configurations pile up and get picked up downstream - the installer
# once shipped a 340 MB Development binary alongside the Shipping one. Start
# from an empty archive so it contains exactly this build.
if (Test-Path $archive) { Remove-Item $archive -Recurse -Force }
& $runUat BuildCookRun "-project=$project" -noP4 -platform=Win64 "-clientconfig=$Config" -build -cook -stage -pak -archive "-archivedirectory=$archive"
if ($LASTEXITCODE -ne 0) { throw "Packaging failed with exit code $LASTEXITCODE" }

