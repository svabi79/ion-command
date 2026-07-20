#!/usr/bin/env python3
"""Fetch the high-resolution NASA earth mosaics and produce the power-of-two
source textures the material pipeline imports.

Day:   Blue Marble Next Generation w/ topography and bathymetry, July 2004,
       5400x2700 (visibleearth record 73751) -> bluemarble-4096.png
Night: Black Marble 2016, 3 km grid, 13500x6750 (record 144898)
       -> earthatnight-4096.png

Both are resized to 4096x2048 (UE needs power-of-two for mips; the previous
sources were 2048x1024 and visibly blurred at wall-resolution zoom levels).
"""
import pathlib
import urllib.request

from PIL import Image

Image.MAX_IMAGE_PIXELS = None

DAY_URL = "https://eoimages.gsfc.nasa.gov/images/imagerecords/73000/73751/world.topo.bathy.200407.3x5400x2700.jpg"
NIGHT_URL = "https://eoimages.gsfc.nasa.gov/images/imagerecords/144000/144898/BlackMarble_2016_3km.jpg"
CLOUD_URL = "https://eoimages.gsfc.nasa.gov/images/imagerecords/57000/57747/cloud_combined_2048.jpg"
TARGET = (4096, 2048)
CLOUD_TARGET = (2048, 1024)
OUTPUT_DIR = pathlib.Path(__file__).resolve().parent.parent / "unreal" / "SourceAssets" / "NASA"


def fetch(url: str, name: str) -> pathlib.Path:
    cache = OUTPUT_DIR / name
    if not cache.exists():
        print(f"downloading {url}")
        request = urllib.request.Request(url, headers={"User-Agent": "ion-command-tools/0.1"})
        cache.write_bytes(urllib.request.urlopen(request, timeout=300).read())
    return cache


def convert(source: pathlib.Path, output_name: str, target=TARGET) -> None:
    image = Image.open(source).convert("RGB")
    print(f"{source.name}: {image.size} -> {target}")
    image = image.resize(target, Image.LANCZOS)
    image.save(OUTPUT_DIR / output_name, optimize=True)
    print(f"wrote {OUTPUT_DIR / output_name}")


# NASA SVS Deep Star Map 2020 (public domain, Hipparcos/Tycho based celestial
# sphere). EXR on purpose: Unreal imports it natively as an HDR texture, no
# local conversion needed.
STARMAP_URL = "https://svs.gsfc.nasa.gov/vis/a000000/a004800/a004851/starmap_2020_4k.exr"


def main() -> None:
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    convert(fetch(DAY_URL, "download-bluemarble-5400.jpg"), "bluemarble-4096.png")
    convert(fetch(NIGHT_URL, "download-blackmarble-13500.jpg"), "earthatnight-4096.png")
    convert(fetch(CLOUD_URL, "download-clouds-2048.jpg"), "clouds-2048.png", CLOUD_TARGET)
    starmap = fetch(STARMAP_URL, "starmap_2020_4k.exr")
    print(f"star map ready: {starmap} ({starmap.stat().st_size // (1 << 20)} MB)")


if __name__ == "__main__":
    main()
