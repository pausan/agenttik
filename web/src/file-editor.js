// Count without allocating one string per line on each keystroke.
export function lineCount(text) {
  let count = 1;
  for (let at = text.indexOf("\n"); at >= 0; at = text.indexOf("\n", at + 1)) count++;
  return count;
}

export function largeFile(text, lines = lineCount(text)) {
  return text.length > 400_000 || lines >= 10_000;
}

// Large editors do not wrap, so a selected line needs no DOM mirror.
export function revealText(area, start, end = start) {
  area.setSelectionRange(start, end);
  const style = getComputedStyle(area);
  const line = lineCount(area.value.slice(0, start)) - 1;
  area.scrollTop = Math.max(0, line * parseFloat(style.lineHeight) - area.clientHeight / 3);
  const canvas = area.ownerDocument.createElement("canvas");
  const context = canvas.getContext("2d");
  context.font = style.font;
  const from = area.value.lastIndexOf("\n", start - 1) + 1;
  const column = area.value.slice(from, start).replace(/\t/g, "  ");
  area.scrollLeft = Math.max(0, context.measureText(column).width - area.clientWidth / 3);
}
