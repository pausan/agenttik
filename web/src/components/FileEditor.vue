<script setup>
/* The editable half of a file tab: a transparent textarea over a coloured
   copy of the same text.

   The overlay is the whole trick. A textarea cannot hold markup, so the
   colours go in a <pre> underneath and the textarea above it keeps only its
   caret and its selection. Both wrap, in the same font at the same size, so
   the two layers break lines identically and stay aligned with no scroll
   syncing at all — the wrapper is what scrolls, and the textarea grows to
   fit its content rather than scrolling inside itself. */
import { computed, nextTick, ref, watch } from "vue";

import { editFile } from "../store";
import { highlight, langOf } from "../highlight";
import { useTextHistory } from "../text-history";

const props = defineProps({ tab: { type: Object, required: true } });

/* Colouring is one pass per changed line, but the pass over a very long file
   still runs on the first keystroke. Past this it is plain text: a file this
   size is being looked at, not read. */
const MAX_HIGHLIGHT = 400_000;

const area = ref(null);
const layer = ref(null);
const text = computed(() => props.tab.edited ?? props.tab.content ?? "");
const lang = computed(() =>
  text.value.length > MAX_HIGHLIGHT ? "" : langOf(props.tab.path),
);
const history = useTextHistory(text, (value) => editFile(props.tab, value));

/* The trailing newline is deliberate: without it the last line of the <pre>
   collapses and the caret sits a line above its own text. */
const coloured = computed(() => highlight(text.value + "\n", lang.value));

function onInput(e) {
  history.input(e);
  editFile(props.tab, e.target.value);
  grow();
}

/* Tab indents rather than moving to the next control: this is an editor.
   Ctrl+S is not handled here — App.vue already answers it for whatever file
   is in front, and the caret being in the text changes nothing about it. */
function onKey(e) {
  if (history.keydown(e)) return;
  if (e.key !== "Tab" || e.shiftKey || e.ctrlKey || e.altKey || e.metaKey) return;
  e.preventDefault();
  const el = e.target;
  const { selectionStart: from, selectionEnd: to } = el;
  el.value = el.value.slice(0, from) + "  " + el.value.slice(to);
  el.selectionStart = el.selectionEnd = from + 2;
  history.input({ target: el });
  editFile(props.tab, el.value);
  grow();
}

/* A textarea has no intrinsic height, so it is set from its own content and
   the wrapper does the scrolling. */
function grow() {
  const el = area.value;
  if (!el) return;
  el.style.height = "0px";
  el.style.height = el.scrollHeight + "px";
}

watch(text, () => nextTick(grow), { immediate: true });

/* A file opened from a path in a transcript arrives with a line to show.
   Selecting it marks it, and the coloured layer is what says where it is:
   a Range over its text reports the line's position even though lines wrap,
   which no arithmetic on the text could. Focusing the field would only
   reveal its top — it is one element as tall as the whole file.

   The request is cleared once served, so the same path clicked twice scrolls
   twice, and it is watched immediately because switching tabs mounts this
   component anew. */
watch(
  () => props.tab.goto,
  async (line) => {
    if (!line) return;
    props.tab.goto = 0;
    await nextTick();
    // An explicit line request takes precedence over the tab's saved offset.
    await nextTick();
    grow(); // the pane can only be measured once the field is its full height
    const el = area.value;
    if (!el || !layer.value) return;
    const lines = text.value.split("\n");
    const n = Math.min(line, lines.length);
    let at = 0;
    for (let i = 0; i < n - 1; i++) at += lines[i].length + 1;
    el.focus({ preventScroll: true });
    el.setSelectionRange(at, at + lines[n - 1].length);
    scrollTo(at);
  },
  { immediate: true },
);

/* Where a line sits can only be measured, not counted: lines wrap, so the
   1500th character is not the 30th line down. A range from the top of the
   text to the line's first character ends exactly where that line begins,
   and the pane is scrolled to put it a third of the way down — a line is
   read with some of what comes before it. */
function scrollTo(at) {
  const pane = scroller(area.value);
  if (!pane) return;
  if (!at) {
    pane.scrollTop = 0;
    return;
  }
  const walk = document.createTreeWalker(layer.value, NodeFilter.SHOW_TEXT);
  const first = walk.nextNode();
  if (!first) return;
  const range = document.createRange();
  range.setStart(first, 0);
  let seen = 0;
  for (let node = first; node; node = walk.nextNode()) {
    if (seen + node.data.length >= at) {
      range.setEnd(node, at - seen);
      break;
    }
    seen += node.data.length;
  }
  if (range.collapsed) return;
  const view = pane.getBoundingClientRect();
  pane.scrollTop += range.getBoundingClientRect().bottom - view.top - view.height / 3;
}

/* The editor does not scroll: the pane it sits in does. */
function scroller(el) {
  for (let e = el.parentElement; e; e = e.parentElement) {
    if (e.scrollHeight > e.clientHeight + 1 && /auto|scroll/.test(getComputedStyle(e).overflowY)) {
      return e;
    }
  }
  return null;
}
</script>

<template>
  <!-- The padding is on both layers, not here: `inset: 0` aligns the coloured
       copy with the wrapper's padding box, so any padding here would offset
       one layer from the other. -->
  <div class="editor font-mono text-xs">
    <pre ref="layer" aria-hidden="true"><code v-html="coloured"></code></pre>
    <textarea
      ref="area"
      :value="text"
      :readonly="tab.readOnly"
      :aria-label="tab.path"
      spellcheck="false"
      autocomplete="off"
      autocapitalize="off"
      autocorrect="off"
      wrap="soft"
      @input="onInput"
      @keydown="onKey"
    ></textarea>
  </div>
</template>
