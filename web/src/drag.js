let dragImage;
let dropRow;
let dropSide = "before";

/* The marker moves with the live-reordered row, without measuring layout on
   every hover. Hovering that row again keeps the last insertion edge. */
export function showDropPosition(from, to) {
  if (!dropRow) return;
  if (from !== to) dropSide = from < to ? "after" : "before";
  if (dropRow.dataset.dropPosition !== dropSide) dropRow.dataset.dropPosition = dropSide;
}

function checkDropTarget(e) {
  if (!e.defaultPrevented) dropRow?.removeAttribute("data-drop-position");
}

function endDrag() {
  dropRow?.removeAttribute("data-drop-position");
  dropRow = null;
  document.removeEventListener("dragover", checkDropTarget);
  document.removeEventListener("drop", endDrag, true);
  document.removeEventListener("dragend", endDrag, true);
}

/* Rows already move under the pointer. A transparent preview keeps the
   browser's native drag image from fading or flying back after release. */
export function beginDrag(e, value, row = e.currentTarget) {
  endDrag();
  dropRow = row;
  dropSide = "before";
  document.addEventListener("dragover", checkDropTarget);
  document.addEventListener("drop", endDrag, true);
  document.addEventListener("dragend", endDrag, true);
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
