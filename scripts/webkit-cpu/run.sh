#!/usr/bin/env bash
# Measures the CPU the UI costs in WebKitGTK, painting in software as the Linux
# desktop window does, in three states: three running tasks in the sidebar, a
# running task open, and a long reply streaming. See specs/079-ui-performance.md.
#
# usage: scripts/webkit-cpu/run.sh [web-binary]   (default bin/agenttik-web)
# needs: xvfb-run, python3-gi with WebKit2 4.1, node and the e2e dependencies
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
bin=$(realpath "${1:-$root/bin/agenttik-web}")
data=$(mktemp -d)
trap 'kill $server 2>/dev/null; rm -rf "$data"' EXIT

cd "$root/e2e"
AGENTTIK_FAKE_PROVIDER=1 SHELL=/bin/sh XDG_DATA_HOME="$data/xdg" \
  "$bin" --web --addr 127.0.0.1:0 --data-dir "$data" >"$data/out.log" 2>&1 &
server=$!
url=
for _ in $(seq 50); do
  url=$(grep -oE 'http://[0-9.]+:[0-9]+' "$data/out.log" | head -1 || true)
  [ -n "$url" ] && break
  sleep 0.2
done
[ -n "$url" ] || { cat "$data/out.log"; exit 1; }
stream=$(node "$here/setup.mjs" "$url")

click() { echo "[...document.querySelectorAll('aside *')].find((e) => !e.children.length && /$1/.test(e.textContent))?.click()"; }
send="fetch('/api/sessions/$stream/messages', {method: 'POST', headers: {'content-type': 'application/json'},
  body: JSON.stringify({prompt: Array.from({length: 120}, (_, i) => '## Part ' + i +
  '\n\nSome **bold** words, a [link](https://example.com) and \`code\` in a sentence that keeps going.\n\n- one item\n- two items').join('\n\n')})})"

measure() { xvfb-run -a -s "-screen 0 1920x1080x24" python3 -I "$here/measure.py" "$url" "$@" 2>&1 | grep -v libEGL; }
measure "running tasks in the sidebar"
measure "running task open" "$(click @wait)"
measure "long reply streaming" "$(click streamtask)" "$send"
