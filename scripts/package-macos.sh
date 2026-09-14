#!/bin/bash
# Package an existing native binary. Uses only tools included with macOS/Xcode.
set -euo pipefail

if [[ $# != 4 || $(uname -s) != Darwin ]]; then
  echo "Usage (on macOS): $0 binary output.app version output.zip" >&2
  exit 1
fi
binary=$1
app=$2
version=$3
archive=$4
if [[ $app != *.app || $archive != *.zip || ! $version =~ ^[A-Za-z0-9.-]+$ ]]; then
  echo "Expected an .app path, a .zip path and an alphanumeric version (dots/dashes allowed)" >&2
  exit 1
fi

root=$(cd "$(dirname "$0")/.." && pwd)
staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT
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

mkdir "$staging/agenttik.iconset"
sips -z 16 16 "$root/app/cmd/agenttik/tray.png" --out "$staging/agenttik.iconset/icon_16x16.png" >/dev/null
sips -z 32 32 "$root/app/cmd/agenttik/tray.png" --out "$staging/agenttik.iconset/icon_32x32.png" >/dev/null
cp "$staging/agenttik.iconset/icon_32x32.png" "$staging/agenttik.iconset/icon_16x16@2x.png"
cp "$root/app/cmd/agenttik/tray.png" "$staging/agenttik.iconset/icon_32x32@2x.png"
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
