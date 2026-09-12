// Decimal units match the KB/MB labels. Round once, promoting a rounded
// 1000.0 to the next unit so the boundary still reads naturally.
export function fileSize(bytes) {
  if (!Number.isFinite(bytes) || bytes < 0) return "";
  const units = ["B", "KB", "MB", "GB", "TB", "PB", "EB"];
  let unit = 0;
  while (bytes >= 999.95 && unit < units.length - 1) {
    bytes /= 1000;
    unit++;
  }
  return `${bytes.toFixed(1)} ${units[unit]}`;
}

export function fileInfoLabel(info, dimensions) {
  const size = fileSize(info?.size);
  if (!size) return "";
  const image = info.image || dimensions;
  const pixels = image?.width > 0 && image?.height > 0 ? `, ${image.width}x${image.height}` : "";
  const depth = pixels && image.bits > 0 ? ` ${image.bits}-bit` : "";
  return `(${size}${pixels}${depth})`;
}
