import sys
from pathlib import Path

import unreal

sys.path.insert(0, str(Path(__file__).resolve().parent))

from _ion_common import ensure_directory, load_class, save_asset


DATA_DIR = "/Game/ION/Data"
AUDIO_DIR = "/Game/ION/Audio"
ASSET_PATH = f"{DATA_DIR}/DA_BandVisualConfig"
SOURCE_DIR = Path(__file__).resolve().parents[1] / "SourceAssets"


def import_ambience() -> None:
    """Import the generated command-deck ambience as a looping sound wave."""
    source = SOURCE_DIR / "Generated" / "deck_ambience.wav"
    if not source.exists():
        raise RuntimeError(f"Missing generated ambience: {source} (run generate_visual_sources.py)")
    ensure_directory(AUDIO_DIR)
    asset_path = f"{AUDIO_DIR}/S_DeckAmbience"
    if unreal.EditorAssetLibrary.does_asset_exist(asset_path):
        unreal.EditorAssetLibrary.delete_asset(asset_path)
    task = unreal.AssetImportTask()
    task.set_editor_property("filename", str(source))
    task.set_editor_property("destination_path", AUDIO_DIR)
    task.set_editor_property("destination_name", "S_DeckAmbience")
    task.set_editor_property("automated", True)
    task.set_editor_property("replace_existing", True)
    task.set_editor_property("save", True)
    unreal.AssetToolsHelpers.get_asset_tools().import_asset_tasks([task])
    imported = unreal.load_asset(asset_path)
    if imported is None:
        raise RuntimeError(f"Could not import {source}")
    imported.set_editor_property("looping", True)
    save_asset(asset_path)
    unreal.log("ION COMMAND: deck ambience imported")


def main() -> None:
    import_ambience()
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
