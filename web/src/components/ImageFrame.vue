<script setup>
/* One picture, fitted to the box it is given: a preview uses one of these, an
   image diff uses two.

   The size underneath is read off the element once the browser has decoded
   it, which is the only place it is known — and in a diff it is most of the
   answer, since an image that changed shape says so before anything else
   does. For the same reason a raster image is never scaled up: two pictures
   both stretched to their pane would look the same size when they are not.
   A vector has no pixels to blur, so `fit` lets an SVG fill the pane.

   A source that fails to load is ordinary on one side of a diff — a file just
   added, or one deleted — so it reads as a note rather than as a broken
   image. */
import { ref, watch } from "vue";

const props = defineProps({
  src: { type: String, required: true },
  label: { type: String, default: "" },
  fit: { type: Boolean, default: false },
  missing: { type: String, default: "Cannot show this image." },
});

const size = ref("");
const failed = ref(false);

/* The same component serves the next file opened in the tab, so the size and
   the failure belong to the source that produced them. */
watch(
  () => props.src,
  () => {
    size.value = "";
    failed.value = false;
  },
);

/* An SVG carrying only a viewBox has a shape but no intrinsic size, and the
   300×150 the browser falls back to is not the file's own. */
function onLoad(e) {
  const { naturalWidth: w, naturalHeight: h } = e.target;
  size.value = w && h ? `${w} × ${h}` : "";
}
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 flex-col items-center gap-2">
    <span v-if="label" class="shrink-0 font-mono text-xs text-dimmed">{{ label }}</span>
    <p v-if="failed" class="flex-1 py-5 text-center text-dimmed">{{ missing }}</p>
    <img
      v-else
      class="checkerboard min-h-0 min-w-0 object-contain"
      :class="fit ? 'h-full w-full' : 'max-h-full max-w-full'"
      :src="src"
      :alt="label || 'preview'"
      @load="onLoad"
      @error="failed = true"
    />
    <span class="h-4 shrink-0 font-mono text-xs text-dimmed">{{ size }}</span>
  </div>
</template>
