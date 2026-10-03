<script setup>
/* One picture, fitted to the box it is given: a preview uses one of these, an
   image diff uses two.

   The size underneath is read off the element once the browser has decoded
   it. In a diff it is most of the answer, since an image that changed shape
   says so before anything else does. For the same reason a raster image is
   never scaled up: two pictures both stretched to their pane would look the
   same size when they are not.
   A vector has no pixels to blur, so `fit` lets an SVG fill the pane.

   A `zoomable` frame also zooms: by its buttons, Ctrl with the wheel or a
   trackpad pinch, and a two-finger pinch on a touch screen. In the desktop
   window Ctrl or Cmd with +, - and 0 do too; in a browser those stay the
   browser's own page zoom. The point under the pointer or the fingers stays
   put, and a picture larger than its pane scrolls. Fit is where it starts and
   where the middle button returns to; 0 is the image's own pixels.

   A diff of two pictures the same size hands both frames one `view`, so they
   zoom and scroll as one. Only one of them, `keys`, answers the chords.

   A source that fails to load is ordinary on one side of a diff — a file just
   added, or one deleted — so it reads as a note rather than as a broken
   image. */
import { computed, nextTick, onUnmounted, reactive, ref, shallowRef, watch } from "vue";

import { clampZoom, stepZoom, wheelZoom, zoomKey } from "../image-zoom";
import { primaryChord } from "../platform";

const props = defineProps({
  src: { type: String, required: true },
  label: { type: String, default: "" },
  fit: { type: Boolean, default: false },
  zoomable: { type: Boolean, default: false },
  keys: { type: Boolean, default: true },
  // { scale, left, top } shared with another frame; a frame alone keeps its own.
  view: { type: Object, default: null },
  missing: { type: String, default: "Cannot show this image." },
});

const size = ref("");
const emit = defineEmits(["dimensions"]);
const failed = ref(false);
const pane = ref(null);
const img = ref(null);
// scale is null when fitted to the pane, else the scale against the image's own size.
const own = reactive({ scale: null, left: 0, top: 0 });
const state = computed(() => props.view ?? own);
const scale = computed(() => state.value.scale);
// The size a scale of 1 draws at. An SVG with only a viewBox has none of its
// own, so it is measured as fitted instead.
const base = shallowRef(null);
const desktop = () => !!window.runtime;

/* The same component serves the next file opened in the tab, so the size, the
   failure and the zoom belong to the source that produced them. */
watch(
  () => props.src,
  () => {
    size.value = "";
    failed.value = false;
    state.value.scale = null;
    base.value = null;
  },
);

/* An SVG carrying only a viewBox has a shape but no intrinsic size, and the
   300×150 the browser falls back to is not the file's own. */
function onLoad(e) {
  const { naturalWidth: w, naturalHeight: h } = e.target;
  size.value = w && h ? `${w} × ${h}` : "";
  if (w && h) base.value = { width: w, height: h };
  emit("dimensions", { width: w, height: h });
}

const zoomed = computed(() => scale.value !== null && base.value !== null);
const drawn = computed(() =>
  zoomed.value ? { width: `${base.value.width * scale.value}px`, height: `${base.value.height * scale.value}px` } : undefined,
);
const percent = computed(() => (scale.value === null ? "Fit" : `${Math.round(scale.value * 100)}%`));

/* The scale the picture is drawn at now, fitted or not. Fitted, it is measured
   afresh, since the pane may have changed size since the last zoom. */
function current() {
  if (scale.value !== null) return scale.value;
  const el = img.value;
  const width = el?.naturalWidth || el?.clientWidth;
  const height = el?.naturalHeight || el?.clientHeight;
  if (!width || !height) return 1;
  base.value = { width, height };
  return Math.min(el.clientWidth / width, el.clientHeight / height);
}

/* Zooms to `next`, keeping the point at (x, y) on screen where it was; the
   pane's middle when no point is given. */
async function zoomTo(next, x, y) {
  const el = img.value;
  current();
  if (!el || !base.value) return;
  const box = pane.value.getBoundingClientRect();
  x ??= box.left + box.width / 2;
  y ??= box.top + box.height / 2;
  const before = el.getBoundingClientRect();
  const fx = (x - before.left) / before.width;
  const fy = (y - before.top) / before.height;
  state.value.scale = next === null ? null : clampZoom(next);
  await nextTick();
  const after = el.getBoundingClientRect();
  pane.value.scrollLeft += after.left + fx * after.width - x;
  pane.value.scrollTop += after.top + fy * after.height - y;
  onScroll();
}

/* A shared view carries the scroll across. Each frame writes where it is and
   follows where the other went; a position already held ends the exchange. */
function onScroll() {
  if (!props.view || !pane.value) return;
  props.view.left = pane.value.scrollLeft;
  props.view.top = pane.value.scrollTop;
}

watch(
  () => props.view && [props.view.left, props.view.top],
  (to) => {
    const el = pane.value;
    if (!to || !el) return;
    if (Math.abs(el.scrollLeft - to[0]) >= 1) el.scrollLeft = to[0];
    if (Math.abs(el.scrollTop - to[1]) >= 1) el.scrollTop = to[1];
  },
);

const zoomIn = () => zoomTo(stepZoom(current(), 1));
const zoomOut = () => zoomTo(stepZoom(current(), -1));

/* The chords work wherever focus is, except in a field that types them, so
   they act on the picture in front without a click on it first. */
function onKey(e) {
  const action = zoomKey(e);
  if (!action || !desktop() || e.defaultPrevented || failed.value) return;
  const t = e.target;
  if (t instanceof HTMLElement && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))) return;
  if (!pane.value?.offsetParent) return;
  e.preventDefault();
  if (action === "in") zoomIn();
  else if (action === "out") zoomOut();
  else zoomTo(1);
}

// Ctrl with the wheel zooms; a trackpad pinch arrives as exactly that.
function onWheel(e) {
  if (!e.ctrlKey && !e.metaKey) return;
  e.preventDefault();
  zoomTo(current() * wheelZoom(e.deltaY, e.deltaMode), e.clientX, e.clientY);
}

/* Two fingers on a touch screen. The pane keeps one-finger panning for its
   own scroll, and leaves pinching to this. */
let pinch = null;
const spread = (t) => Math.hypot(t[0].clientX - t[1].clientX, t[0].clientY - t[1].clientY);

function onTouchStart(e) {
  if (e.touches.length === 2) pinch = { distance: spread(e.touches) || 1, scale: current() };
}

function onTouchMove(e) {
  if (!pinch || e.touches.length !== 2) return;
  const [a, b] = e.touches;
  zoomTo(pinch.scale * (spread(e.touches) / pinch.distance), (a.clientX + b.clientX) / 2, (a.clientY + b.clientY) / 2);
}

function onTouchEnd(e) {
  if (e.touches.length < 2) pinch = null;
}

// A diff learns only once both pictures load whether they zoom together.
watch(
  () => props.zoomable && props.keys,
  (on) => (on ? window.addEventListener("keydown", onKey) : window.removeEventListener("keydown", onKey)),
  { immediate: true },
);
onUnmounted(() => window.removeEventListener("keydown", onKey));
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 flex-col items-center gap-2">
    <span v-if="label" class="shrink-0 font-mono text-xs text-dimmed">{{ label }}</span>
    <p v-if="failed" class="flex-1 py-5 text-center text-dimmed">{{ missing }}</p>
    <div
      v-else
      ref="pane"
      class="flex min-h-0 w-full flex-1"
      :class="zoomable && 'image-zoom-pane overflow-auto'"
      @wheel="zoomable && onWheel($event)"
      @scroll.passive="zoomable && onScroll()"
      @touchstart.passive="zoomable && onTouchStart($event)"
      @touchmove.passive="zoomable && onTouchMove($event)"
      @touchend.passive="zoomable && onTouchEnd($event)"
      @touchcancel.passive="zoomable && onTouchEnd($event)"
    >
      <img
        ref="img"
        class="checkerboard m-auto object-contain"
        :class="zoomed ? 'max-w-none shrink-0' : ['min-h-0 min-w-0', fit ? 'h-full w-full' : 'max-h-full max-w-full']"
        :style="drawn"
        :src="src"
        :alt="label || 'preview'"
        @load="onLoad"
        @error="failed = true"
      />
    </div>
    <div class="flex h-4 shrink-0 items-center gap-3 font-mono text-xs text-dimmed">
      <span>{{ size }}</span>
      <span v-if="zoomable && !failed" class="flex items-center gap-1">
        <button type="button" class="hover:text-default" :title="desktop() ? `Zoom out (${primaryChord('-')})` : 'Zoom out'" aria-label="Zoom out" @click="zoomOut">
          <UIcon name="i-lucide-minus" class="block size-3.5" />
        </button>
        <button type="button" class="min-w-10 hover:text-default" title="Fit to the pane" @click="zoomTo(null)">
          {{ percent }}
        </button>
        <button type="button" class="hover:text-default" :title="desktop() ? `Zoom in (${primaryChord('+')})` : 'Zoom in'" aria-label="Zoom in" @click="zoomIn">
          <UIcon name="i-lucide-plus" class="block size-3.5" />
        </button>
      </span>
    </div>
  </div>
</template>

<style scoped>
/* Pinching is the picture's, not the page's; one finger still scrolls it. */
.image-zoom-pane {
  touch-action: pan-x pan-y;
}
</style>
