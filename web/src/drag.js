let source;
let onDrop;
let dropRow;
// An object, as Node does not remove a listener by a bare true.
const CAPTURE = { capture: true };

function dropAfter(event, row) {
  const box = row.getBoundingClientRect();
  return row.getAttribute("role") === "tab"
    ? event.clientX >= box.left + box.width / 2
    : event.clientY >= box.top + box.height / 2;
}

/* The rows the source can land beside: its siblings with a drag index in the
   same group. A row without an index, like the orchestrator, is skipped. */
function dropRows() {
  return [...(source.parentElement?.children || [])].filter((row) =>
    row.dataset.dragIndex !== undefined && row.dataset.dragGroup === source.dataset.dragGroup);
}

/* The insertion edge nearest the pointer, wherever the pointer is. Past the
   end of the list, or over something that takes no drops, that is still the
   closest edge there is, so the line always has somewhere to be. */
function nearestEdge(event) {
  const rows = dropRows();
  const row = rows.find((candidate) => !dropAfter(event, candidate));
  return row ? { row, after: false } : { row: rows.at(-1), after: true };
}

/* Keep the list still and mark the edge. The whole window takes the drag, so
   this runs first and nothing under the pointer sees it. */
function showDropPosition(event) {
  event.preventDefault();
  event.stopPropagation();
  const { row, after } = nearestEdge(event);
  if (dropRow !== row) clearDropPosition();
  if (!row) return;
  dropRow = row;
  const side = after ? "after" : "before";
  if (row.dataset.dropPosition !== side) row.dataset.dropPosition = side;
}

function clearDropPosition() {
  dropRow?.removeAttribute("data-drop-position");
  dropRow = null;
}

/* from and to are positions in the list, with to the one the source takes
   once it has left its own. over and after name the marked edge. */
function dropOnEdge(event) {
  event.preventDefault();
  event.stopPropagation();
  const { row, after } = nearestEdge(event);
  const from = Number(source.dataset.dragIndex);
  const drop = onDrop;
  endDrag();
  if (!row) return;
  const over = Number(row.dataset.dragIndex);
  const boundary = over + (after ? 1 : 0);
  const to = boundary - (from < boundary ? 1 : 0);
  if (from !== to) drop(from, to, over, after);
}

function endDrag() {
  clearDropPosition();
  source = onDrop = null;
  document.removeEventListener("dragover", showDropPosition, CAPTURE);
  document.removeEventListener("drop", dropOnEdge, CAPTURE);
  document.removeEventListener("dragend", endDrag, CAPTURE);
}

/* Use the browser's translucent preview, with the source row left in place.
   A favourite's grip passes its whole row as the preview. Text dragged out of
   a row that takes no part in ordering stays the browser's. */
export function beginDrag(event, drop, preview = event.currentTarget) {
  const row = event.currentTarget.closest("[data-drag-index]");
  if (!row) return;
  endDrag();
  source = row;
  onDrop = drop;
  document.addEventListener("dragover", showDropPosition, CAPTURE);
  document.addEventListener("drop", dropOnEdge, CAPTURE);
  document.addEventListener("dragend", endDrag, CAPTURE);
  event.dataTransfer.effectAllowed = "move";
  // Firefox does not start a drag unless the transfer carries data.
  event.dataTransfer.setData("text/plain", row.dataset.dragIndex);
  const box = preview.getBoundingClientRect();
  event.dataTransfer.setDragImage(preview, event.clientX - box.left, event.clientY - box.top);
}
