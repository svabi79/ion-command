# Components

**This file is the canonical list of what exists.** Other documents link here
instead of repeating it, so there is only one place to keep current.

Authoritative source: the compile-time registration in
`collector/cmd/ion-collector/main.go` and the module list in
`unreal/IonCommand.uproject`. `tools/validate_repository.py` fails if the source
types registered in the collector and the ones listed below drift apart.

Default column = shipped `collector/configs/default.json`, which the installer
installs as `live.json`.

## Source plugins

A source turns one external feed into raw records. It never produces canonical
messages itself — that is the domain's job.

| `type` | Feed | Transport | Default | Needs identity | Constraints |
| --- | --- | --- | --- | --- | --- |
| `pskreporter.mqtt` | PSKReporter reception reports | MQTT subscribe | **on** | no | none published |
| `wsjtx.udp` | local WSJT-X instance | UDP listener | off | no | local only |
| `spaceweather.swpc` | NOAA SWPC + GOES | HTTP poll | **on** | no | public domain |
| `ionosonde.kc2g` | GIRO soundings via prop.kc2g.com | HTTP poll | **on** | no | CC BY-NC-SA 4.0 |
| `lightning.blitzortung` | Blitzortung strike stream | WebSocket | **on** | no | non-commercial; participant-oriented; not a warning system |
| `earthquake.usgs` | USGS earthquake feed | HTTP poll | **on** | no | public domain |
| `orbital.celestrak` | CelesTrak TLEs (SGP4 locally) | HTTP poll | **on** | no | usage policy: stop on non-200 |
| `aviation.adsb` | adsb.lol point query | HTTP poll | **on** (one example region) | no | ODbL 1.0 |
| `aviation.opensky` | OpenSky global state snapshot | HTTP poll | **on** | optional | non-profit research/education only |
| `hamradio.rbn` | Reverse Beacon Network | telnet | off | **yes** (your callsign) | no published licence |
| `mock.*` | deterministic synthetic traffic | in-process | off | no | development and tests only |

Terms and attribution in full: [DATA-SOURCES.md](DATA-SOURCES.md).
Per-field configuration: [CONFIGURATION.md](CONFIGURATION.md).

## Domain plugins

A domain normalises raw records from one or more sources into the canonical
envelope. Domains own the vocabulary; nothing above them does.

| Domain | Emits (`semanticType`) | Fed by |
| --- | --- | --- |
| `hamradio` | `radio.reception`, `radio.station` | `pskreporter.mqtt`, `hamradio.rbn`, `wsjtx.udp` |
| `aviation` | `aviation.aircraft` | `aviation.adsb`, `aviation.opensky` |
| `weather` | `weather.lightning` | `lightning.blitzortung` |
| `spaceweather` | `spaceweather.state` | `spaceweather.swpc` |
| `ionosphere` | `ionosphere.sounding` | `ionosonde.kc2g` |
| `geophysics` | `geophysics.earthquake` | `earthquake.usgs` |
| `orbital` | `orbital.position` | `orbital.celestrak` |

## Context plugins

A context derives interpretation from canonical messages without owning a feed.

| Context | Purpose |
| --- | --- |
| `hfpropagation` | HF-propagation interpretation over radio and space-weather messages |

## Unreal modules

| Module | Responsibility | Domain vocabulary allowed |
| --- | --- | --- |
| `IonCommandCore` | geo maths, generic types, the pinned sphere frame | no |
| `IonCommandData` | envelope parsing, stream/data/timeline/replay subsystems | no |
| `IonCommandVisualization` | globe, atmosphere, arc layer, point/marker layer, heatmap, ionosphere shells | no |
| `IonCommandUI` | cockpit HUD, overlay menu, settings panel, tooltips | no |
| `IonCommandHamRadio` | own station, band/DXCC panels, HF conditions and path analysis | **yes** |
| `IonCommand` | game mode, player controller, camera rig | thin glue |

The generic modules must stay free of callsigns, bands and similar. Anything
domain-specific reaches the renderer through the envelope's `display.*` and
`visual.*` properties, or lives in `IonCommandHamRadio`.

## About the word "plugin"

Sources, domains and contexts are **compile-time plugins**: they implement a
registry interface and are registered statically in
`collector/cmd/ion-collector/main.go`. They are *not* dynamically loaded, and
ION COMMAND does not support third-party modules at runtime.

Go's native `plugin` package is deliberately not used — it does not work on
Windows, which is the project's primary platform. The interfaces are narrow
enough that an out-of-process or WASM host could be added later, but none
exists today. Adding a feed therefore means editing the registry and rebuilding.

The declarative manifests under `plugins/` are descriptive metadata and are
**incomplete** (they cover only a subset of what is registered). The registry
code is authoritative; the manifests are not.

## Adding a source

1. Implement the source under `collector/internal/plugins/sources/<name>/`,
   returning raw records — no canonical types.
2. Normalise in a domain under `collector/internal/plugins/domains/<domain>/`,
   emitting geometry, validity and `display.*` / `visual.*` properties.
3. Register both in `collector/cmd/ion-collector/main.go`.
4. Add a fixture-based test from a real captured response.
5. Add a row to this file and to [DATA-SOURCES.md](DATA-SOURCES.md), including
   the provider's attribution and terms.

No renderer change should be necessary. If one is, the property vocabulary is
probably the wrong shape.
