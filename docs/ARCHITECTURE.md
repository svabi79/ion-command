# Architecture

ION COMMAND is split at two boundaries: process isolation between collector and
fat client, and plugin isolation between acquisition, meaning, presentation,
and contextual analysis.

```text
External feed
  -> Source plugin (transport, framing, reconnect, health)
  -> Domain plugin (validation, units, identity, semantics)
  -> Canonical geospatial envelope
  -> bounded pipeline -> recording + live hub + replay
  -> Unreal stream/parser subsystem
  -> data/timeline/context/selection subsystems
  -> layer plugin -> generic render adapter
  -> instanced/Niagara/field/volume renderer
```

## Non-negotiable boundaries

- `IonCommandCore` and the collector `events` package know no callsigns, bands,
  modes, SNR fields, or PSKReporter framing.
- A source plugin owns connectivity but not domain meaning.
- A domain plugin produces WGS84, UTC, and typed canonical messages but no
  Unreal presentation.
- A layer chooses rendering semantics. The generic arc renderer receives only
  an arc and style choice; `IonCommandHamRadio` owns band-to-style mapping.
- Context plugins may combine domains, but derived results must identify their
  sources, model/version, validity, confidence, and measured/modelled status.
- Live and replay share the canonical format and all downstream systems.

## Collector

The collector is a separate Go process. It owns a bounded raw queue, parallel
domain normalization workers, validation, JSONL recording, WebSocket fan-out,
and replay. Each client has a bounded outgoing buffer; a slow visual client can
lose messages without stalling acquisition, and the drop is visible in stats.

### Compile-time plugins

The registry is compile-time: sources, domains and contexts implement registry
interfaces and are registered statically in
`collector/cmd/ion-collector/main.go`. Native Go plugins are deliberately not
used because they do not work on Windows, the project's primary platform.

This means **no dynamic loading and no third-party modules at runtime**. Adding
a feed means editing the registry and rebuilding. The interfaces are narrow
enough that an out-of-process or WASM host could replace the registry later
without changing canonical consumers, but no such host exists today.

The declarative manifests under `plugins/` are descriptive metadata only, and
currently cover a subset of what is registered. **The registry code is
authoritative.**

### What is registered

The current set of sources, domains, contexts and Unreal modules — with their
shipped defaults and data constraints — lives in one place:
**[COMPONENTS.md](COMPONENTS.md)**. It is deliberately not duplicated here, so
this document can stay stable while the component list moves.

`mock.*` sources exist for development and tests: they generate deterministic
synthetic traffic so the pipeline and renderer can be exercised without any
network. They are disabled in shipped configurations.

### Retained state

Slow feeds would otherwise leave a freshly connected client staring at an empty
globe until the next poll — up to half an hour for the global aircraft snapshot.
The hub therefore keeps the **latest message per retain key**
(`semanticType|entityId`) for the semantic types listed in
`pipeline.retainLatest`, and replays that set to every client on connect.

Retention is bounded and self-cleaning:

- each entry carries the envelope's `validUntil`, so dead entities expire;
- a janitor goroutine purges expired entries on an interval, independently of
  traffic;
- at the cap, the entry expiring soonest is evicted rather than refusing new
  keys;
- the replay is non-blocking, and anything that does not fit a client's queue is
  counted as a drop like any other.

The client queue must therefore be sized **above** the retain cap, or a
connecting client silently receives a truncated world.

### Rate limiting and provider policy

Acquisition is deliberately conservative, because most feeds are volunteer-run:
aviation requests pass a global spacing gate shared by all instances; HTTP 429
and 420 responses trigger exponential backoff honouring `Retry-After`; and a
source that keeps failing stops polling rather than retrying forever. A failed
fetch must never turn a periodic loop into a download loop — see
[DATA-SOURCES.md](DATA-SOURCES.md).

## Canonical model

The envelope separates message kind from semantic type. The message kind is one
of Entity, Observation, Relationship, Track, Area, Field, Volume, or Annotation.
Semantic types are hierarchical strings registered by plugins. Unknown semantic
types remain recordable and replayable.

Point and GreatCircle are fully parsed by the client. Interfaces and
schema names are reserved for tracks, polygons, grids, raster fields, shells,
and volumes.

## Unreal modules

| Module | Responsibility |
|---|---|
| `IonCommandCore` | canonical structs, geometry/layer contracts, geospatial and solar math |
| `IonCommandData` | WebSocket, JSON parsing, bounded history, timeline, replay, selection, context queries |
| `IonCommandVisualization` | layer registry and domain-neutral point/arc render adapters; globe and environmental instruments |
| `IonCommandHamRadio` | radio semantic selection and centrally configured band visuals |
| `IonCommandUI` | physical command console and world-space telemetry |
| `IonCommand` | application shell, camera, player controller, game mode, runtime composition |

The runtime scene is created from C++ components and can also be placed through
idempotent editor scripts. This keeps binary assets reproducible and lets the
application start from a minimal level.

## Threading and backpressure

Collector sources feed a bounded raw channel. Domain workers validate and
normalize in parallel. The Unreal WebSocket callback only appends to a bounded
MPSC queue. A core ticker parses and imports a configurable batch each frame.
No renderer parses network data, and no unbounded array is used.

## Rendering path

The renderer uses one hierarchical instanced mesh component per palette entry
for arc segments, and one component per point class. It never creates an Actor
or material per message. `AGeoArcLayerActor` is domain-neutral;
`AHamRadioLinkLayerActor` reuses it and supplies band styling. The next renderer
implementation can replace HISM segments with Niagara Data Channels behind the
same adapter.

Path selection performs a bounded CPU ray-to-segment query only on click. This
keeps collision disabled on the large HISM batches. The selected envelope is
copied into the generic selection subsystem and rendered through one dedicated
highlight batch, so it remains visible after the normal live window expires.
Domain plugins may attach `display.*` metadata; the generic spatial console
uses it without importing domain classes.

Observed radio reception is labelled `Observed Link`. Arc height is a visual
function of great-circle distance and is never presented as a measured ray path.

All time-aware rendering reads `UGeoTimelineSubsystem`. Pausing captures an
immutable timeline instant, and replay advances that same clock from canonical
observed timestamps; wall-clock UTC is used only in live mode.
