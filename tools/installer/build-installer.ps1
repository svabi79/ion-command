# Stages the release payload and compiles the Windows installer.
#
#   powershell -ExecutionPolicy Bypass -File tools\installer\build-installer.ps1 -Version 1.0.0
#
# Prerequisites:
#   - Shipping client archived to dist\release  (tools\package.ps1 -Config Shipping,
#     or RunUAT BuildCookRun ... -archivedirectory=dist\release)
#   - Collector built to collector\bin\ion-collector.exe (tools\build.ps1)
#   - Inno Setup 6 (winget install JRSoftware.InnoSetup)
param(
    [string]$Version = "1.0.0",
    [string]$ISCC = ""
)

$ErrorActionPreference = 'Stop'
$repo    = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$client  = Join-Path $repo 'dist\release\Windows'
$stage   = Join-Path $repo 'dist\installer-stage'
$outDir  = Join-Path $repo 'dist\installer'

if (-not (Test-Path (Join-Path $client 'IonCommand.exe'))) {
    throw "Shipping client missing at $client - run the Shipping package first."
}
$collectorExe = Join-Path $repo 'collector\bin\ion-collector.exe'
if (-not (Test-Path $collectorExe)) { throw "Collector missing at $collectorExe - run tools\build.ps1." }

if (-not $ISCC) {
    $candidates = @(
        "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe",
        "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
        "$env:ProgramFiles\Inno Setup 6\ISCC.exe"
    )
    $ISCC = $candidates | Where-Object { Test-Path $_ } | Select-Object -First 1
}
if (-not $ISCC) { throw 'Inno Setup (ISCC.exe) not found. Install it: winget install JRSoftware.InnoSetup' }

Write-Host "Staging payload -> $stage"
if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }
New-Item -ItemType Directory -Path $stage -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $stage 'collector\configs') -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $stage 'docs') -Force | Out-Null

# Client: everything the packaged build produced except debug symbols, which
# are ~220 MB and useless to end users.
robocopy $client (Join-Path $stage 'client') /E /XF *.pdb /NFL /NDL /NJH /NJS /NP | Out-Null
if ($LASTEXITCODE -ge 8) { throw "robocopy failed with $LASTEXITCODE" }
$global:LASTEXITCODE = 0

Copy-Item $collectorExe (Join-Path $stage 'collector\ion-collector.exe') -Force
# The neutral default configuration is installed as live.json.
Copy-Item (Join-Path $repo 'collector\configs\default.json') (Join-Path $stage 'collector\configs\live.json') -Force

Copy-Item (Join-Path $PSScriptRoot 'launch.ps1') $stage -Force
Copy-Item (Join-Path $PSScriptRoot 'launch.cmd') $stage -Force
Copy-Item (Join-Path $repo 'README.md') $stage -Force
Copy-Item (Join-Path $repo 'LICENSE') (Join-Path $stage 'LICENSE.txt') -Force
foreach ($doc in @('CONFIGURATION.md','DATA-SOURCES.md','BUILDING.md','TROUBLESHOOTING.md')) {
    $path = Join-Path $repo "docs\$doc"
    if (Test-Path $path) { Copy-Item $path (Join-Path $stage 'docs') -Force }
}

@"
ION COMMAND $Version

This installs a live geospatial situation display and the local data collector
that feeds it. On first start the collector connects to public data services
(PSKReporter, OpenSky, adsb.lol, Blitzortung, NOAA SWPC, USGS, CelesTrak,
EUMETSAT and others) over the internet.

Nothing is uploaded about you. Set your callsign and grid locator in the
in-app SETTINGS panel; see docs\DATA-SOURCES.md for the services used and
their attribution.
"@ | Set-Content -Path (Join-Path $stage 'BEFORE-INSTALL.txt') -Encoding UTF8

$stageBytes = (Get-ChildItem $stage -Recurse -File | Measure-Object -Property Length -Sum).Sum
Write-Host ("Staged {0:N0} MB" -f ($stageBytes / 1MB))

New-Item -ItemType Directory -Path $outDir -Force | Out-Null
Write-Host 'Compiling installer (LZMA2/max, this takes a few minutes)...'
& $ISCC "/DAppVersion=$Version" "/DStageDir=$stage" "/DOutDir=$outDir" (Join-Path $PSScriptRoot 'ion-command.iss')
if ($LASTEXITCODE -ne 0) { throw "ISCC failed with $LASTEXITCODE" }

$setup = Join-Path $outDir "ION-COMMAND-$Version-Setup.exe"
if (-not (Test-Path $setup)) { throw "Installer not produced at $setup" }
$mb = (Get-Item $setup).Length / 1MB
Write-Host ("Installer ready: {0} ({1:N0} MB)" -f $setup, $mb)
