# Layer backlog

Candidate data layers, ordered by what they give the operator and by what the
platform already supports. A layer is a source plugin plus a domain normaliser;
when its geometry is `Point`, `GreatCircle`, `LineString`, `Track` or `Area`,
no renderer change is required. Anything needing `Field`, `Raster` or `Volume`
is blocked on new geometry support and is marked accordingly.

Priorities for the platform as a whole are in
[USER_VALUE_ROADMAP.md](USER_VALUE_ROADMAP.md); what exists today is in
[COMPONENTS.md](COMPONENTS.md).

## Status legend

| Status | Meaning |
| --- | --- |
| `in progress` | An implementation run is active |
| `ready` | Fully specified, no blocker, can start |
| `blocked` | Needs a platform capability that does not exist yet |
| `needs decision` | Needs a licence, credential or scope decision first |
| `shipped` | Implemented, tested and registered; see [COMPONENTS.md](COMPONENTS.md) for whether it is on or off by default |

## Wave 1 — moving objects and events (no renderer change)

| Layer | Feed | Geometry | Credentials | Status |
| --- | --- | --- | --- | --- |
| **APRS-IS** — vehicles, balloons, digipeaters, weather stations | APRS-IS TCP stream | Point + Track | Callsign; read-only login works with passcode `-1` | `shipped` — registered as `aprs.is` in live.json |
| **AIS ships** — global maritime traffic | `aisstream.io` WebSocket | Point + Track | Free API key required | `shipped` — live-verified 2026-08-28 against European waters; disabled by default (needs an operator key) |
| **Wildfires** — active fire detections | NASA FIRMS | Point | Free MAP_KEY may be required | `shipped` — registered as `wildfire.firms`, key-free tier |
| **DX cluster + WSPR** — announced DX and weak-signal propagation reports | DX cluster telnet, `wspr.live` | GreatCircle, Point | Callsign for the cluster login | `shipped` — `hamradio.dxcluster` and `hamradio.wspr` |

Rationale: aircraft and ships in motion, balloons climbing and drifting, and
fires appearing and dying make the globe a living picture rather than a static
one. All four fit the existing renderer.

## Wave 2 — derived value from data already flowing

| Layer / feature | Basis | Blocker | Status |
| --- | --- | --- | --- |
| **Emergency squawks** (7500/7600/7700) | The live ADS-B stream already carries them | none | `shipped` — `emergencySquawks` in the aviation domain tags 7500/7600/7700 with a red tint, double marker scale and a sticky `visual.emergency` flag; the point layer honours all three |
| **Satellite passes over the own station** | SGP4 already runs locally in the collector (`orbital.celestrak`) | none | `shipped` — look angles on every position plus `orbital.pass` predictions every 10 min; SATELLITES cockpit panel |
| **Brandmeister DMR last-heard** — who is speaking on which talkgroup | Brandmeister API | Shares the `hamradio` domain with wave 1 work | `shipped` — `hamradio.brandmeister` → `radio.activity` Points |
| **Repeater directory** | RepeaterBook or a national register | Licence per source | `needs decision` |

## Wave 3 — needs new geometry support

These unlock several layers at once and should be planned together with the
remaining `Field` geometry work. `Area` is now rendered.

| Layer | Feed | Geometry needed |
| --- | --- | --- |
| **Satellite footprints** — who can hear which satellite now | Derived from SGP4 | `Area` — **shipped** as `orbital.footprint` (visibility circle from altitude) |
| **Grayline as a real surface** rather than an implied line | Derived from solar geometry | `Area` — **shipped** as `solar.grayline` (terminator → nautical twilight) |
| **Precipitation radar** | RainViewer or national services | `Field` / raster — no clean hobby-globe API/terms found; left deferred |
| **Ionospheric maps** (foF2, MUF, TEC) | Already fetched from KC2G as soundings | `Field` / raster |
| **Tropical storm tracks and cones** | NOAA NHC | `Track` + `Area` — **centres and 5-day cones shipped** as `weather.nhc` / `weather.storm` + `weather.storm.cone` |
| **Submarine cables** — context for global connectivity | TeleGeography | `LineString` / `MultiLineString` — **shipped** as real routes + landing points (`geography.cables`); color = planned vs in-service length band |
| **Country borders and place labels** | Natural Earth 110m (bundled) | `LineString` borders + river centerlines + Point labels — **shipped** as `geography.naturalearth` (`geography.border` / `.city` / `.country` / `.river` / `.landmark`); overlay **BORDERS** / `B` |

## Wave 4 — the operator's own receiver

**Argus SDR integration.** ION COMMAND currently shows what *the world* hears.
The operator also runs a wideband GPU receiver that produces local detections,
band occupancy, noise floor and decoded traffic. A local source plugin would
put "what I hear here" next to "what the world hears" on the same globe, which
no public service can offer.

Blocked on a decision about the interface the receiver exposes (file, socket,
HTTP) and on which detection semantics are worth normalising.

## Deferred leftovers

These stay deferred (rasters need renderer work; the rest need a
cleaner licence or API than we have today):

| Candidate | Why deferred |
| --- | --- |
| ReliefWeb disasters | API requires a pre-approved `appname` since November 2025; overlaps GDACS/EONET |
| ACLED conflict events | Keyed + terms; only acceptable as `local.json` fail-closed, and the globe already has GDACS |
| RainViewer radar | Needs `Field` / raster geometry; terms not a clean hobby overlay |
| Nuclear facilities / undersea pipelines | No properly licensed, attributable, removable global point bundle found. GEM oil/gas trackers are CC BY 4.0 but download is registration-gated and operator-mediated — not a runtime fetch |
| NOTAMs | No clean, attributable, hobby-usable global JSON/API (FAA developer portal is keyed and US-centric; ICAO is paid; no scraping) |
| JMA / JTWC western-Pacific centres | No stable documented JSON comparable to NHC `CurrentStorms.json` |
| WMO SWIC CAP list | `wmo_all.json` has no coordinates without a second lookup |
| News / Telegram / webcams / markets | Out of product scope |

## Rules for every new layer

1. Source plugin under `collector/internal/plugins/sources/<name>/` returns raw
   records only — never canonical types.
2. A domain normaliser owns the vocabulary and emits geometry, validity and
   generic `display.*` / `visual.*` properties, so no renderer change is needed.
3. Register both in `collector/cmd/ion-collector/main.go`.
4. Fixture-based tests from a real captured response.
5. Rows added to [COMPONENTS.md](COMPONENTS.md) and
   [DATA-SOURCES.md](DATA-SOURCES.md), including the provider's terms and
   attribution.
6. Respect the provider: request spacing, backoff on 429/420 with `Retry-After`,
   and a hard stop on repeated failures. Credentials come from configuration and
   are never committed.
7. A layer whose provider forbids the use, or whose terms are unclear, is not
   shipped enabled by default.
