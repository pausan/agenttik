# Video previews

`.mp4`, `.m4v`, `.mov`, `.webm`, `.ogv`, `.mkv`, `.avi`, `.mpg`, `.mpeg`,
`.wmv`, `.flv`, `.3gp` and `.m2ts`, matched without case sensitivity, offer
Diff, Preview and [Hex](088-binary-files.md) like [fonts](058-font-preview.md).
Preview and Diff have no text editor or line count. Hex previews the first
2 MiB and can replace bytes in files up to that limit. Videos open in Preview
unless the remembered view is Diff, which is git's binary line. `.ts` and
`.mts` are left out: they are TypeScript far more often than MPEG streams. The header shows the size and the picture's
dimensions, read off the element.

## The player

`VideoPreview` draws the video filling the pane on black, with a bar beneath:
play/pause, time as `m:ss` (or `h:mm:ss`) over the length, a seek slider,
speed (0.25× to 4×), mute and volume, loop and full screen. A click on the
picture plays or pauses it and a double click toggles full screen. Keys act
only while the player itself has focus, so they never reach the app's
shortcuts or take arrows from the sliders: Space or K plays and pauses, ←/→
seek 5 s, J/L seek 10 s, Home goes to the start, ↑/↓ change volume, M mutes,
F is full screen, `<` and `>` step the speed. Volume and mute carry over to the
next video; speed, loop and position reset with the file.

## Streaming and conversion

`GET /api/projects/:id/video?path=` serves the working-tree file by HTTP range
from an open file handle, so a seek reads only what it lands on and nothing is
held in memory. There is no 16 MiB cap and no `rev`. The content type comes
from an allowlist, with `no-store` and `nosniff`, as for the raw endpoint.

A browser plays only some of these formats. When the element reports a decode
or unsupported-format error — or opens the file but has no picture to show
while ffprobe says there is one — the player asks
`GET /api/projects/:id/video-info?path=`, which runs ffprobe for the length,
size and codecs and says whether ffmpeg is installed. With ffmpeg it switches
to `video?path=&transcode=1&start=`: ffmpeg converts from `start` seconds to
VP9 and Opus in WebM (realtime settings, about five times faster than playback
at 1080p), piped straight to the response. VP9/Opus rather than H.264/AAC
because every Chromium, Firefox and WebKitGTK build decodes them. The bar
says “converted”. Without ffmpeg the player says it is needed.

A converted stream has no length of its own and no ranges, so the slider uses
ffprobe's length and the displayed time adds the stream's start. A seek inside
what has already arrived (`buffered`) moves within the stream; anything else
starts a new conversion from there. `seekable` is not used: browsers report a
live stream seekable to Infinity and then stall on a seek past its end. The
slider commits a converted seek on release rather than on every step. ffmpeg
stops when the client drops the connection — which the browser does on every
new source — or when the server shuts down.

## The desktop window

WebKitGTK's media player cannot read from Wails' custom URI scheme at all —
every source fails as unsupported, whatever the response — while the same
bytes over plain HTTP play, AVI included, through GStreamer. So each desktop
window starts a media bridge (`desktop_media.go`): a loopback listener whose
only route is `/media/<random token>/api/projects/:id/video`, forwarded to the
window's own handler, so profiles and remote connections behave as in the
window. The window answers `GET /desktop/media` with the bridge's base URL;
`mediaBase()` asks once, and in a browser, where that path is just the app's
page, the base is the page's own origin. The 128-bit token is new each
launch, so no other local process or web page can read project files through
the bridge, and nothing but the video route is forwarded.

## Tests

Go tests cover whole and ranged reads, an unsatisfiable range, rejection of
non-video paths, and, with ffmpeg installed, ffprobe metadata and a real
conversion (codecs, start offset, odd-width rounding). Unit tests cover time
formatting, speed steps and range checks. The browser test plays a native WebM
and a converted AVI, checking speed, volume, mute, loop, seeking, keys, reset
of speed and carry-over of volume between files. A Go test checks the bridge
forwards only the video route under its token. `make test-desktop-video` plays
a clip in the real Wails/WebKit window: from the custom scheme it must still
fail — or the bridge can go — and through the bridge it must seek and play.
