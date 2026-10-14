<script>
// Measurements belong to open tabs. Closing one releases its cache, and
// switching away retains no message components or rendered markdown.
const measurements = new WeakMap();
const EMPTY = [];
const MARGIN = 800;
</script>

<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, shallowRef, watch } from "vue";

import TranscriptRow from "./TranscriptRow.vue";

const props = defineProps({ rows: Array, threads: Map, root: Object, tab: Object, expanded: Boolean, following: Function });
const emit = defineEmits(["layout"]);
const list = ref(null);
const width = ref(0);
const visible = shallowRef(new Set(props.rows.slice(-12).map(row => row.at)));
const eager = computed(() => props.expanded || props.rows.length <= 80 || !globalThis.IntersectionObserver);
let cache = measurements.get(props.tab);
if (!cache) measurements.set(props.tab, (cache = new Map()));
const elements = new Map();
let observer;
let resizeObserver;
let mutationObserver;
let frame = 0;
let alive = true;

const first = row => row.message || row.tools[0];
function height(row) {
  const saved = cache.get(row.at);
  return saved?.id === first(row).id && saved?.width === width.value ? saved.height : 120;
}
function measure(el, row) {
  cache.set(row.at, { id: first(row).id, width: width.value, height: el.getBoundingClientRect().height });
}
function keep(el) {
  // Editors and open threads hold local state. Selections and focused controls
  // must also survive scrolling, including an image's thread action.
  const selection = window.getSelection();
  return el.contains(document.activeElement) ||
    (selection && !selection.isCollapsed && (el.contains(selection.anchorNode) || el.contains(selection.focusNode))) ||
    el.querySelector('form, [data-threads] section, [aria-expanded="true"]');
}
function changed(next) {
  const bottom = props.following();
  visible.value = next;
  nextTick(() => {
    if (!alive) return;
    if (bottom && props.following() && props.root) props.root.scrollTop = props.root.scrollHeight;
    emit("layout");
  });
}
function intersect(entries) {
  if (eager.value) return;
  const next = new Set(visible.value);
  for (const entry of entries) {
    const at = Number(entry.target.dataset.transcriptRow);
    if (!alive || !elements.has(at)) continue;
    if (entry.isIntersecting) next.add(at);
    else if (next.has(at) && !keep(entry.target)) {
      measure(entry.target, elements.get(at).row);
      next.delete(at);
    }
  }
  if (next.size !== visible.value.size || [...next].some(at => !visible.value.has(at))) changed(next);
}
function prune() {
  frame = 0;
  if (eager.value || !props.root) return;
  const view = props.root.getBoundingClientRect();
  const next = new Set(visible.value);
  for (const at of next) {
    const item = elements.get(at);
    if (!item) { next.delete(at); continue; }
    const rect = item.el.getBoundingClientRect();
    if (rect.bottom < view.top - MARGIN || rect.top > view.bottom + MARGIN) {
      if (keep(item.el)) continue;
      measure(item.el, item.row);
      next.delete(at);
    }
  }
  if (next.size !== visible.value.size) changed(next);
}
function schedulePrune() {
  if (!frame && !eager.value) frame = requestAnimationFrame(prune);
}
const vDeferred = {
  mounted(el, { value: row }) {
    elements.set(row.at, { el, row });
    observer?.observe(el);
  },
  updated(el, { value: row }) {
    elements.set(row.at, { el, row });
  },
  beforeUnmount(el, { value: row }) {
    if (eager.value || visible.value.has(row.at)) measure(el, row);
    elements.delete(row.at);
    observer?.unobserve(el);
  },
};

watch(eager, (value, previous) => {
  if (value || !previous) return;
  // Find renders every row. Save their real heights before releasing them.
  const view = props.root.getBoundingClientRect();
  const next = new Set();
  for (const [at, { el, row }] of elements) {
    measure(el, row);
    const rect = el.getBoundingClientRect();
    if ((rect.bottom >= view.top - MARGIN && rect.top <= view.bottom + MARGIN) || keep(el)) next.add(at);
  }
  changed(next);
});
function observe() {
  observer?.disconnect();
  if (!globalThis.IntersectionObserver || !props.root) return;
  observer = new IntersectionObserver(intersect, { root: props.root, rootMargin: `${MARGIN}px` });
  for (const { el } of elements.values()) observer.observe(el);
}
watch(() => props.root, observe, { flush: "post" });
onMounted(() => {
  width.value = list.value.clientWidth;
  resizeObserver = new ResizeObserver(() => { width.value = list.value.clientWidth; });
  resizeObserver.observe(list.value);
  // A child can close an editor or collapse tools without updating this
  // component. Release those rows even if they are already outside the view.
  mutationObserver = new MutationObserver(schedulePrune);
  mutationObserver.observe(list.value, { childList: true, subtree: true, attributes: true, attributeFilter: ["aria-expanded"] });
  document.addEventListener("selectionchange", schedulePrune);
  observe();
});
onUnmounted(() => {
  alive = false;
  observer?.disconnect();
  resizeObserver?.disconnect();
  mutationObserver?.disconnect();
  document.removeEventListener("selectionchange", schedulePrune);
  cancelAnimationFrame(frame);
});
async function reveal(at) {
  if (!eager.value && !visible.value.has(at)) changed(new Set([...visible.value, at]));
  await nextTick();
  return elements.get(at)?.el;
}
defineExpose({ reveal });
</script>

<template>
  <div ref="list" @focusout="schedulePrune">
    <div
      v-for="row in rows"
      :key="row.at"
      v-memo="[eager || visible.has(row.at), width, row.message, row.tools, row.summary, row.outputs, threads.get(row.message?.id)]"
      v-deferred="row"
      class="flow-root"
      :data-transcript-row="row.at"
      :data-human-message="row.message?.role === 'user' ? row.at : undefined"
      :style="eager || visible.has(row.at) ? undefined : { height: `${height(row)}px` }"
    >
      <TranscriptRow v-if="eager || visible.has(row.at)" :row="row" :threads="threads.get(row.message?.id) || EMPTY" />
    </div>
  </div>
</template>
