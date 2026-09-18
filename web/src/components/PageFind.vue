<script setup>
import { nextTick, onBeforeUnmount, ref, watch } from "vue";
import { pageRanges } from "../page-find";

const props = defineProps({ root: Object, context: String });
const open = ref(false);
const query = ref("");
const input = ref(null);
const count = ref(0);
const current = ref(-1);
let ranges = [];
let observer;
let timer;
let previousFocus;
let selectedRange;

async function show() {
  previousFocus = open.value ? previousFocus : document.activeElement;
  open.value = true;
  await nextTick();
  input.value?.focus();
  input.value?.select();
  search();
}
function clear() {
  globalThis.CSS?.highlights?.delete("page-find");
  globalThis.CSS?.highlights?.delete("page-find-current");
  const selection = window.getSelection();
  if (selectedRange && selection?.rangeCount && selection.getRangeAt(0) === selectedRange) selection.removeAllRanges();
  selectedRange = null;
}
function close() {
  open.value = false;
  observer?.disconnect();
  clearTimeout(timer);
  clear();
  ranges = [];
  if (previousFocus?.isConnected) previousFocus.focus({ preventScroll: true });
  previousFocus = null;
}
function search() {
  if (!open.value) return;
  ranges = pageRanges(props.root?.querySelector(".editor") || props.root, query.value);
  count.value = ranges.length;
  current.value = ranges.length ? 0 : -1;
  clear();
  if (globalThis.CSS?.highlights && globalThis.Highlight) {
    CSS.highlights.set("page-find", new Highlight(...ranges));
  }
  reveal();
}
function reveal() {
  const range = ranges[current.value];
  if (!range) return;
  if (globalThis.CSS?.highlights && globalThis.Highlight) {
    CSS.highlights.set("page-find-current", new Highlight(range));
  } else {
    const selection = window.getSelection();
    selection.removeAllRanges();
    selection.addRange(range);
    selectedRange = range;
  }
  // Scroll the range itself, including matches deep inside a single pre.
  const rect = range.getBoundingClientRect();
  for (let el = range.startContainer.parentElement; el && props.root.contains(el); el = el.parentElement) {
    if (el.scrollHeight > el.clientHeight && /auto|scroll/.test(getComputedStyle(el).overflowY)) {
      const view = el.getBoundingClientRect();
      el.scrollTop += rect.top - view.top - view.height / 3;
      break;
    }
  }
}
function move(direction) {
  if (!ranges.length) return;
  current.value = (current.value + direction + ranges.length) % ranges.length;
  reveal();
}
watch(query, search);
watch(() => props.context, close);
watch([open, () => props.root], () => {
  observer?.disconnect();
  if (!open.value || !props.root) return;
  observer = new MutationObserver(() => {
    clearTimeout(timer);
    timer = setTimeout(search, 120);
  });
  observer.observe(props.root, { childList: true, subtree: true, characterData: true });
});
onBeforeUnmount(close);
defineExpose({ show });
</script>

<template>
  <div v-if="open" role="search" aria-label="Find in current page" class="flex shrink-0 items-center gap-2 border-b border-default bg-default px-3 py-2" @keydown.esc.stop.prevent="close">
    <input ref="input" v-model="query" aria-label="Find in current page" placeholder="Find in current page…" class="min-w-0 flex-1 rounded border border-default bg-default px-2 py-1 text-sm" @keydown.enter.stop.prevent="move($event.shiftKey ? -1 : 1)" />
    <span role="status" class="whitespace-nowrap text-xs text-muted">{{ count ? `${current + 1} of ${count}` : query ? 'No matches' : '0 matches' }}</span>
    <UButton icon="i-lucide-chevron-up" aria-label="Previous match" color="neutral" variant="ghost" size="xs" :disabled="!count" @click="move(-1)" />
    <UButton icon="i-lucide-chevron-down" aria-label="Next match" color="neutral" variant="ghost" size="xs" :disabled="!count" @click="move(1)" />
    <UButton icon="i-lucide-x" aria-label="Close find" color="neutral" variant="ghost" size="xs" @click="close" />
  </div>
</template>
