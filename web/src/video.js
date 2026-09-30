// Playback speeds on offer, slowest first.
export const SPEEDS = [0.25, 0.5, 0.75, 1, 1.25, 1.5, 1.75, 2, 3, 4];

/* formatTime writes seconds as m:ss, or h:mm:ss once an hour is reached. A
   length not yet known reads as dashes rather than as a false zero. */
export function formatTime(seconds) {
  if (!Number.isFinite(seconds) || seconds < 0) return "–:––";
  const s = Math.floor(seconds);
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const ss = String(s % 60).padStart(2, "0");
  return h ? `${h}:${String(m).padStart(2, "0")}:${ss}` : `${m}:${ss}`;
}

/* stepSpeed moves one step along SPEEDS; a rate between two steps moves to
   the nearer one in that direction. */
export function stepSpeed(rate, dir) {
  if (dir > 0) return SPEEDS.find((s) => s > rate + 1e-9) ?? SPEEDS.at(-1);
  return SPEEDS.findLast((s) => s < rate - 1e-9) ?? SPEEDS[0];
}

/* inRanges says whether a time falls inside a TimeRanges. A converted
   stream can seek only within what has arrived: browsers call a live stream
   seekable to Infinity and then stall on a seek past its end. */
export function inRanges(ranges, time) {
  for (let i = 0; i < ranges.length; i++) {
    if (time >= ranges.start(i) && time <= ranges.end(i)) return true;
  }
  return false;
}

/* mediaBase is where videos stream from: the page's own origin in a browser,
   a loopback bridge in the desktop window, whose WebKit cannot play media
   from its custom scheme. Asked once; anything but the window's answer means
   the page's origin. */
let base;
export function mediaBase() {
  base ??= fetch("/desktop/media", { cache: "no-store" })
    .then((r) => (r.ok ? r.json() : {}))
    .then((b) => (typeof b?.base === "string" && b.base.startsWith("http://127.0.0.1:") ? b.base : ""))
    .catch(() => "");
  return base;
}
