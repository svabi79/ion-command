# Performance architecture

## Budgets

- target: 60 FPS at 5120 x 1440 on RTX-4090-class hardware;
- acceptable high-load floor: 45 FPS;
- early target: 10,000 visible arcs and 50,000 active canonical messages;
- UI and interaction latency: under 100 ms;
- no monotonically growing CPU or GPU allocations.

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

