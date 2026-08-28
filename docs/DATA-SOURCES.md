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
| **APRS-IS** | Data courtesy of APRS-IS and the amateur radio operators whose stations, digipeaters and IGates feed it. APRS-IS is maintained and operated by volunteer amateur radio operators to support Amateur Radio APRS on RF; "APRS" is a trademark of Tucson Amateur Packet Radio (TAPR). No formal data licence is published, and the network's own maintainers have publicly discouraged commercial harvesting of the feed. Requires **your own** amateur callsign as login (the read-only passcode `-1` needs no verification, but ION COMMAND still asks for a real callsign rather than a shared placeholder). Contact the APRS-IS maintainers before any commercial or bulk-redistribution use. | [aprs-is.net](https://www.aprs-is.net) |
| **NOAA / NWS SWPC** | Space-weather data from the NOAA Space Weather Prediction Center (public domain, US Government work). NOAA does not endorse this project. ION COMMAND's HF-conditions verdict is a **derived heuristic, not an official NOAA forecast**. | [swpc.noaa.gov](https://www.swpc.noaa.gov) |
| **U.S. Geological Survey** | Earthquake data from the U.S. Geological Survey (public domain). | [earthquake.usgs.gov](https://earthquake.usgs.gov/earthquakes/feed/) |
| **CelesTrak** | Orbital elements courtesy of CelesTrak (Dr. T.S. Kelso). Users must observe the CelesTrak usage policy; the collector backs off and stops on repeated non-200 responses. | [celestrak.org](https://celestrak.org/usage-policy.php) |
| **GIRO / prop.kc2g.com** | Ionosonde data distributed through GIRO and INGV, made available via prop.kc2g.com (Ian, KC2G), funded by WWROF. Licensed **CC BY-NC-SA 4.0 — non-commercial, share-alike**; recorded ionosonde data inherits this licence. Cite: Reinisch, B. W., and I. A. Galkin, *Global Ionospheric Radio Observatory (GIRO)*, Earth, Planets and Space, 63, 377–381, doi:10.5047/eps.2011.03.001, 2011. | [GIRO Rules of the Road](https://giro.uml.edu/didbase/RulesOfTheRoad.html) |
| **Blitzortung.org** | Lightning data courtesy of Blitzortung.org and its volunteer station operators. Private and entertainment use only; **commercial use is prohibited**; raw data access is intended for **participating station operators**. Not an official information service and **not a warning system** — never rely on it for safety decisions. See the note below. | [blitzortung.org](https://www.blitzortung.org) |
| **adsb.lol** | Aircraft data from adsb.lol, licensed **ODbL 1.0**. Redistributing recorded ADS-B data means releasing that data under ODbL. Consider feeding adsb.lol to give back. | [adsb.lol](https://www.adsb.lol) · [ODbL](https://opendatacommons.org/licenses/odbl/1-0/) |
| **adsbdb** | Flight-route data (callsign → origin/destination airport) from adsbdb.com, a free community API (MIT-licensed service). The route data itself is **the work of David Taylor, Edinburgh and Jim Mason, Glasgow, and "may not be copied, published, or incorporated into other databases without the explicit permission of David J Taylor, Edinburgh"**. ION COMMAND displays routes to a single local operator and does not publish or redistribute them; recording is off by default, and any local recording is private and rotates within a size cap. If you intend to publish, share, or otherwise redistribute recorded route data, ask for permission first, or set `routeLookup: false`. Lookups are cached and globally spaced to stay hobby-scale. Routes are filed schedules, not clearances — an aircraft may actually be going somewhere else. | [adsbdb.com](https://www.adsbdb.com) · [source](https://github.com/mrjackwills/adsbdb) |
| **OpenSky Network** | Aircraft data from the OpenSky Network. Licensed for **non-profit research and education only**; commercial or operational use requires written permission from OpenSky. Conditions pass through to downstream users. Cite: Schäfer, M., Strohmeier, M., Lenders, V., Martinovic, I., Wilhelm, M., *Bringing Up OpenSky: A Large-scale ADS-B Sensor Network for Research*, IPSN 2014, pp. 83–94. | [Terms of use](https://opensky-network.org/about/terms-of-use) |
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
| `orbital.celestrak` | celestrak.org `gp.php` TLEs, SGP4-propagated locally | TLE refresh 6 h, positions 10 s | `orbital.position` | yes |
| `aviation.adsb` | adsb.lol `/v2/point/<lat>/<lon>/<nm>`; route lookups via api.adsbdb.com (cached, global 2 s gate, `routeLookup: false` disables) | 60 s, global 4 s request gate | `aviation.aircraft` | yes (one example circle) |
| `aviation.opensky` | opensky-network.org `/api/states/all` | 1800 s anonymous | `aviation.aircraft` | yes |
| `hamradio.rbn` | telnet `telnet.reversebeacon.net` | streaming | `hamradio` spots | **no** (needs your callsign) |
| `aprs.is` | TCP `rotate.aprs2.net:14580`, read-only login (passcode `-1`) | streaming | `aprs` → `aprs.station`, `aprs.object` | **no** (needs your callsign) |
| `wsjtx.udp` | local UDP listener | streaming | `hamradio` | no |

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
- Sources that need an identity (RBN) ship **disabled** rather than with a
  placeholder callsign.

Please keep it that way if you change the configuration. These services are
free because people pay for them with their own time and hardware.

## A word about APRS-IS

The APRS-IS layer is **off by default**, for the same reason as the RBN: it
needs your own amateur callsign, so it cannot ship pre-configured.

APRS-IS exists to support Amateur Radio APRS on RF. Its own maintainers ask
that:

- Traffic on APRS-IS stay suitable for gating to and from real RF networks -
  ION COMMAND's use is receive-only, so this does not apply to it directly,
  but it is why the collector always logs in with the documented **read-only
  passcode (`-1`)**: it can never inject anything onto the network.
- Client software identify itself properly rather than impersonating another
  tool - the collector's `vers` login field says `ion-command-collector 0.1`.
- Nobody use the public servers to **harvest data for commercial purposes**;
  the network's maintainers have said in public discussion that they do not
  consider themselves able to grant that, even though the data itself is not
  formally copyrighted.

So please, if you turn this source on:

- Use **your own** licensed amateur callsign in `login`, not a shared
  placeholder - a real callsign costs nothing to use for a read-only
  connection and is how the network expects clients to identify themselves.
- Keep the shipped default (or your own) **filter** narrow rather than
  subscribing to the entire world: see [CONFIGURATION.md](CONFIGURATION.md).
- Do not use this source for commercial data harvesting.

### How the collector tries to be a good citizen

- Always logs in with passcode `-1` (read-only); it has no code path that
  could transmit a packet onto the network.
- Ships with a bounded server-side filter by default - an example 300 km
  circle plus position-bearing packet types only - instead of the full
  firehose, and documents how to point it at your own area.
- Reconnects with exponential backoff (5 s up to 5 min) rather than
  hammering a server that drops the connection.

## Known gaps

- **OpenSky authentication**: the `login`/`password` fields use HTTP basic
  auth, which OpenSky has retired in favour of OAuth2 client credentials.
  Anonymous access still works; authenticated access currently does not.
- PSKReporter, prop.kc2g.com, the RBN and APRS-IS publish no formal data
  licence. Usage here is hobby-scale and attributed; contact them before any
  larger or commercial use.
- **APRS-IS Mic-E course/speed** is intentionally not decoded (position and
  symbol are); see the collector's `aprs` domain package comment for why the
  published algorithm could not be verified against its own worked example.
