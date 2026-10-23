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
