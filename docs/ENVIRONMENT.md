# Bootstrap environment

Inspection date: **2026-07-18**, timezone Europe/Zurich.

## Workspace

- Root: `D:\code\ION COMMAND`
- Operating system: Windows, x64
- Git repository: initialized, branch `master`, no commits at inspection time
- Existing project files before bootstrap: none
- Existing `AGENTS.md`: none

## Available toolchain

| Component | Detected state |
|---|---|
| Go | `go1.26.0 windows/amd64` at `C:\Program Files\Go\bin\go.exe` |
| Git | `2.48.1.windows.1` |
| Python | `3.12.9` |
| CMake | installed |
| Ninja | installed through WinGet |
| .NET | runtimes 6, 7, and 8; UE also supplies its bundled .NET runtime |
| Visual Studio | Community 2022 17.14.36 plus Build Tools 2019 16.11.44 |
| MSVC | current v143 plus side-by-side 14.38 x64/x86 toolsets |
| Windows SDK | 10.0.26100 plus 10.0.19041.0 and older SDKs |
| ripgrep | installed |

VS2022 includes Desktop development with C++, Game development with C++, the
Unreal IDE and Blueprint debugger integrations, Unreal Test Adapter,
AddressSanitizer, and the Windows 11 SDK. `cl.exe` and `msbuild.exe` are loaded
through the Visual Studio developer environment rather than the ordinary shell
`PATH`.

## Unreal Engine

Unreal Engine **5.8.0**, changelist 55116800, is installed through Epic at:

```text
D:\Epic Games\UE_5.8
```

The installation includes engine source, templates, Feature Packs, Fab, and
Quixel Bridge components. The project association was updated from 5.6 to 5.8
to follow the installed engine as required by the bootstrap specification.

Set `ION_COMMAND_UNREAL_ROOT` after installation, or pass `-UnrealRoot` to the
PowerShell tools. `tools/find-unreal.ps1` also discovers Epic manifests.

## Verification sequence

1. Run `tools/build.ps1 -Unreal`.
2. Run `tools/run-editor.ps1`; this creates materials, data assets, and the
   command-deck level before opening the editor.
3. Run the editor validation and screenshot scripts.
