"""Measure a sphere mesh's UV convention against the project's
latitude/longitude world math: for equator vertices, print UV.u versus the
world longitude atan2(Y, X); for a meridian slice, print UV.v versus
latitude asin(Z / R). Read-only diagnostic.

Edit MESH_PATH below to probe a different mesh, e.g. to compare a newly
generated globe mesh against this same baseline for the engine primitive.
generate_globe_mesh.py runs the identical probe against its own output in
the same run, printing UVPROBE_NEW instead, so the two are easy to diff
from one log."""
import math

import unreal

MESH_PATH = "/Engine/BasicShapes/Sphere.Sphere"

mesh = unreal.load_asset(MESH_PATH)
vertices, triangles, normals, uvs, tangents = unreal.ProceduralMeshLibrary.get_section_from_static_mesh(mesh, 0, 0)
unreal.log(f"UVPROBE mesh={MESH_PATH} vertices={len(vertices)} uvs={len(uvs)}")

radius = max(math.sqrt(v.x * v.x + v.y * v.y + v.z * v.z) for v in vertices)
equator_band = max(2.0, radius * 0.02)

# U vs longitude, sampled near the equator.
samples = []
for index, vertex in enumerate(vertices):
    if abs(vertex.z) < equator_band:
        lon = math.degrees(math.atan2(vertex.y, vertex.x))
        samples.append((uvs[index].x, lon))
samples.sort()
last_u = None
for u, lon in samples:
    if last_u is None or u - last_u > 0.04:
        unreal.log(f"UVPROBE u={u:.4f} lon={lon:8.2f}")
        last_u = u

# V vs latitude, sampled near the longitude=90 meridian (x near 0, y > 0).
meridian_band = max(2.0, radius * 0.02)
lat_samples = []
for index, vertex in enumerate(vertices):
    if abs(vertex.x) < meridian_band and vertex.y > 0:
        lat = math.degrees(math.asin(max(-1.0, min(1.0, vertex.z / radius))))
        lat_samples.append((lat, uvs[index].y))
lat_samples.sort()
last_lat = None
for lat, v in lat_samples:
    if last_lat is None or lat - last_lat > 3.0:
        unreal.log(f"UVPROBE lat={lat:7.2f} v={v:.4f}")
        last_lat = lat
