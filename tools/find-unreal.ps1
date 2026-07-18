param([string]$RequestedRoot = "")

$candidates = @()
if ($RequestedRoot) { $candidates += $RequestedRoot }
if ($env:ION_COMMAND_UNREAL_ROOT) { $candidates += $env:ION_COMMAND_UNREAL_ROOT }

$manifestDirectory = 'C:\ProgramData\Epic\EpicGamesLauncher\Data\Manifests'
if (Test-Path -LiteralPath $manifestDirectory) {
    Get-ChildItem -LiteralPath $manifestDirectory -Filter *.item -File -ErrorAction SilentlyContinue | ForEach-Object {
        try {
            $manifest = Get-Content -LiteralPath $_.FullName -Raw | ConvertFrom-Json
            if ($manifest.AppName -like 'UE_*' -or $manifest.DisplayName -like 'Unreal Engine*') { $candidates += $manifest.InstallLocation }
        } catch {}
    }
}

$commonRoots = @('C:\Program Files\Epic Games','D:\Epic Games','G:\Epic Games')
foreach ($commonRoot in $commonRoots) {
    if (Test-Path -LiteralPath $commonRoot) { $candidates += Get-ChildItem -LiteralPath $commonRoot -Directory -Filter 'UE_*' -ErrorAction SilentlyContinue | Select-Object -ExpandProperty FullName }
}

foreach ($candidate in $candidates | Select-Object -Unique) {
    $resolved = Resolve-Path -LiteralPath $candidate -ErrorAction SilentlyContinue
    if ($resolved -and (Test-Path -LiteralPath (Join-Path $resolved 'Engine\Binaries\Win64\UnrealEditor.exe'))) { $resolved.Path; return }
}
throw 'UE5 was not found. Install it or set ION_COMMAND_UNREAL_ROOT to the engine root.'

