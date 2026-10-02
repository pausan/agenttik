"""Writes scripts/macos/DS_Store, the Finder layout of the agenttik disk image.

package-macos.sh copies it to the image root as .DS_Store, beside
.background/background.tiff, the same way CPack builds KeePassXC's image.
Finder finds the background through an alias naming the volume "agenttik" and
the file's path, so both must match what package-macos.sh creates.

Regenerate after changing the layout (any OS):
  python3 -m venv /tmp/dsstore && /tmp/dsstore/bin/pip install ds_store mac_alias
  /tmp/dsstore/bin/python scripts/macos/make-ds-store.py
"""

import datetime
import os

from ds_store import DSStore
from mac_alias import Alias, TargetInfo, VolumeInfo

VOLUME = "agenttik"
WIDTH, HEIGHT = 660, 400  # points; the background images match this size
ICON_Y = 175

# Fixed dates keep the output reproducible. Finder does not need them to match
# the image: when they differ it resolves the alias by volume name and path.
date = datetime.datetime(2026, 1, 1, tzinfo=datetime.timezone.utc)
background = Alias(
    volume=VolumeInfo(VOLUME, date, b"H+", 5, 0, b"\0\0", posix_path="/Volumes/" + VOLUME),
    target=TargetInfo(
        0,
        "background.tiff",
        20,
        21,
        date,
        b"\0\0\0\0",
        b"\0\0\0\0",
        folder_name=".background",
        cnid_path=(20,),
        carbon_path=VOLUME + ":.background:\0background.tiff",
        posix_path="/.background/background.tiff",
    ),
)

path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "DS_Store")
if os.path.exists(path):
    os.remove(path)
with DSStore.open(path, "w+") as d:
    d["."]["vSrn"] = ("long", 1)
    # Height adds the title bar, so the content area matches the background.
    d["."]["bwsp"] = {
        "WindowBounds": "{{200, 200}, {%d, %d}}" % (WIDTH, HEIGHT + 23),
        "ShowStatusBar": False,
        "ShowSidebar": False,
        "ContainerShowSidebar": False,
        "ShowToolbar": False,
        "ShowTabView": False,
        "ShowPathbar": False,
    }
    d["."]["icvp"] = {
        "viewOptionsVersion": 1,
        "backgroundType": 2,
        "backgroundImageAlias": background.to_bytes(),
        "backgroundColorRed": 1.0,
        "backgroundColorGreen": 1.0,
        "backgroundColorBlue": 1.0,
        "iconSize": 156.0,
        "textSize": 12.0,
        "gridSpacing": 52.0,
        "gridOffsetX": 0.0,
        "gridOffsetY": 0.0,
        "labelOnBottom": True,
        "showItemInfo": False,
        "showIconPreview": True,
        "arrangeBy": "none",
        "scrollPositionX": 0.0,
        "scrollPositionY": 0.0,
    }
    # Either side of the chevron drawn at x=330 in the background.
    d["agenttik.app"]["Iloc"] = (165, ICON_Y)
    d["Applications"]["Iloc"] = (495, ICON_Y)
    # Hidden entries sit below the window, out of sight even if shown.
    d[".background"]["Iloc"] = (165, HEIGHT + 100)
    d[".fseventsd"]["Iloc"] = (495, HEIGHT + 100)
