// Zoom steps for a previewed image, smallest first. 1 is the image's own pixels.
export const ZOOMS = [0.05, 0.1, 0.25, 0.33, 0.5, 0.67, 0.75, 1, 1.25, 1.5, 2, 3, 4, 6, 8, 12, 16, 24, 32];

export const clampZoom = (z) => Math.min(ZOOMS.at(-1), Math.max(ZOOMS[0], z));

/* stepZoom moves one step along ZOOMS; a scale between two steps, as a fitted
   image or a pinch leaves it, moves to the nearer one in that direction. */
export function stepZoom(scale, dir) {
  if (dir > 0) return ZOOMS.find((z) => z > scale + 1e-9) ?? ZOOMS.at(-1);
  return ZOOMS.findLast((z) => z < scale - 1e-9) ?? ZOOMS[0];
}

/* zoomKey reads the browser's own zoom chords: Ctrl or Cmd with +, - or 0.
   "=" and "_" count because + and - share their keys. */
export function zoomKey(e) {
  if (!(e.ctrlKey || e.metaKey) || e.altKey) return null;
  if (e.key === "+" || e.key === "=") return "in";
  if (e.key === "-" || e.key === "_") return "out";
  if (e.key === "0") return "reset";
  return null;
}

/* wheelZoom turns a wheel event into a scale factor. A mouse notch and a
   trackpad pinch, which arrives as a wheel with Ctrl held, differ a hundred
   times in delta, so each event is capped to keep a notch one comfortable
   step while a pinch stays smooth. */
export function wheelZoom(deltaY, deltaMode = 0) {
  const px = deltaMode === 1 ? deltaY * 16 : deltaMode === 2 ? deltaY * 400 : deltaY;
  return Math.exp(-Math.max(-50, Math.min(50, px)) * 0.006);
}
