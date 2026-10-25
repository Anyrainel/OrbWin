"""Generate the OrbWin logo assets from one geometry definition.

Writes assets/orbwin.svg, assets/orbwin-mono.svg and the tracked raster icons in
src-tauri/icons (icon.png, icon.ico, 32x32, 64x64, 128x128, 128x128@2x).
Requires Pillow and numpy. Run from the repository root:

    python scripts/generate-icons.py

The mark is a sphere cut by a window cross (gap 14, pane corner radius 12) on a
plum tile; the top-right pane is lit. Geometry lives in the constants below and
is mirrored by hand in the SVG strings, so change both together.
"""
from pathlib import Path

import numpy as np
from PIL import Image, ImageDraw

ROOT = Path(__file__).resolve().parent.parent
GAP, RADIUS, ORB_R, TILE_R, C = 14, 12, 88, 56, 128
H = GAP / 2

ORB = ((0xF0, 0x6B, 0xFF), (0xB9, 0x1A, 0xCD), (0x6A, 0x0F, 0x86))
LIT = ((0xFF, 0xD0, 0xFF), (0xF0, 0x6B, 0xFF), (0xC5, 0x2F, 0xDC))
TILE = ((0x3B, 0x27, 0x40), (0x1C, 0x14, 0x1E))
STOPS = (0.0, 0.55, 1.0)

# Panes in 256-space: (x0, y0, x1, y1); only the corner at the cross is visible.
PANES = {
    "tl": (-100, -100, C - H, C - H),
    "tr": (C + H, -100, 356, C - H),
    "bl": (-100, C + H, C - H, 356),
    "br": (C + H, C + H, 356, 356),
}


def svg_gradient(gid, stops):
    s = "".join(
        f'<stop offset="{o}" stop-color="#{r:02x}{g:02x}{b:02x}"/>'
        for o, (r, g, b) in zip(STOPS, stops)
    )
    return (
        f'<linearGradient id="{gid}" gradientUnits="userSpaceOnUse" '
        f'x1="70" y1="30" x2="190" y2="226">{s}</linearGradient>'
    )


def svg_panes(fills):
    return "".join(
        f'<rect x="{x0}" y="{y0}" width="{x1 - x0}" height="{y1 - y0}" rx="{RADIUS}" fill="{fills[k]}"/>'
        for k, (x0, y0, x1, y1) in PANES.items()
    )


def write_svgs():
    full = (
        '<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256" viewBox="0 0 256 256">'
        "<defs>"
        + svg_gradient("orb", ORB)
        + svg_gradient("lit", LIT)
        + '<linearGradient id="tile" x1="0" y1="0" x2="0" y2="1">'
        '<stop offset="0" stop-color="#3b2740"/><stop offset="1" stop-color="#1c141e"/></linearGradient>'
        f'<clipPath id="disc"><circle cx="{C}" cy="{C}" r="{ORB_R}"/></clipPath>'
        "</defs>"
        f'<rect width="256" height="256" rx="{TILE_R}" fill="url(#tile)"/>'
        '<g clip-path="url(#disc)">'
        + svg_panes({"tl": "url(#orb)", "tr": "url(#lit)", "bl": "url(#orb)", "br": "url(#orb)"})
        + "</g></svg>\n"
    )
    mono = (
        '<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256" viewBox="0 0 256 256">'
        f'<defs><clipPath id="disc"><circle cx="{C}" cy="{C}" r="{ORB_R}"/></clipPath></defs>'
        '<g clip-path="url(#disc)" fill="currentColor">'
        + svg_panes({k: "currentColor" for k in PANES})
        + "</g></svg>\n"
    )
    (ROOT / "assets" / "orbwin.svg").write_text(full, encoding="utf-8")
    (ROOT / "assets" / "orbwin-mono.svg").write_text(mono, encoding="utf-8")


def gradient(xs, ys, stops):
    # Projection onto the (70,30)->(190,226) axis, clamped, piecewise linear.
    dx, dy = 120.0, 196.0
    t = np.clip(((xs - 70) * dx + (ys - 30) * dy) / (dx * dx + dy * dy), 0, 1)
    out = np.zeros(t.shape + (3,))
    for ch in range(3):
        out[..., ch] = np.interp(t, STOPS, [s[ch] for s in stops])
    return out


def render(size=1024):
    k = size / 256
    ss = 2  # extra supersampling before the final downscale
    n = int(size * ss)
    s = n / 256
    ys, xs = np.mgrid[0:n, 0:n]
    xs, ys = (xs + 0.5) / s, (ys + 0.5) / s

    def mask(draw_fn):
        m = Image.new("L", (n, n), 0)
        draw_fn(ImageDraw.Draw(m))
        return np.asarray(m, dtype=float)[..., None] / 255

    tile_m = mask(lambda d: d.rounded_rectangle([0, 0, n - 1, n - 1], TILE_R * s, fill=255))
    disc_m = mask(lambda d: d.ellipse([(C - ORB_R) * s, (C - ORB_R) * s, (C + ORB_R) * s, (C + ORB_R) * s], fill=255))
    img = np.zeros((n, n, 3))
    t = np.clip(ys / 256, 0, 1)[..., None]
    img[:] = np.array(TILE[0]) * (1 - t) + np.array(TILE[1]) * t
    orb, lit = gradient(xs, ys, ORB), gradient(xs, ys, LIT)
    for key, (x0, y0, x1, y1) in PANES.items():
        pm = mask(lambda d: d.rounded_rectangle([x0 * s, y0 * s, x1 * s, y1 * s], RADIUS * s, fill=255)) * disc_m
        img = img * (1 - pm) + (lit if key == "tr" else orb) * pm
    rgba = np.concatenate([img, tile_m * 255], axis=-1)
    rgba[..., :3] = np.where(tile_m > 0, rgba[..., :3], 0)
    im = Image.fromarray(np.clip(rgba, 0, 255).astype("uint8"), "RGBA")
    return im.resize((size, size), Image.LANCZOS)


def write_rasters():
    icons = ROOT / "src-tauri" / "icons"
    master = render(1024)
    for name, px in {
        "icon.png": 512,
        "128x128@2x.png": 256,
        "128x128.png": 128,
        "64x64.png": 64,
        "32x32.png": 32,
    }.items():
        master.resize((px, px), Image.LANCZOS).save(icons / name)
    sizes = [16, 24, 32, 48, 64, 256]
    frames = [master.resize((px, px), Image.LANCZOS) for px in sizes]
    frames[-1].save(icons / "icon.ico", format="ICO", sizes=[(p, p) for p in sizes], append_images=frames[:-1])
    # Untracked, regenerable Windows bundle sizes keep local builds consistent.
    for p in (30, 44, 71, 89, 107, 142, 150, 284, 310):
        master.resize((p, p), Image.LANCZOS).save(icons / f"Square{p}x{p}Logo.png")
    master.resize((50, 50), Image.LANCZOS).save(icons / "StoreLogo.png")


if __name__ == "__main__":
    write_svgs()
    write_rasters()
