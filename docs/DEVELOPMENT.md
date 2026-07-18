# Development

## Collector

From the repository root:

```powershell
.\tools\build.ps1
.\tools\run-collector.ps1
```

Development endpoints are on `127.0.0.1:7810`. Recordings go below
`data/recordings/YYYY/MM/DD/`. Stop with Ctrl+C to flush cleanly.

Run all repository checks:

```powershell
.\tools\test.ps1
```

Run the collector process smoke test (100 live radio links plus recording and
replay):

```powershell
.\tools\smoke.ps1
```

## Unreal

Install UE5.6 and a supported Visual Studio 2022 C++ workload, then either set
`ION_COMMAND_UNREAL_ROOT` or pass `-UnrealRoot`.

```powershell
.\tools\build.ps1 -Unreal
.\tools\run-editor.ps1
```

The editor launcher runs, in order:

1. `create_material_instances.py`
2. `create_data_assets.py`
3. `create_bootstrap_level.py`
4. `validate_project.py`

It then opens an interactive editor window. Use `-SkipBootstrap` only after
assets exist. Start the collector first, then Play in Editor.

Controls: right mouse orbit, mouse wheel zoom, left mouse select, Escape clear
selection, F focus selection, I toggle ionosphere shells, Space pause, L Return
to Live.

## Adding a feed

1. Implement `plugins.Source` for connection/framing.
2. Implement or reuse a `plugins.Domain` normalizer.
3. register both in `cmd/ion-collector/main.go` during bootstrap, and add a
   declarative manifest below `plugins/`.
4. emit canonical messages covered by the contract.
5. use an existing layer/renderer or add a domain layer specialization.
6. add fixtures, schema validation, reconnect tests, and load tests.

Do not modify the canonical core merely to add a semantic type.
