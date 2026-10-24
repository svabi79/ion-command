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
(licence-clean and large, up to ~150 MB for the day texture alone); fetch
them once:

```powershell
python tools\fetch-earth-textures.py
```

This downloads NASA Blue Marble (21600x10800), Black Marble (13500x6750), a
cloud climatology (2048x1024, native resolution for that composite) and the
SVS Deep Star Map into `unreal/SourceAssets/NASA/`. See the docstring in
that script and `unreal/SourceAssets/NASA/ATTRIBUTION.md` for exactly what
resolution is and is not available under a licence this project can use.

Then generate the procedural assets (starfield fallback, marker icon atlas,
deck ambience), the high-resolution globe mesh, and rebuild every master
material deterministically:

```powershell
python unreal\Scripts\generate_visual_sources.py
.\tools\run-editor.ps1            # runs the editor bootstrap scripts
```

`run-editor.ps1` runs `generate_globe_mesh.py` first: it replaces the engine's
placeholder sphere primitive with a 400x200-segment mesh built from Unreal's
GeometryScript primitive generator (reproducible, not hand-authored — see the
script for why it needs a UV correction after generation and how that is
verified). The Earth day/night textures import as Streaming Virtual Textures
(`r.VirtualTextures=True` in `DefaultEngine.ini`), so their VRAM cost tracks
what is actually on screen rather than the full 21600x10800 source.

`unreal/Scripts/create_material_instances.py` rebuilds all master materials
from scratch on every run — never hand-edit the generated materials, change the
script instead.

**The materials and textures are build output, not tracked files.** Everything
in `unreal/Content/ION/Materials/` and `unreal/Content/ION/Textures/` is
produced by that script (it re-imports every texture on each run) and is
gitignored. A fresh clone
therefore has to run, in this order:

1. `python tools\fetch-earth-textures.py` — the NASA source images
2. `.\tools\run-editor.ps1` — generates the textures and materials (do not
   pass `-SkipBootstrap` on a fresh clone)
3. `.\tools\package.ps1` — cooks the client

`tools\package.ps1` starts with `tools\check-generated-assets.ps1`, which
refuses to cook (and names the missing assets) if any material or texture the
script defines is absent. Without that check a missing material is only a cook
warning and the globe would ship with Unreal's default material. Both lists are
read from `create_material_instances.py` itself (every `import_texture` call
must pass its asset name as a string literal), so a new material or texture
needs no change to the check.

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
# Requires the Shipping package from step 3 (archived to dist\windows):
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

## Quality gates: what runs where

Merging is gated in two places. They cover different failures; neither
replaces the other.

**Hosted CI** (`.github/workflows/ci.yml`, GitHub Actions, one `ubuntu-latest`
job on every pull request and every push to `master`):

| Check | Command |
| --- | --- |
| Go static analysis | `go vet ./...` in `collector/` |
| Go tests | `go test ./...` in `collector/` |
| Repository validation | `python tools/validate_repository.py` |
| PowerShell syntax of `tools/*.ps1` | `tools/check-powershell-syntax.ps1` |

The Go toolchain is pinned by `collector/go.mod`. Its `go 1.24.0` directive is
patch-qualified, so CI uses exactly Go 1.24.0. `gofmt -l` is deliberately not
part of CI yet: the existing collector code is not gofmt-clean (15 files), and
reformatting it is a separate change.

You can run the same checks locally with `.	ools	est.ps1`.

**Local, before merge** (on a machine with Unreal Engine 5.8 installed):

1. `.	oolsuild.ps1 -Unreal` — compiles **both** Unreal targets, the editor
   target `IonCommandEditor` and the game target `IonCommand` (tracked in
   [#34](https://github.com/svabi79/ion-command/issues/34)).
2. `.	ools\package.ps1` — the cook and package.

> **Hosted CI does not compile or cook the Unreal project.** UE 5.8 is a
> ~100 GB local install and cannot run on a hosted runner. The four packaging
> failures that reached `master` on 2026-09-13 (#15 `EKeys::NumPadEnter`,
> #17 Material usage flags, #22 `Role` renamed to `PathRole`, #23 place-label LOD
> sort) were all UE 5.8 compile or cook errors. A green hosted run says nothing
> about them; the local compile of both targets and the cook are the gate that
> catches them. Do not merge a change that touches `unreal/` without running
> both.

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
  Source/IonCommandVisualization/  globe, arcs, markers, areas, paths, heatmap
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
