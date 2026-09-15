function platformName() {
  if (typeof navigator === "undefined") return "";
  return navigator.userAgentData?.platform || navigator.platform || "";
}

export function isMacOS(platform = platformName()) {
  return /^mac/i.test(platform);
}

// WebKitGTK paints native overlay scrollbars above portalled UI when Wails
// disables GPU acceleration. CSS scrollbars stay in the page's paint order.
export function needsCSSScrollbars(platform = platformName(), runtime = globalThis.window?.runtime) {
  return /^linux/i.test(platform) && !!runtime;
}

export const MACOS = isMacOS();
export const PRIMARY_MODIFIER = MACOS ? "Cmd" : "Ctrl";

export function primaryChord(key, macOS = MACOS) {
  return `${macOS ? "Cmd" : "Ctrl"}+${key}`;
}

// Meta was the recorder's old name for Command. Keep saved bindings working
// while spelling the key the way macOS users see it.
export function platformChord(chord, macOS = MACOS) {
  if (!macOS) return chord;
  return chord
    .split("+")
    .map((part) => (part === "Meta" ? "Cmd" : part))
    .join("+");
}

export function desktopChord(chord, macOS = MACOS) {
  if (!macOS) return chord;
  return platformChord(chord.replace(/^Ctrl(?=\+)/, "Cmd"), true);
}
