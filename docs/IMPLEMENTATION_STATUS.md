# Implementation status

What works today, what is rough, and what is not there. Kept deliberately
short; the day-by-day development history lives in
[history/DEVELOPMENT-LOG.md](history/DEVELOPMENT-LOG.md) and released changes in
[../CHANGELOG.md](../CHANGELOG.md).

**Applies to:** `v0.9.0` and the current default branch.
The component list is in [COMPONENTS.md](COMPONENTS.md) — not repeated here.

## Implemented

**Collector**

- Compile-time source/domain/context registries; 10 source types, 7 domains,
  1 context (see [COMPONENTS.md](COMPONENTS.md))
- Bounded queue with parallel domain workers and drop/evict metrics
- Canonical envelope with Point and GreatCircle geometry, validity windows and
  generic `display.*` / `visual.*` presentation properties
- Retained state per entity with TTL, background purge and replay on connect
- JSONL recording with hourly rotation and a total-size cap; time-scaled replay
- HTTP API (`/api/health`, `/api/status`, `/api/statistics`) and
  `ws://…/ws/live`
- Rate-limit discipline: global request spacing for aviation, 429/420 backoff
  with `Retry-After`, hard stop on repeated failures

**Client**

- Lit globe with day/night textures, atmosphere, aurora scaled by Kp, and an
  hourly live cloud composite
- Real star sky rotated to Greenwich Mean Sidereal Time
- Instanced arc layer with band palette, age fade, congestion dimming and
  click selection
- Instanced marker layer: per-type pictogram atlas, per-instance tint, camera
  facing, heading orientation, dead reckoning between updates, altitude
  exaggeration toggle, aviation altitude/ground filter
- Instanced motion-trail layer: a bounded, GPU age-faded comet tail behind any
  moving entity, built client-side from repeated Point sightings of one
  stable entity id (not from the reserved Track geometry payload); stationary
  entities never accumulate one. Toggle: `T` key or the overlay menu
- Cockpit HUD: status bar, band histogram, path-rate sparkline, top DXCC
  regions with flags, auroral oval, HF conditions estimate, hop/MUF path
  analysis, hover tooltips, own-station reticle
- Overlay menu (per-layer and per-domain visibility) and an in-app settings
  panel with text entry for callsign and grid locator, persisted to `Game.ini`
- Timeline: pause, replay of the recent window, variable speed, return to live

**Delivery**

- Shipping package plus an Inno Setup installer, verified install → run →
  uninstall
- Reproducible asset generation (icon atlas, starfield fallback, ambience) and
  deterministic material rebuilds from Python scripts

## Experimental / rough edges

- **HF conditions and path analysis are heuristics.** Panels are labelled
  `// HEURISTIC`. They are not forecasts, and arcs are reported reception
  links, not measured ray paths.
- **WSJT-X source** is implemented and unit-tested against synthetic UDP, but
  has not been validated against a live WSJT-X session.
- **OpenSky authentication** is ineffective: the `login`/`password` fields use
  HTTP basic auth, which OpenSky retired in favour of OAuth2. Anonymous access
  works.
- **Plugin manifests** under `plugins/` are incomplete descriptive metadata;
  the registry code is authoritative.

## Known limitations

- Windows x64 only; no macOS or Linux client.
- Single-operator local use. The collector binds `127.0.0.1` and has **no
  authentication** on its HTTP or WebSocket endpoints.
- The installer is not code-signed, so SmartScreen warns.
- Recording is off by default because the live feeds produce several GB per
  hour.
- Globe, night, cloud and star textures are not in the repository and must be
  fetched before a source build (`tools/fetch-earth-textures.py`).
- Aircraft coverage depends on the configured regions plus the global snapshot;
  the anonymous OpenSky poll is slow by design.

## Not implemented

- Niagara-based renderer (ADR 0003 describes the intent; the renderer uses
  hierarchical instanced meshes)
- Geometry beyond Point and GreatCircle — the `Track` semantic geometry
  payload itself, plus polygons, grids, raster fields, shells and volumes,
  are reserved in the contract only and parsed but not rendered. (The client
  motion-trail layer above is a distinct client-side visualization built from
  ordinary Point messages, not an implementation of this payload.)
- Dynamic or third-party plugin loading
- Any multi-user, server-hosted or authenticated deployment mode

## Verification

- `go test ./...` in `collector/` covers parsers, domains, hub retention and
  recording, using fixtures captured from real provider responses.
- `tools/validate_repository.py` checks repository invariants, including that
  the source types registered in the collector match
  [COMPONENTS.md](COMPONENTS.md).
- `tools/first-light.ps1` runs collector plus client end to end, captures a
  frame and fails on any error in the game log.
- Renderer and data changes are verified against live feeds with screenshot
  captures before release; see the development log for the individual proofs.
