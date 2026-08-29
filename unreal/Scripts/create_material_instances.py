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


def mask(material, source, channels, x, y):
    """Component mask, e.g. mask(m, node, "rg", x, y) for a float2 of X and Y."""
    node = expression(material, unreal.MaterialExpressionComponentMask, x, y)
    for channel in "rgba":
        node.set_editor_property(channel, channel in channels)
    MEL.connect_material_expressions(source, "", node, "")
    return node


def constant(material, value, x, y):
    node = expression(material, unreal.MaterialExpressionConstant, x, y)
    node.set_editor_property("r", value)
    return node


def import_texture(source: Path, asset_name: str, virtual_texture: bool = False) -> unreal.Texture2D:
    if not source.exists():
        raise RuntimeError(f"Missing visual source texture: {source}")
    asset_path = f"{TEXTURE_DIR}/{asset_name}"
    # Always reimport: a cached asset would silently keep an outdated source
    # resolution (the 2048 -> 4096 upgrade was invisible until forced).
    if unreal.EditorAssetLibrary.does_asset_exist(asset_path):
        unreal.EditorAssetLibrary.delete_asset(asset_path)

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
    if virtual_texture:
        # Streaming Virtual Texture: only the mip/tile actually on screen is
        # ever resident in VRAM, so a 21600x10800 source does not need to fit
        # in full. Requires r.VirtualTextures=True (DefaultEngine.ini) to
        # actually stream as VT instead of silently falling back. Mutually
        # exclusive with never_stream below (which forces full residency -
        # the opposite intent).
        #
        # power_of_two_mode must be set explicitly too: without it, VT
        # silently fails to build for a non-power-of-two source and
        # virtual_texture_streaming reads back False after saving, with no
        # error anywhere (verified: T_EarthNight's 13500x6750 stayed
        # un-padded and non-VT until this property was set).
        #
        # It must be STRETCH, never PAD. Padding is NOT transparent to
        # material UVs: it leaves the source image in the corner of a larger
        # power-of-two canvas, so UV 1.0 lands in the empty padding instead
        # of at longitude +180. The whole map is then stretched east by the
        # padding ratio - 21600/32768 for the day texture and 13500/16384
        # for the night one, which is also why the two sides of the
        # terminator were displaced by different amounts. Measured on the
        # packaged client: a camera centred on the operator's own station
        # (47.52 N, 9.21 E) rendered 38 N / 50 E (the Caspian) under the
        # night texture, and a camera on California rendered Panama under
        # the day one - both exactly the padding ratio predicts. Stretching
        # resamples the source to fill the canvas, so UV 0..1 spans the
        # full map and no material needs to know the source dimensions.
        imported.set_editor_property("power_of_two_mode", unreal.TexturePowerOfTwoSetting.STRETCH_TO_POWER_OF_TWO)
        imported.set_editor_property("virtual_texture_streaming", True)
        # Read back rather than trust the setter: a padded texture renders a
        # plausible-looking Earth in the wrong place, which no amount of
        # looking at a screenshot reliably catches.
        mode = imported.get_editor_property("power_of_two_mode")
        if mode != unreal.TexturePowerOfTwoSetting.STRETCH_TO_POWER_OF_TWO:
            raise RuntimeError(
                f"{asset_name}: power_of_two_mode is {mode}; a PAD mode displaces the "
                f"whole map east by the padding ratio (see the comment above)")
    else:
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

    # Custom data 2 dims arcs converging on a congested endpoint. Meshes that
    # only allocate two custom floats read 0 there, which must mean "full
    # brightness" - hence the If mapping 0 -> 1.
    # brightness = data2 + (1 - saturate(data2 * 1e6)): exactly 1 when the
    # mesh carries no third custom float (reads 0), otherwise data2. Built
    # from plain nodes because the If node's python pin names silently fail
    # to connect ("Missing If AGreaterThanB input" broke the whole material).
    congestion = expression(material, unreal.MaterialExpressionPerInstanceCustomData, -900, 640)
    congestion.set_editor_property("data_index", 2)
    huge = expression(material, unreal.MaterialExpressionConstant, -900, 720)
    huge.set_editor_property("r", 1000000.0)
    amplified = expression(material, unreal.MaterialExpressionMultiply, -780, 660)
    MEL.connect_material_expressions(congestion, "", amplified, "A")
    MEL.connect_material_expressions(huge, "", amplified, "B")
    presence = expression(material, unreal.MaterialExpressionSaturate, -690, 660)
    MEL.connect_material_expressions(amplified, "", presence, "")
    absent = expression(material, unreal.MaterialExpressionOneMinus, -620, 660)
    MEL.connect_material_expressions(presence, "", absent, "")
    brightness = expression(material, unreal.MaterialExpressionAdd, -540, 640)
    MEL.connect_material_expressions(congestion, "", brightness, "A")
    MEL.connect_material_expressions(absent, "", brightness, "B")

    faded_emissive = expression(material, unreal.MaterialExpressionMultiply, -180, 0)
    MEL.connect_material_expressions(multiply, "", faded_emissive, "A")
    MEL.connect_material_expressions(fade, "", faded_emissive, "B")
    dimmed_emissive = expression(material, unreal.MaterialExpressionMultiply, -60, 40)
    MEL.connect_material_expressions(faded_emissive, "", dimmed_emissive, "A")
    MEL.connect_material_expressions(brightness, "", dimmed_emissive, "B")
    faded_opacity = expression(material, unreal.MaterialExpressionMultiply, -180, 260)
    MEL.connect_material_expressions(opacity, "", faded_opacity, "A")
    MEL.connect_material_expressions(fade, "", faded_opacity, "B")

    MEL.connect_material_property(dimmed_emissive, "", unreal.MaterialProperty.MP_EMISSIVE_COLOR)
    MEL.connect_material_property(faded_opacity, "", unreal.MaterialProperty.MP_OPACITY)
    MEL.recompile_material(material)
    save_asset(f"{MATERIAL_DIR}/M_HolographicSignal")
    return material


def build_scatter_atmosphere_master() -> unreal.Material:
    """Physically inspired scattering shell: a Fresnel rim whose color follows
    the sun angle - Rayleigh blue on the day side, a warm terminator band,
    near-black night. Not a radiative transfer solve; an artist's model."""
    material = ensure_material("M_AtmosphereScatter")
    material.set_editor_property("blend_mode", unreal.BlendMode.BLEND_TRANSLUCENT)
    material.set_editor_property("shading_model", unreal.MaterialShadingModel.MSM_UNLIT)
    material.set_editor_property("two_sided", False)

    sun = vector(material, "SunDirection", unreal.LinearColor(1.0, 0.0, 0.0, 0.0), -1150, 260)
    intensity = scalar(material, "Intensity", 2.2, -1150, 100)

    normal = expression(material, unreal.MaterialExpressionPixelNormalWS, -1150, 420)
    sun_dot = expression(material, unreal.MaterialExpressionDotProduct, -960, 340)
    MEL.connect_material_expressions(normal, "", sun_dot, "A")
    MEL.connect_material_expressions(sun, "", sun_dot, "B")

    # Day factor: saturate(sunDot); terminator factor: 1 - |sunDot| squared.
    day = expression(material, unreal.MaterialExpressionSaturate, -820, 300)
    MEL.connect_material_expressions(sun_dot, "", day, "")
    absdot = expression(material, unreal.MaterialExpressionAbs, -820, 400)
    MEL.connect_material_expressions(sun_dot, "", absdot, "")
    inv = expression(material, unreal.MaterialExpressionOneMinus, -720, 400)
    MEL.connect_material_expressions(absdot, "", inv, "")
    term_sq = expression(material, unreal.MaterialExpressionMultiply, -620, 400)
    MEL.connect_material_expressions(inv, "", term_sq, "A")
    MEL.connect_material_expressions(inv, "", term_sq, "B")
    term_cub = expression(material, unreal.MaterialExpressionMultiply, -530, 420)
    MEL.connect_material_expressions(term_sq, "", term_cub, "A")
    MEL.connect_material_expressions(inv, "", term_cub, "B")

    rayleigh = expression(material, unreal.MaterialExpressionConstant3Vector, -820, 120)
    rayleigh.set_editor_property("constant", unreal.LinearColor(0.18, 0.42, 1.0, 1.0))
    sunset = expression(material, unreal.MaterialExpressionConstant3Vector, -820, 200)
    sunset.set_editor_property("constant", unreal.LinearColor(1.0, 0.38, 0.10, 1.0))

    day_color = expression(material, unreal.MaterialExpressionMultiply, -620, 160)
    MEL.connect_material_expressions(rayleigh, "", day_color, "A")
    MEL.connect_material_expressions(day, "", day_color, "B")
    sunset_gain = expression(material, unreal.MaterialExpressionConstant, -620, 260)
    sunset_gain.set_editor_property("r", 1.4)
    term_amount = expression(material, unreal.MaterialExpressionMultiply, -530, 300)
    MEL.connect_material_expressions(term_cub, "", term_amount, "A")
    MEL.connect_material_expressions(sunset_gain, "", term_amount, "B")
    sunset_color = expression(material, unreal.MaterialExpressionMultiply, -440, 220)
    MEL.connect_material_expressions(sunset, "", sunset_color, "A")
    MEL.connect_material_expressions(term_amount, "", sunset_color, "B")
    mixed = expression(material, unreal.MaterialExpressionAdd, -340, 170)
    MEL.connect_material_expressions(day_color, "", mixed, "A")
    MEL.connect_material_expressions(sunset_color, "", mixed, "B")

    fresnel = expression(material, unreal.MaterialExpressionFresnel, -820, -40)
    fresnel.set_editor_property("exponent", 3.2)
    rim = expression(material, unreal.MaterialExpressionMultiply, -520, 40)
    MEL.connect_material_expressions(fresnel, "", rim, "A")
    MEL.connect_material_expressions(intensity, "", rim, "B")

    emissive = expression(material, unreal.MaterialExpressionMultiply, -220, 100)
    MEL.connect_material_expressions(mixed, "", emissive, "A")
    MEL.connect_material_expressions(rim, "", emissive, "B")

    # Opacity: rim strength scaled by how lit the limb is (night rim fades).
    night_floor = expression(material, unreal.MaterialExpressionConstant, -520, 520)
    night_floor.set_editor_property("r", 0.12)
    lit_amount = expression(material, unreal.MaterialExpressionAdd, -430, 470)
    MEL.connect_material_expressions(day, "", lit_amount, "A")
    MEL.connect_material_expressions(night_floor, "", lit_amount, "B")
    lit_clamped = expression(material, unreal.MaterialExpressionSaturate, -350, 470)
    MEL.connect_material_expressions(lit_amount, "", lit_clamped, "")
    opacity_raw = expression(material, unreal.MaterialExpressionMultiply, -260, 320)
    MEL.connect_material_expressions(fresnel, "", opacity_raw, "A")
    MEL.connect_material_expressions(lit_clamped, "", opacity_raw, "B")
    opacity = expression(material, unreal.MaterialExpressionSaturate, -170, 320)
    MEL.connect_material_expressions(opacity_raw, "", opacity, "")

    MEL.connect_material_property(emissive, "", unreal.MaterialProperty.MP_EMISSIVE_COLOR)
    MEL.connect_material_property(opacity, "", unreal.MaterialProperty.MP_OPACITY)
    MEL.recompile_material(material)
    save_asset(f"{MATERIAL_DIR}/M_AtmosphereScatter")
    return material


def build_cloud_master(cloud_texture) -> unreal.Material:
    """Lit translucent cloud shell: the mosaic brightness becomes opacity, so
    lighting gives day-side white clouds and a dark night side for free."""
    material = ensure_material("M_CloudLayer")
    material.set_editor_property("blend_mode", unreal.BlendMode.BLEND_TRANSLUCENT)
    material.set_editor_property("shading_model", unreal.MaterialShadingModel.MSM_DEFAULT_LIT)
    material.set_editor_property("two_sided", False)

    # Texture PARAMETER: the globe actor swaps in the live EUMETSAT world IR
    # composite at runtime; the packaged static texture is the offline default.
    sample = expression(material, unreal.MaterialExpressionTextureSampleParameter2D, -700, -60)
    sample.set_editor_property("parameter_name", "CloudMap")
    sample.set_editor_property("texture", cloud_texture)
    sample.set_editor_property("sampler_type", unreal.MaterialSamplerType.SAMPLERTYPE_LINEAR_COLOR)
    base = expression(material, unreal.MaterialExpressionConstant3Vector, -700, -220)
    base.set_editor_property("constant", unreal.LinearColor(0.92, 0.94, 0.98, 1.0))
    density = scalar(material, "Density", 0.55, -700, 140)
    opacity = expression(material, unreal.MaterialExpressionMultiply, -420, 60)
    MEL.connect_material_expressions(sample, "R", opacity, "A")
    MEL.connect_material_expressions(density, "", opacity, "B")
    rough = expression(material, unreal.MaterialExpressionConstant, -420, 220)
    rough.set_editor_property("r", 0.9)

    MEL.connect_material_property(base, "", unreal.MaterialProperty.MP_BASE_COLOR)
    MEL.connect_material_property(opacity, "", unreal.MaterialProperty.MP_OPACITY)
    MEL.connect_material_property(rough, "", unreal.MaterialProperty.MP_ROUGHNESS)
    MEL.recompile_material(material)
    save_asset(f"{MATERIAL_DIR}/M_CloudLayer")
    return material


def build_selected_path_master() -> unreal.Material:
    """Energy pulse for the selected path: custom data 0 carries the segment's
    position along the path, and a travelling sine wave sweeps TX -> RX."""
    material = ensure_material("M_SelectedPath")
    material.set_editor_property("blend_mode", unreal.BlendMode.BLEND_ADDITIVE)
    material.set_editor_property("shading_model", unreal.MaterialShadingModel.MSM_UNLIT)
    material.set_editor_property("two_sided", True)
    material.set_editor_property("used_with_instanced_static_meshes", True)

    color = vector(material, "Color", unreal.LinearColor(0.75, 1.0, 1.0, 1.0), -900, -160)
    intensity = scalar(material, "Intensity", 9.0, -900, 0)

    alpha = expression(material, unreal.MaterialExpressionPerInstanceCustomData, -900, 200)
    alpha.set_editor_property("data_index", 0)
    now = expression(material, unreal.MaterialExpressionTime, -900, 300)
    speed = expression(material, unreal.MaterialExpressionConstant, -900, 380)
    speed.set_editor_property("r", 1.1)
    travel = expression(material, unreal.MaterialExpressionMultiply, -760, 320)
    MEL.connect_material_expressions(now, "", travel, "A")
    MEL.connect_material_expressions(speed, "", travel, "B")
    stretch = expression(material, unreal.MaterialExpressionConstant, -900, 460)
    stretch.set_editor_property("r", 2.4)
    phase_pos = expression(material, unreal.MaterialExpressionMultiply, -760, 420)
    MEL.connect_material_expressions(alpha, "", phase_pos, "A")
    MEL.connect_material_expressions(stretch, "", phase_pos, "B")
    phase = expression(material, unreal.MaterialExpressionSubtract, -620, 360)
    MEL.connect_material_expressions(travel, "", phase, "A")
    MEL.connect_material_expressions(phase_pos, "", phase, "B")
    wave = expression(material, unreal.MaterialExpressionSine, -520, 360)
    MEL.connect_material_expressions(phase, "", wave, "")
    half = expression(material, unreal.MaterialExpressionConstant, -520, 440)
    half.set_editor_property("r", 0.45)
    swing = expression(material, unreal.MaterialExpressionMultiply, -420, 380)
    MEL.connect_material_expressions(wave, "", swing, "A")
    MEL.connect_material_expressions(half, "", swing, "B")
    base_level = expression(material, unreal.MaterialExpressionConstant, -420, 460)
    base_level.set_editor_property("r", 0.6)
    pulse = expression(material, unreal.MaterialExpressionAdd, -320, 400)
    MEL.connect_material_expressions(swing, "", pulse, "A")
    MEL.connect_material_expressions(base_level, "", pulse, "B")

    lit = expression(material, unreal.MaterialExpressionMultiply, -560, -100)
    MEL.connect_material_expressions(color, "", lit, "A")
    MEL.connect_material_expressions(intensity, "", lit, "B")
    emissive = expression(material, unreal.MaterialExpressionMultiply, -300, -40)
    MEL.connect_material_expressions(lit, "", emissive, "A")
    MEL.connect_material_expressions(pulse, "", emissive, "B")
    opacity = expression(material, unreal.MaterialExpressionSaturate, -300, 200)
    MEL.connect_material_expressions(pulse, "", opacity, "")

    MEL.connect_material_property(emissive, "", unreal.MaterialProperty.MP_EMISSIVE_COLOR)
    MEL.connect_material_property(opacity, "", unreal.MaterialProperty.MP_OPACITY)
    MEL.recompile_material(material)
    save_asset(f"{MATERIAL_DIR}/M_SelectedPath")
    return material


def build_marker_icon_master(icon_texture) -> unreal.Material:
    """Camera-facing pictogram markers: one instanced quad per marker. The
    per-instance contract (GeoPointLayerActor sets 7 custom floats):
      0 = atlas tile index (4x2 grid, see generate_visual_sources.py)
      1..3 = RGB tint
      4..6 = instance origin in world space (drives the billboard rotation;
             passing it as data avoids relying on Local->World including the
             instance transform, which is vertex-factory dependent).
    Billboard math: T = vertex world pos - origin is the scaled local offset
    (instances never rotate), and the vertex is re-aimed into the camera
    plane via WPO = right*T.x + up*T.y - T."""
    material = ensure_material("M_MarkerIcon")
    material.set_editor_property("blend_mode", unreal.BlendMode.BLEND_ADDITIVE)
    material.set_editor_property("shading_model", unreal.MaterialShadingModel.MSM_UNLIT)
    material.set_editor_property("two_sided", True)
    material.set_editor_property("used_with_instanced_static_meshes", True)

    # --- atlas UV from custom data 0 ---
    icon_index = expression(material, unreal.MaterialExpressionPerInstanceCustomData, -1500, -300)
    icon_index.set_editor_property("data_index", 0)
    columns = expression(material, unreal.MaterialExpressionConstant, -1500, -220)
    columns.set_editor_property("r", 4.0)
    column = expression(material, unreal.MaterialExpressionFmod, -1340, -320)
    MEL.connect_material_expressions(icon_index, "", column, "A")
    MEL.connect_material_expressions(columns, "", column, "B")
    row_raw = expression(material, unreal.MaterialExpressionDivide, -1340, -220)
    MEL.connect_material_expressions(icon_index, "", row_raw, "A")
    MEL.connect_material_expressions(columns, "", row_raw, "B")
    row = expression(material, unreal.MaterialExpressionFloor, -1220, -220)
    MEL.connect_material_expressions(row_raw, "", row, "")
    tile = expression(material, unreal.MaterialExpressionAppendVector, -1120, -280)
    MEL.connect_material_expressions(column, "", tile, "A")
    MEL.connect_material_expressions(row, "", tile, "B")
    base_uv = expression(material, unreal.MaterialExpressionTextureCoordinate, -1120, -380)
    uv_sum = expression(material, unreal.MaterialExpressionAdd, -980, -330)
    MEL.connect_material_expressions(base_uv, "", uv_sum, "A")
    MEL.connect_material_expressions(tile, "", uv_sum, "B")
    tile_scale = expression(material, unreal.MaterialExpressionConstant2Vector, -980, -240)
    tile_scale.set_editor_property("r", 0.25)
    tile_scale.set_editor_property("g", 0.25)
    uv = expression(material, unreal.MaterialExpressionMultiply, -850, -300)
    MEL.connect_material_expressions(uv_sum, "", uv, "A")
    MEL.connect_material_expressions(tile_scale, "", uv, "B")
    sample = expression(material, unreal.MaterialExpressionTextureSample, -700, -340)
    sample.set_editor_property("texture", icon_texture)
    MEL.connect_material_expressions(uv, "", sample, "UVs")

    # --- tint from custom data 1..3 ---
    tint_r = expression(material, unreal.MaterialExpressionPerInstanceCustomData, -1500, -120)
    tint_r.set_editor_property("data_index", 1)
    tint_g = expression(material, unreal.MaterialExpressionPerInstanceCustomData, -1500, -40)
    tint_g.set_editor_property("data_index", 2)
    tint_b = expression(material, unreal.MaterialExpressionPerInstanceCustomData, -1500, 40)
    tint_b.set_editor_property("data_index", 3)
    tint_rg = expression(material, unreal.MaterialExpressionAppendVector, -1340, -80)
    MEL.connect_material_expressions(tint_r, "", tint_rg, "A")
    MEL.connect_material_expressions(tint_g, "", tint_rg, "B")
    tint = expression(material, unreal.MaterialExpressionAppendVector, -1220, -60)
    MEL.connect_material_expressions(tint_rg, "", tint, "A")
    MEL.connect_material_expressions(tint_b, "", tint, "B")
    intensity = scalar(material, "Intensity", 5.0, -1220, 40)
    tinted = expression(material, unreal.MaterialExpressionMultiply, -1080, -20)
    MEL.connect_material_expressions(tint, "", tinted, "A")
    MEL.connect_material_expressions(intensity, "", tinted, "B")
    emissive = expression(material, unreal.MaterialExpressionMultiply, -520, -160)
    MEL.connect_material_expressions(sample, "RGB", emissive, "A")
    MEL.connect_material_expressions(tinted, "", emissive, "B")

    opacity_param = scalar(material, "Opacity", 0.95, -700, -80)
    opacity = expression(material, unreal.MaterialExpressionMultiply, -520, 0)
    MEL.connect_material_expressions(sample, "R", opacity, "A")
    MEL.connect_material_expressions(opacity_param, "", opacity, "B")

    # --- billboard world position offset from custom data 4..6 ---
    origin_x = expression(material, unreal.MaterialExpressionPerInstanceCustomData, -1500, 200)
    origin_x.set_editor_property("data_index", 4)
    origin_y = expression(material, unreal.MaterialExpressionPerInstanceCustomData, -1500, 280)
    origin_y.set_editor_property("data_index", 5)
    origin_z = expression(material, unreal.MaterialExpressionPerInstanceCustomData, -1500, 360)
    origin_z.set_editor_property("data_index", 6)
    origin_xy = expression(material, unreal.MaterialExpressionAppendVector, -1340, 240)
    MEL.connect_material_expressions(origin_x, "", origin_xy, "A")
    MEL.connect_material_expressions(origin_y, "", origin_xy, "B")
    origin = expression(material, unreal.MaterialExpressionAppendVector, -1220, 260)
    MEL.connect_material_expressions(origin_xy, "", origin, "A")
    MEL.connect_material_expressions(origin_z, "", origin, "B")

    world_pos = expression(material, unreal.MaterialExpressionWorldPosition, -1220, 400)
    local_offset = expression(material, unreal.MaterialExpressionSubtract, -1060, 340)
    MEL.connect_material_expressions(world_pos, "", local_offset, "A")
    MEL.connect_material_expressions(origin, "", local_offset, "B")

    camera = expression(material, unreal.MaterialExpressionCameraPositionWS, -1220, 500)
    to_marker = expression(material, unreal.MaterialExpressionSubtract, -1060, 480)
    MEL.connect_material_expressions(origin, "", to_marker, "A")
    MEL.connect_material_expressions(camera, "", to_marker, "B")
    forward = expression(material, unreal.MaterialExpressionNormalize, -940, 480)
    MEL.connect_material_expressions(to_marker, "", forward, "")
    # Default billboard frame. Guarded against the pole-on degeneracy where
    # forward is parallel to world-up: Cross(world_up, forward) -> 0 ->
    # Normalize NaN would poison the whole quad. Blend to an X-axis frame
    # before normalizing so the cross is always non-zero. (Audit finding #18.)
    world_up = expression(material, unreal.MaterialExpressionConstant3Vector, -1060, 560)
    world_up.set_editor_property("constant", unreal.LinearColor(0.0, 0.0, 1.0, 0.0))
    world_x = expression(material, unreal.MaterialExpressionConstant3Vector, -1060, 640)
    world_x.set_editor_property("constant", unreal.LinearColor(1.0, 0.0, 0.0, 0.0))
    right_raw = expression(material, unreal.MaterialExpressionCrossProduct, -900, 520)
    MEL.connect_material_expressions(world_up, "", right_raw, "A")
    MEL.connect_material_expressions(forward, "", right_raw, "B")
    right_alt = expression(material, unreal.MaterialExpressionCrossProduct, -900, 600)
    MEL.connect_material_expressions(world_x, "", right_alt, "A")
    MEL.connect_material_expressions(forward, "", right_alt, "B")
    right_len2 = expression(material, unreal.MaterialExpressionDotProduct, -820, 560)
    MEL.connect_material_expressions(right_raw, "", right_len2, "A")
    MEL.connect_material_expressions(right_raw, "", right_len2, "B")
    right_gain = expression(material, unreal.MaterialExpressionConstant, -820, 640)
    right_gain.set_editor_property("r", 10000.0)
    right_scaled = expression(material, unreal.MaterialExpressionMultiply, -740, 580)
    MEL.connect_material_expressions(right_len2, "", right_scaled, "A")
    MEL.connect_material_expressions(right_gain, "", right_scaled, "B")
    right_mask = expression(material, unreal.MaterialExpressionSaturate, -680, 580)
    MEL.connect_material_expressions(right_scaled, "", right_mask, "")
    right_pre = expression(material, unreal.MaterialExpressionLinearInterpolate, -620, 540)
    MEL.connect_material_expressions(right_alt, "", right_pre, "A")
    MEL.connect_material_expressions(right_raw, "", right_pre, "B")
    MEL.connect_material_expressions(right_mask, "", right_pre, "Alpha")
    right_default = expression(material, unreal.MaterialExpressionNormalize, -540, 540)
    MEL.connect_material_expressions(right_pre, "", right_default, "")
    up_default = expression(material, unreal.MaterialExpressionCrossProduct, -540, 620)
    MEL.connect_material_expressions(forward, "", up_default, "A")
    MEL.connect_material_expressions(right_default, "", up_default, "B")

    # --- optional glyph orientation from custom data 7..9 (world heading) ---
    # The heading projected into the billboard plane rotates the icon's up
    # axis toward its course; a zero heading (the common stationary case)
    # keeps the world-up default. CRITICAL: the planar heading is blended
    # into the up axis and only THEN normalized (audit finding #1). Blending
    # normalized vectors first meant Normalize((0,0,0))=NaN, and lerp(x,NaN,0)
    # is still NaN, so every stationary marker collapsed to an invisible quad.
    heading_x = expression(material, unreal.MaterialExpressionPerInstanceCustomData, -1500, 700)
    heading_x.set_editor_property("data_index", 7)
    heading_y = expression(material, unreal.MaterialExpressionPerInstanceCustomData, -1500, 780)
    heading_y.set_editor_property("data_index", 8)
    heading_z = expression(material, unreal.MaterialExpressionPerInstanceCustomData, -1500, 860)
    heading_z.set_editor_property("data_index", 9)
    heading_xy = expression(material, unreal.MaterialExpressionAppendVector, -1340, 740)
    MEL.connect_material_expressions(heading_x, "", heading_xy, "A")
    MEL.connect_material_expressions(heading_y, "", heading_xy, "B")
    heading = expression(material, unreal.MaterialExpressionAppendVector, -1220, 760)
    MEL.connect_material_expressions(heading_xy, "", heading, "A")
    MEL.connect_material_expressions(heading_z, "", heading, "B")
    heading_dot_fwd = expression(material, unreal.MaterialExpressionDotProduct, -1060, 740)
    MEL.connect_material_expressions(heading, "", heading_dot_fwd, "A")
    MEL.connect_material_expressions(forward, "", heading_dot_fwd, "B")
    fwd_component = expression(material, unreal.MaterialExpressionMultiply, -940, 760)
    MEL.connect_material_expressions(forward, "", fwd_component, "A")
    MEL.connect_material_expressions(heading_dot_fwd, "", fwd_component, "B")
    heading_planar = expression(material, unreal.MaterialExpressionSubtract, -820, 740)
    MEL.connect_material_expressions(heading, "", heading_planar, "A")
    MEL.connect_material_expressions(fwd_component, "", heading_planar, "B")

    # presence ramps smoothly with the planar magnitude squared instead of a
    # hard 1e6 saturation: 0 below ~1.1deg off the view axis, 1 by ~9deg. The
    # gentle band stops the glyph from spinning when a valid heading is viewed
    # nearly head-on (audit finding #13).
    planar_len2 = expression(material, unreal.MaterialExpressionDotProduct, -820, 900)
    MEL.connect_material_expressions(heading_planar, "", planar_len2, "A")
    MEL.connect_material_expressions(heading_planar, "", planar_len2, "B")
    presence_floor = expression(material, unreal.MaterialExpressionConstant, -820, 980)
    presence_floor.set_editor_property("r", 0.0004)
    planar_over = expression(material, unreal.MaterialExpressionSubtract, -720, 920)
    MEL.connect_material_expressions(planar_len2, "", planar_over, "A")
    MEL.connect_material_expressions(presence_floor, "", planar_over, "B")
    presence_gain = expression(material, unreal.MaterialExpressionConstant, -720, 1000)
    presence_gain.set_editor_property("r", 40.0)
    presence_ramp = expression(material, unreal.MaterialExpressionMultiply, -640, 940)
    MEL.connect_material_expressions(planar_over, "", presence_ramp, "A")
    MEL.connect_material_expressions(presence_gain, "", presence_ramp, "B")
    presence = expression(material, unreal.MaterialExpressionSaturate, -580, 940)
    MEL.connect_material_expressions(presence_ramp, "", presence, "")

    # up = Normalize(lerp(up_default, planar_heading, presence)): both inputs
    # finite, result non-zero, so no NaN regardless of heading.
    up_blend = expression(material, unreal.MaterialExpressionLinearInterpolate, -480, 660)
    MEL.connect_material_expressions(up_default, "", up_blend, "A")
    MEL.connect_material_expressions(heading_planar, "", up_blend, "B")
    MEL.connect_material_expressions(presence, "", up_blend, "Alpha")
    up = expression(material, unreal.MaterialExpressionNormalize, -400, 660)
    MEL.connect_material_expressions(up_blend, "", up, "")
    right_cross = expression(material, unreal.MaterialExpressionCrossProduct, -400, 560)
    MEL.connect_material_expressions(up, "", right_cross, "A")
    MEL.connect_material_expressions(forward, "", right_cross, "B")
    right = expression(material, unreal.MaterialExpressionNormalize, -320, 560)
    MEL.connect_material_expressions(right_cross, "", right, "")

    offset_x = expression(material, unreal.MaterialExpressionComponentMask, -920, 340)
    offset_x.set_editor_property("r", True)
    offset_x.set_editor_property("g", False)
    offset_x.set_editor_property("b", False)
    offset_x.set_editor_property("a", False)
    MEL.connect_material_expressions(local_offset, "", offset_x, "")
    offset_y = expression(material, unreal.MaterialExpressionComponentMask, -920, 420)
    offset_y.set_editor_property("r", False)
    offset_y.set_editor_property("g", True)
    offset_y.set_editor_property("b", False)
    offset_y.set_editor_property("a", False)
    MEL.connect_material_expressions(local_offset, "", offset_y, "")

    billboard_x = expression(material, unreal.MaterialExpressionMultiply, -560, 400)
    MEL.connect_material_expressions(right, "", billboard_x, "A")
    MEL.connect_material_expressions(offset_x, "", billboard_x, "B")
    billboard_y = expression(material, unreal.MaterialExpressionMultiply, -560, 480)
    MEL.connect_material_expressions(up, "", billboard_y, "A")
    MEL.connect_material_expressions(offset_y, "", billboard_y, "B")
    aimed = expression(material, unreal.MaterialExpressionAdd, -440, 440)
    MEL.connect_material_expressions(billboard_x, "", aimed, "A")
    MEL.connect_material_expressions(billboard_y, "", aimed, "B")
    wpo = expression(material, unreal.MaterialExpressionSubtract, -320, 400)
    MEL.connect_material_expressions(aimed, "", wpo, "A")
    MEL.connect_material_expressions(local_offset, "", wpo, "B")

    MEL.connect_material_property(emissive, "", unreal.MaterialProperty.MP_EMISSIVE_COLOR)
    MEL.connect_material_property(opacity, "", unreal.MaterialProperty.MP_OPACITY)
    MEL.connect_material_property(wpo, "", unreal.MaterialProperty.MP_WORLD_POSITION_OFFSET)
    MEL.recompile_material(material)
    save_asset(f"{MATERIAL_DIR}/M_MarkerIcon")
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
    # generate_globe_mesh.py enables Nanite on SM_GlobeSphere (the Earth
    # component's mesh); a Nanite mesh's material must carry this flag or UE
    # refuses to compile the material for the Nanite codepath and silently
    # renders the flat gray engine default instead - verified by capturing
    # the live client before this flag was added (LogMaterial: "missing
    # usage flag Nanite! Default Material will be used in game").
    material.set_editor_property("used_with_nanite", True)

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

    # --- Close-orbit detail window ---
    # No global texture can serve the closest orbit: at ~43 km across the
    # frame the screen wants ~40 m per pixel, which would be a million-pixel
    # wide Earth. UGeoTileMosaic instead keeps a window of map tiles that
    # follows the camera, and hands it over as DetailMap plus the window's
    # bounds in this material's own UV space. Inside the window we sample
    # that; everywhere else the global day texture stands, unchanged.
    uv = expression(material, unreal.MaterialExpressionTextureCoordinate, -1560, -640)
    # (uMin, vMin, uSpan, vSpan). The default is the full sphere so the graph
    # is well-defined before the first region exists - DetailBlend is 0 then
    # anyway, but a zero span here would divide by zero.
    bounds = vector(material, "DetailBounds", unreal.LinearColor(0.0, 0.0, 1.0, 1.0), -1560, -560)
    # Build the two float2s from the parameter's individual channel outputs.
    # A ComponentMask cannot reach B/A here: a VectorParameter's default
    # output is float3, so masking "ba" fails the material to compile with
    # "Not enough components in (float3) for component mask 0011".
    window_min = expression(material, unreal.MaterialExpressionAppendVector, -1400, -580)
    MEL.connect_material_expressions(bounds, "R", window_min, "A")
    MEL.connect_material_expressions(bounds, "G", window_min, "B")
    window_span = expression(material, unreal.MaterialExpressionAppendVector, -1400, -500)
    MEL.connect_material_expressions(bounds, "B", window_span, "A")
    MEL.connect_material_expressions(bounds, "A", window_span, "B")
    local_uv = expression(material, unreal.MaterialExpressionSubtract, -1280, -640)
    MEL.connect_material_expressions(uv, "", local_uv, "A")
    MEL.connect_material_expressions(window_min, "", local_uv, "B")
    detail_uv = expression(material, unreal.MaterialExpressionDivide, -1160, -640)
    MEL.connect_material_expressions(local_uv, "", detail_uv, "A")
    MEL.connect_material_expressions(window_span, "", detail_uv, "B")

    detail = expression(material, unreal.MaterialExpressionTextureSampleParameter2D, -1010, -760)
    detail.set_editor_property("parameter_name", "DetailMap")
    # The default must NOT be a virtual texture. A TextureSampleParameter2D
    # samples with a plain sampler, and binding a VT to it fails the whole
    # material to compile - silently, because the failure only surfaces at
    # cook time and the packaged client then falls back to the default
    # material, which renders the globe as an unrecognisable grey mass.
    # The runtime value (the tile mosaic) is an ordinary transient texture,
    # so this placeholder only has to be a valid non-VT stand-in.
    fallback = unreal.EditorAssetLibrary.load_asset("/Engine/EngineResources/Black")
    if fallback is None or fallback.get_editor_property("virtual_texture_streaming"):
        raise RuntimeError("DetailMap needs a non-virtual fallback texture; /Engine/EngineResources/Black is unusable")
    detail.set_editor_property("texture", fallback)
    MEL.connect_material_expressions(detail_uv, "", detail, "UVs")

    # One expression settles both "is this pixel inside the window" and "fade
    # the seam": distance from the window centre is 0 at the middle, 1 at the
    # border and above 1 outside, so a saturated ramp off the border is
    # exactly the mask we want and there is no hard edge to see.
    centre_offset = expression(material, unreal.MaterialExpressionSubtract, -1010, -560)
    MEL.connect_material_expressions(detail_uv, "", centre_offset, "A")
    MEL.connect_material_expressions(constant(material, 0.5, -1160, -500), "", centre_offset, "B")
    centre_distance = expression(material, unreal.MaterialExpressionAbs, -940, -560)
    MEL.connect_material_expressions(centre_offset, "", centre_distance, "")
    distance = expression(material, unreal.MaterialExpressionMultiply, -880, -560)
    MEL.connect_material_expressions(centre_distance, "", distance, "A")
    MEL.connect_material_expressions(constant(material, 2.0, -940, -480), "", distance, "B")
    furthest_axis = expression(material, unreal.MaterialExpressionMax, -760, -560)
    MEL.connect_material_expressions(mask(material, distance, "r", -820, -600), "", furthest_axis, "A")
    MEL.connect_material_expressions(mask(material, distance, "g", -820, -520), "", furthest_axis, "B")
    inside = expression(material, unreal.MaterialExpressionOneMinus, -650, -560)
    MEL.connect_material_expressions(furthest_axis, "", inside, "")
    seam = expression(material, unreal.MaterialExpressionDivide, -540, -560)
    MEL.connect_material_expressions(inside, "", seam, "A")
    MEL.connect_material_expressions(constant(material, 0.08, -650, -490), "", seam, "B")
    seam_clamped = expression(material, unreal.MaterialExpressionSaturate, -430, -560)
    MEL.connect_material_expressions(seam, "", seam_clamped, "")

    # Driven by the actor: coverage of the current region, faded out again as
    # the globe recedes and the window stops being worth anything.
    detail_blend = scalar(material, "DetailBlend", 0.0, -540, -680)
    detail_alpha = expression(material, unreal.MaterialExpressionMultiply, -320, -600)
    MEL.connect_material_expressions(seam_clamped, "", detail_alpha, "A")
    MEL.connect_material_expressions(detail_blend, "", detail_alpha, "B")

    surface_colour = expression(material, unreal.MaterialExpressionLinearInterpolate, -180, -300)
    MEL.connect_material_expressions(day, "RGB", surface_colour, "A")
    MEL.connect_material_expressions(detail, "RGB", surface_colour, "B")
    MEL.connect_material_expressions(detail_alpha, "", surface_colour, "Alpha")

    MEL.connect_material_property(surface_colour, "", unreal.MaterialProperty.MP_BASE_COLOR)
    MEL.connect_material_property(night_emissive, "", unreal.MaterialProperty.MP_EMISSIVE_COLOR)
    MEL.connect_material_property(roughness, "", unreal.MaterialProperty.MP_ROUGHNESS)
    MEL.recompile_material(material)
    bound = MEL.get_material_default_texture_parameter_value(material, "DetailMap")
    if bound is None or bound.get_editor_property("virtual_texture_streaming"):
        raise RuntimeError(
            f"M_EarthSurface DetailMap resolved to {bound.get_name() if bound else None}, which is a virtual "
            f"texture; the material would fail to compile at cook time and the globe would render as the "
            f"default material")
    save_asset(f"{MATERIAL_DIR}/M_EarthSurface")
    return material


def build_starfield_master(texture) -> unreal.Material:
    material = ensure_material("M_Starfield")
    material.set_editor_property("blend_mode", unreal.BlendMode.BLEND_OPAQUE)
    material.set_editor_property("shading_model", unreal.MaterialShadingModel.MSM_UNLIT)
    material.set_editor_property("two_sided", True)
    # The NASA star map is stored in sky convention (RA increasing LEFTWARD,
    # left edge RA 180) — measured against LMC/SMC/galactic bulge positions.
    # Flipping U restores the map convention the sphere UV expects, so every
    # star texel lands on its true celestial direction; the actor then only
    # needs yaw = GMST for the sky to stand correctly over the earth.
    uv = expression(material, unreal.MaterialExpressionTextureCoordinate, -1000, -80)
    u_mask = expression(material, unreal.MaterialExpressionComponentMask, -880, -120)
    u_mask.set_editor_property("r", True)
    u_mask.set_editor_property("g", False)
    u_mask.set_editor_property("b", False)
    u_mask.set_editor_property("a", False)
    MEL.connect_material_expressions(uv, "", u_mask, "")
    v_mask = expression(material, unreal.MaterialExpressionComponentMask, -880, -40)
    v_mask.set_editor_property("r", False)
    v_mask.set_editor_property("g", True)
    v_mask.set_editor_property("b", False)
    v_mask.set_editor_property("a", False)
    MEL.connect_material_expressions(uv, "", v_mask, "")
    u_flip = expression(material, unreal.MaterialExpressionOneMinus, -770, -120)
    MEL.connect_material_expressions(u_mask, "", u_flip, "")
    flipped_uv = expression(material, unreal.MaterialExpressionAppendVector, -680, -90)
    MEL.connect_material_expressions(u_flip, "", flipped_uv, "A")
    MEL.connect_material_expressions(v_mask, "", flipped_uv, "B")
    sample = expression(material, unreal.MaterialExpressionTextureSample, -600, -80)
    sample.set_editor_property("texture", texture)
    MEL.connect_material_expressions(flipped_uv, "", sample, "UVs")
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

    day = import_texture(SOURCE_DIR / "NASA" / "bluemarble-21600.png", "T_EarthDay", virtual_texture=True)
    night = import_texture(SOURCE_DIR / "NASA" / "earthatnight-13500.png", "T_EarthNight", virtual_texture=True)
    clouds = import_texture(SOURCE_DIR / "NASA" / "clouds-2048.png", "T_CloudFraction")
    # Linear read: the runtime replacement (EUMETSAT IR remap) is linear gray,
    # and the sampler types of default and override must match.
    clouds.set_editor_property("srgb", False)
    save_asset(f"{TEXTURE_DIR}/T_CloudFraction")
    build_cloud_master(clouds)
    build_selected_path_master()
    build_scatter_atmosphere_master()
    # Real sky photo map (NASA SVS Deep Star Map 2020, public domain) when
    # fetched; the procedurally generated field stays as offline fallback.
    starmap_source = SOURCE_DIR / "NASA" / "starmap_2020_4k.exr"
    if not starmap_source.exists():
        starmap_source = SOURCE_DIR / "Generated" / "starfield.png"
    stars = import_texture(starmap_source, "T_Starfield")
    icons = import_texture(SOURCE_DIR / "Generated" / "marker_icons.png", "T_MarkerIcons")
    # White-on-black mask: linear values keep thin strokes from washing out.
    icons.set_editor_property("srgb", False)
    save_asset(f"{TEXTURE_DIR}/T_MarkerIcons")
    build_marker_icon_master(icons)
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
    # Motion trails (AGeoTrackLayerActor): a cool blue-cyan close to the point
    # layer's default entity-marker tint, so a trail reads as "belongs to the
    # marker it follows" rather than a specific band. Dimmer than the numbered
    # band palette and far dimmer than the click-selection highlight, so a
    # trail stays a subtle wake instead of competing with markers and arcs.
    create_instance("MI_Track", signal, unreal.LinearColor(0.05, 0.65, 1.0, 1.0), 0.5, intensity=2.4)
    # Point markers moved to the pictogram master (M_MarkerIcon); drop the
    # obsolete sphere instances so stale assets cannot be referenced again.
    for obsolete in ("MI_Point_Entity", "MI_Point_Observation"):
        obsolete_path = f"{MATERIAL_DIR}/{obsolete}"
        if unreal.EditorAssetLibrary.does_asset_exist(obsolete_path):
            unreal.EditorAssetLibrary.delete_asset(obsolete_path)
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
