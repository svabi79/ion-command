# Data sources and attribution

ION COMMAND does not host any data. Everything on the globe comes from public
services run by other people — most of them volunteers, several of them
non-commercial only.

> **The MIT licence covers ION COMMAND's own code, not the data it fetches.**
> Commercial use of ION COMMAND does **not** grant commercial use of these
> feeds. If you fork or redistribute, change the collector's User-Agent
> (`ion-command-collector/0.1 (+https://github.com/svabi79/ion-command)`) to
> identify your build, and set your own callsign where a source requires one.

## Attribution

| Provider | Credit / terms | Link |
| --- | --- | --- |
| **PSKReporter** | Spot data by PSKReporter (Philip Gladstone, N1DQ). The MQTT feed is operated by Tom M0LTE on donated hosting. No formal terms published. | [pskreporter.info](https://pskreporter.info) |
| **Reverse Beacon Network** | Data courtesy of the Reverse Beacon Network and its skimmer operators. Requires **your own** amateur callsign as login. No formal data licence published; contact RBN before non-hobby use. | [reversebeacon.net](https://reversebeacon.net) |
| **NOAA / NWS SWPC** | Space-weather data from the NOAA Space Weather Prediction Center (public domain, US Government work). NOAA does not endorse this project. ION COMMAND's HF-conditions verdict is a **derived heuristic, not an official NOAA forecast**. | [swpc.noaa.gov](https://www.swpc.noaa.gov) |
| **U.S. Geological Survey** | Earthquake data from the U.S. Geological Survey (public domain). | [earthquake.usgs.gov](https://earthquake.usgs.gov/earthquakes/feed/) |
| **NASA FIRMS** | Active-fire thermal-anomaly detections from NASA's Fire Information for Resource Management System (FIRMS): VIIRS (S-NPP, NOAA-20, NOAA-21) and MODIS (Terra/Aqua), distributed through LANCE. This is near-real-time data — generated fast rather than fully quality-checked like the standard science product — provided "as is"; NASA does not endorse this project, and a detection is a thermal anomaly, not a confirmed fire (ION COMMAND's own display text says so). Cite: NASA VIIRS Land Science Team, *VIIRS Active Fire Product NRT* (S-NPP), DOI [10.5067/FIRMS/VIIRS/VNP14IMGT_NRT.002](https://doi.org/10.5067/FIRMS/VIIRS/VNP14IMGT_NRT.002); *MODIS Collection 6.1 NRT Hotspot/Active Fire Detections MCD14DL*, DOI [10.5067/FIRMS/MODIS/MCD14DL.NRT.006](https://doi.org/10.5067/FIRMS/MODIS/MCD14DL.NRT.006). | [firms.modaps.eosdis.nasa.gov](https://firms.modaps.eosdis.nasa.gov) |
| **CelesTrak** | Orbital elements courtesy of CelesTrak (Dr. T.S. Kelso). Users must observe the CelesTrak usage policy; the collector backs off and stops on repeated non-200 responses. | [celestrak.org](https://celestrak.org/usage-policy.php) |
| **GIRO / prop.kc2g.com** | Ionosonde data distributed through GIRO and INGV, made available via prop.kc2g.com (Ian, KC2G), funded by WWROF. Licensed **CC BY-NC-SA 4.0 — non-commercial, share-alike**; recorded ionosonde data inherits this licence. Cite: Reinisch, B. W., and I. A. Galkin, *Global Ionospheric Radio Observatory (GIRO)*, Earth, Planets and Space, 63, 377–381, doi:10.5047/eps.2011.03.001, 2011. | [GIRO Rules of the Road](https://giro.uml.edu/didbase/RulesOfTheRoad.html) |
| **Blitzortung.org** | Lightning data courtesy of Blitzortung.org and its volunteer station operators. Private and entertainment use only; **commercial use is prohibited**; raw data access is intended for **participating station operators**. Not an official information service and **not a warning system** — never rely on it for safety decisions. See the note below. | [blitzortung.org](https://www.blitzortung.org) |
| **adsb.lol** | Aircraft data from adsb.lol, licensed **ODbL 1.0**. Redistributing recorded ADS-B data means releasing that data under ODbL. Consider feeding adsb.lol to give back. | [adsb.lol](https://www.adsb.lol) · [ODbL](https://opendatacommons.org/licenses/odbl/1-0/) |
| **adsbdb** | Flight-route data (callsign → origin/destination airport) from adsbdb.com, a free community API (MIT-licensed service). The route data itself is **the work of David Taylor, Edinburgh and Jim Mason, Glasgow, and "may not be copied, published, or incorporated into other databases without the explicit permission of David J Taylor, Edinburgh"**. ION COMMAND displays routes to a single local operator and does not publish or redistribute them; recording is off by default, and any local recording is private and rotates within a size cap. If you intend to publish, share, or otherwise redistribute recorded route data, ask for permission first, or set `routeLookup: false`. Lookups are cached and globally spaced to stay hobby-scale. Routes are filed schedules, not clearances — an aircraft may actually be going somewhere else. | [adsbdb.com](https://www.adsbdb.com) · [source](https://github.com/mrjackwills/adsbdb) |
| **OpenSky Network** | Aircraft data from the OpenSky Network. Licensed for **non-profit research and education only**; commercial or operational use requires written permission from OpenSky. Conditions pass through to downstream users. Cite: Schäfer, M., Strohmeier, M., Lenders, V., Martinovic, I., Wilhelm, M., *Bringing Up OpenSky: A Large-scale ADS-B Sensor Network for Research*, IPSN 2014, pp. 83–94. | [Terms of use](https://opensky-network.org/about/terms-of-use) |
| **AIS Stream (aisstream.io)** | Vessel AIS data from aisstream.io, free with a self-service API key. No commercial-use restriction or redistribution licence is published as of this writing; treated with the same hobby-scale, attributed posture as the other unlicensed feeds below until the operator confirms otherwise. Direct browser connections to the stream are against its terms — ION COMMAND already only connects from the collector process, never from client JavaScript, so this is satisfied by the existing architecture. See [below](#enabling-ais-ships-aisstreamio) for the full picture. | [aisstream.io](https://aisstream.io) · [docs](https://aisstream.io/documentation) |
| **EUMETSAT** | Cloud imagery ©EUMETSAT 2026. Governed by the EUMETSAT Data Policy, not Creative Commons. | [Terms of use](https://www.eumetsat.int/about-us/terms-use) |
| **NASA** | Day globe: Blue Marble Next Generation, NASA Earth Observatory (Reto Stockli, NASA/GSFC). Night: Black Marble 2016, NASA Earth Observatory/NOAA NCEI (Joshua Stevens, Suomi NPP VIIRS data, Miguel Román, NASA/GSFC). Clouds: NASA Earth Observatory Blue Marble (record 57747). Star map: NASA/GSFC Scientific Visualization Studio, *Deep Star Maps 2020*; Gaia DR2: ESA/Gaia/DPAC; also Hipparcos-2, Tycho-2, UCAC3; constellation figures based on those developed for the IAU by Alan MacRobert of Sky & Telescope (Roger Sinnott and Rick Fienberg). NASA does not endorse ION COMMAND. | [SVS 4851](https://svs.gsfc.nasa.gov/4851/) |
| **AD1C** | DXCC prefix data from the Big CTY file by Jim Reisert, AD1C (version VER20260714). Full copyright and permission notice in [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md). | [country-files.com](https://www.country-files.com) |
| **ADIF** | DXCC entity table derived from the ADIF specification. | [adif.org](https://adif.org) |
| **Epic Games** | ION COMMAND uses the Unreal® Engine. Unreal® is a trademark or registered trademark of Epic Games, Inc. in the United States of America and elsewhere. Unreal® Engine, Copyright 1998 – 2026, Epic Games, Inc. All rights reserved. | [unrealengine.com](https://www.unrealengine.com) |

WSJT-X is deliberately absent from the table: that source is a purely local UDP
listener, so no third-party terms apply.

## Technical reference

What each source actually does, in the order it appears in `live.json`:

| `type` | Endpoint / transport | Default cadence | Emits | Enabled by default |
| --- | --- | --- | --- | --- |
| `pskreporter.mqtt` | `mqtt.pskreporter.info:1883`, topic `pskr/filter/v2/#` | streaming, ~300–500 spots/s | `hamradio` → `radio.link`, `radio.station` | yes |
| `spaceweather.swpc` | services.swpc.noaa.gov JSON + `wwv.txt` | 300 s | `spaceweather.state` | yes |
| `ionosonde.kc2g` | prop.kc2g.com `stations.json` | 600 s (floor 300 s) | `ionosphere.sounding` | yes |
| `lightning.blitzortung` | `wss://ws*.blitzortung.org/` | streaming | `weather.lightning` | yes — [read this](#a-word-about-blitzortung) |
| `earthquake.usgs` | earthquake.usgs.gov GeoJSON feed | 600 s | `geophysics.earthquake` | yes |
| `wildfire.firms` | firms.modaps.eosdis.nasa.gov global per-satellite CSV snapshot (no key), or the MAP_KEY-scoped Area API when `mapKey` is set | 10800 s (floor 1800 s) | `wildfire.detection` | yes — one example area (US West) |
| `orbital.celestrak` | celestrak.org `gp.php` TLEs, SGP4-propagated locally | TLE refresh 6 h, positions 10 s | `orbital.position` | yes |
| `aviation.adsb` | adsb.lol `/v2/point/<lat>/<lon>/<nm>`; route lookups via api.adsbdb.com (cached, global 2 s gate, `routeLookup: false` disables) | 60 s, global 4 s request gate | `aviation.aircraft` | yes (one example circle) |
| `aviation.opensky` | opensky-network.org `/api/states/all` | 1800 s anonymous | `aviation.aircraft` | yes |
| `hamradio.rbn` | telnet `telnet.reversebeacon.net` | streaming | `hamradio` spots | **no** (needs your callsign) |
| `wsjtx.udp` | local UDP listener | streaming | `hamradio` | no |
| `ais.aisstream` | `wss://stream.aisstream.io/v0/stream` | streaming | `maritime.vessel` | **no** (needs a free API key) |

## A word about Blitzortung

The lightning layer is **on by default**, and you should know what that means
before you leave it on.

Blitzortung.org is not a company. It is a network of volunteers who each bought
and maintain a detector, and who share the result for free. Their terms are
explicit:

- **Private and entertainment use only. Commercial use is prohibited.**
- **Raw data access is intended for people who operate a station** and
  contribute data back to the network.
- It is **not an official information service and not a warning system.** Never
  use it for any safety-relevant decision.

ION COMMAND consumes the raw stream. If you are not running a detector, you are
taking from that network without giving back. That is tolerated, not invited.

So please, one of these:

- **[Run a station](https://www.blitzortung.org/en/cover_your_area.php)** if you
  can — that is what keeps the network alive; or
- **turn the layer off** if you do not need it: set
  `"enabled": false` on the `lightning.blitzortung` source in
  `collector/configs/live.json`, or simply hide `WEATHER MARKERS` in the
  overlay menu (that only stops the drawing — set `enabled: false` to stop the
  connection).

If you redistribute a build of ION COMMAND commercially, you **must** disable
this source: the MIT licence on this project's code does not, and cannot, grant
you commercial rights to Blitzortung's data.

## Enabling AIS ships (aisstream.io)

The maritime layer ships **disabled by default** because it needs a
credential nothing in this repository can supply. Turning it on is a
deliberate, three-step operator decision:

1. **Create a free account** at [aisstream.io](https://aisstream.io) and
   generate an API key from its Account page. No payment details are
   requested for the free tier at the time of writing.
2. **Edit `collector/configs/live.json`**: set `"enabled": true` on the
   `ais.aisstream` source, paste the key into `"apiKey"`, and replace the
   example `boundingBoxes` entry with the ocean area(s) you actually want —
   see [CONFIGURATION.md](CONFIGURATION.md) for the field shapes.
3. **Restart the collector.** If the key or bounding boxes are missing, the
   collector refuses to start with a clear error rather than silently doing
   nothing — the same fail-fast behaviour as the Reverse Beacon Network
   source when its callsign is missing.

### Why aisstream.io and not something else

Global, real-time AIS coverage without owning a physical receiver is a
narrower field than it looks:

- **AISHub** is a *reciprocal* network: an API credential requires operating
  your own AIS receiver and feeding it raw NMEA data back for about a week
  before access is granted. There is no way to just sign up. Not usable here.
- **MarineTraffic, VesselFinder** and similar commercial aggregators are paid
  services with restrictive redistribution terms; they do not fit a
  self-hosted hobby project the way the rest of this table does.
- **National feeds** (e.g. the Danish Maritime Authority, Norway's
  BarentsWatch, the US Coast Guard's NAIS) exist and some are free, but each
  covers only that country's waters — not the "global maritime traffic" this
  layer is meant to show — and each has its own separate registration
  friction on top.
- **aisstream.io** is free, genuinely global, requires only a self-service
  API key (no receiver, no waiting period, no payment details), and is
  already what several other open-source AIS hobby projects build on.

### What its terms actually say

As of this writing, aisstream.io's documentation and site do **not** publish
a dedicated terms-of-service page, and no explicit commercial-use or
redistribution restriction was found during research for this feature —
unlike Blitzortung or OpenSky above, there is no "non-commercial only"
clause to quote. What the documentation **does** state, and what this
integration honours:

- **API keys are personal and server-side.** "Direct browser connections are
  not permitted; connect from your own server and proxy only the information
  your clients need." ION COMMAND's Unreal client never talks to aisstream.io
  directly — it only ever talks to the local collector, which holds the key.
  This requirement is satisfied by the existing architecture, not by
  anything added for this source specifically.
- **Rate limits**: three subscribed connections per account, three open
  connections per source IP, a subscription must arrive within three seconds
  of connecting, and at most one subscription update per second. This
  collector opens exactly one connection per configured `ais.aisstream`
  source instance and never resubscribes on a running connection.
- **No uptime or delivery guarantee**, and messages are dropped, not queued,
  if a client reads too slowly. The source reconnects with exponential
  backoff (10 s up to 5 minutes) rather than retrying in a tight loop, and
  resets the backoff after a connection has stayed healthy for a couple of
  minutes, so a bad key or an outage cannot look like abuse.

Given the absence of a published licence, this feed is treated the same way
PSKReporter, prop.kc2g.com and the RBN are treated elsewhere in this file:
hobby-scale, attributed, non-redistributed by default. If you intend
anything beyond that — republishing recorded AIS data, a commercial
deployment — contact aisstream.io first and do not assume the silence on
their site means permission.

### The well-known AIS pitfalls this integration handles

AIS is a much fussier protocol than it looks from the outside, and getting
these wrong silently produces a globe full of wrong ships:

- **Non-vessel MMSIs.** The same message transport also carries base
  stations, aids to navigation, SAR aircraft, and a handful of smaller
  categories, identified by the leading digits of their MMSI rather than by
  the message type alone. The `maritime` domain classifies every MMSI before
  it can become a vessel entity, on top of the source only ever subscribing
  to vessel message types in the first place.
- **"Not available" sentinel values.** A missing GPS fix is longitude 181 /
  latitude 91, not an omitted field; missing speed is 102.3 knots; missing
  course is 360.0°; missing heading is 511; missing rate of turn is -128. All
  five are recognised explicitly and turned into an absent property rather
  than a bogus 181°E marker or a "360° heading" tooltip.
- **Position vs. static/voyage data.** A vessel's name, type, call sign,
  destination and ETA arrive on a completely separate, much slower schedule
  than its position fixes (Class A) or as two independent split messages
  (Class B). The domain keeps a bounded per-MMSI cache and joins whatever it
  has learned onto each new position update.
- **Reporting cadence.** AIS itself expects a moving Class A vessel to report
  every 2–10 seconds but a moored or anchored one only every three minutes.
  The globe's validity window and the domain's own re-affirmation cadence are
  both derived from that difference, rather than one fixed number that would
  either flood the pipeline for busy ports or let quiet anchorages flicker
  off the map between reports.

### How ION COMMAND tries to be a good citizen

- All aviation requests pass a **global 4-second gate**, so several configured
  regions can never burst the aggregator.
- Route lookups (adsbdb.com) run on **one worker with a global 2-second gate**,
  every answer — including "unknown callsign" — is **cached for hours**, and a
  429 pauses the worker honouring `Retry-After` (five minutes when absent);
  transient transport errors only pause it briefly. A callsign is asked about
  at most once per TTL, however many aircraft are in view.
- HTTP **429** and adsb.lol's **420 "Enhance Your Calm"** trigger exponential
  backoff honouring `Retry-After`.
- CelesTrak failures back off from 5 minutes to 2 hours and **stop entirely**
  after repeated failures, as their usage policy requires.
- OpenSky is polled every 30 minutes anonymously to stay inside the credit
  budget; the last snapshot is retained and replayed to new clients instead of
  re-fetching.
- FIRMS regenerates its NRT snapshots roughly once an hour; the source polls
  every **three hours by default** (floor: thirty minutes, enforced whether or
  not a MAP_KEY is configured) and ships with **one bounded area of interest**
  rather than pulling the whole-world file and keeping all of it. A poll's
  fetched/kept/emitted/duplicate/invalid counts are logged every time.
- Sources that need an identity (RBN, aisstream.io) ship **disabled** rather
  than with a placeholder credential.
- The AIS source reconnects with **exponential backoff (10 s → 5 min)**
  rather than a tight retry loop, honouring the provider's three
  connections-per-account / three connections-per-IP limits, and resets the
  backoff only after a connection has proven healthy for a couple of minutes.

Please keep it that way if you change the configuration. These services are
free because people pay for them with their own time and hardware.

## Known gaps

- **OpenSky authentication**: the `login`/`password` fields use HTTP basic
  auth, which OpenSky has retired in favour of OAuth2 client credentials.
  Anonymous access still works; authenticated access currently does not.
- **FIRMS MAP_KEY (Area API) path**: implemented and unit-tested — URL
  construction and CSV parsing, since the Area API returns the same column
  shape as the no-key snapshots — but not exercised against a real key (none
  was available to this change). The verified, default path is the no-key
  global snapshot, filtered locally to the configured area and window.
- PSKReporter, prop.kc2g.com, the RBN and aisstream.io publish no formal data
  licence. Usage here is hobby-scale and attributed; contact them before any
  larger or commercial use.
- **The `ais.aisstream` live connection is unverified.** It was built and
  unit-tested against fixtures taken from aisstream.io's published
  documentation and OpenAPI schema, without ever holding a real API key — see
  [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md). A placeholder key was
  used to confirm the TLS/WebSocket handshake against the real
  `wss://stream.aisstream.io/v0/stream` succeeds and the subscribe message is
  accepted for delivery, but with no real key available, an authenticated
  session, real traffic actually reaching `ParseFrame`, sustained reconnect
  behaviour, and real-world message volume have not been exercised.
