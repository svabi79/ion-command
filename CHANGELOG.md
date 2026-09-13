# Changelog

All notable changes to ION COMMAND are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The day-by-day engineering log, including retracted measurements and
superseded configurations, is in
[docs/history/DEVELOPMENT-LOG.md](docs/history/DEVELOPMENT-LOG.md).

## [Unreleased]

### Added

- **Country borders and place labels on the globe.** `geography.naturalearth`
  now embeds a 110m Natural Earth extract of admin-0 boundary lines, cities,
  country names, major river centerlines, and physical landmarks — not just
  the old region/ocean points. The collector emits `geography.border`,
  `.city`, `.country`, `.river`, and `.landmark`. A second
  `AGeoPathLayerActor` role (`Cartography`) draws muted borders and rivers
  with the same `ZoomThickness` squeeze as cables, without joining the
  cable legend or `C` toggle. HUD place labels are zoom-aware (far:
  continents / oceans / megacities; closer: more cities and landmarks).
  Overlay **BORDERS** or `B`. Recook with `go run ./cmd/neextract`. Enabled
  in `live.json`. Wall visual check still remaining (no UE on this VM).

### Changed

- **Cable paths are map lines, not neon tubes.** `AGeoPathLayerActor`
  drove `MI_Track` at Intensity 3.6 with hot legend hues. The material is
  unlit additive (`emissive = Color * Intensity`), so that bloomed on the
  dark globe. Intensity is now 0.4; the five planned / length-bucket
  classes stay, slightly desaturated. Collector `visual.color` strings
  match. Hover/pick from `#19` is unchanged.
- **Cable thickness follows camera distance like FT8 arcs.** Paths kept a
  fixed `PathThickness` cube scale, so close zoom read as fat tubes.
  They already use `MI_Track` (`M_HolographicSignal`), which squeezes
  segment width via `ZoomThickness`. The path layer now drives that the
  same way `AGeoArcLayerActor` does (orbit-normalised, 0.012 floor,
  update when the value moves by 0.002). Instances are not rebuilt.
  Arc `ZoomDim` is left at 1 — cables are a sparse layer already at
  Intensity 0.4, not a stacking additive weave.

### Fixed

- **UE 5.8 Area fill bootstrap after `#16`.** `create_material_instances.py`
  set `used_with_procedural_meshes` on `M_AreaFill`. That property is not
  on Material in 5.8 (Python API: `used_with_static_mesh`,
  `used_with_instanced_static_meshes`, `used_with_nanite`, … — no
  procedural-mesh usage). The setter raised, so `MI_AreaFill` was never
  written and the runtime fell back to `MI_Atmosphere`. The fill master
  now sets `used_with_static_mesh`, which is the permutation
  `UProceduralMeshComponent` uses. Outline / cone runtime is unchanged.
- **UE 5.8 package compile after `#14`.** `#14` committed SETTINGS/search
  on `EKeys::Enter || EKeys::NumPadEnter`. `NumPadEnter` is not a member
  of `EKeys` on 5.8 (`InputCoreTypes.h` has `Enter` and `NumPadZero`–
  `NumPadNine` only), so `tools/package.ps1` / UAT exited 6. Commit stays
  on `EKeys::Enter`. The other keys in that file (`BackSpace`, `Escape`,
  `Up`/`Down`, `Slash`, `Hyphen`, `Underscore`, `SpaceBar`) are valid.
- **Detail imagery felt serial even with a warm TileCache.** `BeginRegion`
  decoded every cached tile on the game thread and fired every miss as an
  HTTP request at once. A wall-resolution window is hundreds of tiles; that
  hitch plus Unreal's low per-host connection cap looked like a cold
  download every zoom. Cache hits now drain a few per frame (warm paint
  first), downloads stay at 8 in flight, abandoned requests are cancelled
  on region change, and cache paths are absolute so `Saved/TileCache` is
  actually consulted. GPU upload is still dirty-CPU + one `UpdateTexture2D`
  per frame (`#13`). Zoom and 5120×1440 are unchanged.
- **SETTINGS callsign/locator snapped back to N0CALL on Enter.** The
  operator ini was written through GConfig but never loaded from disk on
  the next get — `GetString` on a path that is not already cached is a
  miss, so the packaged `N0CALL`/`JN00AA` defaults came back. Enter also
  abandoned the edit when hit-rects had been cleared, and typing appended
  onto the placeholder. The operator file is now owned and reloaded; the
  first keystroke replaces the default; commit no longer depends on the
  hit-rect row list.
- **Natural Earth embed** after `#11`. The bundled `regions.json` extract
  lived under a `data/` path that `.gitignore` swallows, so `go test ./...`
  failed with `pattern data/regions.json: no matching files found`. The
  extract now sits beside the package, matching `cty.dat`.
- **Sensor-look materials on UE 5.8** after `#11` / `#8`. The rebuild
  script used `BlendableLocation.BL_AFTER_TONEMAPPING`, which 5.8 renamed
  to `BL_SCENE_COLOR_AFTER_TONEMAPPING`. Resolve that slot (with a
  getattr fallback) so bloom stays in the remapped scene color. Leftover
  transient rebuild maps are reused instead of `NewLevel` refusing an
  existing destination.

### Added

- **Submarine cable hover names the route.** Hovering a cable LineString
  shows the TeleGeography name, planned vs in-service class, computed
  route length in km, and landing names when a landing sits near a
  kept segment endpoint. Landing-point hover is unchanged. The public
  GeoJSON has no utilization or capacity; those fields are not shown.
- **Submarine cables follow TeleGeography routes.** `geography.cables`
  keeps the MultiLineString (decimated for GPU), and the client draws
  LineString / MultiLineString on the sphere shell — not a first↔last
  great-circle chord. Landings stay Points. Color is operational: planned
  cables (TeleGeography `#939597`) amber; in-service cables by route
  length (short teal / regional cyan / ocean blue / trunk magenta).
  Overlay **CABLES** or `C`; legend under that row. Attribution unchanged
  (CC BY-NC-SA 3.0, removable `data/cables/` cache).
- **Area geometry on the globe.** The client now parses and draws
  Polygon/MultiPolygon envelopes: one batched translucent fill plus a
  brighter outline on the sphere shell, matching Point and GreatCircle.
  Toggle with `Y` or the overlay AREAS row. Not one Actor per polygon.
- **NHC forecast cones.** `weather.nhc` fetches the official 5-day
  track-uncertainty KMZ linked from each storm in `CurrentStorms.json` and
  emits `weather.storm.cone` Area events next to the existing centre
  points. Centres stay Points; the cone is linked with a `forecastFor`
  relationship. NOAA NHC, public-domain US Government work; NOAA does not
  endorse this project.

- **OpenSky OAuth2** (`#9`). The retired HTTP basic-auth path is gone.
  `clientId`/`clientSecret` (or an OpenSky `credentials.json`) live only in
  gitignored `local.json`. Without credentials the source keeps running
  anonymously. Tokens are cached and refreshed before expiry; a 401 retries
  once.
- **New geospatial sources** (`#10` and follow-on catalog). Launch Library 2,
  Open-Meteo, Natural Earth regions, TeleGeography submarine cables, NASA
  EONET, GDACS, gpsjam.org GNSS interference, OpenAQ (disabled until keyed),
  IMF PortWatch chokepoints and recent disruptions, NOAA NHC tropical-cyclone centres, Launch
  Library 2 Earth spaceports, and HDX HAPI displacement (disabled until an
  app identifier is supplied). Each is a Go plugin against the upstream
  API. Attribution credits those providers. Inspired by public OSINT
  dashboards as a catalog of which providers exist; every plugin talks to
  the upstream API.
- **Smooth marker motion** (`#7`). Markers interpolate the last two fixes in
  `M_MarkerIcon` (velocity × time WPO) instead of hopping 1.6 km every two
  seconds. `headingprobe` now also serves a mover, a turner, and a hover.
- **Sensor looks** (`#8`). FLIR white-hot / black-hot, Ironbow, NVG, and CRT
  post-process modes on **F1–F6** and a SETTINGS row. Persist to
  `IonOperator.ini`. Missing materials are a no-op.

- **Motion trails.** Aircraft, satellites, and anything else that reports a
  moving Point now leave a short, fading comet tail along the globe surface
  behind them, built purely from repeated sightings of the same entity — a
  stationary ground station or ionosonde never grows one. Bounded to the
  busiest 300 currently-updating entities and 12 points each, thin and
  quick to fade by default so it stays a subtle wake rather than turning a
  busy globe into spaghetti. Toggle with `T` or the overlay menu's TRAILS row.
- **AIS ships (`ais.aisstream` + `maritime` domain).** Global vessel traffic
  from aisstream.io's free WebSocket feed joins static/voyage data (name,
  type, call sign, destination, ETA) onto each vessel's position stream from
  a bounded per-MMSI cache, classifies and excludes non-vessel AIS
  participants (base stations, aids to navigation, SAR aircraft), and
  recognises every AIS "not available" sentinel (position, speed, course,
  heading, rate of turn) instead of rendering them as real values. Ships
  **disabled by default** — it needs a free API key the operator must obtain
  themselves; see
  [DATA-SOURCES.md](docs/DATA-SOURCES.md#enabling-ais-ships-aisstreamio).
  **The live connection is unverified**: built and unit-tested entirely
  against fixtures from aisstream.io's published documentation and schema,
  with no API key available in the environment this was built in.
- **Flight routes in the aircraft tooltip.** Aircraft seen by an
  `aviation.adsb` circle show their filed origin and destination in plain
  words (`CDG Paris  >  TUN Tunis`), resolved per callsign via adsbdb.com —
  cached, globally rate-gated, and switchable off with `routeLookup: false`
  on the source.
- **Close-range zoom.** The orbit camera now goes down to ~255 km above the
  surface (previously ~2900 km), with wheel steps that scale with distance so
  the last stretch is fine-grained instead of one overshooting notch. Markers
  and the own-station reticle keep a constant screen size through the whole
  range, so individual aircraft separate cleanly on an approach.
- **Deep zoom: 21600x10800 day/night imagery, a generated 200x400-segment
  globe mesh, and a much closer orbit.** Blue Marble and Black Marble now
  fetch at NASA's largest single-file resolution (up from 4096x2048) and
  import as Streaming Virtual Textures, so the close-up view is genuinely
  sharp without holding the full texture in VRAM. The engine's placeholder
  sphere primitive is replaced by a mesh generated from
  `unreal/Scripts/generate_globe_mesh.py` (reproducible, not hand-authored)
  fine enough that the limb and the terrain stay smooth instead of faceted.
  The wheel zoom's near clamp drops from ~255 km to ~32 km above the surface,
  with a correspondingly closer near clip plane so nothing clips. See
  `docs/DATA-SOURCES.md` and `unreal/SourceAssets/NASA/ATTRIBUTION.md` for
  what resolution is and is not available under a licence this project can
  use, and the caveat about markers not yet re-tuned for the closest part of
  the new range.

### Fixed

- **Far zoom at 5120×1440 no longer GPU-crashes the packaged client.** Detail
  imagery composited each tile by calling `UTexture2D::UpdateResource()`,
  which destroys and recreates the 4096×2048 mosaic (and the elevation
  mosaic) on the D3D12 Copy Engine. At wall resolution the first close-orbit
  window is level 5 and a cached region lands tens or hundreds of tiles in
  one go; the next copy then read a resource that had already been released
  — Aftermath reported PageFault / AddressTranslationError on CopyEngine,
  not VRAM exhaustion. Tiles now dirty the CPU window only; one in-place
  `UpdateTextureRegions` copy runs per mosaic per frame, skipped when the
  buffer is invalid and deferred while a copy is still in flight. Max
  resolution stays 5120×1440 and zoom is not capped.
- **A transport blip no longer stalls route lookups for five minutes.** The
  single route-lookup worker treated any fetch error like a rate limit; a DNS
  hiccup or dropped connection now pauses it only 15 seconds, while a real
  429/420 honours the server's `Retry-After` (five-minute fallback), as the
  data-sources documentation promises. ([#5](https://github.com/svabi79/ion-command/issues/5))

## [0.9.2] — 2026-07-26

**The installer published with 0.9.1 was defective and should not be used.** It
contained the 0.9.0 client, so none of the 0.9.1 client-side changes were in it.
The collector and launchers in it were correct. This release ships the payload
0.9.1 was meant to ship, plus the fix below.

### Fixed

- **Settings and overlay rows stayed clickable while the HUD was hidden.**
  `DrawHUD` returns early when the HUD is hidden or still fading in, and those
  paths left the panel's hit rectangles in place while clicks were still routed
  to them. A click near the middle of the screen with the HUD switched off could
  silently change a setting — hide most aircraft, flip the orbit axis, or enter
  text-entry mode — with nothing on screen to explain it, and the change
  persisted. Hit rectangles are now dropped whenever the HUD does not draw, and
  clicks fall through to the world instead. ([#2](https://github.com/svabi79/ion-command/issues/2))

### Build

- `tools\installer\build-installer.ps1` staged the client from `dist\release`,
  while `tools\package.ps1` archives to `dist\windows`. The stale directory
  passed the existence check, which is how 0.9.1 shipped the wrong client. The
  paths now match, and the script prints both binary timestamps and refuses a
  client older than the collector it is packaged with.
- `tools\package.ps1` empties the archive directory first. BuildCookRun adds to
  it rather than replacing it, so a 340 MB Development binary from an earlier
  build was being carried into the installer.

## [0.9.1] — 2026-07-25

A maintenance release, driven by what actually broke for people who installed
0.9.0. Thanks to [@die-Anna](https://github.com/die-Anna) for finding and fixing
the startup failure.

### Added

- **INVERT ORBIT Y** settings row: optionally flip the vertical orbit
  direction of a right-mouse drag. Persisted to `Game.ini` under
  `[IonCommand.Input]`, applied live.
- Collector `-listen` flag to override `server.listenAddress` from the command
  line, so launchers can move to a fallback port without editing the config.

### Changed

- Vertical orbit now follows the same convention as horizontal orbit by
  default; the previous direction is available via **INVERT ORBIT Y**.

### Fixed

- **Installed builds could start with a dead collector.** Windows reserves TCP
  port ranges for Hyper-V/WSL NAT; when 7810 fell inside one, the collector
  exited instantly (`bind: WSAEACCES`) and the launcher started the client
  anyway, with the only warning hidden in an invisible console. The launchers
  now probe the port first, fall back to 17810/27810, detect an early collector
  exit, stop a candidate that never becomes healthy before trying the next port,
  point the client at the working port, and raise a visible message when the
  collector really cannot start.
- Replay honours the `-IonCollectorUrl=` override like the live stream does,
  so **R**/**L** keep working when the collector runs on a fallback port.
- The collector reported its version as `0.1.0-bootstrap` in the startup log
  and on `/api/status`; it now reports the release version.

## [0.9.0] — 2026-07-21

First packaged and published release. Windows x64, pre-1.0.

### Added

- **Windows installer** (Inno Setup): Shipping client without debug symbols,
  Go collector, neutral default configuration, launcher, Start Menu entries and
  an uninstaller; verified install → run → uninstall.
- **Settings panel** in the client (overlay menu → `SETTINGS`): callsign and
  grid locator editable in-app, plus marker lifetime, minimum flight level and
  show-ground-aircraft. Persisted to `Game.ini`, applied live.
- **Aviation declutter**: hide aircraft below a configurable flight level
  and/or on the ground.
- **`MY RX/TX ONLY`** overlay-menu row (equivalent to the `M` key): show only
  paths where the own station is transmitter or receiver.
- **Global aviation** via a new `aviation.opensky` source (one worldwide
  snapshot per request), alongside regional `aviation.adsb` circles.
- **Airframe-type pictograms** (airliner, helicopter, glider, balloon, drone),
  heading-oriented glyphs, dead reckoning between updates, and emergency-squawk
  highlighting for 7500/7600/7700.
- **Live cloud imagery** from the EUMETSAT world infrared composite, refreshed
  hourly, replacing a static climatology texture.
- **Real star sky**: NASA SVS Deep Star Map rotated to Greenwich Mean Sidereal
  Time.
- **Retained state**: the latest message per entity is replayed to every client
  on connect, so the globe fills in seconds instead of waiting for a slow poll.
- **Marker hover tooltips** and a clickable **overlay menu** with per-layer and
  per-domain visibility.
- **Always-visible own-station reticle** drawn in HUD space.
- Data sources added over the cycle: Blitzortung lightning, USGS earthquakes,
  CelesTrak satellites (SGP4), GOES X-ray class, GIRO/KC2G ionosonde soundings,
  Reverse Beacon Network, adsb.lol aircraft.
- Documentation set: README, `docs/COMPONENTS.md`, `CONFIGURATION.md`,
  `DATA-SOURCES.md`, `BUILDING.md`, `TROUBLESHOOTING.md`, plus
  `THIRD_PARTY_NOTICES.md`.

### Changed

- The diegetic deck chrome (floating title and three console panels) is hidden
  by default; `-IonShowDeck` restores it.
- Own-station marker scales with camera distance instead of a fixed size, so
  zooming in no longer turns it into a bloom.
- Anonymous OpenSky polling relaxed to 1800 s to stay inside the credit budget.
- Client queue capacity raised above the retain cap so a connecting client
  cannot receive a truncated snapshot.
- Licence clarified: the MIT grant covers the project's own code; bundled
  third-party content and live data are governed separately.

### Fixed

- **Stationary markers were invisible.** The heading billboard normalised a
  zero heading to NaN, which survived the blend, collapsing every marker
  without a course (stations, satellites, lightning, quakes, sounders) into a
  degenerate quad.
- **CelesTrak retry loop.** A failed fetch left the element set empty, so the
  position ticker refetched every 10 s indefinitely — contrary to CelesTrak's
  usage policy. Now exponential backoff with a hard stop.
- **ADS-B rate limiting.** Five regional queries from one address tripped
  throttling; added a global request gate and backoff for HTTP 429 and
  adsb.lol's 420.
- **Frozen aircraft.** Movement was compared against the continuously updated
  bookkeeping position instead of the rendered one, so markers never moved.
- **Emergency squawk could be cleared** by a later sighting from another source
  without squawk information; the alarm state is now sticky.
- **Georeference.** Stations rendered a mirrored quarter turn off; the world
  frame is now pinned by a test.
- Sun and star sphere now use the same IAU-1982 sidereal time.
- Ghost tooltips on expired markers; stale overlay-menu click targets while the
  HUD was hidden; polar collapse of the compass heading frame.
- Retracted an incorrect renderer benchmark: the original figures were a
  frame-counter wrap artefact.

### Security

- The collector binds `127.0.0.1` only and has no authentication; this is
  documented rather than assumed.
- Maintainer callsign and home locator removed from tracked configuration.

[Unreleased]: https://github.com/svabi79/ion-command/compare/v0.9.2...HEAD
[0.9.2]: https://github.com/svabi79/ion-command/releases/tag/v0.9.2
[0.9.1]: https://github.com/svabi79/ion-command/releases/tag/v0.9.1
[0.9.0]: https://github.com/svabi79/ion-command/releases/tag/v0.9.0
