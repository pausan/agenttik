let dragImage;

/* Rows already move under the pointer. A transparent preview keeps the
   browser's native drag image from fading or flying back after release. */
export function beginDrag(e, value) {
  e.dataTransfer.effectAllowed = "move";
  // Firefox does not start a drag unless the transfer carries data.
  e.dataTransfer.setData("text/plain", String(value));

  if (!dragImage) {
    dragImage = document.createElement("canvas");
    dragImage.width = dragImage.height = 1;
    dragImage.style.cssText = "position:fixed;top:0;left:0;pointer-events:none";
    dragImage.setAttribute("aria-hidden", "true");
    // Keep it in the viewport so every browser can capture the empty image.
    document.body.append(dragImage);
  }
  e.dataTransfer.setDragImage(dragImage, 0, 0);
}
