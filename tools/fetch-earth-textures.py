#!/usr/bin/env python3
"""Fetch the highest-resolution freely licensed NASA earth mosaics and
produce the source textures the material pipeline imports.

Day:   Blue Marble Next Generation w/ topography and bathymetry, July 2004
       (visibleearth record 73751). NASA's largest single-file release is
       21600x10800 -- roughly 2 km/pixel at the equator, not the 500 m/pixel
       figure sometimes quoted for this dataset, which only exists as an
       eight-tile 21600x21600-per-tile set (86400x43200 assembled). See
       "About the true 500 m/pixel mosaic" below for why this script does
       not fetch that set. -> bluemarble-21600.png

Night: Black Marble 2016, native 3 km grid, 13500x6750 (record 144898,
       Suomi NPP VIIRS). This is NASA's highest-resolution single-file
       release of this composite. -> earthatnight-13500.png

Clouds: Blue Marble cloud climatology (record 57747), native 2048x1024;
       this is the ceiling for this specific composite -- no higher-
       resolution file exists at the same source. -> clouds-2048.png

All three keep their native pixel dimensions (no downscaling). They are
imported as Streaming Virtual Textures (create_material_instances.py), so
unlike the old 4096x2048 versions there is no need to downsize to a
power-of-two: SVT tiles arbitrary source dimensions itself, and only the
mip/tile actually on screen is ever resident in VRAM.

About the true 500 m/pixel mosaic:
NASA also publishes the July 2004 Blue Marble at native ~500 m/pixel
resolution, as eight 21600x21600 JPEG tiles (A1/A2/B1/B2/C1/C2/D1/D2,
i.e. an 86400x43200 mosaic once assembled -- confirmed reachable at the
URL pattern below). This script deliberately does not fetch or assemble
it: doing so needs the full canvas resident in Pillow at once, roughly
11 GB of raw RGB, and this script is meant to run on a shared build
machine alongside other Unreal builds. A maintainer with a dedicated
machine (or who extends this to downsample each tile before pasting it
into a smaller canvas, trading true 500 m/pixel detail for a bounded
memory footprint) can fetch them from:
  https://eoimages.gsfc.nasa.gov/images/imagerecords/73000/73751/world.topo.bathy.200407.3x21600x21600.<TILE>.jpg
  for <TILE> in A1 A2 B1 B2 C1 C2 D1 D2
"""
import pathlib
import urllib.request

from PIL import Image

Image.MAX_IMAGE_PIXELS = None

DAY_URL = "https://eoimages.gsfc.nasa.gov/images/imagerecords/73000/73751/world.topo.bathy.200407.3x21600x10800.jpg"
NIGHT_URL = "https://eoimages.gsfc.nasa.gov/images/imagerecords/144000/144898/BlackMarble_2016_3km.jpg"
CLOUD_URL = "https://eoimages.gsfc.nasa.gov/images/imagerecords/57000/57747/cloud_combined_2048.jpg"
STARMAP_URL = "https://svs.gsfc.nasa.gov/vis/a000000/a004800/a004851/starmap_2020_4k.exr"
OUTPUT_DIR = pathlib.Path(__file__).resolve().parent.parent / "unreal" / "SourceAssets" / "NASA"


def fetch(url: str, name: str) -> pathlib.Path:
    cache = OUTPUT_DIR / name
    if not cache.exists():
        print(f"downloading {url}")
        request = urllib.request.Request(url, headers={"User-Agent": "ion-command-tools/0.1"})
        with urllib.request.urlopen(request, timeout=300) as response:
            cache.write_bytes(response.read())
    print(f"{cache.name}: {cache.stat().st_size / (1 << 20):.1f} MB on disk")
    return cache


def convert(source: pathlib.Path, output_name: str, expected_size: tuple[int, int]) -> pathlib.Path:
    """Decode the downloaded JPEG and re-save as PNG at its native size.
    Only resizes if NASA ever changes what is served at this URL -- the
    expected size is an assertion, not a target."""
    image = Image.open(source).convert("RGB")
    if image.size != expected_size:
        print(f"WARNING: {source.name} is {image.size}, expected {expected_size}; "
              f"NASA may have changed this file. Resizing to the expected size.")
        image = image.resize(expected_size, Image.LANCZOS)
    output_path = OUTPUT_DIR / output_name
    print(f"{source.name}: {image.size} -> {output_path.name}")
    image.save(output_path, optimize=True)
    print(f"wrote {output_path} ({output_path.stat().st_size / (1 << 20):.1f} MB)")
    return output_path


def main() -> None:
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    outputs = [
        convert(fetch(DAY_URL, "download-bluemarble-21600.jpg"), "bluemarble-21600.png", (21600, 10800)),
        convert(fetch(NIGHT_URL, "download-blackmarble-13500.jpg"), "earthatnight-13500.png", (13500, 6750)),
        convert(fetch(CLOUD_URL, "download-clouds-2048.jpg"), "clouds-2048.png", (2048, 1024)),
    ]
    starmap = fetch(STARMAP_URL, "starmap_2020_4k.exr")

    downloaded = [OUTPUT_DIR / n for n in ("download-bluemarble-21600.jpg", "download-blackmarble-13500.jpg",
                                            "download-clouds-2048.jpg")] + [starmap]
    total_download_mb = sum(p.stat().st_size for p in downloaded) / (1 << 20)
    total_output_mb = sum(p.stat().st_size for p in outputs) / (1 << 20)
    print(f"total raw download volume: {total_download_mb:.1f} MB")
    print(f"total converted PNG output volume: {total_output_mb:.1f} MB (plus the {starmap.stat().st_size / (1 << 20):.1f} MB EXR star map, imported unconverted)")


if __name__ == "__main__":
    main()
