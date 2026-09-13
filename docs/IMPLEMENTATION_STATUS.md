# Implementation status

What works today, what is rough, and what is not there. Kept deliberately
short; the day-by-day development history lives in
[history/DEVELOPMENT-LOG.md](history/DEVELOPMENT-LOG.md) and released changes in
[../CHANGELOG.md](../CHANGELOG.md).

**Applies to:** `v0.9.0` and the current default branch.
The component list is in [COMPONENTS.md](COMPONENTS.md) — not repeated here.

## Implemented

**Collector**

- Compile-time source/domain/context registries; 11 source types, 8 domains,
  1 context (see [COMPONENTS.md](COMPONENTS.md))
- Bounded queue with parallel domain workers and drop/evict metrics
- Canonical envelope with Point, GreatCircle, LineString/MultiLineString and
  Polygon (Area) geometry, validity windows and generic `display.*` /
  `visual.*` presentation properties
- Retained state per entity with TTL, background purge and replay on connect
- JSONL recording with hourly rotation and a total-size cap; time-scaled replay
- HTTP API (`/api/health`, `/api/status`, `/api/stats`, `/api/sources`) and
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
- Batched area layer: closed Polygon/MultiPolygon rings projected onto the
  globe shell with a translucent fill and a brighter outline. One fill mesh
  and one outline batch for the whole layer; not one Actor per area.
  Toggle: `Y` key or the overlay menu's AREAS row. Satellite
  `orbital.footprint` Areas stay hidden until the operator pins that
  satellite (`P` while hovered/selected); grayline and NHC cones do not
  require those pins.
- Batched path layer: open LineString/MultiLineString routes projected onto
  the globe shell (densified like Area edges). Used for TeleGeography
  submarine cables; color is planned vs in-service length band. Hover
  (same ray-to-segment pick as click) shows name, planned vs in-service,
  route km, and linked landing names. Toggle: `C` or the overlay menu's
  CABLES row (legend under the row)
- Cartography path role: Natural Earth admin-0 borders and major rivers as
  muted LineStrings (thinner, lower intensity than cables; same ZoomThickness).
  HUD draws zoom-aware city / country / landmark / region labels. Toggle: `B`
  or the overlay menu's BORDERS row. Cable hover and `C` stay on the cable role.
- Cockpit HUD: status bar, band histogram, path-rate sparkline, top DXCC
  regions with flags, auroral oval, HF conditions estimate, hop/MUF path
  analysis, hover tooltips, own-station reticle
- Overlay menu (per-layer and per-domain visibility) and an in-app settings
  panel with text entry for callsign and grid locator, persisted to `Game.ini`
- Timeline: pause, replay of the recent window, variable speed, return to live
- Keyboard-first search overlay (`/` or `S`) over a bounded, timeline-aware
  index of accepted canonical messages, grouped by stable entity id; results
  focus/select Point, GreatCircle, LineString and Polygon geometry
- Watchlist and alerts (`W`): save a search query as a watch, persisted to
  `Game.ini`; a bounded, live-only alert list with an at-a-glance unseen count

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
- **AIS ships (`ais.aisstream`) is live-verified** (2026-08-28). Built and
  unit-tested against fixtures from the provider's published documentation
  and OpenAPI schema (position decoding, every AIS "not available"
  sentinel, the static/voyage join, non-vessel MMSI rejection, bounded-cache
  eviction), then exercised against the real feed with an operator key: two
  European bounding boxes produced ~2,000 raw messages per 45 s with 0
  invalid and 0 dropped, and vessels render with their own pictogram. It
  now ships **enabled** with an empty key and idles until `local.json`
  overlays one; see
  [DATA-SOURCES.md](DATA-SOURCES.md#enabling-ais-ships-aisstreamio).
  Sustained multi-hour reconnect behaviour remains unexercised.
- **OpenSky OAuth2** is implemented (`oauth` when `clientId`/`clientSecret` or
  a `credentials.json` path are present in gitignored `local.json`; `anon`
  otherwise). Token cache, refresh-before-expiry, single 401 retry, daily
  credit budget, and `Retry-After` are covered by httptest. A live OpenSky
  account was not available in this environment.
- **Additional geospatial sources** (Launch Library 2 upcoming launches and
  Earth spaceports, Open-Meteo, Natural Earth 110m cartography (borders,
  cities, rivers, landmarks), TeleGeography cables, NASA
  EONET, GDACS, gpsjam.org, OpenAQ, IMF PortWatch, NOAA NHC storm centres,
  HDX HAPI displacement) are implemented as Go source plugins against the
  upstream APIs. OpenAQ and HAPI ship disabled (need an operator
  identifier). gpsjam hex centres are an approximate independent H3 decode,
  honest at globe scale, not a surveyed hex. NOAA NHC now also fetches the
  official 5-day forecast-cone KMZ linked from `CurrentStorms.json` and
  emits it as `weather.storm.cone` Area geometry. Live fetches of the new
  HTTP sources were unit-tested against fixtures, not against a long-running
  collector session. PortWatch also emits recent disruption events from
  the same IMF FeatureServer family as the chokepoint layer.
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
  fetched before a source build (`tools/fetch-earth-textures.py`). The day
  texture alone is a 147 MB PNG that compiles to a 165 MB virtual-texture
  asset, well past what belongs in git.
- **Close-range imagery needs the network.** Below roughly 1000 km the globe
  stops relying on the packaged global texture and streams map tiles from
  NASA GIBS into a window that follows the camera, down to ~30 m per pixel
  (see DATA-SOURCES.md). Offline, or with `-IonNoTileImagery`, the close
  approach falls back to the 2 km/pixel global texture and resolves into
  coloured mush below roughly 300 km, which is the limit a single global
  texture can reach at all.
- **Relief is shaded, not displaced.** The height field drives a surface
  normal, so terrain catches the light correctly at any zoom, but the
  silhouette at the horizon stays smooth and terrain never occludes terrain.
  The globe mesh has 400x200 quads - one quad spans ~100 km against a 43 km
  frame at the closest orbit - so there is no vertex in view to displace.
- **The tile cache is never evicted.** `<Saved>/TileCache` grows with every
  place the operator visits and is only reclaimed by deleting the directory.
- Aircraft coverage depends on the configured regions plus the global snapshot;
  the anonymous OpenSky poll is slow by design. OAuth credentials in
  `local.json` allow a faster poll within the authenticated credit budget.

## Not implemented

- Niagara-based renderer (ADR 0003 describes the intent; the renderer uses
  hierarchical instanced meshes)
- Geometry beyond Point, GreatCircle, LineString/MultiLineString and Polygon —
  the `Track` semantic geometry payload itself, plus grids, raster fields,
  shells and volumes, are reserved in the contract only and parsed but not
  rendered. (The client motion-trail layer above is a distinct client-side
  visualization built from ordinary Point messages, not an implementation of
  this payload.)
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
