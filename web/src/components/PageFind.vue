<script setup>
import { nextTick, onBeforeUnmount, ref, watch } from "vue";
import { findMatches, pageRanges } from "../page-find";
import { revealText } from "../file-editor";

const props = defineProps({ root: Object, context: String });
const emit = defineEmits(["open-change"]);
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
let textArea;

async function show() {
  const wasOpen = open.value;
  previousFocus = wasOpen ? previousFocus : document.activeElement;
  open.value = true;
  await nextTick();
  input.value?.focus();
  input.value?.select();
  if (wasOpen) return;
  // Paint the bar before mounting a long history, which can take seconds.
  await new Promise((resolve) => requestAnimationFrame(() => setTimeout(resolve)));
  if (!open.value) return;
  emit("open-change", true);
  await nextTick();
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
  emit("open-change", false);
  observer?.disconnect();
  clearTimeout(timer);
  clear();
  ranges = [];
  if (previousFocus?.isConnected) previousFocus.focus({ preventScroll: true });
  previousFocus = null;
}
// A refresh after the content changed keeps the current match and the
// reader's scroll position; only a new query starts over at the first match.
function search(refresh = false) {
  if (!open.value) return;
  // Changes made before this search are already covered by it.
  clearTimeout(timer);
  observer?.takeRecords();
  textArea = props.root?.querySelector(".large-editor textarea");
  ranges = textArea ? findMatches(textArea.value, query.value) : pageRanges(props.root?.querySelector(".editor") || props.root, query.value);
  count.value = ranges.length;
  current.value = Math.min(refresh ? Math.max(current.value, 0) : 0, ranges.length - 1);
  clear();
  if (!textArea && globalThis.CSS?.highlights && globalThis.Highlight) {
    CSS.highlights.set("page-find", new Highlight(...ranges));
  }
  // Selecting in a text area would also move the caret of someone typing.
  if (!refresh) reveal();
  else if (!textArea) mark();
}
function mark() {
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
}
function reveal() {
  const range = ranges[current.value];
  if (!range) return;
  if (textArea) {
    revealText(textArea, range.start, range.end);
    return;
  }
  mark();
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
watch(query, () => search());
watch(() => props.context, close);
watch([open, () => props.root], () => {
  observer?.disconnect();
  if (!open.value || !props.root) return;
  // Ticking clocks are not searched, so their updates need no new search.
  observer = new MutationObserver((records) => {
    if (records.every(({ target }) => (target.nodeType === Node.ELEMENT_NODE ? target : target.parentElement)?.closest("[data-find-ignore]"))) return;
    clearTimeout(timer);
    timer = setTimeout(search, 120, true);
  });
  observer.observe(props.root, { childList: true, subtree: true, characterData: true });
});
function onInput() {
  clearTimeout(timer);
  timer = setTimeout(search, 120, true);
}
watch(() => props.root, (root, previous) => {
  previous?.removeEventListener("input", onInput);
  root?.addEventListener("input", onInput);
}, { immediate: true });
onBeforeUnmount(() => props.root?.removeEventListener("input", onInput));
onBeforeUnmount(close);
defineExpose({ show });
</script>

<template>
  <div v-if="open" role="search" aria-label="Find in current page" class="flex shrink-0 items-center gap-2 border-b border-default bg-default px-3 py-2" @keydown.esc.stop.prevent="close">
    <input ref="input" v-model="query" aria-label="Find in current page" placeholder="Find in current page…" class="min-w-0 flex-1 rounded border border-default bg-default px-2 py-1 text-sm" @keydown.enter.stop.prevent="move($event.shiftKey ? -1 : 1)" @keydown.up.exact.stop.prevent="move(-1)" @keydown.down.exact.stop.prevent="move(1)" />
    <span role="status" class="whitespace-nowrap text-xs text-muted">{{ count ? `${current + 1} of ${count}` : query ? 'No matches' : '0 matches' }}</span>
    <UButton icon="i-lucide-chevron-up" aria-label="Previous match" title="Previous match (↑ or Shift+Enter)" color="neutral" variant="ghost" size="xs" :disabled="!count" @click="move(-1)" />
    <UButton icon="i-lucide-chevron-down" aria-label="Next match" title="Next match (↓ or Enter)" color="neutral" variant="ghost" size="xs" :disabled="!count" @click="move(1)" />
    <UButton icon="i-lucide-x" aria-label="Close find" color="neutral" variant="ghost" size="xs" @click="close" />
  </div>
</template>
