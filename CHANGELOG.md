# Changelog

All notable changes to ION COMMAND are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The day-by-day engineering log, including retracted measurements and
superseded configurations, is in
[docs/history/DEVELOPMENT-LOG.md](docs/history/DEVELOPMENT-LOG.md).

## [Unreleased]

Nothing yet.

## [0.9.0] — 2026-07-21

First packaged and published release. Windows x64, pre-1.0.

### Added

- **Windows installer** (Inno Setup): Shipping client without debug symbols,
  Go collector, neutral default configuration, launcher, Start Menu entries and
  an uninstaller; verified install → run → uninstall.
- **Settings panel** in the client (overlay menu → `SETTINGS`): callsign and
  grid locator editable in-app, plus marker lifetime, minimum flight level and
  show-ground-aircraft. Persisted to `Game.ini`, applied live.
- **Aviation declutter**: hide aircraft below a configurable flight level
  and/or on the ground.
- **`MY RX/TX ONLY`** overlay-menu row (equivalent to the `M` key): show only
  paths where the own station is transmitter or receiver.
- **Global aviation** via a new `aviation.opensky` source (one worldwide
  snapshot per request), alongside regional `aviation.adsb` circles.
- **Airframe-type pictograms** (airliner, helicopter, glider, balloon, drone),
  heading-oriented glyphs, dead reckoning between updates, and emergency-squawk
  highlighting for 7500/7600/7700.
- **Live cloud imagery** from the EUMETSAT world infrared composite, refreshed
  hourly, replacing a static climatology texture.
- **Real star sky**: NASA SVS Deep Star Map rotated to Greenwich Mean Sidereal
  Time.
- **Retained state**: the latest message per entity is replayed to every client
  on connect, so the globe fills in seconds instead of waiting for a slow poll.
- **Marker hover tooltips** and a clickable **overlay menu** with per-layer and
  per-domain visibility.
- **Always-visible own-station reticle** drawn in HUD space.
- Data sources added over the cycle: Blitzortung lightning, USGS earthquakes,
  CelesTrak satellites (SGP4), GOES X-ray class, GIRO/KC2G ionosonde soundings,
  Reverse Beacon Network, adsb.lol aircraft.
- Documentation set: README, `docs/COMPONENTS.md`, `CONFIGURATION.md`,
  `DATA-SOURCES.md`, `BUILDING.md`, `TROUBLESHOOTING.md`, plus
  `THIRD_PARTY_NOTICES.md`.

### Changed

- The diegetic deck chrome (floating title and three console panels) is hidden
  by default; `-IonShowDeck` restores it.
- Own-station marker scales with camera distance instead of a fixed size, so
  zooming in no longer turns it into a bloom.
- Anonymous OpenSky polling relaxed to 1800 s to stay inside the credit budget.
- Client queue capacity raised above the retain cap so a connecting client
  cannot receive a truncated snapshot.
- Licence clarified: the MIT grant covers the project's own code; bundled
  third-party content and live data are governed separately.

### Fixed

- **Stationary markers were invisible.** The heading billboard normalised a
  zero heading to NaN, which survived the blend, collapsing every marker
  without a course (stations, satellites, lightning, quakes, sounders) into a
  degenerate quad.
- **CelesTrak retry loop.** A failed fetch left the element set empty, so the
  position ticker refetched every 10 s indefinitely — contrary to CelesTrak's
  usage policy. Now exponential backoff with a hard stop.
- **ADS-B rate limiting.** Five regional queries from one address tripped
  throttling; added a global request gate and backoff for HTTP 429 and
  adsb.lol's 420.
- **Frozen aircraft.** Movement was compared against the continuously updated
  bookkeeping position instead of the rendered one, so markers never moved.
- **Emergency squawk could be cleared** by a later sighting from another source
  without squawk information; the alarm state is now sticky.
- **Georeference.** Stations rendered a mirrored quarter turn off; the world
  frame is now pinned by a test.
- Sun and star sphere now use the same IAU-1982 sidereal time.
- Ghost tooltips on expired markers; stale overlay-menu click targets while the
  HUD was hidden; polar collapse of the compass heading frame.
- Retracted an incorrect renderer benchmark: the original figures were a
  frame-counter wrap artefact.

### Security

- The collector binds `127.0.0.1` only and has no authentication; this is
  documented rather than assumed.
- Maintainer callsign and home locator removed from tracked configuration.

[Unreleased]: https://github.com/svabi79/ion-command/compare/v0.9.0...HEAD
[0.9.0]: https://github.com/svabi79/ion-command/releases/tag/v0.9.0
