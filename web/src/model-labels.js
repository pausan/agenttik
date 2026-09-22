/* Model names as the pickers draw them.

   An API catalogue names a model in full, vendor included: an Anthropic
   connection offers "Anthropic: Claude Sonnet 4.5", OpenRouter offers
   "anthropic/claude-sonnet-4.5". The vendor earns its place where it tells two
   rows apart and is pure noise where the connection already says it, so it is
   dropped only when the provider's own name contains it. */

const VENDOR_PREFIX = /^([^:/]{2,24}?)\s*[:/]\s*/;
const squash = (text) => String(text || "").toLowerCase().replace(/[^a-z0-9]/g, "");

export function compactModelLabel(providerLabel, label) {
  const match = VENDOR_PREFIX.exec(label || "");
  if (!match) return label || "";
  const vendor = squash(match[1]);
  return vendor && squash(providerLabel).includes(vendor)
    ? label.slice(match[0].length)
    : label;
}
