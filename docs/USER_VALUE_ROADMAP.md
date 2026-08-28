# User-value roadmap

This document ranks unfinished work by expected benefit to the operator, not
by implementation convenience. It is a prioritisation aid, not a release
commitment. Current implementation facts remain in
[IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md).

## Priorities

| Rank | Outcome for the operator | Why it matters | Indicative effort |
| ---: | --- | --- | --- |
| 1 | **Readable, smooth operation under heavy live traffic** | The globe must remain legible and responsive when tens of thousands of aircraft and hundreds of paths per second arrive. | Large |
| 2 | **Search and focus** | Find a callsign, flight, ICAO address, satellite, ionosonde, or region and isolate its relevant objects and relationships. | Medium |
| 3 | **Smart filters and saved layer presets** | Switch quickly between HF, aviation, space-weather, local-area, and emergency views without rebuilding filters each time. | Medium |
| 4 | **Recording and replay controlled from the client** | Start and stop recording, see storage use, select a time range, scrub the timeline, and change speed without editing JSON. | Large |
| 5 | **Alerts and watchlists** | Surface own-station activity, emergency squawks, strong earthquakes, high Kp, flares, and region-specific events without constant visual scanning. | Large |
| 6 | **Guided first-run setup** | Configure location, callsign, sources, provider obligations, aviation coverage, and graphics quality without touching configuration files. | Medium |
| 7 | **Visible source health and fresher coverage** | Show data age, last success, rate limits, failures, and geographic coverage in the client; add current OpenSky authentication. | Medium |
| 8 | **Zoom-aware clustering and detail reduction** | Summarise dense populations at global scale and reveal individual objects only when the view can support them. | Large |
| 9 | **Hardware quality profiles** | Provide laptop, desktop, 4K, and video-wall presets for arc count, marker count, clouds, atmosphere, and update cadence. | Medium |
| 10 | **Spatial and temporal analysis** | Answer questions such as what changed in a region, how activity evolved, and which tracks or paths dominated a time window. | Large |
| 11 | **More geometry types** | Tracks, polygons, raster fields, volumes, and shells unlock weather fields, coverage areas, trajectories, and richer forecasts. | Very large |
| 12 | **Signed installer and simpler updates** | Remove the SmartScreen trust barrier and reduce the work required to stay current. | Medium |
| 13 | **Stronger end-to-end and visual regression coverage** | Prevent performance, selection, filtering, replay, and presentation regressions from reaching operators. | Medium |
| 14 | **Clean architecture boundaries and metadata** | Remove remaining domain vocabulary from generic modules and align versions, API documentation, and manifests. | Medium |
| 15 | **Dynamic plugins, multi-user operation, and more platforms** | Valuable later, but currently less useful than making the single-operator Windows experience fast and effortless. | Very large |

## Priority 1: readable, smooth operation under heavy live traffic

The goal is not merely a higher frame-rate counter. The operator must be able
to understand the display and interact with it while the real feeds are busy.
This combines rendering throughput, visual density management, input latency,
and predictable memory use.

### Current constraints

- Every visible arc is represented by multiple instanced mesh segments. At the
  current default of 16 segments and 12,000 arcs, the renderer may manage up to
  192,000 segment instances.
- Expiry, filters, and capacity trims can rebuild large instance batches.
- Moving point markers are periodically rebuilt for dead reckoning.
- The client imports at most 512 messages per ticker iteration and bounds its
  pending queue and active history. This protects memory, but sustained input
  above the processing/rendering budget still appears as latency or drops.
- Drawing fewer objects alone is insufficient: a globe covered by overlapping
  paths or aircraft can be unreadable even when it renders quickly.

### Desired operator outcome

- Camera movement, hover, selection, menus, and filtering remain responsive at
  representative peak traffic.
- The display presents structure instead of a wall of overlapping marks.
- Selected and watched objects remain visible regardless of aggregation.
- Degradation is graceful and explicit: the client reduces detail before it
  stalls, and diagnostics explain what was reduced or dropped.
- CPU and GPU memory remain bounded during multi-hour operation.

### Problem to solution map

| Observed problem | Required solution | Proof required |
| --- | --- | --- |
| Peak traffic creates too many CPU-managed arc segment instances. | Establish a measured baseline, then replace per-segment CPU ownership with compact per-arc GPU input behind `IGeoRenderAdapter`. | Unreal Insights capture plus before/after packaged-build benchmark. |
| Expiry, capacity trims, and filter changes rebuild large instance batches. | Introduce stable render IDs/slots, incremental insert/update/expiry, and bounded compaction outside latency-sensitive interaction. | Rebuild count and game-thread spikes remain within an explicit budget during peak replay. |
| Moving aircraft cause periodic full marker-pool rebuilds. | Separate static and kinematic populations and update only changed moving-marker slots. | Marker update cost scales with changed markers rather than total visible markers. |
| A technically fast globe can still become visually saturated. | Apply screen-space budgets, zoom-aware clustering, endpoint/path aggregation, and importance-based sampling. | Reference captures at global, continental, and regional zoom remain readable. |
| Important objects could disappear during aggregation. | Reserve render priority for selection, watchlists, emergencies, own-station traffic, and locally focused objects. | Automated scenarios prove priority objects remain visible and selectable under overload. |
| Overload currently presents mainly as latency or drops. | Degrade detail before responsiveness, expose the active quality state, queue pressure, reductions, and drops. | Deliberate overload stays interactive and reports every reduction/drop category. |
| Performance work risks changing selection, replay, or domain boundaries. | Keep the canonical contract and render-adapter boundary stable; add regression coverage before replacing the renderer. | Existing data, timeline, selection, filter, and replay tests plus new peak-load checks pass. |

### Autonomous-run brief

A future autonomous implementation run should treat this section as its
execution contract.

**Objective:** deliver the largest verified improvement in heavy-traffic
responsiveness and readability without weakening queue/history bounds or
changing canonical domain semantics.

**Order of work:**

1. Capture a reproducible packaged-build baseline before changing renderer
   architecture. Preserve the fixture, commands, hardware profile, logs, and
   screenshots in a documented benchmark procedure.
2. Attribute cost to game thread, render thread, GPU, import/parsing, arc
   rebuilds, marker rebuilds, and visual density. Do not choose Niagara or a
   custom buffer until the measurements distinguish the limiting stages.
3. Implement and verify low-risk incremental-update improvements first.
4. Select and implement the GPU arc path only when the baseline shows that it
   addresses a material remaining bottleneck. Keep `IGeoRenderAdapter` as the
   replacement seam.
5. Add density management and priority rules as an operator-facing feature,
   not as an undocumented emergency clamp.
6. Run functional, performance, soak, overload, and visual checks. Compare all
   results with the preserved baseline and document regressions as well as
   gains.

**Non-negotiable constraints:**

- Never create an Actor, widget, material, or Niagara system per event.
- Keep all queues, render populations, histories, and maps bounded.
- Preserve live/replay parity, canonical envelopes, UTC/WGS84 semantics,
  selection persistence, and unknown-type forwarding.
- Do not trade readability for a frame-rate number: selection and priority
  objects must remain prominent.
- Do not claim the target from editor-only or synthetic timing. The final
  result requires a packaged-build test with a versioned peak-load fixture.

**Expected deliverables:** benchmark fixture and runner, captured baseline,
profiling evidence, implementation, regression tests, reference screenshots,
updated performance documentation, and a final before/after result table.

### Recommended delivery sequence

1. **Measure a repeatable baseline.** Capture frame time, game-thread time,
   render-thread time, GPU time, instance counts, queue depth, import latency,
   rebuild frequency, and memory against recorded high-load traffic.
2. **Remove avoidable rebuilds.** Use stable slots/IDs and incremental updates
   for arcs and moving markers; separate expiry from full component rebuilds.
3. **Move arc ageing and animation fully to the GPU.** Feed compact arc records
   through Niagara Data Channels or a custom GPU buffer behind the existing
   render-adapter contract.
4. **Add view-dependent density management.** Apply screen-space budgets,
   endpoint/path aggregation, zoom-aware clustering, and prioritisation of
   selected, watched, emergency, and local objects.
5. **Ship adaptive quality profiles.** Tune explicit budgets from measured
   hardware results and expose the current quality/degradation state.

### Acceptance criteria

- A recorded, versioned peak-load fixture produces repeatable results.
- The target hardware sustains 60 FPS at 5120x1440; the documented high-load
  floor is 45 FPS.
- Interaction latency remains below 100 ms during the benchmark.
- No unbounded CPU/GPU allocation growth occurs over a multi-hour soak.
- Queue drops and render-budget reductions are zero in the normal target case
  and visible in diagnostics when deliberately overloaded.
- Selection, replay, filters, and visual priority rules still work under load.

## Priority 2: search and focus

The operator may already know the callsign, flight, ICAO address, satellite,
ionosonde, entity ID, or region of interest. Requiring them to discover it by
rotating and hovering over a dense globe turns available data into inaccessible
data. Search must locate the canonical object, focus the camera, select it, and
optionally isolate its immediate context.

### Problem to solution map

| Observed problem | Required solution | Proof required |
| --- | --- | --- |
| Known objects are difficult to find in dense traffic. | Add a keyboard-first search overlay with incremental results and clear type, age, source, and identity labels. | Representative callsign, flight, ICAO, satellite, sounding, and entity-ID queries return the expected result. |
| Renderers contain only the currently drawn subset and are not a reliable search source. | Build a bounded, timeline-aware search subsystem from accepted canonical messages rather than render instances. | Search still finds valid objects when their visual representation is clustered, hidden, or temporarily outside a render budget. |
| Repeated observations can produce many duplicate-looking hits. | Group current state by stable entity ID and expose individual messages/relationships as secondary results. | A frequently updated aircraft has one primary entity result with current details rather than hundreds of fixes. |
| Point markers currently hover but path selection owns most of the selection workflow. | Extend generic selection and camera framing to points as well as paths. | Selecting a point opens details and moves the camera without domain-specific controller logic. |
| Finding an object does not reveal its relevant context. | Offer explicit `FOCUS`, `DETAILS`, and temporary `ISOLATE` actions; show current relationships without creating a persistent preset. | A station can reveal current RX/TX relationships and clearing isolation restores the previous view. |
| Live state can leak into pause or replay results. | Tie indexing, expiry, result age, and focus actions to `UGeoTimelineSubsystem`; rebuild/reset correctly on stream-mode transitions. | The same query produces results appropriate to live, paused, and replay time. |
| Search terms risk leaking callsign/flight vocabulary into generic UI and data modules. | Index generic IDs and `display.*` metadata in the core search path; add domain-owned aliases through a narrow provider/property contract. | Generic modules contain no hard-coded domain identifiers or field interpretation. |
| Text entry can trigger global hotkeys. | Reuse a single explicit text-capture/input-routing state for settings and search. | Typing every supported search character causes no camera, layer, band, or timeline action. |
| Stale results can outlive their canonical validity. | Apply `validUntil`, active-window, capacity, and data-reset rules to every indexed document. | Expired and reset objects disappear without unbounded index growth. |

### Autonomous-run brief

A future autonomous implementation run should treat this section as its
execution contract.

**Objective:** let an operator find, inspect, focus, and temporarily isolate a
known current or replay-time object in seconds, while preserving generic module
boundaries and bounded data behavior.

**Order of work:**

1. Define a small domain-neutral search document and result/action contract.
   Include stable key, display label, domain, semantic type, geometry, source,
   observed/valid time, generic aliases, and the canonical selection payload.
2. Implement a bounded `UGeoSearchSubsystem` fed incrementally by
   `UGeoDataSubsystem` acceptance/reset events. Measure a simple scan against
   50,000 active messages before adding unnecessary indexing complexity.
3. Group entity observations by stable entity ID while retaining relationships
   and one-shot observations as separately discoverable results.
4. Add the search overlay and shared text-input capture. Provide keyboard and
   mouse operation, deterministic result ranking, and explicit empty/error
   states.
5. Generalise camera focus and selection for Point and path geometry. Keep the
   focused result visually prioritised even when normal rendering aggregates or
   filters it.
6. Add temporary relationship isolation with lossless restoration of the prior
   view. Do not turn this into saved presets or alerts; those belong to later
   priorities.
7. Verify live, pause, replay, reset, expiry, overload, keyboard routing, and
   domain-boundary behavior with automated tests and reference captures.

**Non-negotiable constraints:**

- Search uses canonical data, not renderer-private arrays, as its source of
  truth.
- Index and result populations are bounded and follow canonical validity and
  timeline semantics.
- Generic modules do not hard-code callsign, band, flight, satellite, or other
  domain-specific field names.
- Search does not silently alter persistent layer/filter settings.
- Selected search results remain visible and selectable under aggregation or
  render-budget pressure.
- Search performance must be measured before introducing a complex external
  indexing dependency.

**Expected deliverables:** search/result contract, bounded search subsystem,
search overlay, point/path focus behavior, temporary isolation, automated
live/replay/expiry/input tests, reference captures, user documentation, and a
query-latency result for the 50,000-message target window.

### Acceptance criteria

- Incremental results appear within 100 ms at the 50,000-message target.
- Stable IDs and generic display metadata are searchable; domain providers can
  add aliases without contaminating generic modules.
- Points and paths can both be selected, framed, inspected, and cleared.
- Results and ages are correct in live, pause, and replay modes.
- Expired/reset data disappears and the index remains bounded in a soak test.
- Search text never triggers ordinary hotkeys.
- Clearing focus/isolation restores the previous view without data loss.

## Priority 3: smart filters and saved view presets

ION COMMAND already exposes individual controls for paths, domains, bands,
modes, own-station relationships, aircraft altitude, ground traffic, heatmap,
ionosphere, and HUD state. These controls are distributed across the player
controller, HUD, layer subsystem, and individual renderers. The operator cannot
reliably reproduce, name, save, or switch a complete operational view.

A view preset should combine deterministic data filters with presentation
state. Examples include HF operations, air traffic, space, own station, and a
clean video-wall view. “Smart” means composable and understandable rules, not
opaque AI behavior.

### Problem to solution map

| Observed problem | Required solution | Proof required |
| --- | --- | --- |
| Filter and visibility state is scattered across controllers, HUD code, layer actors, and renderer-specific fields. | Introduce one generic, authoritative view-state subsystem and migrate existing controls behind it. | Every current toggle changes the central state, and every affected renderer reflects the same state. |
| Useful combinations must be rebuilt manually after switching tasks or restarting. | Add named built-in and user presets with versioned persistence, duplication, reset, rename, and delete behavior. | A custom view survives restart and can be restored with one action. |
| Existing filters cannot express common combinations consistently. | Define a serializable generic rule model for domain, semantic type, source, time, spatial scope, property comparison, relationship, quality, and priority. | Representative HF, aviation, space, local-area, and own-station presets produce the expected included/excluded fixtures. |
| Numeric canonical properties are stringified in the Unreal client. | Give the rule evaluator explicit string, numeric, boolean, existence, set, and range operators with strict conversion/error behavior. | Altitude, magnitude, Kp, band/mode, boolean ground state, and missing-value tests are deterministic. |
| Hiding a layer could be confused with stopping acquisition or deleting data. | Keep view filtering downstream of canonical ingestion; surface that hidden data continues to flow and remains searchable/replayable. | Preset switching is immediate and requires no collector reconnect or data refill. |
| Search isolation, user edits, and saved presets can overwrite each other. | Layer state explicitly: base preset → unsaved operator modifications → temporary search/selection override. Restore each layer losslessly. | Clearing search isolation returns to the exact modified preset; loading another preset discards only with explicit behavior. |
| Domain-specific presets could leak vocabulary into generic UI modules. | Keep the rule engine domain-neutral and allow modules/data assets to contribute labelled preset definitions and property keys. | Generic modules contain no hard-coded callsign, band, or provider-specific interpretation. |
| A restrictive rule set can produce an unexplained empty globe. | Display active rules, result counts, modified state, conflicts/conversion failures, and a one-action reset. | Zero-result fixtures show why nothing matches and let the operator recover immediately. |
| Filter application can trigger costly full renderer rebuilds. | Compile predicates once, calculate state changes incrementally where practical, and coordinate rebuild budgets with Priority 1. | Preset changes complete within the interaction budget and do not create repeated per-frame rebuild spikes. |
| Presets can behave differently in live and replay. | Apply the same rule model to the active canonical timeline and make relative-time rules timeline-aware. | The same preset produces semantically equivalent live, paused, and replay views. |

### Autonomous-run brief

A future autonomous implementation run should treat this section as its
execution contract.

**Objective:** let the operator switch complete, understandable operational
views with one action, customise and persist them safely, and retain full
canonical data for instant switching, search, context, and replay.

**Order of work:**

1. Inventory every existing view/filter control and its owner, default,
   persistence, hotkey, renderer effect, and interaction with selection/replay.
   Add characterization tests before moving behavior.
2. Define versioned, serializable `FGeoViewPreset`, `FGeoFilterRule`, and layered
   state contracts. Specify operators, missing values, conversion errors,
   ordering, conflicts, defaults, and migration behavior before building UI.
3. Implement a generic `UGeoViewStateSubsystem` as the single source of truth,
   with change events and read-only evaluation APIs for renderers and UI.
4. Migrate existing layer/domain visibility, band, mode, own-station,
   altitude, ground-aircraft, heatmap, ionosphere, and HUD controls without
   changing their current user-visible behavior or hotkeys.
5. Add built-in presets through data/module contributions. Start with HF
   operations, air traffic, space, own station, and clean wall; keep names and
   rules editable in data rather than hard-coded branching UI.
6. Add preset selection and editing to the overlay, including active/modified
   state, match counts, save-as, reset, rename, delete, and clear explanations.
7. Add versioned user persistence with safe handling of missing modules,
   unknown future rules, corrupt files, and renamed canonical properties.
8. Integrate temporary search/selection isolation as a non-persistent override
   above the current preset. Verify exact restoration afterward.
9. Run functional, persistence, migration, performance, live/replay, keyboard,
   domain-boundary, and visual checks. Document every shipped preset.

**Non-negotiable constraints:**

- Filtering changes presentation, not source acquisition, canonical storage,
  recording, or replay content.
- The rule engine and generic UI remain domain-neutral; domain modules or data
  contribute vocabulary and preset definitions.
- Presets and rule collections are bounded, versioned, deterministic, and
  recoverable from invalid persisted state.
- Temporary search/selection overrides never silently mutate saved presets.
- Existing hotkeys remain valid aliases for the corresponding central state.
- Unknown domains, properties, and future rule types fail visibly and safely;
  they do not crash or accidentally match everything.
- Filter evaluation and renderer updates respect the interaction and rebuild
  budgets established by Priority 1.

**Expected deliverables:** state/rule/preset contracts, central view-state
subsystem, migration of existing controls, built-in presets, preset UI, user
persistence and migration, temporary-override integration, automated tests,
reference captures, user documentation, and measured preset-switch latency.

### Acceptance criteria

- The shipped HF, aviation, space, own-station, and clean-wall views activate
  with one action and have documented deterministic rules.
- Operators can save, restore, duplicate, rename, reset, and delete custom
  presets; valid presets survive restart and schema migration.
- The UI always shows the active preset, unsaved modification state, active
  rules, and current match counts.
- Every existing filter/toggle and hotkey is backed by the central state and
  retains its previous behavior.
- Hidden data remains available to search, selection context, preset switching,
  recording, and replay without reconnection.
- Temporary focus/isolation restores the exact prior modified preset state.
- Relative-time and validity rules behave correctly in live, pause, and replay.
- Representative preset switches finish within 100 ms excluding intentional
  camera animation; no repeated rebuild spike violates the Priority 1 budget.
- Invalid or future persisted rules degrade visibly and safely without data
  loss, crash, or accidental broad matching.
