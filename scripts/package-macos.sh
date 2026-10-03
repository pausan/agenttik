#!/bin/bash
# Package an existing native binary. Uses only tools included with macOS/Xcode.
set -euo pipefail

if [[ $# != 5 || $(uname -s) != Darwin ]]; then
  echo "Usage (on macOS): $0 binary output.app version output.zip output.dmg" >&2
  exit 1
fi
binary=$1
app=$2
version=$3
archive=$4
dmg=$5
if [[ $app != *.app || $archive != *.zip || $dmg != *.dmg || ! $version =~ ^[A-Za-z0-9.-]+$ ]]; then
  echo "Expected an .app path, a .zip path, a .dmg path and an alphanumeric version (dots/dashes allowed)" >&2
  exit 1
fi

root=$(cd "$(dirname "$0")/.." && pwd)
staging=$(mktemp -d)
trap 'hdiutil detach "$staging/mounted" -quiet 2>/dev/null || true; rm -rf "$staging"' EXIT
bundle="$staging/agenttik.app"
mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources"
cp "$binary" "$bundle/Contents/MacOS/agenttik"
chmod 755 "$bundle/Contents/MacOS/agenttik"

# Commit builds still need a numeric bundle version; the full version remains
# available through --version and the bundle's informational version string.
bundle_version=0.0.0
if [[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  bundle_version=$version
fi
cat > "$bundle/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key><string>agenttik</string>
  <key>CFBundleIdentifier</key><string>com.pausan.agenttik</string>
  <key>CFBundleName</key><string>agenttik</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>$bundle_version</string>
  <key>CFBundleVersion</key><string>$bundle_version</string>
  <key>CFBundleGetInfoString</key><string>agenttik $version</string>
  <key>CFBundleIconFile</key><string>agenttik</string>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
PLIST

# scripts/macos/render-images.mjs renders the 1024px icon from the web logo.
mkdir "$staging/agenttik.iconset"
for size in 16 32 128 256 512; do
  sips -z $size $size "$root/app/cmd/agenttik/appicon.png" --out "$staging/agenttik.iconset/icon_${size}x${size}.png" >/dev/null
  sips -z $((size * 2)) $((size * 2)) "$root/app/cmd/agenttik/appicon.png" --out "$staging/agenttik.iconset/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$staging/agenttik.iconset" -o "$bundle/Contents/Resources/agenttik.icns"
plutil -lint "$bundle/Contents/Info.plist"
codesign --force --sign - "$bundle"
codesign --verify --strict --verbose=2 "$bundle"

# ditto preserves executable modes and the signed bundle in the ZIP.
mkdir -p "$(dirname "$app")" "$(dirname "$archive")"
rm -rf "$app"
ditto "$bundle" "$app"
rm -f "$archive"
ditto -c -k --sequesterRsrc --keepParent "$app" "$archive"

# Verify what users actually extract, including its signature and version.
ditto -x -k "$archive" "$staging/unpacked"
extracted="$staging/unpacked/$(basename "$app")"
codesign --verify --strict --verbose=2 "$extracted"
test "$("$extracted/Contents/MacOS/agenttik" --version)" = "agenttik $version"

# The disk image holds the app next to a link to /Applications, so users
# install by dragging one onto the other. Like KeePassXC's, it carries a
# prebuilt Finder layout (scripts/macos/make-ds-store.py) and a 1x/2x
# background; Finder finds that background by volume name and path, so both
# must stay as they are. HFS+ keeps it readable on older macOS.
mkdir -p "$staging/dmg/.background"
ditto "$bundle" "$staging/dmg/agenttik.app"
ln -s /Applications "$staging/dmg/Applications"
cp "$root/scripts/macos/DS_Store" "$staging/dmg/.DS_Store"
tiffutil -cathidpicheck "$root/scripts/macos/dmg-background.png" "$root/scripts/macos/dmg-background@2x.png" \
  -out "$staging/dmg/.background/background.tiff"
mkdir -p "$(dirname "$dmg")"
hdiutil create -quiet -ov -volname agenttik -fs HFS+ -format UDZO -srcfolder "$staging/dmg" "$dmg"

# Verify the mounted image the same way as the ZIP.
hdiutil verify -quiet "$dmg"
hdiutil attach -quiet -readonly -nobrowse -noautoopen -mountpoint "$staging/mounted" "$dmg"
test "$(readlink "$staging/mounted/Applications")" = /Applications
cmp "$staging/mounted/.DS_Store" "$root/scripts/macos/DS_Store"
test -s "$staging/mounted/.background/background.tiff"
mounted="$staging/mounted/agenttik.app"
codesign --verify --strict --verbose=2 "$mounted"
test "$("$mounted/Contents/MacOS/agenttik" --version)" = "agenttik $version"
hdiutil detach -quiet "$staging/mounted"
