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

const props = defineProps({ tab: { type: Object, required: true } });

/* Colouring is one pass per changed line, but the pass over a very long file
   still runs on the first keystroke. Past this it is plain text: a file this
   size is being looked at, not read. */
const MAX_HIGHLIGHT = 400_000;

const area = ref(null);
const text = computed(() => props.tab.edited ?? props.tab.content ?? "");
const lang = computed(() =>
  text.value.length > MAX_HIGHLIGHT ? "" : langOf(props.tab.path),
);

/* The trailing newline is deliberate: without it the last line of the <pre>
   collapses and the caret sits a line above its own text. */
const coloured = computed(() => highlight(text.value + "\n", lang.value));

function onInput(e) {
  editFile(props.tab, e.target.value);
  grow();
}

/* Tab indents rather than moving to the next control: this is an editor.
   Ctrl+S is not handled here — App.vue already answers it for whatever file
   is in front, and the caret being in the text changes nothing about it. */
function onKey(e) {
  if (e.key !== "Tab" || e.shiftKey || e.ctrlKey || e.altKey || e.metaKey) return;
  e.preventDefault();
  const el = e.target;
  const { selectionStart: from, selectionEnd: to } = el;
  el.value = el.value.slice(0, from) + "  " + el.value.slice(to);
  el.selectionStart = el.selectionEnd = from + 2;
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
</script>

<template>
  <!-- The padding is on both layers, not here: `inset: 0` aligns the coloured
       copy with the wrapper's padding box, so any padding here would offset
       one layer from the other. -->
  <div class="editor font-mono text-xs">
    <pre aria-hidden="true"><code v-html="coloured"></code></pre>
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
