# Configuration

ION COMMAND has two halves that are configured separately:

| Part | What it does | Where it is configured |
| --- | --- | --- |
| **Client** (the globe) | rendering, your station identity, display filters | in-app **SETTINGS** panel → written to `Saved/Config/IonOperator.ini` |
| **Collector** (the data feed) | which services are polled, how often, recording | `collector/configs/live.json` (installed copy: `<install>\collector\configs\live.json`) |

Client settings apply immediately. Collector settings need a collector restart.

---

## Client: the SETTINGS panel

Press **O** to open the overlay menu, then click **SETTINGS >**.
(For unattended use you can also start the client with `-IonSettings`.)

| Row | Meaning |
| --- | --- |
| **CALLSIGN** | Your callsign. Click the row and type (first keystroke replaces the placeholder); **Enter** saves, **Esc** cancels. Drives the "you are here" reticle and the *MY RX/TX* path filter. |
| **GRID LOCATOR** | Your Maidenhead locator, e.g. `JO62qm`. Moves the home marker and reticle immediately. |
| **MARKER LIFETIME** | How long a marker stays after its last sighting (60 / 120 / 300 / 600 / 1200 s). Lower = tidier globe, higher = longer trails of activity. |
| **MIN FLIGHT LEVEL** | Hides aircraft below this level (OFF / FL050 / FL100 / FL200 / FL300). The quickest way to thin out dense airspace — at FL100 the airport clutter disappears and only cruising traffic remains. |
| **SHOW GROUND A/C** | Show or hide aircraft reported as on the ground. |
| **INVERT ORBIT Y** | Flip the vertical orbit direction of a right-mouse drag. OFF matches the horizontal drag convention; ON restores the pre-0.9.1 direction. |
| **SENSOR LOOK** | Camera post-process: OFF, FLIR white-hot, FLIR black-hot, Ironbow, NVG, CRT. Also **F1–F6**. Number keys 1–9 stay band presets. Missing materials are a no-op. |

While a text field is focused all hotkeys are suspended, so typing a callsign
cannot toggle layers.

Values persist to `<Saved>/Config/IonOperator.ini` under `[IonCommand.Station]`
(callsign, locator), `[IonCommand.Display]` (`MarkerLifetime`,
`MinFlightLevelFt`, `ShowGround`, `SensorLook`), `[IonCommand.Input]` (`InvertOrbitY`) and
`[IonCommand.Watchlist]` (`Query`).

That file is the client's own, deliberately outside Unreal's config hierarchy.
Settings used to be written to the saved `Game.ini`, which does not survive:
Unreal rewrites the saved hierarchy at shutdown and keeps only what the engine
itself knows about, so a hand-provisioned station - or one saved from the
settings panel - was silently gone by the next start, falling back to the
`N0CALL` / `JN00AA` placeholder. The placeholder is a real grid square on the
Spanish coast, so the marker did not disappear; it moved. An unconfigured
station now draws nothing at all rather than asserting a position the operator
never set.

To provision a station without opening the settings panel, write the file
directly before first start:

```ini
[IonCommand.Station]
Callsign=HB9HSJ
Locator=JN47om
```

For a packaged build that is `dist/windows/IonCommand/Saved/Config/IonOperator.ini`.

## Collector: satellite passes over your station

`orbital.celestrak` tracks the CelesTrak `amateur` group. Give it a ground
station and it also answers the two questions a station operator has - what
is above me now, and when is the next pass:

```json
{
  "id": "orbital-primary",
  "type": "orbital.celestrak",
  "enabled": true,
  "latitude": 47.52,
  "longitude": 9.21,
  "altitudeM": 440
}
```

Without `latitude`/`longitude` the source behaves exactly as before, emitting
positions and nothing else. With them:

- every position also carries `elevationDeg`, `azimuthDeg`, `rangeKm` and
  `aboveHorizon`, and satellites that are up are drawn brighter and larger;
- every 10 minutes each satellite's next pass within 24 hours is emitted as
  `orbital.pass` with acquisition, culmination, loss, peak elevation and the
  compass bearing to point at (`"SE -> N"`). Passes peaking below 10 degrees
  are not reported - they are inside any real station's ground clutter.

Predictions are recomputed every 10 minutes but re-announced on every
position tick. That is deliberate: the collector has no state snapshot on
connect, so a client joining between sweeps would otherwise show "awaiting
prediction" for up to ten minutes. Re-emitting is nearly free - the search
was the expensive part, not the record - and a repeat for the same pass
supersedes rather than accumulates, because the message id is keyed on the
acquisition time.

The client shows both in the **SATELLITES** cockpit panel, titled with your
locator. Note this is the collector's station, configured here, not the
client's `[IonCommand.Station]` - the prediction happens where SGP4 runs.
Keeping them in step is on you; a mismatch means the panel is titled with one
place and predicting for another.

## Client: overlay menu

Press **O**. Every row is clickable and shows its state:

- **SETTINGS >** — opens the panel above
- **PATHS** — the propagation arcs
- **MY RX/TX ONLY** — only paths where your station is transmitter or receiver
- **HEATMAP** — activity density splats
- **IONOSPHERE SHELLS** — ionosonde shells
- **TRAILS** — motion trails behind moving markers
- **AREAS** — filled polygons (NHC forecast cones, solar grayline /
  nautical-twilight band, NWS alerts, SIGMET / G-AIRMET rings, SPC
  outlooks, optional OpenAIP airspaces). Satellite footprints stay off
  until pinned.
- **&lt;SAT&gt; FOOTPRINT** — one row per pinned satellite; click to unpin
- **CABLES** — submarine cable routes (TeleGeography). Color: planned amber;
  in-service by length (short teal / regional cyan / ocean blue / trunk magenta)
- **BORDERS** — Natural Earth admin-0 country lines, coarsened Marine
  Regions 200 NM EEZ lines, and zoom-aware place labels (cities, rivers,
  landmarks, regions)
- **ALT EXAGGERATION 12X** — aircraft altitude exaggerated so flight level is
  visible at globe scale; off renders true scale
- **&lt;DOMAIN&gt; MARKERS** — one row per marker domain currently present
  (aviation, hamradio, orbital, weather, geophysics, ionosphere …). Hiding a
  domain only stops it drawing; the data keeps flowing.

## Client: keyboard and mouse

| Input | Action |
| --- | --- |
| **Left mouse** | select a path / click menu rows |
| **Right mouse** (drag) | orbit the globe |
| **Mouse wheel** | zoom — steps scale with distance, down to ~32 km above the surface |
| **Hover a marker** | tooltip with callsign/flight level/type/…; aircraft seen by an `aviation.adsb` circle also show their filed route (`CDG Paris  >  TUN Tunis`) once resolved. Over a satellite: **P FOOTPRINT**. Also names cable routes, area fills (alerts/cones/grayline when drawn), and cartography lines. Hidden, filtered, expired, unpinned, or far-side features do not show a tooltip. |
| **Tab** | cycle HUD: full → minimal → hidden |
| **O** | overlay menu |
| **V** | show/hide paths |
| **M** | only my station's RX/TX paths |
| **H** | activity heatmap |
| **I** | ionosphere shells |
| **T** | show/hide trails |
| **Y** | show/hide areas (grayline, NHC cones, NWS/SPC/SIGMET; not a global sat-footprint dump) |
| **P** | pin/unpin the hovered or selected satellite's footprint |
| **C** | show/hide cable routes |
| **B** | show/hide country borders and place labels |
| **N** | cycle transmission-mode filter |
| **F** | focus the selected path |
| **Esc** | clear selection |
| **1** … **9** | band presets |
| **0** | all bands |
| **Space** | pause/resume the timeline |
| **R** | replay the last 15 minutes |
| **,** / **.** | replay slower / faster |
| **L** | return to live |

## Client: command-line switches

Useful when driving the client from a script or a video wall:

| Switch | Effect |
| --- | --- |
| `-windowed` / `-fullscreen`, `-ResX=` `-ResY=` | window mode and resolution |
| `-IonCollectorUrl=ws://host:port/ws/live` | connect to a non-default collector |
| `-IonShowDeck` | restore the diegetic console panels (off by default) |
| `-IonSettings` | open the settings panel at startup |
| `-IonOverlayMenu` | open the overlay menu at startup |
| `-IonPathsHidden` | start with the path layer hidden |
| `-IonNoLiveClouds` | keep the offline cloud texture instead of fetching live imagery |
| `-IonNoTileImagery` | keep the packaged global textures instead of fetching close-orbit map tiles. Use for deterministic captures and offline demos |
| `-IonTileBaseUrl=` | override the WMTS endpoint the close-orbit tiles come from (default: NASA GIBS EPSG:4326) |
| `-IonElevationUrl=` | override the terrain tile template used for relief (default: AWS Terrain Tiles) |
| `-IonDetailTileUrl=` | override the close-orbit detail imagery (default: Sentinel-2 cloudless from EOX) |
| `-IonTileCacheBudgetMB=` | disk budget for `<Saved>/TileCache`; the oldest tiles are deleted at startup once it is exceeded (default 512) |
| `-IonMute` | no ambience audio |
| `-IonCameraDistance=` `-IonCameraLongitude=` `-IonCameraLatitude=` | pin the camera (captures) |
| `-IonScreenshotAfter=<s>` `-IonScreenshotFile=<path>` `-IonExitAfterScreenshot` | unattended screenshots |

> Use an absolute path **without spaces** for `-IonScreenshotFile`; the engine
> splits the command line on spaces.

---

## Collector: `live.json`

```jsonc
{
  "server":   { "listenAddress": "127.0.0.1:7810", "writeTimeoutSeconds": 10 },
  "pipeline": {
    "queueCapacity": 32768,
    "clientQueueCapacity": 49152,   // must exceed the retained-state count
    "workerCount": 4,
    "retainLatest": ["spaceweather.state", "ionosphere.sounding", "aviation.aircraft"]
  },
  "recording": {
    "enabled": false,               // ON writes several GB per hour
    "directory": "../data/recordings",
    "flushIntervalSeconds": 1,
    "maxTotalGigabytes": 20
  },
  "sources": [ /* see below */ ]
}
```

`retainLatest` lists semantic types whose newest message per entity is kept and
replayed to every client that connects, so the globe is populated within
seconds instead of waiting for the next poll.

### Source entries

Every source has `id`, `type`, `enabled`. The canonical list of source types,
with their shipped defaults and data constraints, is
[COMPONENTS.md](COMPONENTS.md); the table below documents their *configuration
fields*.

Additional fields by type:

| `type` | Extra fields | Notes |
| --- | --- | --- |
| `pskreporter.mqtt` | `broker`, `topic`, `clientId` | Public broker, no credentials. ~300–500 spots/s. |
| `spaceweather.swpc` | `pollSeconds` | Kp, solar flux, wind, Bz, A-index, GOES X-ray. |
| `ionosonde.kc2g` | `pollSeconds` | foF2 / MUF soundings. |
| `lightning.blitzortung` | — | WebSocket stream of strikes. Enabled by default; read the [terms](DATA-SOURCES.md#a-word-about-blitzortung). |
| `earthquake.usgs` | `pollSeconds` | Recent quakes. |
| `wildfire.firms` | `satellite`, `boxWest`, `boxSouth`, `boxEast`, `boxNorth`, `lookBackHours`, `pollSeconds`, `mapKey` | VIIRS/MODIS thermal-anomaly detections. `satellite` is one of `VIIRS_SNPP` (default), `VIIRS_NOAA20`, `VIIRS_NOAA21`, `MODIS` — add one entry per satellite you want. The box defaults to one example area (US West) if all four `box*` fields are left at 0; it does not wrap the antimeridian. `mapKey` is optional — see below. |
| `orbital.celestrak` | `pollSeconds` | TLEs, propagated with SGP4. |
| `aviation.adsb` | `latitude`, `longitude`, `radiusNm`, `pollSeconds`, `routeLookup` | Regional circle around a point (max 250 nm). Add one entry per area you care about. `routeLookup: false` turns off the callsign → origin/destination enrichment (adsbdb.com); it is on by default. |
| `aviation.opensky` | `pollSeconds`, `clientId`, `clientSecret`, `credentialsFile` | One global snapshot per request. Anonymous access is credit-limited (100/day, 5 min floor, shipped 1800 s). OAuth2 client credentials from gitignored `local.json` switch the source to `oauth` (1000/day, 10 s floor). `login`/`password` are ignored. |
| `aviation.gpsjam` | `pollSeconds`, `broker` (CSV base), `topic` (manifest URL) | Daily H3-resolution-4 GNSS interference. Floor one hour. |
| `space.launchlibrary` | `pollSeconds`, `apiKey`, `broker`, `cacheDirectory` | Upcoming launches, 15 min floor. Optional `Authorization: Token`. |
| `weather.openmeteo` | `latitude`, `longitude`, `pollSeconds` | Current weather at a 0.1° cell. Requires coordinates. |
| `weather.openaq` | `apiKey`, `latitude`, `longitude`, `radiusNm`, `pollSeconds` | Air-quality stations. **Requires a free Explorer key** in `local.json`; refuse to start if enabled without one. |
| `geography.naturalearth` | — | Bundled public-domain 110m cartography (borders, cities, rivers, landmarks, regions). |
| `geography.cables` | `pollSeconds`, `cacheDirectory` | TeleGeography cable **routes** + landings, 6 h floor, removable `routes.json` cache. |
| `geophysics.eonet` | `pollSeconds` | NASA EONET open events. |
| `geophysics.gdacs` | `pollSeconds` | GDACS disaster alerts. |
| `maritime.portwatch` | `pollSeconds` | IMF PortWatch chokepoints, daily transits, and recent disruptions. |
| `weather.nhc` | `pollSeconds` | NOAA NHC active storm centres and 5-day forecast cones. Floor five minutes. |
| `space.pads` | `pollSeconds`, `apiKey`, `broker`, `cacheDirectory` | Earth spaceports from Launch Library 2. Floor six hours. |
| `humanitarian.hapi` | `apiKey`, `pollSeconds` | UNHCR refugee host/origin countries. **Requires an HDX HAPI app identifier** in `local.json`; refuse to start if enabled without one. |
| `humanitarian.reliefweb` | `apiKey`, `pollSeconds` | ReliefWeb current/alert disasters. **Requires a pre-approved appname** as `apiKey` in `local.json`; refuse to start if enabled without one. Floor five minutes. |
| `conflict.acled` | `login`, `password`, `apiKey`, `pollSeconds`, `lookBackHours` | ACLED recent events. **Requires myACLED `login`+`password` (OAuth) or a Bearer `apiKey`** in `local.json`; refuse to start if enabled without credentials. Floor ten minutes; look-back floor 24 hours (default 168). |
| `weather.nws` | `pollSeconds`, `broker`, `cacheDirectory` | NWS active alerts with native polygons. Floor two minutes. |
| `aviation.aviationweather` | `pollSeconds`, `cacheDirectory` | SIGMET + G-AIRMET. Floor two minutes. Open contours dropped. |
| `weather.spc` | `pollSeconds`, `cacheDirectory` | SPC day 1–3 categorical outlooks. Floor five minutes. |
| `geophysics.usgsvolcano` | `pollSeconds`, `cacheDirectory` | USGS volcano status points. Floor five minutes. |
| `geophysics.gvp` | `pollSeconds`, `broker`, `cacheDirectory` | Smithsonian GVP WFS catalogue. Floor six hours. |
| `earthquake.emsc` | `pollSeconds`, `cacheDirectory` | EMSC FDSN seismic events. Floor one minute. |
| `aviation.ourairports` | `pollSeconds`, `broker`, `cacheDirectory` | OurAirports CSV. Floor six hours. |
| `geography.marineregions` | `pollSeconds`, `cacheDirectory` | 200 NM EEZ lines, coarsened. Floor 24 hours. |
| `geography.powerplants` | `pollSeconds`, `broker`, `cacheDirectory` | WRI GPPD CSV. Floor 24 hours. |
| `orbital.satnogs` | `pollSeconds`, `cacheDirectory` | SatNOGS Online stations. Floor one hour. |
| `geography.ioda` | `pollSeconds`, `cacheDirectory` | IODA country outage alerts. Floor five minutes. |
| `humanitarian.unhcr` | `pollSeconds`, `broker`, `cacheDirectory` | UNHCR PoC sites. Floor six hours. |
| `aviation.openaip` | `apiKey`, `pollSeconds`, `broker`, `cacheDirectory` | OpenAIP airspaces. **Requires an API key** in `local.json`; refuse to start if enabled without one. CC BY-NC. Floor six hours. |
| `conflict.ucdp` | `apiKey`, `pollSeconds`, `broker`, `cacheDirectory` | UCDP GED events. **Requires a token** in `local.json`; refuse to start if enabled without one. Floor one hour. |
| `geography.cloudflare` | `apiKey`, `pollSeconds`, `broker`, `cacheDirectory` | Cloudflare Radar outages. **Requires a bearer token** in `local.json`; refuse to start if enabled without one. Floor five minutes. |
| `maritime.gfw` | `apiKey`, `pollSeconds`, `broker`, `cacheDirectory` | GFW fishing events. **Requires a bearer token** in `local.json`; refuse to start if enabled without one. CC BY-NC. Floor one hour. |
| `hamradio.rbn` | `login` | Reverse Beacon Network telnet; **requires a real callsign**. Disabled by default. |
| `aprs.is` | `login`, `filter`, `broker`, `latitude`/`longitude`/`radiusNm` | APRS-IS packet stream; **requires a real callsign**. Disabled by default. |
| `wsjtx.udp` | `broker` (listen address) | Local WSJT-X UDP feed. |
| `ais.aisstream` | `apiKey`, `boundingBoxes` | aisstream.io global AIS stream; **requires a free API key** (see [DATA-SOURCES.md](DATA-SOURCES.md#enabling-ais-ships-aisstreamio)). Disabled by default. `boundingBoxes` is a list of `{minLatitude, maxLatitude, minLongitude, maxLongitude}` rectangles — at least one is required; add more entries for several regions at once. |

### Common adjustments

**Point the aircraft view at your own area.** The shipped configuration
contains one illustrative circle (`adsb-region-example`, 50.0/8.0, 250 nm).
Replace it with your own — this is an example, not a recommended location:

```json
{ "id": "adsb-home", "type": "aviation.adsb", "enabled": true,
  "latitude": 47.3, "longitude": 8.5, "radiusNm": 250, "pollSeconds": 60 }
```

Add more entries with different centres for wider coverage. All sources share a
global request gate and honour rate limiting, but keep intervals ≥ 30 s and be
considerate: these are volunteer-run services.

**Point the wildfire layer at your own area.** The shipped configuration
contains one illustrative box (`firms-westus-example`, roughly the US West).
Replace it — this is an example, not a recommended location:

```json
{ "id": "firms-home", "type": "wildfire.firms", "enabled": true,
  "satellite": "VIIRS_SNPP",
  "boxWest": 5.9, "boxSouth": 45.8, "boxEast": 10.5, "boxNorth": 47.8,
  "lookBackHours": 24, "pollSeconds": 10800 }
```

Add more entries with different boxes or a different `satellite` for wider
coverage; each source instance polls one satellite's feed. Keep `pollSeconds`
at 1800 or above (see [DATA-SOURCES.md](DATA-SOURCES.md)) — FIRMS itself only
refreshes its products roughly once an hour, so polling faster just repeats
the same download.

**Use a FIRMS MAP_KEY (optional).** By default this source downloads NASA's
no-key global CSV snapshot and filters it locally, which needs no credentials
at all. Registering a free
[MAP_KEY](https://firms.modaps.eosdis.nasa.gov/api/map_key/) and setting it
as `mapKey` switches the source to FIRMS' Area API instead, which filters
server-side to your box — far less bandwidth, at the cost of a five-minute
sign-up. Never commit a real key; set it only in your local `live.json`.

**Reverse Beacon Network / DX cluster / APRS-IS** ship enabled with
placeholder login `HB9HSJ`. Overlay your own callsign on those source IDs
in `local.json`. APRS-IS always logs in read-only (passcode `-1`).

**Enable AIS ships** — create a free API key at
[aisstream.io](https://aisstream.io) and put it on `ais-aisstream-example`
in `local.json`. The source is already enabled; without a key it idles
instead of taking the collector down. The shipped boxes are regional
high-traffic areas (not the whole ocean). Replace them if you want a
different watch:

```json
{ "id": "ais-home", "type": "ais.aisstream", "enabled": true,
  "apiKey": "<your key>",
  "boundingBoxes": [
    { "minLatitude": 25.6, "maxLatitude": 25.9, "minLongitude": -80.9, "maxLongitude": -79.9 }
  ] }
```

Bounding boxes are still required. See [DATA-SOURCES.md](DATA-SOURCES.md#enabling-ais-ships-aisstreamio)
for what the provider's terms expect in return.

**Enable APRS-IS** — already on; overlay your callsign. By default it subscribes to an
illustrative 300 km circle around the same example point as `aviation.adsb`
plus position/object/item packet types only — not the entire world feed.
Point it at your own area instead:

```json
{ "id": "aprsis-home", "type": "aprs.is", "enabled": true,
  "login": "YOUR-CALLSIGN", "latitude": 47.3, "longitude": 8.5, "radiusNm": 100 }
```

Or set `filter` directly to any [APRS-IS filter spec](https://www.aprs-is.net/javAPRSFilter.aspx)
(e.g. `"filter": "r/47.3/8.5/150 t/poi"`) for full control; the special value
`"filter": "world"` removes the filter entirely (the full global feed — mind
the volume). Supported packet types are uncompressed and compressed position
reports, Objects, Items, and Mic-E (position and symbol, not course/speed);
everything else (messages, status, telemetry, positionless weather,
third-party) is intentionally skipped.

**OpenSky OAuth2 (optional).** Copy `collector/configs/local.json.example` to
the gitignored `collector/configs/local.json` and put an OpenSky API client
`clientId` / `clientSecret` on the `opensky-world` source (or point
`credentialsFile` at the `credentials.json` OpenSky offers for download).
The overlay is merged onto `live.json` at load time; tracked configs never
carry secrets. Without credentials the source stays in `anon` mode. Lowering
`pollSeconds` without credentials is rejected (5 minute anonymous floor).

**OpenAQ (optional).** Same overlay: put an Explorer API key on
`openaq-example`, then set `"enabled": true` on that source in `live.json`
or on the same overlay entry. The collector refuses to start if the source
is enabled without a key. Left **disabled** in tracked `live.json` for that
reason.

**HDX HAPI displacement (optional).** Mint an app identifier (application
name + contact email, not a secret) from
[HAPI's encode endpoint](https://hapi.humdata.org/api/v2/encode_app_identifier),
put it on `hapi-displacement` as `apiKey` in `local.json`, then set
`"enabled": true` on that source in `live.json` or on the same overlay
entry. The collector refuses to start if the source is enabled without an
identifier. Left **disabled** in tracked `live.json` for that reason.

**ReliefWeb disasters (optional).** Request a pre-approved `appname` from
[ReliefWeb's form](https://docs.google.com/forms/d/e/1FAIpQLScR5EE_SBhweLLg_2xMCnXNbT6md4zxqIB00OL0yZWyrqX_Nw/viewform?usp=header),
put it on `reliefweb-disasters` as `apiKey` in `local.json`, then set
`"enabled": true` on that overlay entry (or in `live.json`). The collector
refuses to start if the source is enabled without an appname. Left
**disabled** in tracked `live.json`.

**ACLED conflict events (optional).** Register at
[myACLED](https://acleddata.com/user/register), overlay `login` (email) and
`password` on `acled-events` in `local.json` (or a 24-hour Bearer token as
`apiKey`), then set `"enabled": true` on that overlay entry. The collector
refuses to start if the source is enabled without credentials. Left
**disabled** in tracked `live.json`. Do not commit tokens. This is not a
redistributable bundled dataset.

**OpenAIP / UCDP / Cloudflare Radar / Global Fishing Watch (optional).**
Same overlay pattern as OpenAQ: put the token on the matching source id
in `local.json` (`openaip-airspace`, `ucdp-ged`, `cloudflare-radar`,
`gfw-fishing`), then set `"enabled": true` on that source in `live.json`.
The collector refuses to start if any of these is enabled without a key.
All four stay **disabled** in tracked `live.json`. OpenAIP and GFW are
CC BY-NC.

**Turn on recording** (`recording.enabled: true`) to capture a JSONL event log
for later replay. Mind the volume — the live feeds produce several GB per hour;
`maxTotalGigabytes` deletes the oldest hourly files once the cap is reached.

### Command-line flags

| Flag | Effect |
| --- | --- |
| `-config <path>` | path to the JSON configuration |
| `-listen <addr>` | override `server.listenAddress`, e.g. `-listen 127.0.0.1:17810` |

The launchers use `-listen` automatically: if the canonical port 7810 is in use
or reserved by Windows, they fall back to 17810, then 27810, and point the
client at whichever port the collector actually got (via `-IonCollectorUrl`).
See [TROUBLESHOOTING.md](TROUBLESHOOTING.md#the-globe-is-empty) for the
reserved-port background.

### Checking the collector

```
http://127.0.0.1:7810/api/health      status
http://127.0.0.1:7810/api/status      sources and their state
http://127.0.0.1:7810/api/stats  accepted / dropped / evicted counters
```

(Substitute the port if the launcher fell back to 17810 or 27810.)

Installed builds write collector logs to
`%LOCALAPPDATA%\IonCommand\logs\collector-out.log`.
