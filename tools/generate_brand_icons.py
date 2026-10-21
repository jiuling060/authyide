#!/usr/bin/env python3
"""Generate AuthyIDE brand assets (bear head on blue) from a single vector-ish spec.

Run with:  python3 tools/generate_brand_icons.py
Requires:  Pillow

All output sizes are rendered by drawing at 4x and downscaling for smooth edges.
"""
import os
from PIL import Image, ImageDraw

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SS = 4  # supersample factor


def _gradient(size, top, bottom):
    img = Image.new("RGB", (1, size), top)
    d = ImageDraw.Draw(img)
    for y in range(size):
        t = y / max(size - 1, 1)
        c = tuple(int(top[i] + (bottom[i] - top[i]) * t) for i in range(3))
        d.point((0, y), fill=c)
    return img.resize((size, size))


def bear_layer(size, with_bg=True):
    """Return an RGBA image of the bear head on an (optional) blue rounded square."""
    s = size * SS
    img = Image.new("RGBA", (s, s), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    blue_top = (59, 130, 246)
    blue_bottom = (29, 78, 216)
    if with_bg:
        grad = _gradient(s, blue_top, blue_bottom).convert("RGBA")
        mask = Image.new("L", (s, s), 0)
        ImageDraw.Draw(mask).rounded_rectangle([0, 0, s - 1, s - 1], radius=int(s * 0.22), fill=255)
        img.paste(grad, (0, 0), mask)

    u = s / 1024.0  # design units

    def P(*v):
        return tuple(int(round(x * u)) for x in v)

    white = (255, 255, 255, 255)
    inner_blue = (29, 78, 216, 255)
    dark = (11, 18, 32, 255)

    # ears
    for ex in (320, 704):
        d.ellipse([*P(ex - 115, 340 - 115), *P(ex + 115, 340 + 115)], fill=white)
    for ex in (320, 704):
        d.ellipse([*P(ex - 58, 340 - 58), *P(ex + 58, 340 + 58)], fill=inner_blue)
    # head
    d.ellipse([*P(212, 275), *P(812, 845)], fill=white)
    # eyes
    d.ellipse([*P(376, 466), *P(444, 534)], fill=dark)
    d.ellipse([*P(580, 466), *P(648, 534)], fill=dark)
    # snout
    d.ellipse([*P(377, 540), *P(647, 740)], fill=inner_blue)
    # nose
    d.ellipse([*P(460, 578), *P(564, 654)], fill=dark)

    return img.resize((size, size), Image.LANCZOS)


def save(img, path, size=None):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    out = img if size is None else img.resize((size, size), Image.LANCZOS)
    out.save(path)
    print("wrote", os.path.relpath(path, ROOT), out.size)


def main():
    front = os.path.join(ROOT, "frontend", "public")
    mob = os.path.join(ROOT, "mobile", "assets")
    desk = os.path.join(ROOT, "desktop", "icons")

    master = bear_layer(1024)
    # transparent-background variant (tray / adaptive foreground)
    fg = bear_layer(1024, with_bg=False)

    # web
    save(master, os.path.join(front, "logo-light.png"), 512)
    save(master, os.path.join(front, "logo-dark.png"), 512)
    save(master, os.path.join(front, "logo.png"), 512)
    save(master, os.path.join(front, "logo-colored.png"), 512)

    # mobile / expo — App Store / Play require opaque (no alpha) square icons
    opaque = Image.new("RGBA", (1024, 1024), (29, 78, 216, 255))
    opaque.alpha_composite(master)
    opaque_rgb = opaque.convert("RGB")
    save(opaque_rgb, os.path.join(mob, "icon.png"), 1024)
    save(opaque_rgb, os.path.join(mob, "icon-dark.png"), 1024)
    save(fg, os.path.join(mob, "adaptive-icon.png"), 1024)
    save(master, os.path.join(mob, "favicon.png"), 64)
    # splash: logo centered on white / dark canvas
    for name, bg in (("splash.png", (255, 255, 255)), ("splash-dark.png", (11, 11, 14))):
        canvas = Image.new("RGB", (1024, 1024), bg)
        logo = master.resize((420, 420), Image.LANCZOS)
        canvas.paste(logo, ((1024 - 420) // 2, (1024 - 420) // 2), logo)
        canvas.save(os.path.join(mob, name))
        print("wrote", os.path.relpath(os.path.join(mob, name), ROOT))

    # desktop / tauri
    for size in (32, 64, 128, 256, 512):
        save(master, os.path.join(desk, f"{size}x{size}.png"), size)
    save(master, os.path.join(desk, "128x128@2x.png"), 256)
    save(master, os.path.join(desk, "icon.png"), 512)
    save(master, os.path.join(desk, "icon-dark.png"), 512)
    save(master, os.path.join(desk, "source.png"), 1024)
    for n in (30, 44, 71, 89, 107, 142, 150, 284, 310):
        save(master, os.path.join(desk, f"Square{n}x{n}Logo.png"), n)
    save(master, os.path.join(desk, "StoreLogo.png"), 50)
    save(fg, os.path.join(desk, "tray.png"), 64)
    save(fg, os.path.join(desk, "tray-mac.png"), 64)
    # windows ico
    master.resize((256, 256), Image.LANCZOS).save(
        os.path.join(desk, "icon.ico"),
        sizes=[(16, 16), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)],
    )
    print("wrote desktop/icons/icon.ico")
    print("NOTE: desktop/icons/icon.icns must be regenerated on macOS (iconutil) or left as-is.")


if __name__ == "__main__":
    main()
