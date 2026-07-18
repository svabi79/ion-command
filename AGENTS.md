# ION COMMAND agent guide

## Product boundary

ION COMMAND is a native Unreal Engine application built on a generic,
time-aware geospatial data platform. Amateur radio is the first domain, not a
dependency of the core.

## Architectural rules

- Keep feed acquisition, domain normalization, canonical storage, layer
  processing, rendering, and contextual analysis separate.
- Core packages and modules must not contain ham-radio vocabulary.
- All external data enters through a source plugin and a domain normalizer.
- Unknown semantic types remain recordable and replayable.
- Live and replay must use the same canonical pipeline.
- Keep all queues and histories bounded and expose drops as metrics.
- Never create one Unreal Actor, widget, material, or Niagara system per event.
- Binary Unreal assets must be reproducible through scripts or documented.
- All timestamps are UTC and all geographic coordinates are WGS84 longitude,
  latitude, optional altitude.

## Verification

- Run `go test ./...` from `collector/` after Go changes.
- Run `tools/test.ps1` for repository validation.
- When UE5 is installed, run `tools/build.ps1 -Unreal` and the editor
  validation script before claiming the Unreal client builds.

