# Screen viewing and control

agenttik can show another machine's screen and drive its mouse and keyboard:
for a person, in a Screen tab, and for an agent, through an HTTP API.

## The encoder

Screens travel as VP9, or VP8 for browsers that cannot decode VP9, encoded by
libvpx. Nothing is installed on the machine and nothing goes through cgo:

- `scripts/build-vpx.sh [goos] [goarch]` builds a static libvpx (pinned
  version, VP8/VP9 encoders only, realtime only) and links it with
  `app/internal/screen/vpx/shim/agenttik_vpx.c` into one shared library,
  `app/internal/screen/vpx/lib/<goos>_<goarch>/`. `make build` and
  `make build-web` run it when the library is missing (`make vpx`). CI builds
  it for Linux, the only platform with screen capture so far.
- The release binary embeds its platform's library (about 1.8 MB). On first
  use it is written to the user cache directory, named by its hash, and loaded
  with purego, which works with `CGO_ENABLED=0` (web and Windows builds).
- A build without the library still works. `vpx.Load` gives the reason and
  the screen is sent as JPEG instead.

The shim is the whole API agenttik uses: `avx_open`, `avx_encode`,
`avx_close`, `avx_error`, with plain integers and pointers. libvpx's own API
takes structs whose layout changes between versions and a variadic control
call that cannot be made portably from Go. The shim also converts BGRA to
I420 (BT.601), optionally halving each side by averaging.

Settings come from measurements of 1080p desktop video (scrolling and typing):

| Choice | Why |
|--------|-----|
| Constant quality with a bitrate cap (3 Mbps) | A fixed bitrate spends its whole budget on an unchanged screen; CQ halved typing traffic |
| VP9 speed 8, screen tuning, `aq-mode 3`, row multithreading | About 30% smaller than VP8 when scrolling, at equal SSIM |
| VP8 speed 12, static threshold 100 | Above 12, quality falls apart in motion and bitrate doubles |
| No lookahead, keyframes only on request | Lowest latency; a keyframe costs 10–30× a delta |
| Up to 4 threads | 1080p scrolling encodes in about 10 ms a frame, conversion included |

`go test ./app/internal/screen/vpx -bench .` checks keyframes, near-free
unchanged frames, halving, and reports ms and Mbps for a scrolling 1080p
screen.

## Capture and input

`screen.Display` is one machine's screen, pointer, keyboard and clipboard.
Linux X11 is the only implementation so far (`x11_linux.go`). It speaks the
X protocol from Go (`jezek/xgb`, whose Close is safe when the server goes
away) and links nothing:

- **Capture:** MIT-SHM. A System V segment is shared with the server and
  marked for removal at once, so it goes when both sides detach. A server on
  another machine cannot attach, so plain `GetImage` is used instead. The
  pixel format must be 32 bits per pixel, little-endian (BGRX).
- **Changes:** DAMAGE on the root window bumps a counter, as do cursor shape
  changes (XFIXES) and screen resizes (root `ConfigureNotify`, which also
  resizes the segment). A viewer whose count has not moved skips the grab.
- **Pointer:** XTEST motion and buttons. Buttons 4–7 are the wheel.
- **Keys:** everything is a keysym, so the viewer's layout does not matter.
  `Key` presses the key that types a keysym, as is, so held modifiers apply.
  This is how Ctrl+C travels. `Type` taps one character and adds or lifts
  Shift as the server's layout needs, allowing for Caps Lock. A keysym with
  no key is mapped onto a spare key first, as xdotool does.
- **Clipboard:** text only, as `UTF8_STRING`. An invisible window owns
  `CLIPBOARD` to set it and answers `TARGETS`, `UTF8_STRING`, `TEXT` and
  `STRING`. Reading asks the owner to convert into a property, with a
  two-second limit. XFIXES selection events announce when another program
  takes the clipboard. Text is limited to one X request (about 256 KB); INCR
  transfers are not supported.

Wayland sessions report themselves unavailable. With no `$DISPLAY`, `:0` is
tried, for a server started outside the desktop session. macOS and Windows
report "not supported yet".

Tests run their own Xvfb. They check a grabbed window's pixels and the damage
count, pointer moves and clicks reaching a window, Shift added for `Q`, `é`
on a spare key, Ctrl+C with its modifier, and clipboard text both ways between
two programs.

## Viewers

`screen.Manager` is the machine's one screen. The display opens on first use
and a failure is remembered for five seconds. Each viewer (`Watch`) has its
own goroutine, encoder and stream of messages:

| Message | Carries |
|---------|---------|
| `hello` | viewer id, codec, screen size |
| `frame` | sequence number, key flag, frame and screen size, base64 VP8/VP9 frame or JPEG bands |
| `cursor` | pointer position, in screen pixels |
| `shape` | pointer image as PNG, with its hot spot |
| `clip` | text another program put on the clipboard |

The loop runs 30 times a second. The pointer is drawn by the viewer, so moving
it sends a `cursor` message, not a frame; shapes are sent when the XFIXES
serial changes. A frame is grabbed only when the damage count moved (or a
keyframe was asked for). It is not encoded when its pixels equal the last
frame's. Two buffers swap, so a steady stream allocates nothing per frame.

**Pacing.** A viewer acknowledges each frame it has drawn. While it has 6
frames or 256 KB unacknowledged, no new frame is grabbed. This bounds latency
by what is in flight, not by the buffers of every proxy in between (the
desktop window, a remote profile, the quick tunnel). A slow link gets fewer
frames, each still current. Acknowledgements missing for two seconds are
assumed lost.

**Keyframes** are sent first, after a size change (a new encoder) and when
the viewer asks (`RequestKey`, after a decode error).

**JPEG fallback.** With no VP8/VP9 library, or for a client without
WebCodecs, frames are 64-row bands; only the bands that differ from the last
frame are sent. Each is encoded on its own core at quality 75.

Screens wider than 2560 or taller than 1600 are halved before VP8/VP9
encoding.

**Input.** A viewer's events are applied in order: `m` move, `b` button, `w`
wheel steps, `k` key down/up by browser `key` and `code`, `c` text. Keys are
remembered by `code`, so a key released after Shift changed what it types
releases the key it pressed. Closing a viewer releases every key and button
it still holds. Coordinates are clamped to the screen; wheel events are
capped at 50 steps.

## Agent API

`Manager.Screenshot(width, format)` grabs once and scales down by area
averaging, as PNG or JPEG. `Manager.Do` runs a list of actions shaped like
the computer-use tool's: `mouse_move`, `left_click`, `right_click`,
`middle_click`, `double_click`, `triple_click`, `left_mouse_down`,
`left_mouse_up`, `left_click_drag`, `scroll`, `type`, `key`, `hold_key`,
`wait`, `cursor_position`. Coordinates are `[x, y]` in the space of the
request's `width` and `height` (the screenshot the agent saw), or screen
pixels without them. Clicks and scrolls hold the modifiers named in `text`.
Keys use xdotool names (`ctrl+shift+t`, `Return`, `super`). The first failing
action stops the list, and the answer names it. Waits are capped at 30
seconds. The answer is the pointer's position in the request's space.

Tests use `screen.Fake`: frames only on change, changed bands only, keyframes
on request, pacing by acknowledgement, input order and release on close,
clipboard forwarding, screenshot scaling and action coordinates.
