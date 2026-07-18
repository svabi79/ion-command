import sys
from pathlib import Path

import unreal

sys.path.insert(0, str(Path(__file__).resolve().parent))

from _ion_common import load_class


REQUIRED_CLASSES = [
    ("IonCommand", "IonCommandGameMode"),
    ("IonCommandCore", "GeoMathLibrary"),
    ("IonCommandVisualization", "IonGlobeActor"),
    ("IonCommandVisualization", "GeoArcLayerActor"),
    ("IonCommandHamRadio", "HamRadioLinkLayerActor"),
    ("IonCommandUI", "IonCommandDeckActor"),
]
REQUIRED_ASSETS = [
    "/Game/ION/Maps/L_CommandDeck",
    "/Game/ION/Materials/MI_Atmosphere",
    "/Game/ION/Materials/MI_Ionosphere",
    "/Game/ION/Materials/M_EarthSurface",
    "/Game/ION/Materials/M_Starfield",
    "/Game/ION/Materials/M_HolographicSignal",
    "/Game/ION/Materials/MI_Console",
    "/Game/ION/Textures/T_EarthDay",
    "/Game/ION/Textures/T_EarthNight",
    "/Game/ION/Textures/T_Starfield",
    "/Game/ION/Data/DA_BandVisualConfig",
]


def main() -> None:
    failures = []
    for module, class_name in REQUIRED_CLASSES:
        try:
            load_class(module, class_name)
        except RuntimeError as error:
            failures.append(str(error))
    for path in REQUIRED_ASSETS:
        if not unreal.EditorAssetLibrary.does_asset_exist(path):
            failures.append(f"Missing asset: {path}")
    if failures:
        for failure in failures:
            unreal.log_error(f"ION COMMAND validation: {failure}")
        raise RuntimeError(f"ION COMMAND project validation failed with {len(failures)} issue(s)")
    unreal.log("ION COMMAND: project validation passed")


if __name__ == "__main__":
    main()
