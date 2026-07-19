# Implementation status

Status date: **2026-07-19** (eleventh pass: live lightning, scattering-look
atmosphere).

## Live lightning and atmosphere (2026-07-19)

- **Blitzortung.org source** (`lightning.blitzortung`): streams the community
  network's websocket fan-out (ws1/ws7/ws8, rotated, reconnect with backoff;
  data courtesy of Blitzortung.org and its station operators, non-commercial
  use). Frames arrive LZW-compressed with the project's dictionary scheme;
  the decoder is regression-tested against a captured live frame. Strikes
  flow through the *existing* weather domain untouched by design — the
  platform's modularity claim held: a real feed replaced the mock without any
  core or client change. Live: 1,895 strikes recorded in the first ten
  minutes, sky-to-disk latency ~4.5 s. One-shot observations now expire
  after 30 s in the point layer (entities keep 300 s), so strikes flash and
  fade while stations persist.
- **Scattering-look atmosphere** (`M_AtmosphereScatter`): the Fresnel shell's
  color now follows the sun angle — Rayleigh blue on the day side, a warm
  terminator band, near-black night — driven by the same SunDirection the
  Earth material uses. Explicitly an artist's model, not radiative transfer.
- **Fix**: the PSKReporter dedupe window was not thread-safe (the MQTT
  adapter decodes concurrently) and crashed the collector with "concurrent
  map writes" on startup; now locked.
- **Capture gate v2**: the neon metric is split by dominant channel — healthy
  scenes always contain green-dominant beams (band palette), while the gray
  default-material failures only had the blue atmosphere rim. Re-calibrated
  against four real captures (two healthy, two broken).

### Data roadmap (candidates, not commitments)

The envelope/domain pattern makes these cheap to add: USGS earthquakes
(GeoJSON feed -> geophysics domain), satellite TLE ground tracks
(CelesTrak -> Track messages), NOAA GOES X-ray flux (flare alerts for the
conditions panel), aircraft via local ADS-B (Argus link), reverse-beacon
network CW/RTTY spots, and WSPR challenge data. Each needs a live format
probe first, per the house rule.

## Cockpit polish (2026-07-19)

- **Retained-state snapshot on connect**: the hub keeps the latest message
  per configured semantic type and entity (`pipeline.retainLatest`, bounded)
  and replays that snapshot to every new live client before the stream, so
  KP/FLUX/A and the sounding set are populated seconds after a client starts
  instead of staying `--` until the next 5/10-minute poll. Generic and
  config-driven; unit-tested (latest-wins, transient types not retained,
  bounded map). Live-proven: a fresh client showed full space weather within
  90 s of launch.
- **DROP vs EVICT**: window-cap evictions (bounded active history working as
  designed at firehose rates) are now `EvictedMessages`, shown dim in the
  status bar and deck; `DROP` only counts real backpressure and stays green
  at zero. Live: DROP 0 / EVICT 40k after one minute at ~800 msg/s.
- **Region flags**: the top-regions panel draws simplified canvas flags
  (stripes, Nordic/plain crosses, discs, US canton) for ~50 of the most
  active DXCC regions, keyed by the generic display-region name with a
  text-only fallback.

## Georeference fix (2026-07-19)

Operator report: a German station rendered over Siberia. Root cause: the
engine BasicShapes sphere maps its equirectangular texture with U starting at
world longitude -90 and running **westward** (measured with the new
`Scripts/probe_sphere_uv.py` diagnostic), so the terrain visible at classic
right-handed longitude L is `90 - L` — mirrored plus a quarter turn. All
geographic content (arcs, markers, labels, own station, subsolar point) used
the right-handed `X = cos(lon), Y = sin(lon)` frame; everything was mutually
consistent and the terminator is computed from world normals, so nothing
looked obviously wrong until callsigns were checked against terrain.

Fix: `LatitudeLongitudeToUnitSphere`/`UnitSphereToLatitudeLongitude` now use
`X = sin(lon), Y = cos(lon)` (and `atan2(X, Y)`), anchoring the world frame
to the rendered Earth: Greenwich at +Y, 90E at +X. A new
`IONCOMMAND.Core.Geo.SphereFrame` automation test pins the convention to the
measured mapping. All spherical math (distance, bearing, interpolation,
Grayline) works in the lat/lon domain and is unaffected.

Verified: Core.Geo automation tests green; live capture at 09:45 UTC shows
the Americas night side under the default camera with K9/VE3/W4 callsigns
and an `SWL/FN11OD` locator label over northeastern North America and PT5PR
over southern Brazil. Lesson recorded: "First Light" passes never included a
known-landmark check — station-vs-terrain is now part of visual QA.

## Cockpit stage 3 (2026-07-19)

- **Ionosonde source** (`ionosonde.kc2g`): polls the GIRO station snapshot at
  prop.kc2g.com/api/stations.json (data courtesy of the Global Ionosphere
  Radio Observatory; 10-min default poll, 5-min hard floor). The feed's traps
  are handled and regression-tested against a captured live fixture: string
  coordinates, 0-360 longitudes, months-stale entries for silent stations
  (2-hour age gate), autoscaler nulls, and repeated readings (per-station
  dedupe). A new generic `ionosphere` domain normalizes soundings to
  per-station `ionosphere.sounding` observations (foF2, MUF(3000), hmF2, foE,
  M(3000) factor, TEC, confidence) that render as station markers via the
  generic point layer. `mock.ionosonde` cycles eight plausible sites so the
  instruments verify offline.
- **PATH ANALYSIS panel** (HamRadio module, explicitly heuristic): for the
  selected link it derives hop count (max ~3,500 km per F2 hop), judges every
  hop's reflection midpoint by the nearest fresh sounding within 3,000 km,
  interpolates the hop MUF from foF2 and the M(3000) factor, and reports the
  controlling minimum plus a SUPPORTED / MARGINAL / ABOVE F2 MUF verdict
  against the link frequency, with honest coverage ("3/4 HOPS JUDGED") when
  sounder geometry leaves gaps.
- **Activity heatmap**: a generic globe overlay of decaying endpoint density
  (72x36 grid, bounded splat budget, sqrt contrast, soft additive splats from
  the scripted `M_ActivityHeat` master). Hidden by default, H toggles it.
- **Automation hooks**: `-IonAutoSelectAfter=<s>` selects the first active
  path unattended and `-IonHeatmapVisible` shows the heatmap, wired into
  `first-light.ps1 -AutoSelect -ShowHeatmap` so captures prove the selection
  staging, the path panel, and the heatmap without an operator.

Verification: collector build/tests/vet green (live-fixture tests for the
KC2G source and ionosphere domain), editor build clean, First Light on port
7811 with both switches: PATH ANALYSIS showed "HOPS 4 x 2820 KM / MUF EST
18.0 MHZ VIA BC840 / COVER 3/4 HOPS JUDGED / LINK 7.074 MHZ / SUPPORTED",
HF CONDITIONS correctly degraded two steps under the mock's A=33, the heatmap
rendered over the traffic clusters, zero game-log errors.

## Renderer performance (2026-07-19)

**Correction:** the earlier packaged-build figures (24 seg = 6.5 fps, 16 seg =
10.3 fps, "overdraw-bound, 60 fps requires the Niagara renderer") were
measurement artifacts. The benchmark parsed the log line's own frame column,
which wraps at 1000 frames — the real counts were ~1000 frames higher. The
harness now logs `GFrameCounter` explicitly and `bench.ps1` parses that.

Honest same-session series (12,000-arc cap, 600 mock spots/s, packaged
Development client at 5120x1440 on the RTX 4090, one change per measurement):

| Configuration | avg fps |
|---|---|
| Cylinder segments, engine-default lens flare (yesterday's shipped state) | 80.3 |
| Cylinder segments, lens flare off | 86.9 |
| **Cube segments (12 tris), lens flare off (shipped)** | **125.3** |

- The arc segment mesh is now the engine cube: at a 2-unit beam width under
  bloom the cross-section is invisible (stress-ball A/B captures are visually
  identical), while the smooth BasicShapes cylinder cost three orders of
  magnitude more triangles. The renderer was primitive-bound, not
  overdraw-bound.
- Lens flare stays off (see the hotspot ghost fix); it was worth ~8%.
- The 60 fps target is exceeded 2x at the 12k-arc cap. The Niagara Data
  Channel renderer (ADR 0003) is no longer required at this scale and is
  deferred until a concrete need (e.g. far larger arc counts or per-particle
  motion) appears.

## Cockpit stage 2 (2026-07-19)

- **DXCC regions end to end**: the PSKReporter decoder now carries the
  broker's `sa`/`ra` ADIF DXCC entity codes (verified against live frames;
  optional, nil-safe), and the ham-radio normalizer resolves them through a
  generated entity table (`dxcc_gen.go`, 403 entities incl. deleted,
  regenerable via `tools/gen-dxcc.py` from the ADIF 3.1.5 spec) into generic
  `display.fromRegion`/`display.toRegion` properties plus numeric
  `txDxcc`/`rxDxcc`. Code 0 ("not within any entity") stays unnamed; unknown
  codes fall back to `DXCC <n>`. The mock radio feed emits a weighted spread
  of real codes so instruments verify without the live feed.
- **TOP REGIONS panel**: the HUD tallies the generic region properties of
  path traffic (bounded map, same 0.97 decay cadence as endpoints) and shows
  the top eight with share-of-traffic bars.
- **Cockpit panel provider API**: `UIonCockpitPanelSubsystem` lets domain
  modules contribute read-only panels (title + rows of colored cells) without
  the UI module learning their vocabulary.
- **HF CONDITIONS panel** (HamRadio module, explicitly labeled HEURISTIC):
  classic solar-widget style day/night ratings per band group from solar
  flux, degraded by geomagnetic activity (Kp >= 4 or A >= 20 one step,
  Kp >= 5 or A >= 30 two); driven by the live `spaceweather.state` samples.
- **Isolated verification**: `first-light.ps1 -Port <n>` refuses a busy port,
  patches a temp collector config (BOM-less UTF-8 — PowerShell 5.1's UTF8
  default emits a BOM that Go's JSON decoder rejects), and passes
  `-IonCollectorUrl=` to the client, so verification never hijacks an
  operator's live collector/client pair on 7810 (which is exactly what the
  first stage-1 run did, silently).

Verification: collector tests green (`sa`/`ra` fixture from a captured live
frame, region/deleted/unknown/zero cases), editor build clean, mock First
Light on port 7811 while the live pair kept running on 7810: TOP REGIONS
showed the seeded country spread (United States 23%, Germany 15%, ...),
HF CONDITIONS showed correctly degraded ratings at Kp 4.2 (30M-20M DAY FAIR),
zero log errors, DROP 0.

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
  16 segments/arc = 10.3 fps avg (one change per measurement).
  **RETRACTED 2026-07-19**: these figures were corrupted by the log frame
  counter wrapping at 1000; see "Renderer performance" above for the
  corrected series (~80 fps then, 125 fps now).
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
