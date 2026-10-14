// Coordinates are fractions of the original image, independent of its display size.
export function imagePoint(x, y, rect) {
  return {
    x: Math.max(0, Math.min(1, (x - rect.left) / rect.width)),
    y: Math.max(0, Math.min(1, (y - rect.top) / rect.height)),
  };
}

export function imageArea(start, end) {
  return {
    x: Math.min(start.x, end.x), y: Math.min(start.y, end.y),
    width: Math.abs(end.x - start.x), height: Math.abs(end.y - start.y),
  };
}

export function areaQuote(label, area, image) {
  const w = image.naturalWidth, h = image.naturalHeight;
  return `Circled area of ${label}: x ${Math.round(area.x * w)}–${Math.round((area.x + area.width) * w)}, y ${Math.round(area.y * h)}–${Math.round((area.y + area.height) * h)} in the ${w} × ${h} image. The attached image marks this area in red.`;
}

// Bound the canvas allocation even when a source image is very large.
export async function markedImage(image, area) {
  const ratio = Math.min(1, 2048 / Math.max(image.naturalWidth, image.naturalHeight));
  const canvas = document.createElement("canvas");
  canvas.width = Math.max(1, Math.round(image.naturalWidth * ratio));
  canvas.height = Math.max(1, Math.round(image.naturalHeight * ratio));
  const ctx = canvas.getContext("2d");
  ctx.drawImage(image, 0, 0, canvas.width, canvas.height);
  ctx.beginPath();
  ctx.ellipse((area.x + area.width / 2) * canvas.width, (area.y + area.height / 2) * canvas.height,
    area.width * canvas.width / 2, area.height * canvas.height / 2, 0, 0, Math.PI * 2);
  ctx.strokeStyle = "white";
  ctx.lineWidth = 7;
  ctx.stroke();
  ctx.strokeStyle = "#ef4444";
  ctx.lineWidth = 4;
  ctx.stroke();
  const encode = (type, quality) => new Promise((resolve, reject) => canvas.toBlob(
    (blob) => blob ? resolve(blob) : reject(new Error("Could not mark this image.")), type, quality,
  ));
  const png = await encode("image/png");
  return png.size <= 4 * 1024 * 1024 ? png : encode("image/jpeg", 0.85);
}
