# Configuration

ION COMMAND has two halves that are configured separately:

| Part | What it does | Where it is configured |
| --- | --- | --- |
| **Client** (the globe) | rendering, your station identity, display filters | in-app **SETTINGS** panel → written to `Saved/Config/Windows/Game.ini` |
| **Collector** (the data feed) | which services are polled, how often, recording | `collector/configs/live.json` (installed copy: `<install>\collector\configs\live.json`) |

Client settings apply immediately. Collector settings need a collector restart.

---

## Client: the SETTINGS panel

Press **O** to open the overlay menu, then click **SETTINGS >**.
(For unattended use you can also start the client with `-IonSettings`.)

| Row | Meaning |
| --- | --- |
| **CALLSIGN** | Your callsign. Click the row and type; **Enter** saves, **Esc** cancels. Drives the "you are here" reticle and the *MY RX/TX* path filter. |
| **GRID LOCATOR** | Your Maidenhead locator, e.g. `JO62qm`. Moves the home marker and reticle immediately. |
| **MARKER LIFETIME** | How long a marker stays after its last sighting (60 / 120 / 300 / 600 / 1200 s). Lower = tidier globe, higher = longer trails of activity. |
| **MIN FLIGHT LEVEL** | Hides aircraft below this level (OFF / FL050 / FL100 / FL200 / FL300). The quickest way to thin out dense airspace — at FL100 the airport clutter disappears and only cruising traffic remains. |
| **SHOW GROUND A/C** | Show or hide aircraft reported as on the ground. |
| **INVERT ORBIT Y** | Flip the vertical orbit direction of a right-mouse drag. OFF matches the horizontal drag convention; ON restores the pre-0.9.1 direction. |

While a text field is focused all hotkeys are suspended, so typing a callsign
cannot toggle layers.

Values persist to `Game.ini` under `[IonCommand.Station]` (callsign, locator),
`[IonCommand.Display]` (`MarkerLifetime`, `MinFlightLevelFt`, `ShowGround`) and
`[IonCommand.Input]` (`InvertOrbitY`).

## Client: overlay menu

Press **O**. Every row is clickable and shows its state:

- **SETTINGS >** — opens the panel above
- **PATHS** — the propagation arcs
- **MY RX/TX ONLY** — only paths where your station is transmitter or receiver
- **HEATMAP** — activity density splats
- **IONOSPHERE SHELLS** — ionosonde shells
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
| **Hover a marker** | tooltip with callsign/flight level/type/…; aircraft seen by an `aviation.adsb` circle also show their filed route (`CDG Paris  >  TUN Tunis`) once resolved |
| **Tab** | cycle HUD: full → minimal → hidden |
| **O** | overlay menu |
| **V** | show/hide paths |
| **M** | only my station's RX/TX paths |
| **H** | activity heatmap |
| **I** | ionosphere shells |
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
| `aviation.opensky` | `pollSeconds`, `login` (see note) | One global snapshot per request. Anonymous access is credit-limited; the shipped default is 1800 s. |
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

**Enable the Reverse Beacon Network** — set `enabled: true` and put your own
callsign in `login`.

**Enable AIS ships** — create a free API key at
[aisstream.io](https://aisstream.io), then set `enabled: true`, paste the key
into `apiKey`, and replace the example `boundingBoxes` entry with the area(s)
you actually want to watch:

```json
{ "id": "ais-home", "type": "ais.aisstream", "enabled": true,
  "apiKey": "<your key>",
  "boundingBoxes": [
    { "minLatitude": 25.6, "maxLatitude": 25.9, "minLongitude": -80.9, "maxLongitude": -79.9 }
  ] }
```

The collector refuses to start if `ais.aisstream` is enabled without an
`apiKey` or without at least one bounding box, rather than connecting with a
broken subscription. See [DATA-SOURCES.md](DATA-SOURCES.md#enabling-ais-ships-aisstreamio)
for what the provider's terms expect in return.
**Enable APRS-IS** — set `enabled: true` and put your own callsign in
`login` (the connection always logs in read-only, passcode `-1`, so no real
passcode is ever needed or sent). By default it subscribes to an
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

**OpenSky authentication does not currently work.** The `login`/`password`
fields use HTTP basic auth, which OpenSky retired in favour of OAuth2 client
credentials. Anonymous access works and is what the shipped configuration uses;
lowering `pollSeconds` without a working account will exhaust the anonymous
credit budget.

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
http://127.0.0.1:7810/api/statistics  accepted / dropped / evicted counters
```

(Substitute the port if the launcher fell back to 17810 or 27810.)

Installed builds write collector logs to
`%LOCALAPPDATA%\IonCommand\logs\collector-out.log`.
