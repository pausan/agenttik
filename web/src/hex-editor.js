export const HEX_PAGE_BYTES = 512;

export function parseHex(value) {
  const hex = value.replace(/\s/g, "").toLowerCase();
  if (!hex || hex.length % 2 || !/^[0-9a-f]+$/.test(hex)) return null;
  return hex;
}

export function replaceHex(hex, offset, bytes) {
  if (!Number.isInteger(offset) || offset < 0 || !bytes || offset * 2 + bytes.length > hex.length) return null;
  return hex.slice(0, offset * 2) + bytes + hex.slice(offset * 2 + bytes.length);
}

// Only the current page is decoded and rendered.
export function hexRows(hex, from, count = HEX_PAGE_BYTES) {
  const rows = [];
  const end = Math.min(hex.length / 2, from + count);
  for (let offset = from; offset < end; offset += 16) {
    const bytes = hex.slice(offset * 2, Math.min(offset + 16, end) * 2).match(/../g) || [];
    const ascii = bytes.map(byte => {
      const code = parseInt(byte, 16);
      return code >= 32 && code <= 126 ? String.fromCharCode(code) : ".";
    }).join("");
    rows.push({ offset, bytes, ascii });
  }
  return rows;
}
