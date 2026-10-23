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

A frame is grabbed when the screen changes, not on a clock. The display
signals damage (`ChangedSince`), and the viewer grabs at once if the last frame
is at least 1/30 s old, or when that interval runs out. Polling at 30 Hz
measured 17 fps on a screen changing 30 times a second: ticks landed just
before the damage, so half saw nothing and the next merged two. A display
without damage reports a change every time, which makes the loop grab every
1/30 s. The pointer is polled 30 times a second and drawn by the viewer, so
moving it sends a `cursor` message, not a frame; shapes are sent when the
XFIXES serial changes. A grabbed frame is not encoded when its pixels equal
the last frame's. Two buffers swap, so a steady stream allocates nothing per
frame.

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

The orchestrator's built-in prompt ([054](054-orchestrator-project.md))
documents `GET /api/screen`, the screenshot and the actions through
`agenttik --api`, which writes the PNG to stdout unchanged. It also tells the
orchestrator not to turn sharing on unless the user asks.

`AGENTTIK_SCREEN_MEASURE=1 go test ./app/internal/screen -run TestScreenMeasure -v`
draws on a 1080p Xvfb 30 times a second and reports what a viewer acknowledging
at once receives. On a Ryzen AI 9 365:

| Scene | VP9 | VP8 | JPEG |
|-------|-----|-----|------|
| Scrolling page of word-sized blocks | 28.8 fps, 0.75 Mbps | 28.8 fps, 0.62 Mbps | 28.8 fps, 82 Mbps |
| One word added per frame | 28.6 fps, 0.05 Mbps | 28.8 fps, 0.08 Mbps | 28.6 fps, 3.7 Mbps |

Grabbing a 1080p screen over MIT-SHM takes 0.9 ms, comparing it with the last
0.2 ms, and VP9-encoding an unchanged one 3.6 ms.

Tests use `screen.Fake`: frames only on change, changed bands only, keyframes
on request, pacing by acknowledgement, input order and release on close,
clipboard forwarding, screenshot scaling and action coordinates.

## HTTP

The screen belongs to the machine, so every profile answers from the
instance's one manager and the instance's switch
(`server_config.screen_enabled`, off by default). Everything but
`GET`/`PUT /api/screen` answers 403 while it is off. Turning it off ends every
viewer at once.

| Route | Does |
|-------|------|
| `GET /api/screen` | `{enabled, same_machine, available, reason, width, height, os, codecs}` |
| `PUT /api/screen` | `{enabled}` |
| `GET /api/stream/screen?codec=vp9\|vp8\|jpeg` | one viewer's messages, as SSE `data:` JSON |
| `POST /api/screen/input` | `{viewer, ack, key, events}`: input, the last frame drawn, a keyframe request |
| `GET`/`PUT /api/screen/clipboard` | `{text}` |
| `GET /api/screen/shot?width=&format=png\|jpeg` | a screenshot; `X-Screen-Width/Height` and `X-Image-Width/Height` headers |
| `POST /api/screen/actions` | `{width, height, actions}`; answers `{cursor: {x, y}}` |

Frames travel over SSE and input over POST, like terminals: the desktop window
cannot hold a WebSocket. Frame payloads are base64. The stream is gzipped when
the client accepts it, which wins back most of base64's third. The same SSE
carries over remote profiles and through the quick tunnel's websocket bridge
unchanged. A viewer keeps one input request in flight, which also carries its
acknowledgements.

`same_machine` is how the UI hides the Screen button where it would only show
itself. agenttik's own proxies (the exposed listener and the tunnel) append the
client to `X-Forwarded-For`, so its last entry is trusted; without one, the
connection's address is the client's. A loopback client is this machine. A
remote profile's requests come from the forwarding machine, so they are never
the same machine.

Tests cover the switch, screenshots, actions, the clipboard, same-machine
detection, and a viewer's stream, input and acknowledgements through a remote
profile to a second instance.

## The Screen tab

**Where it is offered.** A **Screen** button sits beside **Terminal** under
the prompt, and the command palette has **Screen**. Both appear only when the
server has a display (`available`) and the browser is on another machine
(`!same_machine`). That covers a remote profile, `--remote`, and a browser or
tablet using the exposed listener or the tunnel. There is one Screen tab per
machine. Opening it from another project moves it there. Like a terminal it
is not saved with the strip. Its stream runs only while the tab is in front.
With sharing off, the tab says so and offers **Turn on screen sharing**.
Settings › Server has the same switch, in desktop and web launches.

**Drawing** (`web/src/screen.js`, `ScreenView.vue`). `chooseCodec` takes the
server's first codec that `VideoDecoder.isConfigSupported` accepts (VP9, then
VP8), else JPEG. VP8/VP9 frames go through WebCodecs with
`optimizeForLatency` and are drawn onto a canvas. Delta frames before the
first keyframe are acknowledged but not decoded. A decoder error closes the
decoder and asks for a keyframe. JPEG bands are decoded with
`createImageBitmap` and drawn at their rows. The canvas keeps the screen's
shape at the largest size the pane allows. The toolbar shows codec, frames
drawn per second, bitrate and screen size.

**Pointer.** With a mouse over the canvas, the pointer is the browser's own,
wearing the remote shape as a CSS cursor, so it moves with no delay. Otherwise
(touch, or an agent moving it) the remote shape is drawn where the remote
pointer is. Moves are coalesced to the latest; buttons, wheel steps
(about 60 px or 3 lines a step) and moves go in order.

**Touch.** Trackpad mode (default) is a laptop trackpad: one finger moves
the pointer by its slide, a tap clicks where the pointer is, a two-finger tap
right-clicks, two fingers sliding scroll, and tap-then-hold drags. The other
mode clicks where the finger touches. A keyboard button focuses the hidden
input so a touch keyboard opens; its `input` events become text, Backspace and
Enter.

**Keyboard.** While the canvas has focus, every key goes to the remote, and
the app's own shortcuts do not fire, as in a terminal. A character typed
without Ctrl, Cmd or (outside a Mac) Alt travels as text, so the two layouts
need not match. AltGr and a Mac's Option type characters. Everything else
travels as a key pressed and released. On a Mac viewing a machine that is not
one, Cmd travels as Ctrl, so Cmd+C copies. Keys still held when focus leaves
are released. Browsers keep some chords for themselves (Ctrl+W in Chrome, and
Alt+Tab or Cmd+Tab everywhere).

**Clipboard.** Ctrl/Cmd+V lets the browser fire `paste`, which carries this
machine's text without a permission prompt. That text is set as the remote
clipboard, then the V keystroke follows, so the remote program pastes it. A
paste with no keystroke behind it (a touch keyboard's menu) is typed. When the
remote clipboard changes, the text is written to this machine's clipboard.
Where the browser refuses without a recent click (Safari), a **Copy remote
clipboard** button does it on the next click.

**Full screen.** The toolbar button uses the Fullscreen API on the tab, with
a CSS stand-in where that is missing (iPhone). In full screen the toolbar
floats faintly at the top and its button exits; Esc also exits native full
screen.

Tests: `web/src/screen.test.js` covers the input queue (one request in flight,
coalescing, newest acknowledgement), key translation (text against keys, Cmd
as Ctrl, AltGr, Option, released typed keys), pointer, fit and wheel
arithmetic, every trackpad gesture, and codec choice.
`e2e/tests/screen.spec.js` runs an Xvfb with an `xev` window per test. With
VP9 and with JPEG it turns sharing on from the tab and checks drawn pixels, a
right click and a key reaching xev, clipboard both ways, full screen in and
out, and 0 fps on a still screen. It also checks a tablet tap clicking where
it lands and the touch keyboard button, and that the machine itself is not
offered its own screen.

`make test-desktop-screen` runs the real `screen.js` in a real Wails/WebKitGTK
window, through the same loopback proxy the app uses, against a fake screen.
It checks the screen is drawn and input comes back, and logs the codec the
window picked. WebKitGTK 2.52 picks VP9: the window is a secure context, so
WebCodecs and the clipboard API are there.
