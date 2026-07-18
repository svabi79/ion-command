import sys
from pathlib import Path

import unreal

sys.path.insert(0, str(Path(__file__).resolve().parent))

from _ion_common import ensure_directory, load_class, save_asset


DATA_DIR = "/Game/ION/Data"
ASSET_PATH = f"{DATA_DIR}/DA_BandVisualConfig"


def main() -> None:
    ensure_directory(DATA_DIR)
    if unreal.EditorAssetLibrary.does_asset_exist(ASSET_PATH):
        unreal.log(f"ION COMMAND: preserving existing {ASSET_PATH}")
        return
    asset_class = load_class("IonCommandHamRadio", "HamBandVisualConfig")
    factory = unreal.DataAssetFactory()
    factory.set_editor_property("data_asset_class", asset_class)
    asset = unreal.AssetToolsHelpers.get_asset_tools().create_asset("DA_BandVisualConfig", DATA_DIR, asset_class, factory)
    if asset is None:
        raise RuntimeError("Could not create DA_BandVisualConfig")
    save_asset(ASSET_PATH)
    unreal.log("ION COMMAND: band visual data asset created from C++ defaults")


if __name__ == "__main__":
    main()
