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

## Something is drawn in the wrong place on the globe

Two different faults look identical from the operator's chair - the marker sits
over the wrong country - and they are told apart by asking whether the marker
moved or the map underneath it did.

**A single marker is wrong, the coastlines are right.** The setting that places
it was lost. Check the startup log:

```
ION COMMAND own station: callsign=HB9HSJ locator=JN47om -> lat=47.5208 lon=9.2083
```

A line reading `not configured` means the client fell back to the placeholder
and hid the marker; a wrong locator there means the config, not the renderer,
is at fault. See CONFIGURATION.md for `IonOperator.ini`.

**Everything is wrong together - the marker, the coastlines, the terminator.**
The Earth texture is misaligned against the geometry. Both markers and the
camera derive their positions from the same sphere convention, so a marker
always lands where the camera says it should; only the imagery can slide out
from under it. Verify with a landmark instead of by eye:

```
IonCommand.exe -IonCameraLatitude=35 -IonCameraLongitude=-120 -IonCameraDistance=1900 ^
  -IonCollectorUrl=http://127.0.0.1:1/ -IonScreenshotAfter=40 -IonScreenshotFile=C:\Temp\cal.png ^
  -IonExitAfterScreenshot
```

The centre of that frame must be California. If it shows Panama, the day
texture is displaced by exactly the ratio between its source width and the
next power of two - the signature of a `PAD_TO_POWER_OF_TWO` import, which
leaves the map in the corner of a larger canvas so UV 1.0 falls in the padding
rather than at longitude +180. Day and night textures pad by different ratios,
so the two sides of the terminator slide by different amounts. The import must
use `STRETCH_TO_POWER_OF_TWO`; `create_material_instances.py` asserts this.

Note that this class of fault survives a casual look at a screenshot: a
displaced Earth still renders as a completely convincing Earth. Only a
named landmark at a known camera position settles it.

## The globe renders as a grey or unrecognisable mass

The Earth material failed to compile and Unreal substituted its default
material. Packaging catches this now (`tools/package.ps1` fails the build),
but if you are running an editor build or an older package, look for:

```
LogMaterial: Warning: Invalid shader map ID caching shaders for 'M_EarthSurface'
LogMaterial: ... Failed to compile Material for platform PCD3D_SM6
```

The reason is on the following line of the *cook* log, not the client log.
Three failures have actually happened here, all silent until cook time:

| Cook message | Cause |
| --- | --- |
| `Sampler type` mismatch | A virtual texture bound to a plain `TextureSampleParameter2D`, or an sRGB texture on a `LinearColor` sampler. The Earth day/night textures are virtual; a parameter's fallback must not be. |
| `Not enough components in (float3) for component mask 0011` | A `ComponentMask` reaching for `.ba` on a `VectorParameter`, whose default output is float3. Use the parameter's own `R`/`G`/`B`/`A` outputs and `AppendVector`. |
| `(Node TextureSample) Missing input texture` | A `TextureObjectParameter` connected to the wrong input name. The texture object input on `TextureSample` is called `Tex`. |

Note that a material failure is only a **warning** to the cook, which then
exits 0. Exit code alone never showed this; the guard reads the captured
output instead.

## Aircraft point the wrong way

Glyph orientation comes from `visual.headingDeg`, turned into a world vector
by the point layer and handed to the material as instance custom data 7..9.
To check it against an answer that is not in doubt, run the probe:

```bash
collector/bin/headingprobe.exe -addr 127.0.0.1:7899
```

then point a client at it:

```bash
IonCommand.exe -IonCollectorUrl=ws://127.0.0.1:7899/ws/live -IonCameraLatitude=47.5 -IonCameraLongitude=9.2 -IonCameraDistance=1120
```

It serves four aircraft on courses 0/90/180/270, arranged north/east/south/west
around the centre. Every nose must point outward, away from the middle. If they
all point inward the orientation is inverted; if they are all rotated the same
way it is an offset; if the pattern is mirrored it is a sign error on one axis.

This is worth doing rather than judging by eye on live traffic: real aircraft
over a region often share a course, so a systematically wrong rotation can look
plausible for a long time.

## Tile seams: a visible step across a straight horizontal line

Not a compositing bug. The close-orbit detail layer is HLS - individual
Landsat and Sentinel-2 overpasses, not a blended mosaic - so neighbouring
tiles were often photographed days apart under different sun angles, and the
brightness genuinely steps at the tile boundary. Measured over one cached
window: 59 of 97 vertical tile joins showed a mean edge step above 12 (of
255), the worst 146.

The base layer beneath it (Blue Marble) is seamless, so the effect only
appears where HLS has coverage. Nothing in the mosaic code can remove it; it
would take either a blended source (Sentinel-2 cloudless is one, but its
licence is non-commercial) or per-tile histogram matching, which would alter
the imagery rather than display it.

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
