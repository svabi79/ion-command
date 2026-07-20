"""Generate deterministic, redistributable source assets for ION COMMAND:
the starfield texture and the command-deck ambience loop."""

from pathlib import Path
import math
import random
import struct
import wave

from PIL import Image, ImageDraw, ImageFilter


WIDTH = 2048
HEIGHT = 1024
OUTPUT = Path(__file__).resolve().parents[1] / "SourceAssets" / "Generated" / "starfield.png"
AMBIENCE = Path(__file__).resolve().parents[1] / "SourceAssets" / "Generated" / "deck_ambience.wav"


def generate_ambience() -> None:
    """Quiet control-room hum: low sine stack plus dozens of seeded partials.
    Every component's frequency is an integer multiple of 1/DURATION, so the
    loop is mathematically seamless."""
    rate = 44100
    duration = 12
    samples = rate * duration
    rng = random.Random(0x10C0A11D)
    partials = [(55.0, 0.16, 0.0), (110.0, 0.09, 1.3), (165.0, 0.04, 2.1)]
    for _ in range(36):
        harmonic = rng.randrange(duration * 30, duration * 400)
        frequency = harmonic / duration
        amplitude = 0.012 * rng.random() * (120.0 / (frequency + 60.0))
        partials.append((frequency, amplitude, rng.random() * math.tau))
    slow = 1.0 / duration
    frames = bytearray()
    for index in range(samples):
        t = index / rate
        breath = 0.85 + 0.15 * math.sin(math.tau * slow * t) * math.sin(math.tau * 2 * slow * t + 0.7)
        value = sum(a * math.sin(math.tau * f * t + p) for f, a, p in partials) * breath
        frames += struct.pack("<h", int(max(-0.95, min(0.95, value)) * 32767))
    AMBIENCE.parent.mkdir(parents=True, exist_ok=True)
    with wave.open(str(AMBIENCE), "wb") as output:
        output.setnchannels(1)
        output.setsampwidth(2)
        output.setframerate(rate)
        output.writeframes(bytes(frames))
    print(f"Generated {AMBIENCE}")


def main() -> None:
    random.seed(0x10C0A11D)
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    image = Image.new("RGB", (WIDTH, HEIGHT), (1, 3, 11))

    # A soft diagonal galactic band gives the command deck depth without using
    # a copyrighted sky photograph. Every point is generated from a fixed seed.
    haze = Image.new("RGBA", image.size, (0, 0, 0, 0))
    haze_draw = ImageDraw.Draw(haze, "RGBA")
    for _ in range(14000):
        x = random.randrange(WIDTH)
        center = HEIGHT * (0.52 + 0.17 * math.sin(x / WIDTH * math.tau + 0.8))
        y = int(random.gauss(center, HEIGHT * 0.075)) % HEIGHT
        alpha = random.randrange(2, 14)
        color = random.choice(((36, 62, 120, alpha), (80, 38, 118, alpha), (22, 95, 116, alpha)))
        haze_draw.point((x, y), fill=color)
    haze = haze.filter(ImageFilter.GaussianBlur(radius=16))
    image = Image.alpha_composite(image.convert("RGBA"), haze)

    glow = Image.new("RGBA", image.size, (0, 0, 0, 0))
    stars = Image.new("RGBA", image.size, (0, 0, 0, 0))
    glow_draw = ImageDraw.Draw(glow, "RGBA")
    star_draw = ImageDraw.Draw(stars, "RGBA")
    palette = ((180, 215, 255), (235, 242, 255), (255, 224, 185), (145, 185, 255))
    for index in range(5200):
        x = random.randrange(WIDTH)
        y = random.randrange(HEIGHT)
        color = random.choice(palette)
        brightness = random.randrange(100, 256)
        if index < 90:
            radius = random.choice((2, 2, 3))
            glow_draw.ellipse((x - radius * 4, y - radius * 4, x + radius * 4, y + radius * 4), fill=(*color, 70))
            star_draw.ellipse((x - radius, y - radius, x + radius, y + radius), fill=(*color, brightness))
            star_draw.line((x - radius * 3, y, x + radius * 3, y), fill=(*color, brightness // 2), width=1)
            star_draw.line((x, y - radius * 3, x, y + radius * 3), fill=(*color, brightness // 2), width=1)
        else:
            star_draw.point((x, y), fill=(*color, brightness))
    glow = glow.filter(ImageFilter.GaussianBlur(radius=7))
    image = Image.alpha_composite(image, glow)
    image = Image.alpha_composite(image, stars)
    image.convert("RGB").save(OUTPUT, optimize=True)
    print(f"Generated {OUTPUT}")
    generate_marker_icons()
    generate_ambience()


ICON_ATLAS = Path(__file__).resolve().parents[1] / "SourceAssets" / "Generated" / "marker_icons.png"
ICON_TILE = 512
ICON_COLS = 4
ICON_ROWS = 4


def _icon_tile(draw_fn) -> Image.Image:
    """Render one atlas tile at 4x and downscale for crisp anti-aliased edges.
    Icons are white silhouettes on black; the material reads R as the mask."""
    scale = 4
    size = ICON_TILE * scale
    tile = Image.new("L", (size, size), 0)
    draw = ImageDraw.Draw(tile)

    def pt(x, y):
        return (x * size, y * size)

    draw_fn(draw, pt, size)
    return tile.resize((ICON_TILE, ICON_TILE), Image.LANCZOS)


def _icon_dot(draw, pt, size):
    draw.ellipse([pt(0.24, 0.24), pt(0.76, 0.76)], fill=255)


def _icon_signal(draw, pt, size):
    # Antenna mast with radiating wave arcs: the generic "transmitter" glyph.
    draw.polygon([pt(0.5, 0.16), pt(0.40, 0.86), pt(0.60, 0.86)], fill=255)
    draw.ellipse([pt(0.44, 0.10), pt(0.56, 0.22)], fill=255)
    stroke = int(size * 0.045)
    for radius in (0.16, 0.28):
        box = [pt(0.5 - radius, 0.16 - radius), pt(0.5 + radius, 0.16 + radius)]
        draw.arc(box, 205, 275, fill=255, width=stroke)
        draw.arc(box, 265, 335, fill=255, width=stroke)


def _icon_aircraft(draw, pt, size):
    # Top-view airliner silhouette.
    draw.polygon(
        [
            pt(0.50, 0.06), pt(0.545, 0.18), pt(0.55, 0.34),
            pt(0.94, 0.55), pt(0.94, 0.63), pt(0.55, 0.52),
            pt(0.54, 0.72), pt(0.68, 0.83), pt(0.68, 0.90), pt(0.50, 0.85),
            pt(0.32, 0.90), pt(0.32, 0.83), pt(0.46, 0.72),
            pt(0.45, 0.52), pt(0.06, 0.63), pt(0.06, 0.55), pt(0.45, 0.34),
            pt(0.455, 0.18),
        ],
        fill=255,
    )


def _icon_satellite(draw, pt, size):
    # Bus with two solar wings on connector booms.
    draw.rectangle([pt(0.40, 0.38), pt(0.60, 0.62)], fill=255)
    stroke = int(size * 0.03)
    draw.line([pt(0.30, 0.5), pt(0.40, 0.5)], fill=255, width=stroke)
    draw.line([pt(0.60, 0.5), pt(0.70, 0.5)], fill=255, width=stroke)
    for x0, x1 in ((0.06, 0.30), (0.70, 0.94)):
        draw.rectangle([pt(x0, 0.40), pt(x1, 0.60)], fill=255)
        # panel cell separators punched back out for texture
        for f in (1 / 3, 2 / 3):
            x = x0 + (x1 - x0) * f
            draw.line([pt(x, 0.40), pt(x, 0.60)], fill=0, width=int(size * 0.018))
    draw.arc([pt(0.42, 0.18), pt(0.58, 0.34)], 200, 340, fill=255, width=stroke)


def _icon_lightning(draw, pt, size):
    draw.polygon(
        [pt(0.60, 0.06), pt(0.33, 0.52), pt(0.49, 0.52), pt(0.38, 0.94), pt(0.70, 0.42), pt(0.53, 0.42)],
        fill=255,
    )


def _icon_sounding(draw, pt, size):
    # Ionosonde: echo arcs stacked above a ground station wedge.
    stroke = int(size * 0.05)
    for radius in (0.18, 0.32, 0.46):
        box = [pt(0.5 - radius, 0.82 - radius), pt(0.5 + radius, 0.82 + radius)]
        draw.arc(box, 220, 320, fill=255, width=stroke)
    draw.polygon([pt(0.5, 0.70), pt(0.42, 0.90), pt(0.58, 0.90)], fill=255)


def _icon_earthquake(draw, pt, size):
    # Seismogram trace inside a ring.
    stroke = int(size * 0.045)
    draw.arc([pt(0.08, 0.08), pt(0.92, 0.92)], 0, 360, fill=255, width=stroke)
    trace = [
        pt(0.18, 0.5), pt(0.34, 0.5), pt(0.42, 0.28), pt(0.52, 0.74),
        pt(0.60, 0.36), pt(0.66, 0.5), pt(0.82, 0.5),
    ]
    draw.line(trace, fill=255, width=stroke, joint="curve")


def _icon_spare(draw, pt, size):
    draw.polygon([pt(0.5, 0.14), pt(0.80, 0.5), pt(0.5, 0.86), pt(0.20, 0.5)], fill=255)


def _icon_helicopter(draw, pt, size):
    # Top view, nose up: rotor disc with crossed blades over a slim
    # fuselage and tail boom with tail rotor bar.
    stroke = int(size * 0.045)
    draw.ellipse([pt(0.18, 0.10), pt(0.82, 0.74)], outline=255, width=int(size * 0.03))
    draw.line([pt(0.24, 0.16), pt(0.76, 0.68)], fill=255, width=stroke)
    draw.line([pt(0.76, 0.16), pt(0.24, 0.68)], fill=255, width=stroke)
    draw.ellipse([pt(0.42, 0.26), pt(0.58, 0.60)], fill=255)
    draw.line([pt(0.5, 0.58), pt(0.5, 0.92)], fill=255, width=stroke)
    draw.line([pt(0.38, 0.90), pt(0.62, 0.90)], fill=255, width=stroke)


def _icon_balloon(draw, pt, size):
    # Envelope with gores, load lines, and a basket.
    draw.ellipse([pt(0.22, 0.06), pt(0.78, 0.62)], fill=255)
    for x in (0.38, 0.5, 0.62):
        draw.line([pt(x, 0.08), pt(x, 0.60)], fill=0, width=int(size * 0.018))
    draw.polygon([pt(0.34, 0.52), pt(0.66, 0.52), pt(0.58, 0.72), pt(0.42, 0.72)], fill=255)
    stroke = int(size * 0.03)
    draw.line([pt(0.42, 0.72), pt(0.44, 0.82)], fill=255, width=stroke)
    draw.line([pt(0.58, 0.72), pt(0.56, 0.82)], fill=255, width=stroke)
    draw.rectangle([pt(0.42, 0.82), pt(0.58, 0.94)], fill=255)


def _icon_drone(draw, pt, size):
    # Quadcopter: X arms, four rotor rings, center body.
    stroke = int(size * 0.05)
    draw.line([pt(0.22, 0.22), pt(0.78, 0.78)], fill=255, width=stroke)
    draw.line([pt(0.78, 0.22), pt(0.22, 0.78)], fill=255, width=stroke)
    ring = int(size * 0.035)
    for cx, cy in ((0.22, 0.22), (0.78, 0.22), (0.22, 0.78), (0.78, 0.78)):
        draw.ellipse([pt(cx - 0.14, cy - 0.14), pt(cx + 0.14, cy + 0.14)], outline=255, width=ring)
    draw.ellipse([pt(0.38, 0.38), pt(0.62, 0.62)], fill=255)


def _icon_glider(draw, pt, size):
    # Top view, nose up: very long thin wings, slim fuselage, small tail.
    draw.polygon([pt(0.5, 0.10), pt(0.545, 0.30), pt(0.53, 0.72), pt(0.47, 0.72), pt(0.455, 0.30)], fill=255)
    draw.polygon([pt(0.04, 0.36), pt(0.96, 0.36), pt(0.96, 0.43), pt(0.04, 0.45)], fill=255)
    draw.polygon([pt(0.34, 0.80), pt(0.66, 0.80), pt(0.66, 0.87), pt(0.34, 0.87)], fill=255)


def generate_marker_icons() -> None:
    """Marker pictogram atlas: 4x4 grid of white-on-black silhouettes. Tile
    order is the client's icon index contract (GeoPointLayerActor):
    0 dot, 1 signal, 2 aircraft, 3 satellite, 4 lightning, 5 sounding,
    6 earthquake, 7 spare, 8 helicopter, 9 balloon, 10 drone, 11 glider;
    12-15 reserved."""
    tiles = [
        _icon_dot, _icon_signal, _icon_aircraft, _icon_satellite,
        _icon_lightning, _icon_sounding, _icon_earthquake, _icon_spare,
        _icon_helicopter, _icon_balloon, _icon_drone, _icon_glider,
    ]
    atlas = Image.new("RGB", (ICON_TILE * ICON_COLS, ICON_TILE * ICON_ROWS), (0, 0, 0))
    for index, draw_fn in enumerate(tiles):
        tile = _icon_tile(draw_fn)
        x = (index % ICON_COLS) * ICON_TILE
        y = (index // ICON_COLS) * ICON_TILE
        atlas.paste(Image.merge("RGB", (tile, tile, tile)), (x, y))
    ICON_ATLAS.parent.mkdir(parents=True, exist_ok=True)
    atlas.save(ICON_ATLAS)
    print(f"Generated {ICON_ATLAS}")


if __name__ == "__main__":
    main()
