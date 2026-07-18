import colorsys
import sys
from pathlib import Path

import unreal

sys.path.insert(0, str(Path(__file__).resolve().parent))

from _ion_common import ensure_directory, save_asset


MATERIAL_DIR = "/Game/ION/Materials"
TEXTURE_DIR = "/Game/ION/Textures"
SOURCE_DIR = Path(__file__).resolve().parents[1] / "SourceAssets"


def create_material(name: str) -> tuple[unreal.Material, bool]:
    path = f"{MATERIAL_DIR}/{name}"
    existing = unreal.load_asset(path)
    if existing:
        return existing, False
    tools = unreal.AssetToolsHelpers.get_asset_tools()
    material = tools.create_asset(name, MATERIAL_DIR, unreal.Material, unreal.MaterialFactoryNew())
    if material is None:
        raise RuntimeError(f"Could not create {path}")
    return material, True


def import_texture(source: Path, asset_name: str) -> unreal.Texture2D:
    if not source.exists():
        raise RuntimeError(f"Missing visual source texture: {source}")
    asset_path = f"{TEXTURE_DIR}/{asset_name}"
    existing = unreal.load_asset(asset_path)
    if existing:
        existing.set_editor_property("never_stream", True)
        save_asset(asset_path)
        return existing

    task = unreal.AssetImportTask()
    task.set_editor_property("filename", str(source))
    task.set_editor_property("destination_path", TEXTURE_DIR)
    task.set_editor_property("destination_name", asset_name)
    task.set_editor_property("automated", True)
    task.set_editor_property("replace_existing", True)
    task.set_editor_property("save", True)
    unreal.AssetToolsHelpers.get_asset_tools().import_asset_tasks([task])
    imported = unreal.load_asset(asset_path)
    if imported is None:
        raise RuntimeError(f"Could not import {source} to {asset_path}")
    imported.set_editor_property("never_stream", True)
    save_asset(asset_path)
    return imported


def create_translucent_master() -> unreal.Material:
    path = f"{MATERIAL_DIR}/M_HolographicShell"
    material, created = create_material("M_HolographicShell")
    if not created:
        return material

    material.set_editor_property("blend_mode", unreal.BlendMode.BLEND_TRANSLUCENT)
    material.set_editor_property("two_sided", True)
    color = unreal.MaterialEditingLibrary.create_material_expression(material, unreal.MaterialExpressionVectorParameter, -600, -100)
    color.set_editor_property("parameter_name", "Color")
    color.set_editor_property("default_value", unreal.LinearColor(0.0, 0.55, 1.0, 1.0))
    fresnel = unreal.MaterialEditingLibrary.create_material_expression(material, unreal.MaterialExpressionFresnel, -600, 150)
    opacity = unreal.MaterialEditingLibrary.create_material_expression(material, unreal.MaterialExpressionScalarParameter, -600, 330)
    opacity.set_editor_property("parameter_name", "Opacity")
    opacity.set_editor_property("default_value", 0.12)
    multiply = unreal.MaterialEditingLibrary.create_material_expression(material, unreal.MaterialExpressionMultiply, -300, 100)
    unreal.MaterialEditingLibrary.connect_material_expressions(color, "", multiply, "A")
    unreal.MaterialEditingLibrary.connect_material_expressions(fresnel, "", multiply, "B")
    unreal.MaterialEditingLibrary.connect_material_property(multiply, "", unreal.MaterialProperty.MP_EMISSIVE_COLOR)
    unreal.MaterialEditingLibrary.connect_material_property(opacity, "", unreal.MaterialProperty.MP_OPACITY)
    unreal.MaterialEditingLibrary.recompile_material(material)
    save_asset(path)
    return material


def create_signal_master() -> unreal.Material:
    path = f"{MATERIAL_DIR}/M_HolographicSignal"
    material, created = create_material("M_HolographicSignal")
    if not created:
        material.set_editor_property("used_with_instanced_static_meshes", True)
        unreal.MaterialEditingLibrary.recompile_material(material)
        save_asset(path)
        return material

    material.set_editor_property("blend_mode", unreal.BlendMode.BLEND_ADDITIVE)
    material.set_editor_property("shading_model", unreal.MaterialShadingModel.MSM_UNLIT)
    material.set_editor_property("two_sided", True)
    material.set_editor_property("used_with_instanced_static_meshes", True)
    color = unreal.MaterialEditingLibrary.create_material_expression(material, unreal.MaterialExpressionVectorParameter, -650, -120)
    color.set_editor_property("parameter_name", "Color")
    color.set_editor_property("default_value", unreal.LinearColor(0.0, 0.8, 1.0, 1.0))
    intensity = unreal.MaterialEditingLibrary.create_material_expression(material, unreal.MaterialExpressionScalarParameter, -650, 60)
    intensity.set_editor_property("parameter_name", "Intensity")
    intensity.set_editor_property("default_value", 3.0)
    opacity = unreal.MaterialEditingLibrary.create_material_expression(material, unreal.MaterialExpressionScalarParameter, -650, 240)
    opacity.set_editor_property("parameter_name", "Opacity")
    opacity.set_editor_property("default_value", 0.85)
    multiply = unreal.MaterialEditingLibrary.create_material_expression(material, unreal.MaterialExpressionMultiply, -330, -70)
    unreal.MaterialEditingLibrary.connect_material_expressions(color, "", multiply, "A")
    unreal.MaterialEditingLibrary.connect_material_expressions(intensity, "", multiply, "B")
    unreal.MaterialEditingLibrary.connect_material_property(multiply, "", unreal.MaterialProperty.MP_EMISSIVE_COLOR)
    unreal.MaterialEditingLibrary.connect_material_property(opacity, "", unreal.MaterialProperty.MP_OPACITY)
    unreal.MaterialEditingLibrary.recompile_material(material)
    save_asset(path)
    return material


def create_earth_material(day_texture: unreal.Texture2D, night_texture: unreal.Texture2D) -> unreal.Material:
    path = f"{MATERIAL_DIR}/M_EarthSurface"
    material, created = create_material("M_EarthSurface")
    if not created:
        return material

    material.set_editor_property("two_sided", False)
    day = unreal.MaterialEditingLibrary.create_material_expression(material, unreal.MaterialExpressionTextureSample, -720, -150)
    day.set_editor_property("texture", day_texture)
    night = unreal.MaterialEditingLibrary.create_material_expression(material, unreal.MaterialExpressionTextureSample, -720, 140)
    night.set_editor_property("texture", night_texture)
    night_strength = unreal.MaterialEditingLibrary.create_material_expression(material, unreal.MaterialExpressionScalarParameter, -480, 300)
    night_strength.set_editor_property("parameter_name", "NightIntensity")
    night_strength.set_editor_property("default_value", 1.8)
    night_emissive = unreal.MaterialEditingLibrary.create_material_expression(material, unreal.MaterialExpressionMultiply, -250, 120)
    roughness = unreal.MaterialEditingLibrary.create_material_expression(material, unreal.MaterialExpressionScalarParameter, -250, 350)
    roughness.set_editor_property("parameter_name", "Roughness")
    roughness.set_editor_property("default_value", 0.62)
    unreal.MaterialEditingLibrary.connect_material_expressions(night, "RGB", night_emissive, "A")
    unreal.MaterialEditingLibrary.connect_material_expressions(night_strength, "", night_emissive, "B")
    unreal.MaterialEditingLibrary.connect_material_property(day, "RGB", unreal.MaterialProperty.MP_BASE_COLOR)
    unreal.MaterialEditingLibrary.connect_material_property(night_emissive, "", unreal.MaterialProperty.MP_EMISSIVE_COLOR)
    unreal.MaterialEditingLibrary.connect_material_property(roughness, "", unreal.MaterialProperty.MP_ROUGHNESS)
    unreal.MaterialEditingLibrary.recompile_material(material)
    save_asset(path)
    return material


def create_starfield_material(texture: unreal.Texture2D) -> unreal.Material:
    path = f"{MATERIAL_DIR}/M_Starfield"
    material, created = create_material("M_Starfield")
    if not created:
        return material

    material.set_editor_property("shading_model", unreal.MaterialShadingModel.MSM_UNLIT)
    material.set_editor_property("two_sided", True)
    sample = unreal.MaterialEditingLibrary.create_material_expression(material, unreal.MaterialExpressionTextureSample, -600, -80)
    sample.set_editor_property("texture", texture)
    strength = unreal.MaterialEditingLibrary.create_material_expression(material, unreal.MaterialExpressionScalarParameter, -600, 150)
    strength.set_editor_property("parameter_name", "Intensity")
    strength.set_editor_property("default_value", 1.4)
    emissive = unreal.MaterialEditingLibrary.create_material_expression(material, unreal.MaterialExpressionMultiply, -280, -20)
    unreal.MaterialEditingLibrary.connect_material_expressions(sample, "RGB", emissive, "A")
    unreal.MaterialEditingLibrary.connect_material_expressions(strength, "", emissive, "B")
    unreal.MaterialEditingLibrary.connect_material_property(emissive, "", unreal.MaterialProperty.MP_EMISSIVE_COLOR)
    unreal.MaterialEditingLibrary.recompile_material(material)
    save_asset(path)
    return material


def create_instance(
    name: str,
    parent: unreal.Material,
    color: unreal.LinearColor,
    opacity: float,
    intensity: float | None = None,
) -> None:
    path = f"{MATERIAL_DIR}/{name}"
    instance = unreal.load_asset(path)
    if instance is None:
        tools = unreal.AssetToolsHelpers.get_asset_tools()
        instance = tools.create_asset(name, MATERIAL_DIR, unreal.MaterialInstanceConstant, unreal.MaterialInstanceConstantFactoryNew())
        if instance is None:
            raise RuntimeError(f"Could not create {path}")
    unreal.MaterialEditingLibrary.set_material_instance_parent(instance, parent)
    unreal.MaterialEditingLibrary.set_material_instance_vector_parameter_value(instance, "Color", color)
    unreal.MaterialEditingLibrary.set_material_instance_scalar_parameter_value(instance, "Opacity", opacity)
    if intensity is not None:
        unreal.MaterialEditingLibrary.set_material_instance_scalar_parameter_value(instance, "Intensity", intensity)
    save_asset(path)


def main() -> None:
    ensure_directory(MATERIAL_DIR)
    ensure_directory(TEXTURE_DIR)

    day = import_texture(SOURCE_DIR / "NASA" / "bluemarble-2048.png", "T_EarthDay")
    night = import_texture(SOURCE_DIR / "NASA" / "earthatnight-2048.png", "T_EarthNight")
    stars = import_texture(SOURCE_DIR / "Generated" / "starfield.png", "T_Starfield")
    create_earth_material(day, night)
    create_starfield_material(stars)

    shell = create_translucent_master()
    create_instance("MI_Atmosphere", shell, unreal.LinearColor(0.0, 0.35, 1.0, 1.0), 0.08)
    create_instance("MI_Ionosphere", shell, unreal.LinearColor(0.0, 0.8, 1.0, 1.0), 0.035)

    signal = create_signal_master()
    for index in range(11):
        red, green, blue = colorsys.hsv_to_rgb(index / 11.0, 0.86, 1.0)
        create_instance(f"MI_Signal_{index:02d}", signal, unreal.LinearColor(red, green, blue, 1.0), 0.88, 3.4)
    create_instance("MI_Signal_Selected", signal, unreal.LinearColor(0.78, 1.0, 1.0, 1.0), 1.0, 6.0)
    create_instance("MI_Point_Entity", signal, unreal.LinearColor(0.0, 0.82, 1.0, 1.0), 0.95, 4.2)
    create_instance("MI_Point_Observation", signal, unreal.LinearColor(1.0, 0.22, 0.03, 1.0), 1.0, 5.0)
    create_instance("MI_Aurora_North", signal, unreal.LinearColor(0.02, 1.0, 0.38, 1.0), 0.36, 2.2)
    create_instance("MI_Aurora_South", signal, unreal.LinearColor(0.08, 0.55, 1.0, 1.0), 0.32, 2.0)
    create_instance("MI_Console", signal, unreal.LinearColor(0.0, 0.42, 1.0, 1.0), 0.18, 1.5)
    unreal.log("ION COMMAND: visual material bootstrap complete")


if __name__ == "__main__":
    main()
