# Troubleshooting

## The globe is empty

The client draws whatever the collector sends. Check the collector first:

```
http://127.0.0.1:7810/api/health      -> {"status":"ok"}
http://127.0.0.1:7810/api/status      -> per-source state
```

- **Health does not answer** — the collector is not running. Start ION COMMAND
  through the Start Menu shortcut (it starts the collector), or run
  `collector\ion-collector.exe -config collector\configs\live.json`.
  Installed builds log to `%LOCALAPPDATA%\IonCommand\logs\collector-err.log`.
- **The launcher may have used a fallback port.** If 7810 was unavailable it
  tries 17810, then 27810 — check those before concluding the collector is
  down. The client is pointed at the right port automatically.

## The collector exits immediately: `bind: An attempt was made to access a socket…`

Windows reserves whole TCP port ranges for Hyper-V/WSL NAT, and on some
machines the canonical port 7810 falls inside such a range — binding it then
fails with `WSAEACCES` even though nothing is listening. Check with:

```
netsh interface ipv4 show excludedportrange protocol=tcp
```

The launcher detects this and falls back to 17810, then 27810, passing the
actual port to the client. If you start the pieces by hand, do the same:

```
collector\ion-collector.exe -config collector\configs\live.json -listen 127.0.0.1:17810
client\IonCommand.exe -IonCollectorUrl=ws://127.0.0.1:17810/ws/live
```
- **Health is fine but nothing renders** — check the HUD status bar: `LINK`
  should read `CONNECTED`. If it says `OFFLINE`, the client is pointed at a
  different address; the default is `ws://127.0.0.1:7810/ws/live`.
- **Only some marker types are missing** — press **O** and check the domain
  rows; a hidden domain keeps receiving data but stops drawing.

## Aircraft appear only around one spot

That is the shape of the data, not a bug: `aviation.adsb` queries a **circle**
around a coordinate (250 nm max). The default configuration ships one example
circle plus the global OpenSky snapshot.

- Add more `aviation.adsb` entries with different centres, or
- rely on `aviation.opensky` for worldwide coverage.

See [CONFIGURATION.md](CONFIGURATION.md#source-entries).

## Aircraft take minutes to appear after starting

The global OpenSky snapshot is polled every 15 minutes by default (anonymous
access is credit-limited). The collector retains the last snapshot and replays
it to every client that connects, so this only affects the very first minutes
after the **collector** starts — not client restarts. Configure an OpenSky
account for much shorter intervals.

## Too many aircraft / the map is unreadable

Open **O → SETTINGS** and set **MIN FLIGHT LEVEL** to FL100 (removes airport
and low-level clutter) and **SHOW GROUND A/C** to OFF. **V** hides the
propagation paths entirely. On a busy afternoon 12 000–14 000 aircraft
worldwide is normal.

## Sources stop delivering after a while

Community aggregators rate-limit. The collector backs off automatically
(HTTP 429 and adsb.lol's 420 "Enhance Your Calm") and recovers on its own. If
one region stays dark:

- increase `pollSeconds` for the aviation sources,
- reduce the number of `aviation.adsb` circles,
- check `collector-out.log` for `rate limited` warnings.

Please do not lower the intervals below the defaults — these services are run
by volunteers.

## The clouds look like an old snapshot

The client fetches the current EUMETSAT world infrared composite at startup and
then hourly. If the fetch fails it keeps the offline climatology texture and
logs a warning. `-IonNoLiveClouds` forces the offline texture.

## My callsign / grid does not stick

Set them in **O → SETTINGS** (click the row, type, **Enter**). They are written
to `Saved\Config\Windows\Game.ini` next to the client.

If you are running from a self-built package: **packaging deletes that file**,
so re-apply after every package (the shipped launcher writes it for you).

## Typing in the settings panel toggles layers

That should not happen — hotkeys are suspended while a text field is focused.
If you see it, the field lost focus; click the row again so it shows the
green edit cursor.

## Screenshots come out at the wrong path

The engine splits the command line on spaces. Use an absolute path **without
spaces** for `-IonScreenshotFile`, e.g. `C:\Temp\shot.png`.

## Packaging produced a stale build

A running client locks the packaged files; the archive step can skip them while
still reporting success. Close the client, repackage, and verify the timestamp
of `dist\...\IonCommand.exe`.

## Performance

The renderer holds ~125 fps with 12 000 arcs on a desktop GPU. If it drags:

- **V** to hide the paths (they dominate the fill cost),
- lower **MARKER LIFETIME** so fewer markers accumulate,
- use **MIN FLIGHT LEVEL** to cut the aircraft count,
- reduce the window resolution.

## Reporting a problem

Open an issue with:

- what you did and what you expected,
- the HUD status bar (`LINK`, `ACTIVE`, `RX`, `DROP`),
- `%LOCALAPPDATA%\IonCommand\logs\collector-err.log`,
- the client log from `<install>\client\IonCommand\Saved\Logs\`.
