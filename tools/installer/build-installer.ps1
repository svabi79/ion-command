# Stages the release payload and compiles the Windows installer.
#
#   powershell -ExecutionPolicy Bypass -File tools\installer\build-installer.ps1 -Version 1.0.0
#
# Prerequisites:
#   - Shipping client archived to dist\windows  (tools\package.ps1 -Config Shipping)
#   - Collector built to collector\bin\ion-collector.exe (tools\build.ps1)
#   - Inno Setup 6 (winget install JRSoftware.InnoSetup)
param(
    [string]$Version = "1.0.0",
    [string]$ISCC = ""
)

$ErrorActionPreference = 'Stop'
$repo    = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
# Must match the archive directory in tools\package.ps1. These drifted apart
# once and the installer silently shipped the previous release's client.
$client  = Join-Path $repo 'dist\windows'
$stage   = Join-Path $repo 'dist\installer-stage'
$outDir  = Join-Path $repo 'dist\installer'

if (-not (Test-Path (Join-Path $client 'IonCommand.exe'))) {
    throw "Shipping client missing at $client - run the Shipping package first."
}
$collectorExe = Join-Path $repo 'collector\bin\ion-collector.exe'
if (-not (Test-Path $collectorExe)) { throw "Collector missing at $collectorExe - run tools\build.ps1." }

# Existence is not freshness. A leftover client tree from the previous release
# passes a Test-Path check and ships silently, so state what is being packaged
# and refuse a client that predates the collector of this same build.
$clientBinary = Get-Item (Join-Path $client 'IonCommand\Binaries\Win64\IonCommand-Win64-Shipping.exe') -ErrorAction SilentlyContinue
if (-not $clientBinary) { throw "Shipping binary missing under $client - run tools\package.ps1 -Config Shipping." }
$collectorBinary = Get-Item $collectorExe
Write-Host ("Client:    {0:yyyy-MM-dd HH:mm}  {1:N0} bytes" -f $clientBinary.LastWriteTime, $clientBinary.Length)
Write-Host ("Collector: {0:yyyy-MM-dd HH:mm}  {1:N0} bytes" -f $collectorBinary.LastWriteTime, $collectorBinary.Length)
if ($clientBinary.LastWriteTime -lt $collectorBinary.LastWriteTime.AddHours(-12)) {
    throw ("Client binary is from {0:yyyy-MM-dd HH:mm} but the collector is from {1:yyyy-MM-dd HH:mm}. " -f $clientBinary.LastWriteTime, $collectorBinary.LastWriteTime) +
          "That usually means the Shipping package was not rebuilt for this release. Run tools\package.ps1 -Config Shipping."
}

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
Copy-Item (Join-Path $repo 'THIRD_PARTY_NOTICES.md') $stage -Force
# The embedded country file requires its notice to travel with the binary.
Copy-Item (Join-Path $repo 'collector\internal\cty\LICENSE') (Join-Path $stage 'collector\cty-LICENSE.txt') -Force
foreach ($doc in @('CONFIGURATION.md','DATA-SOURCES.md','BUILDING.md','TROUBLESHOOTING.md')) {
    $path = Join-Path $repo "docs\$doc"
    if (Test-Path $path) { Copy-Item $path (Join-Path $stage 'docs') -Force }
}

@"
ION COMMAND $Version

This installs a live situation display and the local data collector that feeds
it. On first start the collector connects over the internet to public data
services: PSKReporter, OpenSky, adsb.lol, Blitzortung, NOAA SWPC, USGS,
CelesTrak, GIRO/KC2G and EUMETSAT.

Nothing about you is uploaded. Set your callsign and grid locator in the in-app
SETTINGS panel.

PLEASE NOTE - the data is not ours, and some of it is restricted:

 * Several services allow NON-COMMERCIAL USE ONLY (Blitzortung, OpenSky,
   GIRO/KC2G). The MIT licence on ION COMMAND's own code does not grant you
   any rights to the data it fetches.
 * The lightning layer streams from Blitzortung.org, a network of volunteers
   who each run their own detector at their own expense. It is enabled by
   default. If you use it, please consider running a station yourself
   (https://www.blitzortung.org/en/cover_your_area.php), and switch the source
   off in any commercial deployment.
 * Blitzortung is NOT a warning system. Nothing shown here may be used for
   safety-relevant decisions.

Full attribution and terms: docs\DATA-SOURCES.md
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
