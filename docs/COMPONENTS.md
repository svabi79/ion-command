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
| `geography.naturalearth` | Bundled Natural Earth 110m borders, places, rivers, landmarks, regions | in-process | **on** | no | public domain |
| `geography.cables` | TeleGeography submarine-cable **routes** + landings | HTTP poll | **on** | no | CC BY-NC-SA 3.0; removable on-disk cache |
| `geophysics.eonet` | NASA EONET open natural events | HTTP poll | **on** | no | public NASA API |
| `geophysics.gdacs` | GDACS disaster alerts | HTTP poll | **on** | no | public GeoJSON search |
| `maritime.portwatch` | IMF PortWatch chokepoints, daily transits, and recent disruptions | HTTP poll | **on** | no | public ArcGIS FeatureServer |
| `weather.nhc` | NOAA NHC active tropical-cyclone centres and 5-day forecast cones | HTTP poll | **on** | no | public domain; Atlantic and eastern Pacific only |
| `space.pads` | Launch Library 2 Earth spaceports | HTTP poll | **on** | optional (`apiKey` as `Authorization: Token`) | daily floor; shares the LL2 15/hour budget |
| `humanitarian.hapi` | UNHCR refugee totals via HDX HAPI (host and origin countries) | HTTP poll | **off** (needs an app identifier) | **yes** (identifier in `local.json`) | fail-closed without identifier; CC BY-IGO |
| `humanitarian.reliefweb` | OCHA ReliefWeb current/alert disasters | HTTP poll | **off** (needs a pre-approved appname) | **yes** (appname as `apiKey` in `local.json`) | fail-closed without appname; overlaps GDACS/EONET |
| `conflict.acled` | ACLED recent conflict events | HTTP poll | **off** (needs myACLED credentials) | **yes** (OAuth login/password or Bearer `apiKey` in `local.json`) | fail-closed without credentials; operator-keyed, not a bundled dataset |
| `hamradio.rbn` | Reverse Beacon Network | telnet | **on** (placeholder login `HB9HSJ`) | **yes** (your callsign in `local.json`) | no published licence |
| `hamradio.dxcluster` | DX cluster spots (DXSpider / AR-Cluster / CC Cluster) | telnet | **on** (placeholder login `HB9HSJ`) | **yes** (your callsign in `local.json`) | no published licence; no single canonical node - address is configured |
| `hamradio.wspr` | WSPR reception reports via wspr.live | HTTP poll | **on** | no | non-commercial use only; 20 req/min |
| `hamradio.brandmeister` | Brandmeister DMR last-heard | Socket.IO | **on** | no | public last-heard feed; hobby-scale throttle |
| `ais.aisstream` | aisstream.io global AIS vessel stream | WebSocket | **on** (idles without a key) | **yes** (free API key in `local.json`) | no explicit commercial/redistribution restriction found; hobby-scale posture applied anyway |
| `aprs.is` | APRS-IS packet stream | TCP | **on** (placeholder login `HB9HSJ`) | **yes** (callsign in `local.json`; passcode fixed at read-only `-1`) | no published data licence; be a good citizen (see below) |
| `solar.grayline` | Derived solar terminator / nautical-twilight band | in-process | **on** | no | no upstream fetch |
| `weather.nws` | NWS active alerts with native GeoJSON polygons | HTTP poll | **on** | no | public domain; zone/county alerts without geometry are skipped; cap 40 |
| `aviation.aviationweather` | AviationWeather SIGMET + G-AIRMET | HTTP poll | **on** | no | public; open G-AIRMET contours dropped (no contour renderer) |
| `weather.spc` | SPC convective categorical outlooks (day 1–3) | HTTP poll | **on** | no | public domain; coarsened MultiPolygons |
| `geophysics.usgsvolcano` | USGS volcano status points | HTTP poll | **on** | no | public domain; HANS/VONA endpoint 404, not fetched |
| `geophysics.gvp` | Smithsonian GVP Holocene volcano catalogue (WFS→GeoJSON) | HTTP poll | **on** | no | attribution required; WFS can timeout — disk cache is used |
| `earthquake.emsc` | EMSC FDSN seismic events | HTTP poll | **on** | no | second quake feed alongside USGS; HTTP poll, not websocket |
| `aviation.ourairports` | OurAirports CSV (large + scheduled medium) | HTTP poll | **on** | no | public domain CSV; cached; cap 1200 |
| `geography.marineregions` | Marine Regions 200 NM EEZ boundary lines | HTTP poll | **on** | no | CC BY 4.0; coarsened LineStrings, cap 220 |
| `geography.powerplants` | WRI Global Power Plant Database | HTTP poll | **on** | no | CC BY 4.0; ≥500 MW, cap 600 |
| `orbital.satnogs` | SatNOGS ground stations (Online) | HTTP poll | **on** | no | Online stations only; cap 400 |
| `geography.ioda` | IODA country internet-outage alerts | HTTP poll | **on** | no | Georgia Tech copyright; hobby display; do not republish cache |
| `humanitarian.unhcr` | UNHCR persons-of-concern site points | HTTP poll | **on** | no | attribute UNHCR; active/open sites; cap 400 |
| `aviation.openaip` | OpenAIP airspace polygons | HTTP poll | **off** (needs API key) | **yes** (`apiKey` in `local.json`) | CC BY-NC; fail-closed; coarsened, cap 40 |
| `conflict.ucdp` | UCDP GED conflict events | HTTP poll | **off** (needs token) | **yes** (`apiKey` in `local.json`) | fail-closed; recent page; cap 80 |
| `geography.cloudflare` | Cloudflare Radar outage annotations | HTTP poll | **off** (needs bearer) | **yes** (`apiKey` in `local.json`) | fail-closed; ISO→centroid; cap 40 |
| `maritime.gfw` | Global Fishing Watch fishing events | HTTP poll | **off** (needs bearer) | **yes** (`apiKey` in `local.json`) | CC BY-NC; fail-closed; Points only, no 4Wings raster |
| `aviation.tfr` | FAA Temporary Flight Restriction polygons (WFS GeoJSON) | HTTP poll | **on** | no | public domain; coarsened, cap 40 |
| `aviation.sua` | FAA Special Use Airspace (P/R/W/MOA/A) | HTTP poll | **on** | no | public domain; ranked + coarsened, cap 40 |
| `wildfire.wfigs` | NIFC/WFIGS current wildland-fire perimeters | HTTP poll | **on** | no | public Interagency layer (token-gated Current_WildlandFire_Perimeters not used); WF only, cap 30 |
| `weather.usdm` | U.S. Drought Monitor current GeoJSON | HTTP poll | **on** | no | public; D0 skipped; D1–D4 coarsened |
| `humanitarian.cems` | Copernicus EMS Rapid Mapping activations | HTTP poll | **on** | no | open activations only, cap 12; WKT extents coarsened |
| `weather.glofas` | GloFAS Rapid Flood Mapping shapefile | HTTP poll | **on** | no | Copernicus EMS; largest rings only, 12 MB cap |
| `geography.milbases` | USDOT BTS NTAD military installations | HTTP poll | **on** | no | US Government work; active sites, cap 40 |
| `maritime.incidentnews` | NOAA IncidentNews pollution/hazard incidents | HTTP poll | **on** | no | public CSV; 180-day window, cap 80 |
| `maritime.wpi` | NGA World Port Index (Pub 150) | HTTP poll | **on** | no | US Government work; Large then Medium harbors, cap 400 |
| `humanitarian.fews` | FEWS NET IPC phase-map polygons | HTTP poll | **on** | no | IPC 3+ only, coarsened, cap 24; ~27 MB payload; attribute FEWS NET / IPC Global Partnership |
| `weather.ndbc` | NDBC latest buoy + DART observations | HTTP poll | **on** | no | public domain; DART first (cap 40) then buoys, 180 points |
| `weather.cpc` | NOAA CPC 8–14 day temperature/precip outlooks | HTTP poll | **on** | no | public domain; Normal/EC skipped; coarsened, cap 16 |
| `weather.meteoalarm` | MeteoAlarm legacy Atom country feeds (`cap:polygon`) | HTTP poll | **on** | no | CC BY 4.0 plus extra redistribution terms; **runtime fetch only, do not bundle or export the raw feed**; Extreme/Severe first, cap 40 |
| `weather.metalerts` | MET Norway MetAlerts 2.0 current GeoJSON | HTTP poll | **on** | no | NLOD 2.0 / CC BY 4.0; identifying User-Agent; cap 24 |
| `weather.eccc` | ECCC / MSC weather-alert GeoJSON | HTTP poll | **on** | no | Government of Canada / MSC; native polygons, cap 40 |
| `weather.nwis` | USGS NWIS IV stream gauges (+ site catalog) | HTTP poll | **on** | no | public domain; observed gauges not flood polygons; cap 250 |
| `weather.coops` | NOAA CO-OPS water-level stations | HTTP poll | **on** | no | public domain; distinct from NDBC buoys; ~300 stations |
| `wildfire.bc` | BC Wildfire current perimeters (ArcGIS GeoJSON) | HTTP poll | **on** | no | OGL-BC; coarsened, cap 24; points layer not fetched |
| `maritime.digitraffic` | Digitraffic ATON faults (+ seasonal winter dirways) | HTTP poll | **on** | no | CC BY 4.0; gzip; AIS locations not fetched; empty dirways skipped |
| `weather.sondehub` | SondeHub v2 live radiosondes | HTTP poll | **on** | no | CC BY-SA 2.0; last 6 h, cap 80 |
| `geography.peeringdb` | PeeringDB facilities with lat/lon (guest GET) | HTTP poll | **on** | no | runtime points only — not a catalog republish; cap 250 |
| `aviation.adsbfi` | adsb.fi military snapshot `/api/v2/mil` | HTTP poll | **on** | no | personal non-commercial; military endpoint only; ≥1 s between requests |
| `mock.*` | deterministic synthetic traffic | in-process | off | no | development and tests only |

Terms and attribution in full: [DATA-SOURCES.md](DATA-SOURCES.md).
Per-field configuration: [CONFIGURATION.md](CONFIGURATION.md).

## Domain plugins

A domain normalises raw records from one or more sources into the canonical
envelope. Domains own the vocabulary; nothing above them does.

| Domain | Emits (`semanticType`) | Fed by |
| --- | --- | --- |
| `hamradio` | `radio.reception`, `radio.station`, `radio.activity` | `pskreporter.mqtt`, `hamradio.rbn`, `wsjtx.udp`, `hamradio.dxcluster`, `hamradio.wspr`, `hamradio.brandmeister` |
| `aprs` | `aprs.station`, `aprs.object` | `aprs.is` |
| `aviation` | `aviation.aircraft`, `aviation.interference`, `aviation.sigmet`, `aviation.airmet`, `aviation.airspace`, `aviation.airport`, `aviation.tfr` | `aviation.adsb`, `aviation.opensky`, `aviation.gpsjam`, `aviation.aviationweather`, `aviation.ourairports`, `aviation.openaip`, `aviation.tfr`, `aviation.sua`, `aviation.adsbfi` |
| `weather` | `weather.lightning`, `weather.observation`, `weather.airquality`, `weather.storm`, `weather.storm.cone`, `weather.alert`, `weather.outlook`, `weather.drought`, `weather.flood`, `weather.buoy`, `weather.gauge`, `weather.tide`, `weather.sonde` | `lightning.blitzortung`, `weather.openmeteo`, `weather.openaq`, `weather.nhc`, `weather.nws`, `weather.spc`, `weather.usdm`, `weather.glofas`, `weather.ndbc`, `weather.cpc`, `weather.meteoalarm`, `weather.metalerts`, `weather.eccc`, `weather.nwis`, `weather.coops`, `weather.sondehub` |
| `spaceweather` | `spaceweather.state` | `spaceweather.swpc` |
| `ionosphere` | `ionosphere.sounding` | `ionosonde.kc2g` |
| `geophysics` | `geophysics.earthquake`, `geophysics.event`, `geophysics.volcano` | `earthquake.usgs`, `earthquake.emsc`, `geophysics.eonet`, `geophysics.gdacs`, `geophysics.usgsvolcano`, `geophysics.gvp` |
| `orbital` | `orbital.position`, `orbital.pass`, `orbital.footprint`, `orbital.groundstation` | `orbital.celestrak`, `orbital.satnogs` |
| `solar` | `solar.grayline` | `solar.grayline` |
| `space` | `space.launch`, `space.pad` | `space.launchlibrary`, `space.pads` |
| `geography` | `geography.region`, `geography.border`, `geography.city`, `geography.country`, `geography.landmark`, `geography.river`, `geography.cable`, `geography.landing`, `geography.eez`, `geography.plant`, `geography.outage`, `geography.base`, `geography.facility` | `geography.naturalearth`, `geography.cables`, `geography.marineregions`, `geography.powerplants`, `geography.ioda`, `geography.cloudflare`, `geography.milbases`, `geography.peeringdb` |
| `humanitarian` | `humanitarian.displacement`, `humanitarian.disaster`, `humanitarian.site`, `humanitarian.activation`, `humanitarian.foodsecurity` | `humanitarian.hapi`, `humanitarian.reliefweb`, `humanitarian.unhcr`, `humanitarian.cems`, `humanitarian.fews` |
| `conflict` | `conflict.event` | `conflict.acled`, `conflict.ucdp` |
| `maritime` | `maritime.vessel`, `maritime.chokepoint`, `maritime.disruption`, `maritime.fishing`, `maritime.incident`, `maritime.port`, `maritime.fault`, `maritime.dirway` | `ais.aisstream`, `maritime.portwatch`, `maritime.gfw`, `maritime.incidentnews`, `maritime.wpi`, `maritime.digitraffic` |
| `wildfire` | `wildfire.detection`, `wildfire.perimeter` | `wildfire.firms`, `wildfire.wfigs`, `wildfire.bc` |

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
| `IonCommandVisualization` | globe, atmosphere, arc layer, point/marker layer, area layer, path/line layer, motion-trail layer, heatmap, ionosphere shells | no |
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
