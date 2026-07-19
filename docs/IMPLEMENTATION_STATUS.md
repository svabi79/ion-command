# Implementation status

Status date: **2026-07-19** (fourth pass: cockpit stage 1 — screen-space
instrument HUD from data already flowing).

## Cockpit stage 1 (2026-07-19)

Stage 1 of the mission-control cockpit ("instruments from data we already
have"; stage 2 = DXCC + forecast, stage 3 = GIRO/path analysis/heatmap):

- **`AIonCockpitHudActor`** (IonCommandUI, Canvas-drawn `AHUD`, no assets):
  - **Status bar**: UTC + timeline mode (LIVE/REPLAY/PAUSED), link state,
    KP / FLUX / A / WIND / BZ with severity colors (`--` until the first
    `spaceweather.state` sample arrives), PATHS/MIN, ACTIVE / RX / DROP.
  - **Traffic panel**: per-palette bars with domain labels and counts from the
    new generic `AGeoArcLayerActor::GetPaletteBreakdown` API; the band-preset
    focus (keys 1-9) highlights its row and dims the rest.
  - **Path-rate panel**: 60 one-second buckets as a sparkline.
  - **Polar dial**: auroral-oval instrument using the same
    `71 - 2.2 * Kp` expansion as the 3D ovals.
  - **On-globe labels**: the busiest *visible* relationship endpoints
    (display.from/display.to aggregation, weight decay, far-side candidates
    yield their slots to visible ones — at 06 UTC the global top list can be
    entirely on the far hemisphere) plus both endpoints of the selection.
  - **Tab navigation**: TAB cycles FULL → MINIMAL (status bar only) → OFF.
  - Aggregation is bounded (4096 endpoint entries, 120 s retention) and
    cadenced (0.5 s); per-frame work is drawing plus one 60-bucket sum.
- **Modularity**: the UI module stays ham-free. Palette labels/panel title come
  from `ResolvePaletteLabel`/`ResolveTrafficPanelTitle` virtuals that the
  HamRadio layer overrides ("BAND ACTIVITY", "20M", "15M/12M", "OTHER");
  station labels are generic `display.*` properties.
- **Collector**: the SWPC source now fills `aIndex` from the wwv.txt
  geophysical alert message ("estimated planetary A-index N", verified against
  the live product; degrades to null), unit-tested with the real format.
- **Automation screenshots** now capture the UI layer
  (`RequestScreenshot(..., bShowUI=true, ...)`), so `first-light.ps1` proves
  the cockpit.

Verification: `go test ./...` + `go vet` green; editor target compiles; mock
First Light (40 links/s, space weather every 10 s) captured the full cockpit —
KP 2.0 / FLUX 234 / A 33 / WIND 672 / BZ -2.9 populated and severity-colored,
all bands with counts, even sparkline, polar dial "KP 2.0 // OVAL 67 N",
endpoint labels on the globe, DROP 0, zero game-log errors. An earlier run
against a stale live collector (~1,150 events/s PSKReporter) also rendered the
cockpit with PATHS/MIN 25,887; its space-weather columns showed `--` because
SWPC polls every 5 min and the live socket only streams new samples — a
last-state snapshot on connect is a known follow-up.

## Phase 2 second slice (2026-07-19, packages A-D)

- **Packaged build**: `tools/package.ps1` (Development by default, `-Config`
  selectable) produces `dist/windows/IonCommand.exe`; the final live capture
  and the benchmark both ran from the packaged client.
- **GPU age fade**: arc instances carry spawn time and 1/lifetime as
  per-instance custom data; the signal material fades emissive and opacity on
  the GPU. CPU expiry rebuilds only once >10% of the list is stale;
  `MaxVisibleArcs` 10,000.
- **Benchmark** (`tools/bench.ps1`, 600 mock spots/s, packaged client at
  5120x1440 on the RTX 4090 workstation): 24 segments/arc = 6.5 fps avg;
  16 segments/arc = 10.3 fps avg (one change per measurement). 10k additive
  ISM arcs are overdraw-bound; the 60 fps target requires the Niagara/GPU
  renderer, which stays the top Phase 2 item.
- **Own station**: `[IonCommand.Station]` in DefaultGame.ini (Callsign,
  Locator; default N0CALL/JN00AA), pulsing halo marker on the globe, M toggles
  a generic entity filter (only links touching the own station). The
  repository modularity gate caught a first draft that leaked `callsign` into
  the generic arc renderer; the filter is now entity-id based, with the
  ham-specific id construction in the HamRadio module.
- **WSJT-X UDP source** (`wsjtx.udp`, default 127.0.0.1:2237): QDataStream
  header/status/decode parser, sender callsign+grid extraction from FT8/FT4
  text (RR73-vs-grid handled), grid cache for non-CQ decodes, band from
  frequency. Unit tests plus a synthetic-datagram end-to-end run (2 decodes →
  4 canonical events, 0 invalid). Disabled by default in live.json.
- **NOAA SWPC source** (`spaceweather.swpc`): polls Kp, 10.7cm flux, solar
  wind speed and IMF Bz every 5 min (configurable `pollSeconds`, hard floor of
  1 min); parses the real product formats (verified against the live API);
  missing summaries degrade to null. The aurora ovals now expand equatorward
  and brighten with live Kp.
- **Operator controls**: 1-9 exclusive band presets (0 = all, pure component
  visibility), R replays the last 15 minutes, comma/period halve/double replay
  speed (0.25x-10x), boot fade-in with diegetic deck boot lines (never blocks
  input). Deviation from the original key spec: R starts replay (the collector
  records continuously anyway).
- **Config hardening**: `config.Load` no longer element-merges the default
  source list into configured sources (a Go `json.Unmarshal` slice pitfall
  that briefly made the SWPC poller inherit `eventsPerSecond: 40` and hammer
  NOAA at 20 req/s until caught in the live check; regression-tested).

## Phase 2 progress (2026-07-19)

- **Real data**: `pskreporter.mqtt` source plugin (public broker
  `mqtt.pskreporter.info:1883`, topic `pskr/filter/v2/#`) with injected decoder
  behind the prepared adapter boundary. Maidenhead 4/6/8-char conversion with
  unit tests; spots without usable locators are skipped, malformed frames are
  logged and dropped without killing the source. Live validation: ~480
  spots/second sustained, 0 invalid, 0 drops, queue depth ≤1
  (`collector/configs/live.json`).
- **Visual overhaul** (verified through repeated live captures):
  - Earth master material rebuilt: city lights masked to the true night side
    via a SunDirection parameter fed by the globe actor each tick, soft
    terminator, glossy oceans/matte land derived from the day albedo.
  - Fixed-histogram exposure (AEM_Manual drowned the stylised scene), soft
    wide bloom, gentle vignette and fringe on the camera.
  - Arc geometry: max apex height cut from 650 to 165 units — tall arcs read
    as orbit rings, low arcs read as propagation paths; 24 segments per arc;
    thin beams (2 units) that glow through bloom.
  - Fresnel-rim hologram shell master (atmosphere halo, ionosphere shells);
    ionosphere starts hidden (I toggles) to keep the composition clean.
  - Aurora rebuilt as 320 flat overlapping segments instead of 180 pearls.
  - Firehose-rate rendering: eviction no longer rebuilds per submit; rebuilds
    are coalesced to at most one per frame and use bulk `AddInstances` per
    palette component. `MaxVisibleArcs` 2400.
- Master materials are now rebuilt deterministically on every script run (the
  script switches to a transient level first; editing a live material's
  expressions asserts in UE 5.8).

## Outcome

Phase 0 is implemented and verified, and the Phase 1 First-Light acceptance
capture has been produced from the live pipeline: collector streaming mock
traffic over WebSocket into the game client, thousands of canonical events
accepted with zero drops, and a native 5120x1440 screenshot showing several
hundred band-coloured arcs, station markers, aurora, and the command deck
(`unreal/Saved/Screenshots/Reference/ION_COMMAND_Live.png`, reproducible via
`tools/first-light.ps1`). A packaged client has not been produced yet.

## Implemented

### Platform and contracts

- generic versioned Entity/Observation/Relationship/Track/Area/Field/Volume/
  Annotation envelope;
- Point and GreatCircle geometry, UTC/WGS84 invariants, schemas and samples;
- plugin manifests for source, domain, layer, context, and tool categories;
- ADRs, build scripts, validation, environment record, architecture, visual
  design, performance plan, and development guide.

### Collector

- compile-time plugin registry suitable for Windows, with portable interfaces;
- bounded concurrent pipeline and per-client backpressure;
- synthetic radio, lightning, and space-weather feeds;
- ham-radio, weather, and space-weather normalizers;
- a real cross-domain bootstrap context processor combining a radio link with
  the latest space-weather state and clearly marking confidence/model metadata;
- prepared PSKReporter integration boundary with injected transport and decoder;
- HTTP health, status, stats, sources, and replay-range APIs;
- live and replay WebSockets;
- hourly UTC JSONL recording and time-scaled ordered replay;
- structured JSON logs and atomic runtime counters.

### Unreal source foundation

- six C++ modules with a domain-free core and separate Ham Radio module;
- canonical JSON parser, bounded MPSC import queue, batched game-thread import,
  reconnect, bounded active history, timeline, replay switching, selection, and
  generic context queries;
- pause-safe shared time semantics: live sun/Grayline state freezes on pause and
  replay arc aging follows replay time rather than the wall clock;
- tested-source geospatial algorithms for Maidenhead, great-circle distance,
  bearing/interpolation, solar subpoint, daylight, and Grayline distance;
- runtime globe, UTC sun, conceptual ionosphere shells, procedural aurora ovals,
  instanced point markers, reusable instanced arc renderer, and ham-radio band
  style specialization;
- orbital mouse camera and a physical three-part world-space command console;
- click selection without per-event collision bodies, a dedicated persistent
  selected-path render batch, and a spatial details instrument showing endpoint
  identity, UTC, representation, distance, initial azimuth, and Grayline
  proximity;
- selection staging: non-selected traffic dims while a path is selected, both
  endpoints receive bright markers, and the camera eases toward the selected
  link (interruptible by manual orbit; re-triggered with F);
- I toggles the conceptual ionosphere shells at runtime;
- an unattended in-game capture path (`-IonScreenshotAfter/-IonScreenshotFile/
  -IonExitAfterScreenshot`) plus `tools/first-light.ps1`, which builds, starts
  the collector, waits for health, runs the client at native resolution, takes
  the screenshot, scans the log for errors, and shuts everything down;
- idempotent source-texture, material, data asset, level, validation, and
  screenshot scripts, including documented NASA Blue Marble attribution;
- deterministic editor First Light with a saved showcase camera and bounded
  generic preview data, plus a PlayerStart-backed ultrawide PIE composition.

## Verification performed

| Check | Result |
|---|---|
| `go test ./...` | passed across all packages |
| collector build | passed; `collector/bin/ion-collector.exe` produced |
| running `/api/health` | `ok` |
| running sources | all three `active` |
| final process smoke | 100 radio links observed, 515 canonical published, 0 invalid, 0 client drops, 3 sources active |
| live WebSocket | canonical message received |
| recording discovery | one hourly file, replay range available |
| replay WebSocket | first three messages replayed in entity/entity/link order |
| automated collector smoke | `tools/smoke.ps1`: health, 100 live radio links, all sources active, recording and replay |
| modular contract tests | passed: second source instance, domain-neutral lightning point, generic satellite track, cross-domain context, and byte-preserving replay of an unknown semantic type |
| repository JSON/Python/module validation | passed: 16 JSON files, 10 unique plugin/layer IDs, JSONL, Python syntax, reflection headers, module boundaries |
| `go vet ./...` | passed |
| Unreal compile/UHT | passed: `IonCommandEditor Win64 Development`, UE 5.8, MSVC 14.44, SDK 26100 |
| Unreal Editor automation | passed: materials, band data asset, command-deck level, and project validation |
| Unreal Automation Tests | passed: GreatCircle, Maidenhead, and CanonicalEnvelope |
| screenshot/visual QA | passed at 5120 x 1408 editor and PIE capture: full-resolution globe/starfield, bounded paths, live telemetry panels, no viewport project warnings |
| live Unreal/collector integration | passed in PIE: one WebSocket client connected, live stats advanced, `LIVE LINK // CONNECTED` rendered |
| First-Light live capture (2026-07-19) | passed: collector + `-game` client at 5120x1440, 8,141 events accepted / 0 drops after 50 s, hundreds of band-coloured arcs and station markers rendered, `LIVE LINK // CONNECTED`, 0 error lines in the game log |
| Unreal Automation Tests (final binaries) | passed: GreatCircle, Maidenhead, CanonicalEnvelope |
| Instanced-mesh render path | switched HISM to `UInstancedStaticMeshComponent` for arcs, points, and aurora; live capture confirms immediate rendering without cluster-tree latency |

The recorded rate above is a short functional smoke measurement, not a load
benchmark and not an Unreal frame-performance result.

The optional Go race detector was not run because this Windows Go installation
has CGO disabled and no compatible GCC toolchain. `tools/test.ps1 -Race` now
fails explicitly unless that prerequisite is supplied; ordinary tests do not
depend on CGO.

## Known limitations

- No actual PSKReporter transport/decoder is implemented because the subscribed
  feed framing and credentials were not placed in the workspace. The adapter
  boundary exists and does not block Mock First Light.
- Arc rendering currently uses shared HISM cylinder segments. It respects the
  no-Actor-per-event rule but is an intermediate path before Niagara Data
  Channels/custom GPU buffers.
- Point instance eviction currently clears a full class at its cap; production
  LOD needs stable IDs, clustering, and age-based GPU expiry.
- Selected-path hit testing and the detail instrument compile successfully, but
  click-selection behavior remains manually unverified. Selection dimming,
  endpoint markers, and the camera focus ease are implemented but likewise
  await manual PIE confirmation; a richer spline/particle treatment remains.
- The editor `take_high_res_screenshot` path drops every translucent/additive
  layer when supersampling above the viewport size (also with
  `r.SeparateTranslucency 0`), so the editor reference capture shows only
  opaque geometry. The authoritative First-Light reference is the native-
  resolution in-game capture from `tools/first-light.ps1`.
- The live capture ran at roughly 13 fps (frame 644 after 50 s) in the
  unoptimised editor-binary `-game` mode at 5120x1440 on the workstation. This
  is a functional smoke measurement, not a performance result; profiling and
  the packaged client remain Phase 2 work.
- The atmosphere and ionosphere are still conceptual shells; their visual
  hierarchy is verified, but cloud motion and physically based scattering remain.
- Earth now uses NASA day and night mosaics and the scene has a deterministic
  starfield. Final reflections, typography, responsive HUD treatment, and audio
  remain.
- The bootstrap HF context annotation is explicitly a low-confidence temporal
  join, not a propagation prediction.

## Exact start instructions

Collector now:

```powershell
.\tools\build.ps1
.\tools\run-collector.ps1
```

With the installed UE5.8 and VS2022 toolchain:

```powershell
$env:ION_COMMAND_UNREAL_ROOT = 'D:\Epic Games\UE_5.8'
.\tools\build.ps1 -Unreal
.\tools\run-editor.ps1
```

Start the collector before Play in Editor. Right mouse orbits, the wheel zooms,
left mouse selects a path, Escape clears selection, F re-centers the camera on
the selection, I toggles the ionosphere shells, Space pauses the shared
timeline, and L returns to live.

Unattended First-Light proof (build, collector, live client, screenshot, log
scan, teardown in one command):

```powershell
$env:ION_COMMAND_UNREAL_ROOT = 'D:\Epic Games\UE_5.8'
.\tools\first-light.ps1
```

## Next three concrete steps

1. Exercise click selection (including the new dimming, endpoint markers, and
   camera ease) and live/replay transitions manually in PIE, then fold the
   `first-light.ps1` capture into an automated screenshot comparison.
2. Implement the Niagara Data Channel arc adapter plus GPU age fade and
   aggregation, then run a repeatable 10,000-arc performance benchmark in a
   packaged Development build (the 13 fps editor-binary figure is the baseline
   to beat).
3. Add the real subscribed PSKReporter transport and decoder behind the prepared
   interface, using sanitized captured fixtures, deduplication, reconnect tests,
   sustained-rate recording tests, and secrets outside the repository.
