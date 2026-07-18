# Bootstrap plan

## Phase 0: reproducible foundation

1. Create the repository contract: `AGENTS.md`, schemas, plugin manifests,
   configuration, ADRs, build and validation scripts.
2. Implement the collector interfaces in `collector/internal/plugins`, the
   canonical model in `collector/internal/events`, and bounded pipeline in
   `collector/internal/pipeline`.
3. Implement health/status/stats/source/replay HTTP endpoints and live/replay
   WebSockets in `collector/internal/api`.
4. Add rotating JSONL storage and ordered, time-scaled replay.
5. Verify all Go packages with unit tests and a running HTTP/WebSocket smoke test.

## Phase 1: First Light source foundation

1. Create UE modules `IonCommandCore`, `IonCommandData`,
   `IonCommandVisualization`, `IonCommandHamRadio`, `IonCommandUI`, and the
   `IonCommand` application module.
2. Implement `FGeoEnvelopeJsonParser`, `UGeoStreamSubsystem`, and
   `UGeoDataSubsystem`. Import no more than `MaxBatchSize` per tick and cap both
   pending and active messages.
3. Implement and unit-test `UGeoMathLibrary`: WGS84 unit sphere, Maidenhead,
   great-circle distance/bearing/interpolation, solar subpoint, daylight, and
   Grayline distance.
4. Implement `AGeoPointLayerActor` and reusable `AGeoArcLayerActor` with HISM.
   Specialize radio styling in `AHamRadioLinkLayerActor`.
5. Compose `AIonGlobeActor`, conceptual ionosphere shells, aurora ovals,
   `AIonCommandDeckActor`, and `AIonCommandCameraPawn` in the application game
   mode.
6. Generate `MI_Atmosphere`, `MI_Ionosphere`, `DA_BandVisualConfig`, and
   `L_CommandDeck` with the scripts in `unreal/Scripts`.
7. Run UE build, automation tests, editor validation, Play-in-Editor smoke test,
   and screenshot automation as soon as UE5 is available.

## Phase 1 completion work after toolchain installation

- Replace instanced cylinder segments with a Niagara Data Channel renderer and
  benchmark 10,000 visible arcs at 5120 x 1440.
- Implement stable render-item IDs, hit testing, selected-path spline, dimming,
  and world-space detail instrument.
- Bind world-space controls to layer manifests, band/mode/SNR filters, pause,
  replay speed, and Return to Live.
- Add Earth day/night textures, city lights, physically plausible atmosphere,
  command-deck reflections, and automatic visual regression screenshots.

## Phase 2: live operations

The concrete PSKReporter subscribed-feed decoder and transport must be supplied
through `collector/internal/plugins/sources/pskreporter`. It maps raw frames to
ham-radio raw records only; neither the canonical core nor Unreal changes.
Then add deduplication keys, real feed fixtures, sustained-load benchmarks,
recording retention, and high-density aggregation.

