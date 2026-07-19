import colorsys
import sys
from pathlib import Path

import unreal

sys.path.insert(0, str(Path(__file__).resolve().parent))

from _ion_common import ensure_directory, save_asset


MATERIAL_DIR = "/Game/ION/Materials"
TEXTURE_DIR = "/Game/ION/Textures"
SOURCE_DIR = Path(__file__).resolve().parents[1] / "SourceAssets"
MEL = unreal.MaterialEditingLibrary


def ensure_material(name: str) -> unreal.Material:
    """Delete and recreate the named master so every run rebuilds the graph
    deterministically. Editing expressions of a live material asserts in 5.8,
    so main() first switches to an empty transient level and collects garbage.
    """
    path = f"{MATERIAL_DIR}/{name}"
    if unreal.EditorAssetLibrary.does_asset_exist(path):
        if not unreal.EditorAssetLibrary.delete_asset(path):
            raise RuntimeError(f"Could not delete {path} for rebuild")
    tools = unreal.AssetToolsHelpers.get_asset_tools()
    material = tools.create_asset(name, MATERIAL_DIR, unreal.Material, unreal.MaterialFactoryNew())
    if material is None:
        raise RuntimeError(f"Could not create {path}")
    return material


def expression(material, kind, x, y):
    return MEL.create_material_expression(material, kind, x, y)


def scalar(material, name, default, x, y):
    node = expression(material, unreal.MaterialExpressionScalarParameter, x, y)
    node.set_editor_property("parameter_name", name)
    node.set_editor_property("default_value", default)
    return node


def vector(material, name, default, x, y):
    node = expression(material, unreal.MaterialExpressionVectorParameter, x, y)
    node.set_editor_property("parameter_name", name)
    node.set_editor_property("default_value", default)
    return node


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


def build_shell_master() -> unreal.Material:
    """Translucent hologram shell: colour appears only on the fresnel rim, so
    spheres read as atmosphere/field volumes instead of solid glass balls."""
    material = ensure_material("M_HolographicShell")
    material.set_editor_property("blend_mode", unreal.BlendMode.BLEND_TRANSLUCENT)
    material.set_editor_property("shading_model", unreal.MaterialShadingModel.MSM_UNLIT)
    material.set_editor_property("two_sided", True)

    color = vector(material, "Color", unreal.LinearColor(0.0, 0.55, 1.0, 1.0), -800, -150)
    intensity = scalar(material, "Intensity", 1.6, -800, 40)
    opacity = scalar(material, "Opacity", 0.35, -800, 400)
    rim_exponent = scalar(material, "RimExponent", 3.2, -800, 220)

    fresnel = expression(material, unreal.MaterialExpressionFresnel, -560, 150)
    rim = expression(material, unreal.MaterialExpressionPower, -420, 150)
    MEL.connect_material_expressions(fresnel, "", rim, "Base")
    MEL.connect_material_expressions(rim_exponent, "", rim, "Exponent")

    tinted = expression(material, unreal.MaterialExpressionMultiply, -420, -80)
    MEL.connect_material_expressions(color, "", tinted, "A")
    MEL.connect_material_expressions(intensity, "", tinted, "B")
    emissive = expression(material, unreal.MaterialExpressionMultiply, -240, 0)
    MEL.connect_material_expressions(tinted, "", emissive, "A")
    MEL.connect_material_expressions(rim, "", emissive, "B")

    rim_opacity = expression(material, unreal.MaterialExpressionMultiply, -240, 330)
    MEL.connect_material_expressions(opacity, "", rim_opacity, "A")
    MEL.connect_material_expressions(rim, "", rim_opacity, "B")

    MEL.connect_material_property(emissive, "", unreal.MaterialProperty.MP_EMISSIVE_COLOR)
    MEL.connect_material_property(rim_opacity, "", unreal.MaterialProperty.MP_OPACITY)
    MEL.recompile_material(material)
    save_asset(f"{MATERIAL_DIR}/M_HolographicShell")
    return material


def build_signal_master() -> unreal.Material:
    material = ensure_material("M_HolographicSignal")
    material.set_editor_property("blend_mode", unreal.BlendMode.BLEND_ADDITIVE)
    material.set_editor_property("shading_model", unreal.MaterialShadingModel.MSM_UNLIT)
    material.set_editor_property("two_sided", True)
    material.set_editor_property("used_with_instanced_static_meshes", True)

    color = vector(material, "Color", unreal.LinearColor(0.0, 0.8, 1.0, 1.0), -650, -120)
    intensity = scalar(material, "Intensity", 3.0, -650, 60)
    opacity = scalar(material, "Opacity", 0.85, -650, 240)
    multiply = expression(material, unreal.MaterialExpressionMultiply, -330, -70)
    MEL.connect_material_expressions(color, "", multiply, "A")
    MEL.connect_material_expressions(intensity, "", multiply, "B")

    # GPU age fade: instance custom data 0 = spawn time (game seconds),
    # 1 = 1/lifetime. Meshes without custom data read the defaults (0/0),
    # which resolves to a constant fade of 1.
    spawn_time = expression(material, unreal.MaterialExpressionPerInstanceCustomData, -900, 420)
    spawn_time.set_editor_property("data_index", 0)
    inv_lifetime = expression(material, unreal.MaterialExpressionPerInstanceCustomData, -900, 500)
    inv_lifetime.set_editor_property("data_index", 1)
    now = expression(material, unreal.MaterialExpressionTime, -900, 340)
    age = expression(material, unreal.MaterialExpressionSubtract, -720, 380)
    MEL.connect_material_expressions(now, "", age, "A")
    MEL.connect_material_expressions(spawn_time, "", age, "B")
    normalized = expression(material, unreal.MaterialExpressionMultiply, -580, 420)
    MEL.connect_material_expressions(age, "", normalized, "A")
    MEL.connect_material_expressions(inv_lifetime, "", normalized, "B")
    clamped_age = expression(material, unreal.MaterialExpressionSaturate, -520, 500)
    MEL.connect_material_expressions(normalized, "", clamped_age, "")
    # Cubic age curve: stays near full brightness for most of the lifetime,
    # then drops off steeply at the end.
    steepness = expression(material, unreal.MaterialExpressionConstant, -520, 580)
    steepness.set_editor_property("r", 3.0)
    curved_age = expression(material, unreal.MaterialExpressionPower, -440, 500)
    MEL.connect_material_expressions(clamped_age, "", curved_age, "Base")
    MEL.connect_material_expressions(steepness, "", curved_age, "Exponent")
    inverted = expression(material, unreal.MaterialExpressionOneMinus, -450, 420)
    MEL.connect_material_expressions(curved_age, "", inverted, "")
    fade = expression(material, unreal.MaterialExpressionSaturate, -330, 420)
    MEL.connect_material_expressions(inverted, "", fade, "")

    faded_emissive = expression(material, unreal.MaterialExpressionMultiply, -180, 0)
    MEL.connect_material_expressions(multiply, "", faded_emissive, "A")
    MEL.connect_material_expressions(fade, "", faded_emissive, "B")
    faded_opacity = expression(material, unreal.MaterialExpressionMultiply, -180, 260)
    MEL.connect_material_expressions(opacity, "", faded_opacity, "A")
    MEL.connect_material_expressions(fade, "", faded_opacity, "B")

    MEL.connect_material_property(faded_emissive, "", unreal.MaterialProperty.MP_EMISSIVE_COLOR)
    MEL.connect_material_property(faded_opacity, "", unreal.MaterialProperty.MP_OPACITY)
    MEL.recompile_material(material)
    save_asset(f"{MATERIAL_DIR}/M_HolographicSignal")
    return material


def build_heat_master() -> unreal.Material:
    """Additive splat for the activity heatmap: instance custom data 0 is the
    normalized heat, the quad UV drives a soft radial falloff, and hot cells
    shift from the base color toward white."""
    material = ensure_material("M_ActivityHeat")
    material.set_editor_property("blend_mode", unreal.BlendMode.BLEND_ADDITIVE)
    material.set_editor_property("shading_model", unreal.MaterialShadingModel.MSM_UNLIT)
    material.set_editor_property("used_with_instanced_static_meshes", True)

    color = vector(material, "Color", unreal.LinearColor(1.0, 0.36, 0.05, 1.0), -1100, -160)
    intensity = scalar(material, "Intensity", 2.6, -1100, 0)
    opacity = scalar(material, "Opacity", 0.85, -1100, 120)

    heat = expression(material, unreal.MaterialExpressionPerInstanceCustomData, -1100, 240)
    heat.set_editor_property("data_index", 0)

    uv = expression(material, unreal.MaterialExpressionTextureCoordinate, -1100, 360)
    center = expression(material, unreal.MaterialExpressionConstant2Vector, -1100, 460)
    center.set_editor_property("r", 0.5)
    center.set_editor_property("g", 0.5)
    dist = expression(material, unreal.MaterialExpressionDistance, -930, 400)
    MEL.connect_material_expressions(uv, "", dist, "A")
    MEL.connect_material_expressions(center, "", dist, "B")
    two = expression(material, unreal.MaterialExpressionConstant, -930, 480)
    two.set_editor_property("r", 2.0)
    doubled = expression(material, unreal.MaterialExpressionMultiply, -790, 420)
    MEL.connect_material_expressions(dist, "", doubled, "A")
    MEL.connect_material_expressions(two, "", doubled, "B")
    inverted = expression(material, unreal.MaterialExpressionOneMinus, -670, 420)
    MEL.connect_material_expressions(doubled, "", inverted, "")
    clamped = expression(material, unreal.MaterialExpressionSaturate, -570, 420)
    MEL.connect_material_expressions(inverted, "", clamped, "")
    exponent = expression(material, unreal.MaterialExpressionConstant, -570, 500)
    exponent.set_editor_property("r", 2.0)
    falloff = expression(material, unreal.MaterialExpressionPower, -470, 420)
    MEL.connect_material_expressions(clamped, "", falloff, "Base")
    MEL.connect_material_expressions(exponent, "", falloff, "Exponent")

    white = expression(material, unreal.MaterialExpressionConstant3Vector, -900, -260)
    white.set_editor_property("constant", unreal.LinearColor(1.0, 0.95, 0.85, 1.0))
    ramp_alpha = expression(material, unreal.MaterialExpressionMultiply, -900, -60)
    hot_shift = expression(material, unreal.MaterialExpressionConstant, -1040, -40)
    hot_shift.set_editor_property("r", 0.6)
    MEL.connect_material_expressions(heat, "", ramp_alpha, "A")
    MEL.connect_material_expressions(hot_shift, "", ramp_alpha, "B")
    ramp = expression(material, unreal.MaterialExpressionLinearInterpolate, -720, -160)
    MEL.connect_material_expressions(color, "", ramp, "A")
    MEL.connect_material_expressions(white, "", ramp, "B")
    MEL.connect_material_expressions(ramp_alpha, "", ramp, "Alpha")

    lit = expression(material, unreal.MaterialExpressionMultiply, -560, -120)
    MEL.connect_material_expressions(ramp, "", lit, "A")
    MEL.connect_material_expressions(intensity, "", lit, "B")
    heated = expression(material, unreal.MaterialExpressionMultiply, -420, -80)
    MEL.connect_material_expressions(lit, "", heated, "A")
    MEL.connect_material_expressions(heat, "", heated, "B")
    emissive = expression(material, unreal.MaterialExpressionMultiply, -280, -40)
    MEL.connect_material_expressions(heated, "", emissive, "A")
    MEL.connect_material_expressions(falloff, "", emissive, "B")

    opacity_heat = expression(material, unreal.MaterialExpressionMultiply, -420, 200)
    MEL.connect_material_expressions(opacity, "", opacity_heat, "A")
    MEL.connect_material_expressions(heat, "", opacity_heat, "B")
    opacity_out = expression(material, unreal.MaterialExpressionMultiply, -280, 240)
    MEL.connect_material_expressions(opacity_heat, "", opacity_out, "A")
    MEL.connect_material_expressions(falloff, "", opacity_out, "B")
    opacity_clamped = expression(material, unreal.MaterialExpressionSaturate, -170, 240)
    MEL.connect_material_expressions(opacity_out, "", opacity_clamped, "")

    MEL.connect_material_property(emissive, "", unreal.MaterialProperty.MP_EMISSIVE_COLOR)
    MEL.connect_material_property(opacity_clamped, "", unreal.MaterialProperty.MP_OPACITY)
    MEL.recompile_material(material)
    save_asset(f"{MATERIAL_DIR}/M_ActivityHeat")
    return material


def build_earth_master(day_texture, night_texture) -> unreal.Material:
    """Physically plausible globe: day albedo, city lights masked to the true
    night side via the SunDirection parameter, glossy oceans, matte land."""
    material = ensure_material("M_EarthSurface")
    material.set_editor_property("blend_mode", unreal.BlendMode.BLEND_OPAQUE)
    material.set_editor_property("shading_model", unreal.MaterialShadingModel.MSM_DEFAULT_LIT)
    material.set_editor_property("two_sided", False)

    day = expression(material, unreal.MaterialExpressionTextureSample, -1050, -260)
    day.set_editor_property("texture", day_texture)
    night = expression(material, unreal.MaterialExpressionTextureSample, -1050, 60)
    night.set_editor_property("texture", night_texture)

    # Night mask: saturate(-dot(PixelNormalWS, SunDirection)) ^ 0.65 keeps a
    # soft terminator and zero city lights on the day side.
    sun_direction = vector(material, "SunDirection", unreal.LinearColor(1.0, 0.0, 0.0, 0.0), -1050, 330)
    normal = expression(material, unreal.MaterialExpressionPixelNormalWS, -1050, 500)
    dot_node = expression(material, unreal.MaterialExpressionDotProduct, -840, 400)
    MEL.connect_material_expressions(normal, "", dot_node, "A")
    MEL.connect_material_expressions(sun_direction, "", dot_node, "B")
    negate = expression(material, unreal.MaterialExpressionMultiply, -700, 420)
    minus_one = expression(material, unreal.MaterialExpressionConstant, -840, 560)
    minus_one.set_editor_property("r", -1.0)
    MEL.connect_material_expressions(dot_node, "", negate, "A")
    MEL.connect_material_expressions(minus_one, "", negate, "B")
    clamped = expression(material, unreal.MaterialExpressionSaturate, -560, 420)
    MEL.connect_material_expressions(negate, "", clamped, "")
    softness = expression(material, unreal.MaterialExpressionConstant, -560, 560)
    softness.set_editor_property("r", 0.65)
    night_mask = expression(material, unreal.MaterialExpressionPower, -430, 440)
    MEL.connect_material_expressions(clamped, "", night_mask, "Base")
    MEL.connect_material_expressions(softness, "", night_mask, "Exponent")

    night_strength = scalar(material, "NightIntensity", 2.4, -700, 180)
    night_scaled = expression(material, unreal.MaterialExpressionMultiply, -520, 120)
    MEL.connect_material_expressions(night, "RGB", night_scaled, "A")
    MEL.connect_material_expressions(night_strength, "", night_scaled, "B")
    night_emissive = expression(material, unreal.MaterialExpressionMultiply, -300, 200)
    MEL.connect_material_expressions(night_scaled, "", night_emissive, "A")
    MEL.connect_material_expressions(night_mask, "", night_emissive, "B")

    # Ocean gloss: blue-dominant day pixels become smooth, land stays matte.
    blue_minus_red = expression(material, unreal.MaterialExpressionSubtract, -840, -80)
    MEL.connect_material_expressions(day, "B", blue_minus_red, "A")
    MEL.connect_material_expressions(day, "R", blue_minus_red, "B")
    ocean_gain = expression(material, unreal.MaterialExpressionConstant, -840, 20)
    ocean_gain.set_editor_property("r", 4.0)
    ocean_scaled = expression(material, unreal.MaterialExpressionMultiply, -700, -60)
    MEL.connect_material_expressions(blue_minus_red, "", ocean_scaled, "A")
    MEL.connect_material_expressions(ocean_gain, "", ocean_scaled, "B")
    ocean_mask = expression(material, unreal.MaterialExpressionSaturate, -560, -60)
    MEL.connect_material_expressions(ocean_scaled, "", ocean_mask, "")
    rough_land = scalar(material, "LandRoughness", 0.88, -560, -220)
    rough_ocean = scalar(material, "OceanRoughness", 0.32, -560, -140)
    roughness = expression(material, unreal.MaterialExpressionLinearInterpolate, -380, -140)
    MEL.connect_material_expressions(rough_land, "", roughness, "A")
    MEL.connect_material_expressions(rough_ocean, "", roughness, "B")
    MEL.connect_material_expressions(ocean_mask, "", roughness, "Alpha")

    MEL.connect_material_property(day, "RGB", unreal.MaterialProperty.MP_BASE_COLOR)
    MEL.connect_material_property(night_emissive, "", unreal.MaterialProperty.MP_EMISSIVE_COLOR)
    MEL.connect_material_property(roughness, "", unreal.MaterialProperty.MP_ROUGHNESS)
    MEL.recompile_material(material)
    save_asset(f"{MATERIAL_DIR}/M_EarthSurface")
    return material


def build_starfield_master(texture) -> unreal.Material:
    material = ensure_material("M_Starfield")
    material.set_editor_property("blend_mode", unreal.BlendMode.BLEND_OPAQUE)
    material.set_editor_property("shading_model", unreal.MaterialShadingModel.MSM_UNLIT)
    material.set_editor_property("two_sided", True)
    sample = expression(material, unreal.MaterialExpressionTextureSample, -600, -80)
    sample.set_editor_property("texture", texture)
    strength = scalar(material, "Intensity", 0.8, -600, 150)
    emissive = expression(material, unreal.MaterialExpressionMultiply, -280, -20)
    MEL.connect_material_expressions(sample, "RGB", emissive, "A")
    MEL.connect_material_expressions(strength, "", emissive, "B")
    MEL.connect_material_property(emissive, "", unreal.MaterialProperty.MP_EMISSIVE_COLOR)
    MEL.recompile_material(material)
    save_asset(f"{MATERIAL_DIR}/M_Starfield")
    return material


def create_instance(name, parent, color, opacity, intensity=None, rim_exponent=None) -> None:
    path = f"{MATERIAL_DIR}/{name}"
    instance = unreal.load_asset(path)
    if instance is None:
        tools = unreal.AssetToolsHelpers.get_asset_tools()
        instance = tools.create_asset(name, MATERIAL_DIR, unreal.MaterialInstanceConstant, unreal.MaterialInstanceConstantFactoryNew())
        if instance is None:
            raise RuntimeError(f"Could not create {path}")
    MEL.set_material_instance_parent(instance, parent)
    MEL.set_material_instance_vector_parameter_value(instance, "Color", color)
    MEL.set_material_instance_scalar_parameter_value(instance, "Opacity", opacity)
    if intensity is not None:
        MEL.set_material_instance_scalar_parameter_value(instance, "Intensity", intensity)
    if rim_exponent is not None:
        MEL.set_material_instance_scalar_parameter_value(instance, "RimExponent", rim_exponent)
    save_asset(path)


def main() -> None:
    ensure_directory(MATERIAL_DIR)
    ensure_directory(TEXTURE_DIR)

    # Release every level reference to the ION materials before rebuilding
    # them: a loaded command-deck level roots the assets and deletion asserts.
    unreal.EditorLevelLibrary.new_level("/Game/ION/Maps/L_TransientRebuild")
    unreal.SystemLibrary.collect_garbage()

    day = import_texture(SOURCE_DIR / "NASA" / "bluemarble-2048.png", "T_EarthDay")
    night = import_texture(SOURCE_DIR / "NASA" / "earthatnight-2048.png", "T_EarthNight")
    stars = import_texture(SOURCE_DIR / "Generated" / "starfield.png", "T_Starfield")
    build_earth_master(day, night)
    build_starfield_master(stars)

    shell = build_shell_master()
    create_instance("MI_Atmosphere", shell, unreal.LinearColor(0.12, 0.45, 1.0, 1.0), 0.62, intensity=2.4, rim_exponent=2.6)
    create_instance("MI_Ionosphere", shell, unreal.LinearColor(0.0, 0.85, 1.0, 1.0), 0.10, intensity=1.0, rim_exponent=4.0)

    signal = build_signal_master()
    for index in range(11):
        red, green, blue = colorsys.hsv_to_rgb(index / 11.0, 0.72, 1.0)
        create_instance(f"MI_Signal_{index:02d}", signal, unreal.LinearColor(red, green, blue, 1.0), 0.62, intensity=4.6)
    create_instance("MI_Signal_Selected", signal, unreal.LinearColor(0.78, 1.0, 1.0, 1.0), 1.0, intensity=11.0)
    create_instance("MI_Point_Entity", signal, unreal.LinearColor(0.0, 0.82, 1.0, 1.0), 0.8, intensity=3.2)
    create_instance("MI_Point_Observation", signal, unreal.LinearColor(1.0, 0.32, 0.05, 1.0), 0.9, intensity=4.0)
    create_instance("MI_Aurora_North", signal, unreal.LinearColor(0.02, 1.0, 0.38, 1.0), 0.22, intensity=1.6)
    create_instance("MI_Aurora_South", signal, unreal.LinearColor(0.08, 0.55, 1.0, 1.0), 0.20, intensity=1.4)
    create_instance("MI_Console", signal, unreal.LinearColor(0.0, 0.16, 0.42, 1.0), 0.30, intensity=0.5)

    build_heat_master()

    if unreal.EditorAssetLibrary.does_asset_exist("/Game/ION/Maps/L_CommandDeck"):
        unreal.EditorLevelLibrary.load_level("/Game/ION/Maps/L_CommandDeck")
    if unreal.EditorAssetLibrary.does_asset_exist("/Game/ION/Maps/L_TransientRebuild"):
        unreal.EditorAssetLibrary.delete_asset("/Game/ION/Maps/L_TransientRebuild")
    unreal.log("ION COMMAND: visual material bootstrap complete")


if __name__ == "__main__":
    main()
