"""Measure the engine BasicShapes sphere UV convention against the project's
latitude/longitude world math: for equator vertices, print UV.u versus the
world longitude atan2(Y, X). Read-only diagnostic."""
import math

import unreal

mesh = unreal.load_asset("/Engine/BasicShapes/Sphere.Sphere")
vertices, triangles, normals, uvs, tangents = unreal.ProceduralMeshLibrary.get_section_from_static_mesh(mesh, 0, 0)
unreal.log(f"UVPROBE vertices={len(vertices)} uvs={len(uvs)}")
samples = []
for index, vertex in enumerate(vertices):
    if abs(vertex.z) < 2.0:  # near the equator of the 100-unit sphere
        lon = math.degrees(math.atan2(vertex.y, vertex.x))
        samples.append((uvs[index].x, lon))
samples.sort()
last_u = None
for u, lon in samples:
    if last_u is None or u - last_u > 0.04:
        unreal.log(f"UVPROBE u={u:.4f} lon={lon:8.2f}")
        last_u = u
