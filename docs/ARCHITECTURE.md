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

The bootstrap registry is compile-time because native Go plugins are not
portable to Windows. Plugin manifests and interfaces preserve the logical
boundary. A future external-process or WASM plugin host can replace the registry
without changing canonical consumers.

Implemented sources:

- `mock.radio`
- `mock.lightning`
- `mock.spaceweather`

Prepared integration boundary:

- `pskreporter.live` with injected transport and decoder

Implemented domains:

- ham radio -> station entities and observed reception relationships;
- weather -> lightning observations;
- space weather -> global state observations.

## Canonical model

The envelope separates message kind from semantic type. The message kind is one
of Entity, Observation, Relationship, Track, Area, Field, Volume, or Annotation.
Semantic types are hierarchical strings registered by plugins. Unknown semantic
types remain recordable and replayable.

Point and GreatCircle are fully parsed by the bootstrap client. Interfaces and
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

The bootstrap uses one hierarchical instanced mesh component per palette entry
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
