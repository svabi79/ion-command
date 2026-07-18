import sys
from pathlib import Path

import unreal

sys.path.insert(0, str(Path(__file__).resolve().parent))

from _ion_common import ensure_directory, load_class


LEVEL_PATH = "/Game/ION/Maps/L_CommandDeck"
SHOWCASE_LOCATION = unreal.Vector(-4200.0, 0.0, 800.0)
SHOWCASE_ROTATION = unreal.Rotator(roll=0.0, pitch=-10.8, yaw=0.0)


SHOWCASE_CLASSES = {
    "IonGlobeActor",
    "HamRadioLinkLayerActor",
    "GeoPointLayerActor",
    "IonIonosphereActor",
    "IonAuroraActor",
    "IonCommandDeckActor",
    "PlayerStart",
}


def reset_showcase_actors() -> None:
    for actor in unreal.EditorLevelLibrary.get_all_level_actors():
        if actor.get_class().get_name() in SHOWCASE_CLASSES:
            unreal.EditorLevelLibrary.destroy_actor(actor)


def spawn(module: str, class_name: str, location=(0.0, 0.0, 0.0)):
    actor_class = load_class(module, class_name)
    actor = unreal.EditorLevelLibrary.spawn_actor_from_class(actor_class, unreal.Vector(*location))
    actor.set_actor_label(class_name.removesuffix("Actor"))
    return actor


def main() -> None:
    ensure_directory("/Game/ION")
    ensure_directory("/Game/ION/Maps")
    if unreal.EditorAssetLibrary.does_asset_exist(LEVEL_PATH):
        unreal.EditorLevelLibrary.load_level(LEVEL_PATH)
    elif not unreal.EditorLevelLibrary.new_level(LEVEL_PATH):
        raise RuntimeError(f"Could not create level {LEVEL_PATH}")

    reset_showcase_actors()
    spawn("IonCommandVisualization", "IonGlobeActor")
    spawn("IonCommandHamRadio", "HamRadioLinkLayerActor")
    spawn("IonCommandVisualization", "GeoPointLayerActor")
    spawn("IonCommandVisualization", "IonIonosphereActor")
    spawn("IonCommandVisualization", "IonAuroraActor")
    spawn("IonCommandUI", "IonCommandDeckActor", (-1300.0, 0.0, 0.0))
    player_start = spawn("Engine", "PlayerStart")
    player_start.set_actor_label("IonCommandPlayerStart")

    world = unreal.EditorLevelLibrary.get_editor_world()
    settings = world.get_world_settings()
    settings.set_editor_property("default_game_mode", load_class("IonCommand", "IonCommandGameMode"))
    unreal.EditorLevelLibrary.save_current_level()
    editor = unreal.get_editor_subsystem(unreal.UnrealEditorSubsystem)
    editor.set_level_viewport_camera_info(SHOWCASE_LOCATION, SHOWCASE_ROTATION)
    unreal.log(f"ION COMMAND: bootstrap level ready at {LEVEL_PATH}")


if __name__ == "__main__":
    main()
