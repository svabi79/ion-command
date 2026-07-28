# Changelog

All notable changes to ION COMMAND are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The day-by-day engineering log, including retracted measurements and
superseded configurations, is in
[docs/history/DEVELOPMENT-LOG.md](docs/history/DEVELOPMENT-LOG.md).

## [Unreleased]

### Added

- **Flight routes in the aircraft tooltip.** Aircraft seen by an
  `aviation.adsb` circle show their filed origin and destination in plain
  words (`CDG Paris  >  TUN Tunis`), resolved per callsign via adsbdb.com —
  cached, globally rate-gated, and switchable off with `routeLookup: false`
  on the source.
- **Close-range zoom.** The orbit camera now goes down to ~255 km above the
  surface (previously ~2900 km), with wheel steps that scale with distance so
  the last stretch is fine-grained instead of one overshooting notch. Markers
  and the own-station reticle keep a constant screen size through the whole
  range, so individual aircraft separate cleanly on an approach.

## [0.9.2] — 2026-07-26

**The installer published with 0.9.1 was defective and should not be used.** It
contained the 0.9.0 client, so none of the 0.9.1 client-side changes were in it.
The collector and launchers in it were correct. This release ships the payload
0.9.1 was meant to ship, plus the fix below.

### Fixed

- **Settings and overlay rows stayed clickable while the HUD was hidden.**
  `DrawHUD` returns early when the HUD is hidden or still fading in, and those
  paths left the panel's hit rectangles in place while clicks were still routed
  to them. A click near the middle of the screen with the HUD switched off could
  silently change a setting — hide most aircraft, flip the orbit axis, or enter
  text-entry mode — with nothing on screen to explain it, and the change
  persisted. Hit rectangles are now dropped whenever the HUD does not draw, and
  clicks fall through to the world instead. ([#2](https://github.com/svabi79/ion-command/issues/2))

### Build

- `tools\installer\build-installer.ps1` staged the client from `dist\release`,
  while `tools\package.ps1` archives to `dist\windows`. The stale directory
  passed the existence check, which is how 0.9.1 shipped the wrong client. The
  paths now match, and the script prints both binary timestamps and refuses a
  client older than the collector it is packaged with.
- `tools\package.ps1` empties the archive directory first. BuildCookRun adds to
  it rather than replacing it, so a 340 MB Development binary from an earlier
  build was being carried into the installer.

## [0.9.1] — 2026-07-25

A maintenance release, driven by what actually broke for people who installed
0.9.0. Thanks to [@die-Anna](https://github.com/die-Anna) for finding and fixing
the startup failure.

### Added

- **INVERT ORBIT Y** settings row: optionally flip the vertical orbit
  direction of a right-mouse drag. Persisted to `Game.ini` under
  `[IonCommand.Input]`, applied live.
- Collector `-listen` flag to override `server.listenAddress` from the command
  line, so launchers can move to a fallback port without editing the config.

### Changed

- Vertical orbit now follows the same convention as horizontal orbit by
  default; the previous direction is available via **INVERT ORBIT Y**.

### Fixed

- **Installed builds could start with a dead collector.** Windows reserves TCP
  port ranges for Hyper-V/WSL NAT; when 7810 fell inside one, the collector
  exited instantly (`bind: WSAEACCES`) and the launcher started the client
  anyway, with the only warning hidden in an invisible console. The launchers
  now probe the port first, fall back to 17810/27810, detect an early collector
  exit, stop a candidate that never becomes healthy before trying the next port,
  point the client at the working port, and raise a visible message when the
  collector really cannot start.
- Replay honours the `-IonCollectorUrl=` override like the live stream does,
  so **R**/**L** keep working when the collector runs on a fallback port.
- The collector reported its version as `0.1.0-bootstrap` in the startup log
  and on `/api/status`; it now reports the release version.

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

[Unreleased]: https://github.com/svabi79/ion-command/compare/v0.9.2...HEAD
[0.9.2]: https://github.com/svabi79/ion-command/releases/tag/v0.9.2
[0.9.1]: https://github.com/svabi79/ion-command/releases/tag/v0.9.1
[0.9.0]: https://github.com/svabi79/ion-command/releases/tag/v0.9.0
