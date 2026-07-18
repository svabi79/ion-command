from __future__ import annotations

import json
import py_compile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def main() -> None:
    json_files = list((ROOT / "schemas").rglob("*.json")) + list((ROOT / "plugins").rglob("*.json"))
    json_files += [ROOT / "collector" / "configs" / "development.json", ROOT / "unreal" / "IonCommand.uproject"]
    for path in json_files:
        with path.open("r", encoding="utf-8") as handle:
            json.load(handle)

    plugin_ids: set[str] = set()
    for path in (ROOT / "plugins").rglob("*.json"):
        payload = json.loads(path.read_text(encoding="utf-8"))
        identifier = payload.get("pluginId") or payload.get("layerId")
        if not identifier:
            raise RuntimeError(f"Plugin/layer manifest has no identifier: {path}")
        if identifier in plugin_ids:
            raise RuntimeError(f"Duplicate plugin/layer identifier: {identifier}")
        plugin_ids.add(identifier)

    for path in (ROOT / "unreal" / "Scripts").glob("*.py"):
        py_compile.compile(str(path), doraise=True)

    for path in (ROOT / "sample-data").glob("*.jsonl"):
        for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
            try:
                json.loads(line)
            except json.JSONDecodeError as error:
                raise RuntimeError(f"Invalid JSONL {path}:{line_number}: {error}") from error

    project = json.loads((ROOT / "unreal" / "IonCommand.uproject").read_text(encoding="utf-8"))
    for module in project["Modules"]:
        module_dir = ROOT / "unreal" / "Source" / module["Name"]
        if not (module_dir / f"{module['Name']}.Build.cs").exists():
            raise RuntimeError(f"Missing Build.cs for {module['Name']}")

    for header in (ROOT / "unreal" / "Source").rglob("*.h"):
        source = header.read_text(encoding="utf-8")
        if ("UCLASS(" in source or "USTRUCT(" in source or "UENUM(" in source) and ".generated.h\"" not in source:
            raise RuntimeError(f"Unreal reflected header is missing generated include: {header}")
    generic_arc = (ROOT / "unreal" / "Source" / "IonCommandVisualization" / "Private" / "GeoArcLayerActor.cpp").read_text(encoding="utf-8").lower()
    forbidden_arc_terms = ["callsign", "pskreporter", "snrdb", 'text("20m")']
    present = [term for term in forbidden_arc_terms if term in generic_arc]
    if present:
        raise RuntimeError(f"Domain terms leaked into generic arc renderer: {present}")

    required = [
        ROOT / "docs" / "ARCHITECTURE.md",
        ROOT / "docs" / "DATA_CONTRACT.md",
        ROOT / "docs" / "ENVIRONMENT.md",
        ROOT / "docs" / "IMPLEMENTATION_STATUS.md",
    ]
    missing = [str(path) for path in required if not path.exists()]
    if missing:
        raise RuntimeError(f"Missing required documentation: {missing}")
    print(f"Repository validation passed ({len(json_files)} JSON files, {len(plugin_ids)} plugin/layer IDs, JSONL, Unreal Python syntax, reflection headers, module boundaries).")


if __name__ == "__main__":
    main()
