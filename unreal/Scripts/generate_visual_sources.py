"""Generate deterministic, redistributable source textures for ION COMMAND."""

from pathlib import Path
import math
import random

from PIL import Image, ImageDraw, ImageFilter


WIDTH = 2048
HEIGHT = 1024
OUTPUT = Path(__file__).resolve().parents[1] / "SourceAssets" / "Generated" / "starfield.png"


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


if __name__ == "__main__":
    main()
