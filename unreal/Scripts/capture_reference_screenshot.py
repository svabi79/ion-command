import os
import unreal


LEVEL_PATH = "/Game/ION/Maps/L_CommandDeck"
SHOWCASE_LOCATION = unreal.Vector(-4200.0, 0.0, 800.0)
SHOWCASE_ROTATION = unreal.Rotator(roll=0.0, pitch=-10.8, yaw=0.0)


def main() -> None:
    project_dir = unreal.Paths.convert_relative_path_to_full(unreal.Paths.project_dir())
    output_dir = os.path.join(project_dir, "Saved", "Screenshots", "Reference")
    os.makedirs(output_dir, exist_ok=True)
    output_path = os.path.join(output_dir, "ION_COMMAND_First_Light.png")
    if unreal.EditorAssetLibrary.does_asset_exist(LEVEL_PATH):
        unreal.EditorLevelLibrary.load_level(LEVEL_PATH)
    world = unreal.EditorLevelLibrary.get_editor_world()
    if world is None:
        raise RuntimeError("No editor world is loaded")
    # The capture must not depend on per-user viewport state: pin the level
    # viewport to the saved showcase composition before requesting the shot.
    editor = unreal.get_editor_subsystem(unreal.UnrealEditorSubsystem)
    editor.set_level_viewport_camera_info(SHOWCASE_LOCATION, SHOWCASE_ROTATION)
    # High-res shots above viewport size drop the separate-translucency pass,
    # which erases every additive hologram layer. Fold translucency back into
    # the main pass for the capture so arcs, shells, and aurora survive.
    unreal.SystemLibrary.execute_console_command(world, "r.SeparateTranslucency 0")
    unreal.SystemLibrary.execute_console_command(world, "r.Translucency.DynamicRes.Enabled 0")
    unreal.AutomationLibrary.take_high_res_screenshot(5120, 1440, output_path)
    unreal.log(f"ION COMMAND: screenshot requested at {output_path}")


if __name__ == "__main__":
    main()
