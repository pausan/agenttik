#!/usr/bin/env bash
# Build the portable CPU-only Whisper CLI embedded with Agenttik. Weights are
# downloaded on first dictation, not at build time. No Python or ffmpeg needed.
set -euo pipefail
speech_os=${1:-$(go env GOOS)}
speech_arch=${2:-$(go env GOARCH)}
root=$(cd "$(dirname "$0")/.." && pwd)
work=$root/tmp-speech
source_dir=$work/whisper.cpp
revision=d1be6fde11ac6e0407606b4e42fe72d34add8037
output=$root/app/internal/speech/bin/whisper-$speech_os-$speech_arch
suffix=
cmake_flags=(-DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF -DGGML_NATIVE=OFF
  -DGGML_OPENMP=OFF -DWHISPER_BUILD_TESTS=OFF -DWHISPER_BUILD_EXAMPLES=ON
  -DGGML_METAL=OFF -DGGML_ACCELERATE=OFF)
case "$speech_os/$speech_arch" in
  linux/amd64) cmake_flags+=("-DCMAKE_EXE_LINKER_FLAGS=-static-libgcc -static-libstdc++") ;;
  linux/arm64)
    cmake_flags+=("-DCMAKE_EXE_LINKER_FLAGS=-static-libgcc -static-libstdc++")
    if [[ $(uname -m) != aarch64 ]]; then
      cmake_flags+=(-DCMAKE_SYSTEM_NAME=Linux -DCMAKE_SYSTEM_PROCESSOR=aarch64
        -DCMAKE_C_COMPILER=aarch64-linux-gnu-gcc -DCMAKE_CXX_COMPILER=aarch64-linux-gnu-g++)
    fi ;;
  darwin/arm64) cmake_flags+=(-DCMAKE_OSX_ARCHITECTURES=arm64) ;;
  darwin/amd64) cmake_flags+=(-DCMAKE_OSX_ARCHITECTURES=x86_64) ;;
  windows/amd64)
    suffix=.exe
    output=$output$suffix
    cmake_flags+=(-DCMAKE_SYSTEM_NAME=Windows -DCMAKE_SYSTEM_PROCESSOR=x86_64 -DCMAKE_C_COMPILER=x86_64-w64-mingw32-gcc
      -DCMAKE_CXX_COMPILER=x86_64-w64-mingw32-g++ -DCMAKE_EXE_LINKER_FLAGS=-static) ;;
  *) echo "Unsupported speech platform: $speech_os/$speech_arch" >&2; exit 1 ;;
esac
if [[ -f $output && $output -nt "$0" ]]; then exit 0; fi
mkdir -p "$work" "$(dirname "$output")"
if [[ ! -d $source_dir ]]; then
  git -c advice.detachedHead=false clone --quiet --depth 1 --branch v1.9.5 https://github.com/ggml-org/whisper.cpp.git "$source_dir"
fi
if [[ $(git -C "$source_dir" rev-parse HEAD) != "$revision" ]]; then
  echo "Whisper source does not match the pinned v1.9.5 revision" >&2
  exit 1
fi
build=$work/build-$speech_os-$speech_arch
cmake -S "$source_dir" -B "$build" "${cmake_flags[@]}"
cmake --build "$build" --target whisper-cli --parallel 4
cp "$build/bin/whisper-cli$suffix" "$output"
echo "Built $output"
