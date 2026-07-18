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

Great-circle arcs are currently represented by twelve instanced cylinder
segments. This proves batching and composition without binary Niagara assets,
but rebuilding instances during expiry is not the final high-density path. The
production renderer should consume compact render items through Niagara Data
Channels or a custom GPU buffer, retain stable IDs, fade on GPU, and aggregate
by screen-space density.

Quality presets will tune visible counts, segment resolution, volumetrics,
atmosphere, clouds, field resolution, and aggregation thresholds. Actual values
must be set from profiling, not guessed.

