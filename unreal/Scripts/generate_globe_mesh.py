"""Generate the high-tessellation Earth sphere mesh reproducibly, from
Unreal's GeometryScript primitive generator (the "GeometryScripting" plugin,
enabled in IonCommand.uproject). Replaces the coarse engine BasicShapes
sphere (12 latitude rings x 24 longitude segments -- visibly faceted, both
in silhouette and in the lit terrain surface, once the camera is close) with
a mesh fine enough to stay smooth at the camera's ~32 km minimum altitude.

Radius is 50, matching the engine primitive's own convention, so this is a
drop-in replacement at whatever scale factor a StaticMeshComponent already
applies (AIonGlobeActor scales its Earth component by 20x for a 1000-unit
globe; the same asset could replace the primitive anywhere else at its
existing scale, though today only the Earth component is switched over --
see the module docstring notes in IonGlobeActor.cpp).

Run standalone or via tools/run-editor.ps1, before create_material_instances
.py and create_bootstrap_level.py (the material script's build_earth_master
does not reference this mesh, but IonGlobeActor's C++ constructor resolves
the hard object reference to this asset path as soon as anything spawns or
loads the class's CDO, which create_bootstrap_level.py does)."""
import math
import sys
from pathlib import Path

import unreal

sys.path.insert(0, str(Path(__file__).resolve().parent))

from _ion_common import ensure_directory, save_asset


MESH_DIR = "/Game/ION/Meshes"
MESH_NAME = "SM_GlobeSphere"
RADIUS = 50.0
# 400 longitude segments x 200 latitude rings. After the Earth component's
# 20x scale (1000-unit globe = Earth's real radius), that is one triangle
# edge roughly every 0.9 degrees in both directions -- well under the
# ~2 km/pixel day texture's own resolution anywhere on the sphere, so the
# mesh is never the visible limiting factor. ~160k triangles (2 per quad,
# minus the pole fans): trivial for a single static mesh even without
# Nanite, which this script also enables (best-effort; see below) since the
# project already turns Nanite on (r.Nanite.ProjectEnabled=True).
STEPS_THETA = 400  # longitude, full circle
STEPS_PHI = 200  # latitude, pole to pole


def build_sphere() -> unreal.DynamicMesh:
    dynamic_mesh = unreal.DynamicMesh()
    options = unreal.GeometryScriptPrimitiveOptions()
    unreal.GeometryScript_Primitives.append_sphere_lat_long(
        dynamic_mesh, options, unreal.Transform(), RADIUS, STEPS_PHI, STEPS_THETA)
    # AppendSphereLatLong's own U convention sits exactly 90 degrees of
    # longitude (u=0.25) east of the engine BasicShapes primitive's
    # convention that the existing Earth/Cloud/Atmosphere materials were
    # authored against -- measured empirically by comparing this script's
    # own UV probe (below) with probe_sphere_uv.py's output against
    # /Engine/BasicShapes/Sphere.Sphere. V (latitude) already matches
    # exactly, no correction needed there. Shifting U back here, once, means
    # this mesh is a true drop-in replacement and no material needs to know
    # or care which primitive generator built the mesh it is sampling.
    unreal.GeometryScript_UVs.translate_mesh_u_vs(
        dynamic_mesh, 0, unreal.Vector2D(-0.25, 0.0), unreal.GeometryScriptMeshSelection())
    return dynamic_mesh


def verify_and_probe(static_mesh: unreal.StaticMesh) -> None:
    """No visual rendering is available in this environment, so this is the
    evidence that stands in for it: outward-facing normals (a flipped
    winding would light the globe as if hollow, from the inside), a sane
    triangle count, and the same UV probe probe_sphere_uv.py runs against
    the engine primitive, so the two conventions can be diffed from the log
    before wiring this mesh into IonGlobeActor."""
    vertices, triangles, normals, uvs, tangents = unreal.ProceduralMeshLibrary.get_section_from_static_mesh(static_mesh, 0, 0)
    if not vertices:
        raise RuntimeError("Generated globe mesh has no vertices in LOD0 section 0")
    triangle_count = len(triangles) // 3
    unreal.log(f"ION COMMAND globe mesh: {len(vertices)} vertices, {triangle_count} triangles")
    if triangle_count < 100000:
        raise RuntimeError(f"Generated globe mesh only has {triangle_count} triangles; expected roughly {STEPS_THETA * STEPS_PHI * 2}")

    stride = max(1, len(vertices) // 400)
    dots = []
    for index in range(0, len(vertices), stride):
        vertex = vertices[index]
        normal = normals[index]
        length = math.sqrt(vertex.x ** 2 + vertex.y ** 2 + vertex.z ** 2)
        if length < 1e-6:
            continue
        dots.append((vertex.x / length) * normal.x + (vertex.y / length) * normal.y + (vertex.z / length) * normal.z)
    average_dot = sum(dots) / len(dots)
    unreal.log(f"ION COMMAND globe mesh: average outward-normal dot over {len(dots)} samples = {average_dot:.4f} (+1.0 = correct outward winding, -1.0 = inverted)")
    if average_dot < 0.9:
        raise RuntimeError(f"Generated globe mesh normals do not point outward (average dot {average_dot:.4f}); the sphere winding is inverted")

    # Same technique as probe_sphere_uv.py, "_NEW" tag so the two are easy
    # to tell apart in one log, PLUS a hard check against the exact formula
    # measured from probe_sphere_uv.py's baseline against the engine
    # primitive (lon = -90 - 360*u, i.e. u = ((-90-lon)/360) mod 1; v =
    # (90-lat)/180), so a regression here fails the build instead of
    # requiring a human to notice a changed log.
    radius = max(math.sqrt(v.x * v.x + v.y * v.y + v.z * v.z) for v in vertices)
    equator_band = max(2.0, radius * 0.02)
    samples = []
    u_errors = []
    for index, vertex in enumerate(vertices):
        if abs(vertex.z) < equator_band:
            lon = math.degrees(math.atan2(vertex.y, vertex.x))
            u = uvs[index].x
            samples.append((u, lon))
            expected_u = ((-90.0 - lon) / 360.0) % 1.0
            u_errors.append(min(abs(u - expected_u), 1.0 - abs(u - expected_u)))
    samples.sort()
    last_u = None
    for u, lon in samples:
        if last_u is None or u - last_u > 0.04:
            unreal.log(f"UVPROBE_NEW u={u:.4f} lon={lon:8.2f}")
            last_u = u
    max_u_error = max(u_errors)
    unreal.log(f"ION COMMAND globe mesh: max U error vs the probe_sphere_uv.py baseline formula = {max_u_error:.4f}")
    if max_u_error > 0.01:
        raise RuntimeError(
            f"Generated globe mesh U does not match the project's longitude convention "
            f"(max error {max_u_error:.4f}); the Earth/Cloud/Atmosphere textures would be "
            f"misaligned. Re-check the translate_mesh_u_vs offset in build_sphere().")

    meridian_band = max(2.0, radius * 0.02)
    lat_samples = []
    v_errors = []
    for index, vertex in enumerate(vertices):
        if abs(vertex.x) < meridian_band and vertex.y > 0:
            lat = math.degrees(math.asin(max(-1.0, min(1.0, vertex.z / radius))))
            v = uvs[index].y
            lat_samples.append((lat, v))
            expected_v = (90.0 - lat) / 180.0
            v_errors.append(abs(v - expected_v))
    lat_samples.sort()
    last_lat = None
    for lat, v in lat_samples:
        if last_lat is None or lat - last_lat > 3.0:
            unreal.log(f"UVPROBE_NEW lat={lat:7.2f} v={v:.4f}")
            last_lat = lat
    max_v_error = max(v_errors)
    unreal.log(f"ION COMMAND globe mesh: max V error vs the probe_sphere_uv.py baseline formula = {max_v_error:.4f}")
    if max_v_error > 0.01:
        raise RuntimeError(
            f"Generated globe mesh V does not match the project's latitude convention "
            f"(max error {max_v_error:.4f}); the Earth/Cloud/Atmosphere textures would be "
            f"misaligned top-to-bottom.")


def main() -> None:
    ensure_directory(MESH_DIR)
    asset_path = f"{MESH_DIR}/{MESH_NAME}"

    # Release any level reference to this asset before rebuild, same
    # constraint create_material_instances.py works around: replacing an
    # asset referenced by a loaded level can assert in UE 5.8.
    unreal.EditorLevelLibrary.new_level("/Game/ION/Maps/L_TransientMeshRebuild")
    unreal.SystemLibrary.collect_garbage()

    if unreal.EditorAssetLibrary.does_asset_exist(asset_path):
        if not unreal.EditorAssetLibrary.delete_asset(asset_path):
            raise RuntimeError(f"Could not delete {asset_path} for rebuild")

    dynamic_mesh = build_sphere()

    options = unreal.GeometryScriptCreateNewStaticMeshAssetOptions()
    # No collision: the camera never collision-tests (bDoCollisionTest =
    # false on the spring arm) and nothing else raycasts against the globe
    # mesh itself (path/marker picking is a bounded CPU ray-to-segment query
    # against the layer data, not physics collision) -- so skip generating
    # a 160k-triangle collision mesh nobody queries.
    options.set_editor_property("enable_collision", False)
    options.set_editor_property("enable_recompute_normals", True)
    options.set_editor_property("enable_recompute_tangents", True)
    # The project already turns Nanite on project-wide
    # (r.Nanite.ProjectEnabled=True); use it here too so this mesh is
    # virtualized like everything else, though a single ~160k-triangle
    # static mesh would render fine either way.
    options.set_editor_property("enable_nanite", True)

    static_mesh, outcome = unreal.GeometryScript_NewAssetUtils.create_new_static_mesh_asset_from_mesh(
        dynamic_mesh, asset_path, options)
    unreal.log(f"ION COMMAND globe mesh: create_new_static_mesh_asset_from_mesh outcome={outcome}")
    if outcome != unreal.GeometryScriptOutcomePins.SUCCESS or static_mesh is None:
        raise RuntimeError(f"create_new_static_mesh_asset_from_mesh reported failure (outcome={outcome})")

    # Verify the actual geometry BEFORE touching Nanite: once Nanite is
    # enabled and the asset is saved (built), get_section_from_static_mesh
    # reads back the small Nanite fallback/proxy mesh instead of the full
    # source geometry (verified: triangle count drops from ~158k to ~256),
    # so this check would silently stop meaning anything if it ran after.
    # Nanite itself still renders the full-detail source regardless -- only
    # this Python-side introspection is affected.
    verify_and_probe(static_mesh)

    # CreateNewStaticMeshAssetOptions.enable_nanite (set above) does not
    # actually flip NaniteSettings.bEnabled on the resulting asset in this
    # engine version (verified: reads back False even though the option was
    # set) -- setting nanite_settings directly on the created asset does
    # take effect and survives a save/reload, so that is the real mechanism.
    nanite_settings = static_mesh.get_editor_property("nanite_settings")
    nanite_settings.set_editor_property("enabled", True)
    static_mesh.set_editor_property("nanite_settings", nanite_settings)
    static_mesh.modify()
    nanite_enabled = static_mesh.get_editor_property("nanite_settings").get_editor_property("enabled")
    unreal.log(f"ION COMMAND globe mesh: Nanite enabled on asset = {nanite_enabled}")
    if not nanite_enabled:
        unreal.log_warning("ION COMMAND globe mesh: Nanite could not be enabled; mesh is still built and usable without it (a single ~160k-triangle static mesh renders fine either way)")

    save_asset(asset_path)

    if unreal.EditorAssetLibrary.does_asset_exist("/Game/ION/Maps/L_CommandDeck"):
        unreal.EditorLevelLibrary.load_level("/Game/ION/Maps/L_CommandDeck")
    if unreal.EditorAssetLibrary.does_asset_exist("/Game/ION/Maps/L_TransientMeshRebuild"):
        unreal.EditorAssetLibrary.delete_asset("/Game/ION/Maps/L_TransientMeshRebuild")
    unreal.log(f"ION COMMAND: globe mesh ready at {asset_path}")


if __name__ == "__main__":
    main()
