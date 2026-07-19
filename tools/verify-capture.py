#!/usr/bin/env python3
"""Structural assertions for First-Light captures.

Live traffic is nondeterministic, so a pixel diff against a golden image is
useless. Instead this asserts the structure every healthy capture must have:
the HUD status bar exists, the globe region is neither black nor blown out,
and the scene is actually colorful (a broken master material renders the
gray engine default - exactly the failure this check exists to catch).

Usage: python tools/verify-capture.py <capture.png>
"""
import sys

from PIL import Image


def region_stats(image, box):
    region = image.crop(box)
    pixels = list(region.getdata())
    count = len(pixels)
    brightness = sum(max(p[:3]) for p in pixels) / count
    colorfulness = sum(max(p[:3]) - min(p[:3]) for p in pixels) / count
    return brightness, colorfulness


def main() -> int:
    path = sys.argv[1]
    image = Image.open(path).convert("RGB")
    width, height = image.size
    failures = []

    if width < 1280 or height < 720:
        failures.append(f"capture is implausibly small: {width}x{height}")

    # HUD status bar strip across the top.
    bar_brightness, _ = region_stats(image, (0, 0, width, max(int(height * 0.03), 8)))
    if bar_brightness < 3:
        failures.append(f"status bar region is black (brightness {bar_brightness:.1f}) - HUD missing?")

    # Globe region: center half of the frame.
    globe_box = (int(width * 0.3), int(height * 0.15), int(width * 0.7), int(height * 0.9))
    globe_brightness, _ = region_stats(image, globe_box)
    if globe_brightness < 5:
        failures.append(f"globe region is black (brightness {globe_brightness:.1f})")
    if globe_brightness > 235:
        failures.append(f"globe region is blown out (brightness {globe_brightness:.1f})")

    # Neon fraction: share of bright, saturated pixels (the additive band
    # beams) outside the HUD margins. Calibrated against real captures:
    # healthy scenes measure 0.018-0.076 day or night; the gray
    # default-material failures measured 0.0053-0.0065. Threshold 0.010.
    scene = image.crop((int(width * 0.08), int(height * 0.04), int(width * 0.92), int(height * 0.96)))
    pixels = list(scene.getdata())
    neon = sum(1 for p in pixels if max(p) > 150 and (max(p) - min(p)) > 0.5 * max(p))
    neon_fraction = neon / len(pixels)
    if neon_fraction < 0.010:
        failures.append(f"scene has no saturated beams (neon fraction {neon_fraction:.4f}) - default material fallback?")

    # Left instrument column.
    panel_brightness, _ = region_stats(image, (0, int(height * 0.04), int(width * 0.08), int(height * 0.5)))
    if panel_brightness < 3:
        failures.append(f"instrument panel region is black (brightness {panel_brightness:.1f})")

    if failures:
        for failure in failures:
            print(f"CAPTURE FAIL: {failure}")
        return 1
    print(f"capture OK: bar {bar_brightness:.1f}, globe {globe_brightness:.1f}, neon {neon_fraction:.4f}, panels {panel_brightness:.1f}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
