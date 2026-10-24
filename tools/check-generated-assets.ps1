<#
.SYNOPSIS
    Fails when generated Unreal assets are missing, before anything is cooked.

.DESCRIPTION
    The master materials and their instances (issue #35) and the imported
    textures (issue #41) are build output, not tracked files. Cooking without
    them is not an error in Unreal: a missing material is only a cook WARNING
    and the globe then ships drawn with the engine's default material. This
    check makes that case a hard failure instead.

    The material names are derived from unreal/Scripts/create_material_instances.py
    itself (every ensure_material / create_instance / build_sensor_mono call,
    including the numbered MI_Signal_NN loop), so the list cannot drift from the
    script. If the derivation yields no names, too few names, or meets a call it
    cannot resolve, that is an error too - never a pass.

    The texture names are derived the same way from the import_texture(source,
    "T_Name", ...) calls: the asset name is the second argument, and it must be
    a plain string literal. A call whose second argument cannot be read as one,
    or fewer textures than expected, is an error.

.PARAMETER ListNames
    Also print every derived material and texture name (for checking the derivation).

.PARAMETER RepositoryRoot
    Repository to check. Defaults to the repository containing this script.
#>
param([string]$RepositoryRoot = "", [switch]$ListNames)

$ErrorActionPreference = 'Stop'
if (-not $RepositoryRoot) { $RepositoryRoot = Split-Path -Parent $PSScriptRoot }

$scriptPath = Join-Path $RepositoryRoot 'unreal\Scripts\create_material_instances.py'
$materialDir = Join-Path $RepositoryRoot 'unreal\Content\ION\Materials'
$textureDir = Join-Path $RepositoryRoot 'unreal\Content\ION\Textures'
# The script defines 34 materials and 5 textures today. Far fewer means the
# derivation broke.
$minimumMaterials = 25
$minimumTextures = 4

$fix = "Run tools\fetch-earth-textures.py (Earth textures), then tools\run-editor.ps1 " +
    "(it runs unreal\Scripts\create_material_instances.py and regenerates the assets); " +
    "then package again."

if (-not (Test-Path -LiteralPath $scriptPath)) {
    throw "Generated-asset preflight cannot derive the material list: $scriptPath not found."
}
$source = Get-Content -LiteralPath $scriptPath -Raw -Encoding UTF8

# Loop bounds for the f-string names: "for index in range(11):" -> index = 11.
$ranges = @{}
foreach ($m in [regex]::Matches($source, '(?m)^\s*for\s+(\w+)\s+in\s+range\((\d+)\)\s*:')) {
    $ranges[$m.Groups[1].Value] = [int]$m.Groups[2].Value
}

$names = New-Object 'System.Collections.Generic.SortedSet[string]'
$unresolved = @()
$callPattern = '(?m)^(?!\s*def\s)[^\r\n]*?\b(ensure_material|create_instance|build_sensor_mono)\(\s*([^,)\r\n]*)'
foreach ($m in [regex]::Matches($source, $callPattern)) {
    $call = $m.Groups[1].Value
    $arg = $m.Groups[2].Value.Trim()
    if ($arg -match '^"([^"{}]+)"$') {
        [void]$names.Add($Matches[1])
    }
    elseif ($arg -match '^f"([^"{}]*)\{(\w+):0?(\d+)d\}([^"{}]*)"$') {
        $prefix = $Matches[1]; $var = $Matches[2]; $width = [int]$Matches[3]; $suffix = $Matches[4]
        if (-not $ranges.ContainsKey($var)) {
            $unresolved += "$call($arg) - no 'for $var in range(N)' found"
            continue
        }
        for ($i = 0; $i -lt $ranges[$var]; $i++) {
            [void]$names.Add($prefix + $i.ToString("D$width") + $suffix)
        }
    }
    elseif ($arg -eq 'name') {
        # Pass-through parameter inside a helper; the callers supply literals.
    }
    else {
        $unresolved += "$call($arg)"
    }
}

if ($unresolved.Count -gt 0) {
    throw ("Generated-asset preflight cannot resolve these calls in create_material_instances.py " +
        "(extend tools\check-generated-assets.ps1):`n  " + ($unresolved -join "`n  "))
}
if ($names.Count -lt $minimumMaterials) {
    throw ("Generated-asset preflight derived only $($names.Count) material names from " +
        "create_material_instances.py (expected at least $minimumMaterials); refusing to pass on a broken derivation.")
}

# Textures: import_texture(<source>, "T_Name", ...). The source argument is an
# arbitrary expression (starmap_source, SOURCE_DIR / "NASA" / "x.png"), so only
# the second argument is read. Comment lines are skipped; every other mention of
# import_texture must be the definition or a call with a literal asset name.
$textureNames = New-Object 'System.Collections.Generic.SortedSet[string]'
$unresolvedTextures = @()
$textureCall = '^[^\r\n]*?\bimport_texture\(\s*[^,\r\n]+,\s*"(T_[A-Za-z0-9_]+)"\s*[,)]'
foreach ($m in [regex]::Matches($source, '(?m)^(?!\s*#)(?!\s*def\s)[^\r\n]*\bimport_texture\b[^\r\n]*')) {
    $line = $m.Value.Trim()
    if ($line -match $textureCall) {
        [void]$textureNames.Add($Matches[1])
    }
    else {
        $unresolvedTextures += $line
    }
}
if ($unresolvedTextures.Count -gt 0) {
    throw ("Generated-asset preflight cannot resolve these import_texture uses in create_material_instances.py " +
        "(the asset name must be a string literal as second argument; extend tools\check-generated-assets.ps1):`n  " +
        ($unresolvedTextures -join "`n  "))
}
if ($textureNames.Count -lt $minimumTextures) {
    throw ("Generated-asset preflight derived only $($textureNames.Count) texture names from " +
        "create_material_instances.py (expected at least $minimumTextures); refusing to pass on a broken derivation.")
}

if ($ListNames) {
    $names | ForEach-Object { Write-Output "derived material: $_" }
    $textureNames | ForEach-Object { Write-Output "derived texture: $_" }
}

$expected = @()
foreach ($n in $names) { $expected += [pscustomobject]@{ Kind = 'material'; Path = Join-Path $materialDir "$n.uasset" } }
foreach ($t in $textureNames) { $expected += [pscustomobject]@{ Kind = 'texture'; Path = Join-Path $textureDir "$t.uasset" } }

$missing = @($expected | Where-Object { -not (Test-Path -LiteralPath $_.Path -PathType Leaf) })
if ($missing.Count -gt 0) {
    $list = ($missing | ForEach-Object { "$($_.Kind) $([IO.Path]::GetFileName($_.Path))" }) -join "`n  "
    throw ("Generated assets are missing ($($missing.Count) of $($expected.Count)); " +
        "cooking now would ship the default material instead:`n  $list`n$fix")
}

Write-Output "Generated assets OK: $($expected.Count) checked ($($names.Count) materials + $($textureNames.Count) textures)."
