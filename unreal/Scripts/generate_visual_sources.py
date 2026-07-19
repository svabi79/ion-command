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
    generate_ambience()


if __name__ == "__main__":
    main()
