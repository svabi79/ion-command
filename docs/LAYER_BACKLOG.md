# Layer backlog

Candidate data layers, ordered by what they give the operator and by what the
platform already supports. A layer is a source plugin plus a domain normaliser;
when its geometry is `Point`, `GreatCircle` or `Track`, no renderer change is
required. Anything needing `Area`, `Field`, `Raster` or `Volume` is blocked on
new geometry support and is marked accordingly.

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
| **Satellite passes over the own station** | SGP4 already runs locally in the collector (`orbital.celestrak`) | Needs a pass-prediction context plugin; the operator's own position is now reliable | `ready to build` |
| **Brandmeister DMR last-heard** — who is speaking on which talkgroup | Brandmeister API | Shares the `hamradio` domain with wave 1 work | `ready` after wave 1 |
| **Repeater directory** | RepeaterBook or a national register | Licence per source | `needs decision` |

## Wave 3 — needs new geometry support

These unlock several layers at once and should be planned together with the
`Area` / `Field` geometry work.

| Layer | Feed | Geometry needed |
| --- | --- | --- |
| **Satellite footprints** — who can hear which satellite now | Derived from SGP4 | `Area` |
| **Grayline as a real surface** rather than an implied line | Derived from solar geometry | `Area` |
| **Precipitation radar** | RainViewer or national services | `Field` / raster |
| **Ionospheric maps** (foF2, MUF, TEC) | Already fetched from KC2G as soundings | `Field` / raster |
| **Tropical storm tracks and cones** | NOAA NHC | `Track` + `Area` |
| **Submarine cables** — context for global connectivity | TeleGeography | `LineString` (static) |

## Wave 4 — the operator's own receiver

**Argus SDR integration.** ION COMMAND currently shows what *the world* hears.
The operator also runs a wideband GPU receiver that produces local detections,
band occupancy, noise floor and decoded traffic. A local source plugin would
put "what I hear here" next to "what the world hears" on the same globe, which
no public service can offer.

Blocked on a decision about the interface the receiver exposes (file, socket,
HTTP) and on which detection semantics are worth normalising.

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
