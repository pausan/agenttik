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
