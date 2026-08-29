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
$cookLog = Join-Path $repositoryRoot 'unreal\Saved\Logs\package-cook.log'
# Capture the lines as objects rather than re-reading the file: Tee-Object on
# Windows PowerShell writes UTF-16, and a later Select-String over that file
# silently matches nothing - which is how this guard first shipped blind.
$cookOutput = & $runUat BuildCookRun "-project=$project" -noP4 -platform=Win64 "-clientconfig=$Config" -build -cook -stage -pak -archive "-archivedirectory=$archive" 2>&1 |
    Tee-Object -FilePath $cookLog
if ($LASTEXITCODE -ne 0) { throw "Packaging failed with exit code $LASTEXITCODE" }

# A material that fails to compile is only a WARNING to the cook, which then
# exits 0 and ships a package whose globe is drawn with the engine's default
# material. That shipped once already: a virtual texture bound to a plain
# TextureSampleParameter2D killed M_EarthSurface, and the only symptom was
# that the Earth rendered as an unrecognisable grey mass. Exit code alone is
# not evidence that the build is sound.
$materialFailures = $cookOutput |
    Select-String -SimpleMatch -Pattern @(
        'Failed to compile Material',
        "doesn't have a valid ShaderMap") |
    ForEach-Object { $_.ToString().Trim() } | Sort-Object -Unique
if ($materialFailures) {
    $detail = ($materialFailures | Select-Object -First 8) -join "`n  "
    throw "Packaging produced materials that will render as the default material:`n  $detail"
}
# Say so on success too. Collecting the cook output above means a clean run
# otherwise prints nothing at all, and "no output yet" is indistinguishable
# from "still running" for anything waiting on this.
Write-Output "Packaging complete: $archive (cook log: $cookLog)"

