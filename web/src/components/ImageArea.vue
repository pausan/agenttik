<script setup>
import { computed, onMounted, onUnmounted, ref, shallowRef } from "vue";
import { imageArea, imagePoint } from "../image-area";

defineProps({ src: { type: String, required: true } });
const emit = defineEmits(["select"]);
const pane = ref(null);
const image = ref(null);
const area = shallowRef(null);
const bounds = shallowRef(null);
const loaded = ref(false);
const failed = ref(false);
let drag = null;
let observer;

function measure() {
  if (!image.value || !pane.value) return;
  const picture = image.value.getBoundingClientRect();
  const box = pane.value.getBoundingClientRect();
  bounds.value = { x: picture.left - box.left, y: picture.top - box.top, width: picture.width, height: picture.height };
}

const ellipse = computed(() => {
  const a = area.value, b = bounds.value;
  return a && b ? { cx: b.x + (a.x + a.width / 2) * b.width, cy: b.y + (a.y + a.height / 2) * b.height,
    rx: a.width * b.width / 2, ry: a.height * b.height / 2 } : null;
});

function start(e) {
  if (!loaded.value || failed.value || e.button !== 0 || !e.isPrimary) return;
  const rect = image.value.getBoundingClientRect();
  if (e.clientX < rect.left || e.clientX > rect.right || e.clientY < rect.top || e.clientY > rect.bottom) return;
  drag = { id: e.pointerId, start: imagePoint(e.clientX, e.clientY, rect) };
  area.value = null;
  emit("select", null);
  // Touch keeps its implicit capture; mouse and pen drags capture the pane.
  if (e.pointerType !== "touch") {
    pane.value.setPointerCapture(e.pointerId);
    e.preventDefault();
  }
}

function move(e) {
  if (!drag || e.pointerId !== drag.id) return;
  area.value = imageArea(drag.start, imagePoint(e.clientX, e.clientY, image.value.getBoundingClientRect()));
  measure();
}

function finish(e) {
  if (!drag || e.pointerId !== drag.id) return;
  move(e);
  drag = null;
  if (!area.value || area.value.width * bounds.value.width < 8 || area.value.height * bounds.value.height < 8) area.value = null;
  emit("select", area.value ? { area: area.value, image: image.value } : null);
  if (pane.value.hasPointerCapture(e.pointerId)) pane.value.releasePointerCapture(e.pointerId);
}

function cancel() {
  drag = null;
  area.value = null;
  emit("select", null);
}

onMounted(() => {
  observer = new ResizeObserver(measure);
  observer.observe(pane.value);
});
onUnmounted(() => observer?.disconnect());
</script>

<template>
  <div ref="pane" class="relative min-h-0 w-full flex-1 touch-none cursor-crosshair overflow-hidden select-none" aria-label="Circle an image area"
    @pointerdown="start" @pointermove="move" @pointerup="finish" @pointercancel="cancel" @lostpointercapture="drag && cancel()">
    <img ref="image" :src="src" alt="Image to circle" draggable="false"
      class="checkerboard absolute top-1/2 left-1/2 max-h-full max-w-full -translate-x-1/2 -translate-y-1/2"
      @load="loaded = true; measure()" @error="failed = true" />
    <svg v-if="ellipse" class="pointer-events-none absolute inset-0 size-full" aria-label="Selected image area">
      <ellipse v-bind="ellipse" fill="none" stroke="white" stroke-width="6" />
      <ellipse v-bind="ellipse" fill="none" stroke="#ef4444" stroke-width="3" />
    </svg>
    <p v-if="failed" class="absolute inset-0 flex items-center justify-center text-dimmed">Cannot show this image.</p>
  </div>
</template>
