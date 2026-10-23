#!/usr/bin/env bash
# build-vpx.sh builds the screen encoder library agenttik embeds: a static
# libvpx (VP8/VP9 encoders only, realtime only) and a static libopus (for the
# screen's sound) linked with the small C shim in app/internal/screen/vpx/shim
# into one shared library, written where the Go package embeds it from. See
# specs/096-screen.md.
#
#   scripts/build-vpx.sh [goos] [goarch]     defaults: this machine
#
# Needs git, curl, make, a C compiler and nasm on x86. Cross builds use:
#   windows/amd64  x86_64-w64-mingw32-gcc
#   darwin/*       run on macOS
set -euo pipefail

VPX_VERSION=v1.15.2
OPUS_VERSION=1.5.2
OPUS_SHA256=65c1d2f78b9f2fb20082c38cbe47c951ad5839345876e46941612ee87f9a7ce1
GOOS=${1:-$(go env GOOS)}
GOARCH=${2:-$(go env GOARCH)}
ROOT=$(cd "$(dirname "$0")/.." && pwd)
SHIM=$ROOT/app/internal/screen/vpx/shim/agenttik_vpx.c
OUT=$ROOT/app/internal/screen/vpx/lib/${GOOS}_${GOARCH}
WORK=${VPX_WORK:-$ROOT/tmp-vpx}

# HOST is libopus's autotools host; ARCH a compiler flag for it.
ARCH=
case "$GOOS/$GOARCH" in
  linux/amd64)   TARGET=x86_64-linux-gcc;   CC=${CC:-gcc}; LIB=libagenttikvpx.so; HOST=x86_64-linux-gnu ;;
  linux/arm64)   TARGET=arm64-linux-gcc;    CC=${CC:-aarch64-linux-gnu-gcc}; LIB=libagenttikvpx.so; HOST=aarch64-linux-gnu ;;
  windows/amd64) TARGET=x86_64-win64-gcc;   CC=${CC:-x86_64-w64-mingw32-gcc}; LIB=agenttikvpx.dll; HOST=x86_64-w64-mingw32
                 export CROSS=${CROSS:-x86_64-w64-mingw32-} ;;
  darwin/arm64)  TARGET=arm64-darwin20-gcc; CC=${CC:-clang}; LIB=libagenttikvpx.dylib; HOST=aarch64-apple-darwin; ARCH="-arch arm64" ;;
  darwin/amd64)  TARGET=x86_64-darwin20-gcc; CC=${CC:-clang}; LIB=libagenttikvpx.dylib; HOST=x86_64-apple-darwin; ARCH="-arch x86_64" ;;
  *) echo "unsupported platform $GOOS/$GOARCH" >&2; exit 1 ;;
esac
if [[ $GOOS/$GOARCH == linux/arm64 && $(uname -m) == aarch64 ]]; then CC=gcc; fi
JOBS=$(getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu)

SRC=$WORK/libvpx-$VPX_VERSION
BUILD=$WORK/build-${GOOS}_${GOARCH}
if [[ ! -d $SRC ]]; then
  mkdir -p "$WORK"
  git -c advice.detachedHead=false clone --quiet --depth 1 --branch "$VPX_VERSION" https://chromium.googlesource.com/webm/libvpx "$SRC"
fi
rm -rf "$BUILD" && mkdir -p "$BUILD"
(
  cd "$BUILD"
  "$SRC/configure" --target="$TARGET" --enable-pic --enable-static --disable-shared \
    --disable-examples --disable-tools --disable-docs --disable-unit-tests \
    --disable-decode-perf-tests --disable-encode-perf-tests \
    --disable-vp8-decoder --disable-vp9-decoder --enable-realtime-only \
    --disable-webm-io --disable-libyuv >/dev/null
  make -j"$JOBS" >/dev/null
)

OSRC=$WORK/opus-$OPUS_VERSION
OBUILD=$WORK/build-opus-${GOOS}_${GOARCH}
if [[ ! -d $OSRC ]]; then
  mkdir -p "$WORK"
  curl -fsSL -o "$WORK/opus.tar.gz" "https://downloads.xiph.org/releases/opus/opus-$OPUS_VERSION.tar.gz"
  if command -v sha256sum >/dev/null; then sum=$(sha256sum "$WORK/opus.tar.gz"); else sum=$(shasum -a 256 "$WORK/opus.tar.gz"); fi
  if [[ ${sum%% *} != "$OPUS_SHA256" ]]; then echo "opus-$OPUS_VERSION.tar.gz: wrong checksum" >&2; exit 1; fi
  tar -xzf "$WORK/opus.tar.gz" -C "$WORK" && rm "$WORK/opus.tar.gz"
fi
rm -rf "$OBUILD" && mkdir -p "$OBUILD"
(
  cd "$OBUILD"
  CC="$CC $ARCH" CFLAGS="-O2 -fPIC -w" "$OSRC/configure" --host="$HOST" --enable-static --disable-shared \
    --with-pic --disable-doc --disable-extra-programs >/dev/null
  make -j"$JOBS" >/dev/null
)
OPUS=("-I$OSRC/include" "$OBUILD/.libs/libopus.a")

mkdir -p "$OUT"
case $GOOS in
  linux)
    "$CC" -O3 -shared -fPIC -fvisibility=hidden -o "$OUT/$LIB" "$SHIM" -I"$SRC" -I"$BUILD" \
      "$BUILD/libvpx.a" "${OPUS[@]}" -Wl,--exclude-libs,ALL -Wl,-z,noexecstack -lm -lpthread -s ;;
  windows)
    "$CC" -O3 -shared -fvisibility=hidden -o "$OUT/$LIB" "$SHIM" -I"$SRC" -I"$BUILD" \
      "$BUILD/libvpx.a" "${OPUS[@]}" -static-libgcc -Wl,-Bstatic -lpthread -s ;;
  darwin)
    "$CC" -O3 -dynamiclib -fvisibility=hidden -o "$OUT/$LIB" "$SHIM" -I"$SRC" -I"$BUILD" \
      "$BUILD/libvpx.a" "${OPUS[@]}" -Wl,-dead_strip ;;
esac
echo "$OUT/$LIB ($(wc -c <"$OUT/$LIB") bytes, libvpx $VPX_VERSION, libopus $OPUS_VERSION)"
