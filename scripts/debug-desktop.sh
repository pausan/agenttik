#!/usr/bin/env bash
# Run the Linux desktop under GDB so a native crash includes C frames and
# library mappings, which Go's gtk_main traceback does not contain.
# Usage: scripts/debug-desktop.sh [binary [arguments...]] >crash.log 2>&1
set -euo pipefail
binary=${1:-bin/agenttik}
if [ "$#" -gt 0 ]; then shift; fi
exec gdb --quiet --batch \
  -ex 'set pagination off' \
  -ex 'handle SIGURG SIGPIPE SIG34 nostop noprint pass' \
  -ex run \
  -ex 'info registers' \
  -ex 'x/12i $pc-20' \
  -ex 'info proc mappings' \
  -ex 'thread apply all bt' \
  -ex 'info sharedlibrary' \
  --args "$binary" "$@"
