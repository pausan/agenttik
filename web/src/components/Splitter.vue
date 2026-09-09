<script setup>
/* The divider between a sidebar and the centre. Dragging it sets the width in
   the store and releasing saves it, so the panels stay where they were put.

   The visible line is one pixel, which is impossible to grab, so the handle
   spreads a few pixels over both neighbours without taking any layout space. */
import { S, saveLayout, setSidebarWidth } from "../store";

const props = defineProps({ side: { type: String, required: true } });

let dragging = false;

function width(e) {
  return props.side === "left" ? e.clientX : window.innerWidth - e.clientX;
}

function onDown(e) {
  dragging = true;
  // Capture, or the drag stops the moment the pointer leaves the line.
  e.currentTarget.setPointerCapture(e.pointerId);
}

function onMove(e) {
  if (dragging) setSidebarWidth(props.side, width(e));
}

function onUp(e) {
  if (!dragging) return;
  dragging = false;
  e.currentTarget.releasePointerCapture(e.pointerId);
  saveLayout();
}

/* The keyboard reaches it too: this is part of the layout, not decoration. */
function nudge(by) {
  setSidebarWidth(props.side, S.layout[props.side] + (props.side === "left" ? by : -by));
  saveLayout();
}
</script>

<template>
  <div
    class="relative touch-none border-l border-default select-none"
    role="separator"
    aria-orientation="vertical"
    :aria-label="`Resize the ${side} panel`"
    :aria-valuenow="S.layout[side]"
    tabindex="0"
    @pointerdown="onDown"
    @pointermove="onMove"
    @pointerup="onUp"
    @pointercancel="onUp"
    @keydown.left.prevent="nudge(-16)"
    @keydown.right.prevent="nudge(16)"
  >
    <span
      class="absolute inset-y-0 -left-[3px] -right-[3px] z-10 cursor-col-resize hover:bg-primary/40"
    />
  </div>
</template>
