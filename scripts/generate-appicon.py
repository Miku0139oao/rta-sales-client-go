"""Generate the Windows app icon from the desktop brand colors."""

from __future__ import annotations

import struct
from io import BytesIO
from pathlib import Path

from PIL import Image, ImageDraw

ROOT = Path(__file__).resolve().parents[1]
PNG_PATH = ROOT / "cmd" / "rta-excel-filler" / "build" / "appicon.png"
ICO_PATH = ROOT / "cmd" / "rta-excel-filler" / "build" / "windows" / "icon.ico"

START = (21, 149, 137, 255)  # #159589
END = (15, 107, 100, 255)  # #0f6b64
GLYPH = (255, 255, 255, 255)
ICO_SIZES = ((16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256))


def lerp(start: int, end: int, t: float) -> int:
    return int(round(start + (end - start) * t))


def rounded_mask(size: int, radius: int) -> Image.Image:
    mask = Image.new("L", (size, size), 0)
    ImageDraw.Draw(mask).rounded_rectangle((0, 0, size - 1, size - 1), radius=radius, fill=255)
    return mask


def paint_tile(size: int) -> Image.Image:
    tile = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    pixels = tile.load()
    for y in range(size):
        for x in range(size):
            t = (x + y) / (2 * (size - 1))
            pixels[x, y] = (
                lerp(START[0], END[0], t),
                lerp(START[1], END[1], t),
                lerp(START[2], END[2], t),
                255,
            )
    return tile


def draw_bars(image: Image.Image, origin: int, inner: int) -> None:
    draw = ImageDraw.Draw(image)
    if inner <= 20:
        bar_width, gap = 3, 1
        heights = (4, 6, 9)
        radius = 1
        baseline = origin + inner - 2
        left = origin + max(1, (inner - (bar_width * 3 + gap * 2)) // 2)
    else:
        content = inner * 0.62
        bar_width = inner * 0.13
        gap = max(1.0, inner * 0.055)
        heights = (content * 0.42, content * 0.68, content)
        radius = max(1, min(bar_width * 0.42, bar_width / 2))
        baseline = origin + inner * 0.78
        left = origin + (inner - (bar_width * 3 + gap * 2)) / 2
    for index, height in enumerate(heights):
        x0 = left + index * (bar_width + gap)
        y0 = baseline - height
        box = (round(x0), round(y0), round(x0 + bar_width) - 1, round(baseline) - 1)
        draw.rounded_rectangle(box, radius=radius, fill=GLYPH)


def render(size: int) -> Image.Image:
    image = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    inset = max(1, round(size * 0.06))
    inner = size - inset * 2
    radius = max(2, round(inner * 0.33))
    tile = paint_tile(inner).convert("RGBA")
    tile.putalpha(rounded_mask(inner, radius))
    image.alpha_composite(tile, (inset, inset))
    draw_bars(image, inset, inner)
    return image


def write_ico(path: Path, frames: list[Image.Image]) -> None:
    payloads: list[tuple[int, int, bytes]] = []
    for frame in frames:
        buffer = BytesIO()
        frame.save(buffer, format="PNG")
        payloads.append((frame.size[0], frame.size[1], buffer.getvalue()))

    offset = 6 + 16 * len(payloads)
    entries = bytearray()
    data = bytearray()
    for width, height, payload in payloads:
        entries.extend(
            struct.pack(
                "<BBBBHHII",
                0 if width >= 256 else width,
                0 if height >= 256 else height,
                0,
                0,
                1,
                32,
                len(payload),
                offset,
            )
        )
        data.extend(payload)
        offset += len(payload)
    path.write_bytes(b"\x00\x00\x01\x00" + struct.pack("<H", len(payloads)) + entries + data)


def main() -> None:
    master = render(1024)
    frames = [render(size) for size, _ in ICO_SIZES]
    PNG_PATH.parent.mkdir(parents=True, exist_ok=True)
    ICO_PATH.parent.mkdir(parents=True, exist_ok=True)
    master.save(PNG_PATH, format="PNG")
    write_ico(ICO_PATH, frames)
    with Image.open(ICO_PATH) as icon:
        print(f"wrote {PNG_PATH} {master.size} {master.mode}")
        print(f"wrote {ICO_PATH} frames={getattr(icon, 'n_frames', 1)}")


if __name__ == "__main__":
    main()
