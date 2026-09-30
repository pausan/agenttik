let dragging = false;
let dropRow;

export function dropAfter(event, row = event.currentTarget) {
  const box = row.getBoundingClientRect();
  return row.getAttribute("role") === "tab"
    ? event.clientX >= box.left + box.width / 2
    : event.clientY >= box.top + box.height / 2;
}

/* Keep the list still. Mark the hovered edge and return the index the source
   will occupy after removing it from its original position. */
export function showDropPosition(event, from, to, row = event.currentTarget) {
  const after = dropAfter(event, row);
  const boundary = to + (after ? 1 : 0);
  const index = boundary - (from < boundary ? 1 : 0);
  if (dropRow !== row || from === index) clearDropPosition();
  if (dragging && from !== index) {
    dropRow = row;
    const side = after ? "after" : "before";
    if (row.dataset.dropPosition !== side) row.dataset.dropPosition = side;
  }
  return index;
}

function clearDropPosition() {
  dropRow?.removeAttribute("data-drop-position");
  dropRow = null;
}

function checkDropTarget(event) {
  if (!event.defaultPrevented) clearDropPosition();
}

function leaveDropTarget(event) {
  if (dropRow?.contains(event.target) && !dropRow.contains(event.relatedTarget)) clearDropPosition();
}

function endDrag() {
  clearDropPosition();
  dragging = false;
  document.removeEventListener("dragover", checkDropTarget);
  document.removeEventListener("dragleave", leaveDropTarget);
  document.removeEventListener("drop", endDrag, true);
  document.removeEventListener("dragend", endDrag, true);
}

/* Use the browser's translucent preview, with the source row left in place.
   A favourite's grip passes its whole row as the preview. */
export function beginDrag(event, value, row = event.currentTarget) {
  endDrag();
  dragging = true;
  document.addEventListener("dragover", checkDropTarget);
  document.addEventListener("dragleave", leaveDropTarget);
  document.addEventListener("drop", endDrag, true);
  document.addEventListener("dragend", endDrag, true);
  event.dataTransfer.effectAllowed = "move";
  // Firefox does not start a drag unless the transfer carries data.
  event.dataTransfer.setData("text/plain", String(value));
  const box = row.getBoundingClientRect();
  event.dataTransfer.setDragImage(row, event.clientX - box.left, event.clientY - box.top);
}
