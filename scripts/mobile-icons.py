#!/usr/bin/env python3
"""Render web/public/agenttik.svg into the Android and iOS app icons.

Needs google-chrome (headless, for the SVG filters) and ImageMagick.
Usage: python3 scripts/mobile-icons.py
"""
import os
import pathlib
import subprocess
import tempfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
SOURCE = (ROOT / "web/public/agenttik.svg").read_text()
RES = ROOT / "mobileapp/android/app/src/main/res"
IOS = ROOT / "mobileapp/ios/App/App/Assets.xcassets/AppIcon.appiconset/AppIcon-512@2x.png"

SIZE = 'width="64" height="64" viewBox="0 0 64 64"'
PLATE = ('  <rect width="64" height="64" rx="16" fill="url(#i11v2-plate)"/>\n'
         '  <rect width="64" height="64" rx="16" fill="url(#i11v2-spot)"/>\n')
RIM = '  <rect x=".75" y=".75" width="62.5" height="62.5" rx="15.25" fill="none" stroke="#ffffff" stroke-opacity=".12"/>\n'
for part in (SIZE, PLATE, RIM):
    assert SOURCE.count(part) == 1, f"agenttik.svg changed, update this script: {part!r}"
HEAD, rest = SOURCE.split(PLATE)
MARK, TAIL = rest.split(RIM)

ROUND_PLATE = PLATE.replace('rect width="64" height="64" rx="16"', 'circle cx="32" cy="32" r="32"')
ROUND_RIM = '  <circle cx="32" cy="32" r="31.25" fill="none" stroke="#ffffff" stroke-opacity=".12"/>\n'
SQUARE_PLATE = PLATE.replace(' rx="16"', "")
# Adaptive icons are 108dp with the middle 72dp visible, so the 64-unit plate
# fills the visible part when the view box grows to 96 units.
WIDE_PLATE = PLATE.replace('width="64" height="64" rx="16"', 'x="-16" y="-16" width="96" height="96"')
WIDE = "-16 -16 96 96"


def svg(size, plate="", mark=MARK, rim="", view="0 0 64 64"):
    head = HEAD.replace(SIZE, f'width="{size}" height="{size}" viewBox="{view}"')
    return head + plate + mark + rim + TAIL


def render(text, size, out, opaque=False):
    with tempfile.TemporaryDirectory() as tmp:
        src = os.path.join(tmp, "icon.svg")
        png = os.path.join(tmp, "icon.png")
        pathlib.Path(src).write_text(text)
        subprocess.run(["google-chrome", "--headless", "--disable-gpu", "--hide-scrollbars",
                        "--default-background-color=00000000", f"--window-size={size},{size}",
                        f"--screenshot={png}", f"file://{src}"], check=True, capture_output=True)
        flatten = ["-background", "#0b0b0e", "-flatten", "-alpha", "off"] if opaque else []
        subprocess.run(["convert", png, *flatten, "-strip", str(out)], check=True)


for density, scale in {"mdpi": 1, "hdpi": 1.5, "xhdpi": 2, "xxhdpi": 3, "xxxhdpi": 4}.items():
    folder = RES / f"mipmap-{density}"
    legacy, adaptive = round(48 * scale), round(108 * scale)
    render(svg(legacy, PLATE, rim=RIM), legacy, folder / "ic_launcher.png")
    render(svg(legacy, ROUND_PLATE, rim=ROUND_RIM), legacy, folder / "ic_launcher_round.png")
    render(svg(adaptive, WIDE_PLATE, mark="", view=WIDE), adaptive, folder / "ic_launcher_background.png")
    render(svg(adaptive, view=WIDE), adaptive, folder / "ic_launcher_foreground.png")

# iOS wants a full-bleed opaque square and rounds the corners itself.
render(svg(1024, SQUARE_PLATE), 1024, IOS, opaque=True)
