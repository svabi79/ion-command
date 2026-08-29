# Performance architecture

## Budgets

- target: 60 FPS at 5120 x 1440 on RTX-4090-class hardware;
- acceptable high-load floor: 45 FPS;
- early target: 10,000 visible arcs and 50,000 active canonical messages;
- UI and interaction latency: under 100 ms;
- no monotonically growing CPU or GPU allocations.

## Measured

`tools/bench.ps1` streams 600 synthetic spots per second into the **packaged**
client at 5120 x 1440 and derives the average frame rate from the frame counter
at capture time. On the RTX 4090 workstation, 2026-08-28, with the incremental
render slots in place:

| Date | Configuration | avg fps |
| --- | --- | --- |
| 2026-07-19 | 12k arcs, cube segments, engine-primitive globe | 125.3 |
| 2026-08-28 | globe + HUD only, no arcs (`-IonPathsHidden`) | 117.7 |
| 2026-08-28 | 12k arcs, cube segments, Nanite globe + virtual textures | **90.2** |
| 2026-08-29 | as above, plus the tile window and terrain relief | **98.7** |

The 60 FPS target is met with 1.5x headroom, but this is a **regression against
the 125.3 FPS measured on 2026-07-19 under the same fixture and hardware** —
roughly 28% lost. The no-arc row localises it: with every arc hidden the scene
still only reaches 117.7 FPS, so most of the loss is fixed cost in the globe
itself (a 158k-triangle Nanite mesh and two streaming virtual textures replacing
the engine primitive and its plain 4K textures), not in the arc renderer. The
12,000 arcs on top cost about 27 FPS.

The 2026-08-29 row adds the close-orbit tile window and the terrain relief
and does **not** cost anything measurable - it lands slightly above the
previous run, which is within what a single measurement on a machine with
other work on it can be trusted to say. Read it as "no regression", not as
a gain.

**What that row does not measure.** The benchmark flies the default distant
view, where the tile window is switched off entirely because the global
texture is already finer than any level worth fetching. So it exercises the
three extra texture reads the Earth material now always performs, and
nothing else: not the tile fetches, not the mosaic uploads, not the two
4096x2048 window textures being resident. A close-orbit figure needs a
fixture that pins the camera low (`-IonCameraDistance`), which `bench.ps1`
does not currently take.

That is a deliberate trade: the high-resolution globe is what makes close-range
zoom possible at all. It is recorded here as a cost, not hidden as an
improvement. If the budget ever needs reclaiming, the first things to measure
are the virtual-texture pool size (`r.VT.PoolSizeMB`, left at engine default)
and whether the globe needs Nanite at all at typical orbit distances.

Retracted earlier figures (6.5 / 10.3 FPS, "overdraw-bound, needs Niagara")
were a frame-counter wrap artifact; see the development log.

The benchmark measures throughput, not legibility: at 10,000 arcs the globe is
a solid white overdraw wall. Screen-space density management (roadmap
priority 1, step 4) is what makes that number useful to an operator.

## Current controls

- collector raw queue: bounded (`queueCapacity`, default 16,384);
- per-client queue: bounded (default 4,096), slow-client drops counted;
- Unreal pending queue: bounded (default 16,384);
- Unreal batch import: default 512 per ticker iteration;
- Unreal active history: bounded at 50,000 with a 900-second window;
- arc count: bounded at 10,000;
- point instances: bounded at 25,000;
- one HISM component per palette/style class, not per event.

## Known bootstrap compromise

Great-circle arcs are currently represented by instanced cylinder segments
(`SegmentsPerArc`, default 16) rather than a compact per-arc GPU record. This
proves batching and composition without binary Niagara assets, but every
segment is still a CPU-owned instance. The production renderer should consume
compact render items through Niagara Data Channels or a custom GPU buffer,
retain stable IDs, fade on GPU, and aggregate by screen-space density.

Expiry, the capacity trim, and moving-marker dead reckoning no longer rebuild
their instanced components: `AGeoArcLayerActor` and `AGeoPointLayerActor` both
give each item a stable render slot (a fixed-size instance block for arcs, a
single instance for points) tracked in a dense, hole-free run per component,
and insert/update/remove touch only the affected slot plus, on removal, the
one slot swapped in to keep the run dense (see `GeoRenderSlotMath.h`).
Removing N items now costs O(N), not O(surviving population). Both actors
expose `GetRenderStatistics()` (full rebuilds, incremental inserts/removals/
updates, capacity/expiry evictions, tracked/rendered counts) so this can be
watched at runtime instead of assumed. What remains unmoved to the GPU is the
per-segment CPU representation itself and the age/fade animation driving it -
that is the Niagara/custom-buffer migration described above, not yet started.

Quality presets will tune visible counts, segment resolution, volumetrics,
atmosphere, clouds, field resolution, and aggregation thresholds. Actual values
must be set from profiling, not guessed.

