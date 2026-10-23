#!/usr/bin/env bash
# build-vpx.sh builds the screen encoder library agenttik embeds: a static
# libvpx (VP8/VP9 encoders only, realtime only) linked with the small C shim in
# app/internal/screen/vpx/shim into one shared library, written where the Go
# package embeds it from. See specs/096-screen.md.
#
#   scripts/build-vpx.sh [goos] [goarch]     defaults: this machine
#
# Needs git, make, a C compiler and nasm on x86. Cross builds use:
#   windows/amd64  x86_64-w64-mingw32-gcc
#   darwin/*       run on macOS
set -euo pipefail

VPX_VERSION=v1.15.2
GOOS=${1:-$(go env GOOS)}
GOARCH=${2:-$(go env GOARCH)}
ROOT=$(cd "$(dirname "$0")/.." && pwd)
SHIM=$ROOT/app/internal/screen/vpx/shim/agenttik_vpx.c
OUT=$ROOT/app/internal/screen/vpx/lib/${GOOS}_${GOARCH}
WORK=${VPX_WORK:-$ROOT/tmp-vpx}

case "$GOOS/$GOARCH" in
  linux/amd64)   TARGET=x86_64-linux-gcc;   CC=${CC:-gcc}; LIB=libagenttikvpx.so ;;
  linux/arm64)   TARGET=arm64-linux-gcc;    CC=${CC:-aarch64-linux-gnu-gcc}; LIB=libagenttikvpx.so ;;
  windows/amd64) TARGET=x86_64-win64-gcc;   CC=${CC:-x86_64-w64-mingw32-gcc}; LIB=agenttikvpx.dll
                 export CROSS=${CROSS:-x86_64-w64-mingw32-} ;;
  darwin/arm64)  TARGET=arm64-darwin20-gcc; CC=${CC:-clang}; LIB=libagenttikvpx.dylib ;;
  darwin/amd64)  TARGET=x86_64-darwin20-gcc; CC=${CC:-clang}; LIB=libagenttikvpx.dylib ;;
  *) echo "unsupported platform $GOOS/$GOARCH" >&2; exit 1 ;;
esac
if [[ $GOOS/$GOARCH == linux/arm64 && $(uname -m) == aarch64 ]]; then CC=gcc; fi

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
  make -j"$(getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu)" >/dev/null
)

mkdir -p "$OUT"
case $GOOS in
  linux)
    "$CC" -O3 -shared -fPIC -fvisibility=hidden -o "$OUT/$LIB" "$SHIM" -I"$SRC" -I"$BUILD" \
      "$BUILD/libvpx.a" -Wl,--exclude-libs,ALL -Wl,-z,noexecstack -lm -lpthread -s ;;
  windows)
    "$CC" -O3 -shared -fvisibility=hidden -o "$OUT/$LIB" "$SHIM" -I"$SRC" -I"$BUILD" \
      "$BUILD/libvpx.a" -static-libgcc -Wl,-Bstatic -lpthread -s ;;
  darwin)
    "$CC" -O3 -dynamiclib -fvisibility=hidden -o "$OUT/$LIB" "$SHIM" -I"$SRC" -I"$BUILD" \
      "$BUILD/libvpx.a" -Wl,-dead_strip ;;
esac
echo "$OUT/$LIB ($(wc -c <"$OUT/$LIB") bytes, libvpx $VPX_VERSION)"
