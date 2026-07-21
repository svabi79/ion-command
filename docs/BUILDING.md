# Building from source

You only need this if you want to change the code — the
[release installer](https://github.com/svabi79/ion-command/releases) contains
everything prebuilt.

## Prerequisites

| Tool | Version | Used for |
| --- | --- | --- |
| **Unreal Engine** | 5.8 | the client |
| **Visual Studio 2022** | with "Game development with C++" | UE toolchain |
| **Go** | 1.24+ | the collector |
| **Python** | 3.10+, with `Pillow` | generated textures, icon atlas, ambience |
| **Inno Setup** | 6 | the installer (`winget install JRSoftware.InnoSetup`) |

Point the scripts at your engine if it is not auto-detected:

```powershell
$env:ION_COMMAND_UNREAL_ROOT = 'D:\Epic Games\UE_5.8'
```

## 1. Collector

```powershell
cd collector
go test ./...
go build -trimpath -o bin/ion-collector.exe ./cmd/ion-collector
```

Run it against the live configuration:

```powershell
.\bin\ion-collector.exe -config .\configs\live.json
```

## 2. Generated assets and materials

Earth/night/cloud textures and the star map are **not** in the repository
(licence-clean and large); fetch them once:

```powershell
python tools\fetch-earth-textures.py
```

This downloads NASA Blue Marble, Black Marble, a cloud climatology and the SVS
Deep Star Map into `unreal/SourceAssets/NASA/`.

Then generate the procedural assets (starfield fallback, marker icon atlas,
deck ambience) and rebuild every master material deterministically:

```powershell
python unreal\Scripts\generate_visual_sources.py
.\tools\run-editor.ps1            # runs the editor bootstrap scripts
```

`unreal/Scripts/create_material_instances.py` rebuilds all master materials
from scratch on every run — never hand-edit the generated materials, change the
script instead.

## 3. Client

```powershell
.\tools\build.ps1 -Unreal          # collector + editor target
```

Development package (fast iteration, large binary):

```powershell
.\tools\package.ps1                       # -> dist\windows
```

Release package (Shipping, what ships in the installer):

```powershell
.\tools\package.ps1 -Config Shipping      # -> dist\windows
```

> Packaging needs ZenServer. If the cook aborts with a Zen connection error,
> start it once with `zen.exe up` from `Engine\Binaries\Win64`.
>
> Stop any running client first — a running instance locks the packaged files
> and the archive step silently skips them, leaving a stale build behind.
> Always check the timestamp of the produced `IonCommand.exe`.

## 4. Installer

```powershell
# Shipping build archived to dist\release first:
powershell -ExecutionPolicy Bypass -File tools\installer\build-installer.ps1 -Version 1.0.0
```

The script stages the client (without debug symbols), the collector, the
default configuration, the launcher and the docs, then compiles
`dist\installer\ION-COMMAND-<version>-Setup.exe`.

## Verification

```powershell
.\tools\first-light.ps1 -Port 7811     # collector + client + screenshot + log scan
```

`first-light.ps1` refuses to run if the port is already in use, so it can never
silently attach to a collector you already have running. It fails on any error
in the game log and runs a structural check on the captured frame.

## Repository layout

```
collector/            Go collector
  cmd/ion-collector/  entry point, source/domain registration
  internal/plugins/   sources/ (feeds) and domains/ (normalisers)
  internal/stream/    WebSocket hub with retained state
  internal/pipeline/  queue, workers, recording, publish
unreal/
  Source/IonCommandCore/           geo math, generic types
  Source/IonCommandData/           envelope parsing, stream/data subsystems
  Source/IonCommandVisualization/  globe, arcs, markers, heatmap
  Source/IonCommandUI/             cockpit HUD, overlay menu, settings
  Source/IonCommandHamRadio/       ham-specific layers (own station, panels)
  Source/IonCommand/               game mode, player controller, camera
  Scripts/                         editor automation (materials, level, assets)
tools/                build/package/verify scripts, installer
docs/                 this documentation
```

The generic modules never contain domain vocabulary — callsigns, bands and
similar live in the HamRadio module or come through the envelope's
`display.*` / `visual.*` properties. Keep it that way when adding features.
