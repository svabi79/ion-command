# ION COMMAND

**Global Geospatial Operations & HF Propagation Command Center**

ION COMMAND is a native Unreal Engine 5 fat client for live, historical, and
contextual geospatial data. Its first application is global amateur-radio
propagation, while the platform core remains independent of callsigns, bands,
and any individual feed.

The current bootstrap includes:

- a bounded, concurrent Go collector with source/domain plugin registries;
- canonical Point and GreatCircle messages over WebSocket;
- radio, lightning, and space-weather mock sources;
- health, status, statistics, source, recording, and replay endpoints;
- JSONL recording and time-scaled replay;
- an Unreal C++ project split into generic core/data/rendering modules and an
  application shell;
- a NASA Blue Marble day/night globe, generated starfield, atmosphere/aurora,
  bounded instanced paths, ultrawide camera rig, and world-space command deck;
- repeatable Unreal Editor automation and repository validation scripts.

## Quick start: collector

```powershell
cd collector
go test ./...
go run ./cmd/ion-collector -config ./configs/development.json
```

Then open:

- `http://127.0.0.1:7810/api/health`
- `http://127.0.0.1:7810/api/status`
- `http://127.0.0.1:7810/api/stats`

The canonical live stream is `ws://127.0.0.1:7810/ws/live`.

## Quick start: Unreal

The verified workstation setup is Unreal Engine 5.8 with Visual Studio 2022
17.14, the v143 compiler, and Windows SDK 26100:

```powershell
.\tools\build.ps1 -Unreal
.\tools\run-editor.ps1
```

Run `unreal/Scripts/create_bootstrap_level.py` from the editor or through the
provided command to generate the `L_CommandDeck` asset.

Start the collector before Play in Editor. Right mouse orbits, the wheel zooms,
left mouse selects the nearest visible path, Escape releases it, Space pauses
the shared timeline, and L returns to live. F re-centers the camera on the
selection and I toggles the conceptual ionosphere shells. A selected path is
pinned and shown as a bright dedicated arc with white endpoint markers while
all other traffic dims, and the camera eases toward the selected link;
observed-link details appear on the center console.

See [development instructions](docs/DEVELOPMENT.md), the
[architecture](docs/ARCHITECTURE.md), and the
[implementation status](docs/IMPLEMENTATION_STATUS.md).
