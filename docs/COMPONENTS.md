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
| `wildfire.firms` | NASA FIRMS VIIRS/MODIS thermal anomalies | HTTP poll | **on** (one example area) | no (optional free MAP_KEY for the scoped Area API) | attribution required; see below |
| `orbital.celestrak` | CelesTrak TLEs (SGP4 locally) | HTTP poll | **on** | no | usage policy: stop on non-200 |
| `aviation.adsb` | adsb.lol point query; callsign→route enrichment via adsbdb.com (`routeLookup`) | HTTP poll | **on** (one example region) | no | ODbL 1.0 |
| `aviation.opensky` | OpenSky global state snapshot (OAuth2 or anonymous) | HTTP poll | **on** | optional (`clientId`/`clientSecret` in gitignored `local.json`) | non-profit research/education only |
| `aviation.gpsjam` | gpsjam.org daily GNSS-interference H3 grid | HTTP poll | **on** | no | public daily CSV; hex centres are approximate at globe scale |
| `space.launchlibrary` | The Space Devs Launch Library 2 upcoming launches | HTTP poll | **on** | optional (`apiKey` as `Authorization: Token`) | 15 calls/hour anonymous; 15 min floor |
| `weather.openmeteo` | Open-Meteo current weather at a configured cell | HTTP poll | **on** (one example cell) | no | attribution required: "Weather data by Open-Meteo.com" |
| `weather.openaq` | OpenAQ v3 air-quality stations near a point | HTTP poll | **off** (needs a free API key) | **yes** (Explorer API key in `local.json`) | fail-closed without a key |
| `geography.naturalearth` | Named regions from a bundled Natural Earth extract | in-process | **on** | no | public domain |
| `geography.cables` | TeleGeography submarine-cable map | HTTP poll | **on** | no | CC BY-NC-SA 3.0; removable on-disk cache |
| `geophysics.eonet` | NASA EONET open natural events | HTTP poll | **on** | no | public NASA API |
| `geophysics.gdacs` | GDACS disaster alerts | HTTP poll | **on** | no | public GeoJSON search |
| `maritime.portwatch` | IMF PortWatch chokepoints and daily transits | HTTP poll | **on** | no | public ArcGIS FeatureServer |
| `weather.nhc` | NOAA NHC active tropical-cyclone centres | HTTP poll | **on** | no | public domain; Atlantic and eastern Pacific only |
| `space.pads` | Launch Library 2 Earth spaceports | HTTP poll | **on** | optional (`apiKey` as `Authorization: Token`) | daily floor; shares the LL2 15/hour budget |
| `humanitarian.hapi` | UNHCR refugee totals via HDX HAPI (host and origin countries) | HTTP poll | **off** (needs an app identifier) | **yes** (identifier in `local.json`) | fail-closed without identifier; CC BY-IGO |
| `hamradio.rbn` | Reverse Beacon Network | telnet | off | **yes** (your callsign) | no published licence |
| `hamradio.dxcluster` | DX cluster spots (DXSpider / AR-Cluster / CC Cluster) | telnet | off | **yes** (your callsign) | no published licence; no single canonical node - address is configured |
| `hamradio.wspr` | WSPR reception reports via wspr.live | HTTP poll | **on** | no | non-commercial use only; 20 req/min |
| `ais.aisstream` | aisstream.io global AIS vessel stream | WebSocket | **off** (needs a free key) | **yes** (free API key) | no explicit commercial/redistribution restriction found; hobby-scale posture applied anyway |
| `aprs.is` | APRS-IS packet stream | TCP | off | **yes** (callsign; passcode fixed at read-only `-1`) | no published data licence; be a good citizen (see below) |
| `mock.*` | deterministic synthetic traffic | in-process | off | no | development and tests only |

Terms and attribution in full: [DATA-SOURCES.md](DATA-SOURCES.md).
Per-field configuration: [CONFIGURATION.md](CONFIGURATION.md).

## Domain plugins

A domain normalises raw records from one or more sources into the canonical
envelope. Domains own the vocabulary; nothing above them does.

| Domain | Emits (`semanticType`) | Fed by |
| --- | --- | --- |
| `hamradio` | `radio.reception`, `radio.station` | `pskreporter.mqtt`, `hamradio.rbn`, `wsjtx.udp`, `hamradio.dxcluster`, `hamradio.wspr` |
| `aprs` | `aprs.station`, `aprs.object` | `aprs.is` |
| `aviation` | `aviation.aircraft`, `aviation.interference` | `aviation.adsb`, `aviation.opensky`, `aviation.gpsjam` |
| `weather` | `weather.lightning`, `weather.observation`, `weather.airquality`, `weather.storm` | `lightning.blitzortung`, `weather.openmeteo`, `weather.openaq`, `weather.nhc` |
| `spaceweather` | `spaceweather.state` | `spaceweather.swpc` |
| `ionosphere` | `ionosphere.sounding` | `ionosonde.kc2g` |
| `geophysics` | `geophysics.earthquake`, `geophysics.event` | `earthquake.usgs`, `geophysics.eonet`, `geophysics.gdacs` |
| `orbital` | `orbital.position` | `orbital.celestrak` |
| `space` | `space.launch`, `space.pad` | `space.launchlibrary`, `space.pads` |
| `geography` | `geography.region`, `geography.cable`, `geography.landing` | `geography.naturalearth`, `geography.cables` |
| `humanitarian` | `humanitarian.displacement` | `humanitarian.hapi` |
| `wildfire` | `wildfire.detection` | `wildfire.firms` |
| `maritime` | `maritime.vessel`, `maritime.chokepoint` | `ais.aisstream`, `maritime.portwatch` |

## Context plugins

A context derives interpretation from canonical messages without owning a feed.

| Context | Purpose |
| --- | --- |
| `hfpropagation` | HF-propagation interpretation over radio and space-weather messages |

## Unreal modules

| Module | Responsibility | Domain vocabulary allowed |
| --- | --- | --- |
| `IonCommandCore` | geo maths, generic types, the pinned sphere frame | no |
| `IonCommandData` | envelope parsing, stream/data/timeline/replay/search/watch subsystems | no |
| `IonCommandVisualization` | globe, atmosphere, arc layer, point/marker layer, motion-trail layer, heatmap, ionosphere shells | no |
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
