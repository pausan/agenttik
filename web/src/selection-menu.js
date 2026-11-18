import { onUnmounted, shallowRef } from "vue";

import { copyText } from "./store";

function selectionAt(element) {
  const selection = window.getSelection();
  if (selection?.rangeCount && !selection.isCollapsed && selection.getRangeAt(0).intersectsNode(element)) {
    return selection;
  }
}

// WebKit can clear the range on a secondary click before contextmenu fires.
export function preserveSelection(event) {
  if (selectionAt(event.currentTarget)) event.preventDefault();
}

export function useSelectionMenu() {
  const selectionItems = shallowRef([]);
  let range = null;

  function aimSelection(event) {
    const selection = selectionAt(event.currentTarget);
    const text = selection?.toString();
    range = text ? selection.getRangeAt(0).cloneRange() : null;
    selectionItems.value = text
      ? [{ label: "Copy", icon: "i-lucide-copy", onSelect: () => copyText(text) }]
      : [];
    return !!text;
  }

  // WebKit can also clear the range when the menu or an item takes focus.
  function restoreSelection(event) {
    if (!range || range.collapsed || !event.target.closest('[role="menu"]')) return;
    const selection = window.getSelection();
    if (selection?.isCollapsed) {
      selection.setBaseAndExtent(range.startContainer, range.startOffset, range.endContainer, range.endOffset);
    }
  }

  function menuOpen(open) {
    if (open && range) {
      document.addEventListener("focusin", restoreSelection);
    } else {
      document.removeEventListener("focusin", restoreSelection);
      range = null;
    }
  }
  onUnmounted(() => menuOpen(false));

  return { selectionItems, aimSelection, menuOpen };
}
