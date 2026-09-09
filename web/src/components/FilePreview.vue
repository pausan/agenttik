<script setup>
/* What a markdown or HTML file renders as, edits included: the preview reads
   the tab's current text, so it follows the editor rather than the disk.

   Markdown goes through the transcript's own renderer, which emits a fixed
   list of tags and escapes everything else. HTML cannot be rendered that way
   — it *is* markup — so it goes in a fully sandboxed iframe: no scripts, no
   same-origin, no navigation, nothing that can reach back into the app. The
   cost is that relative images and stylesheets do not resolve, which is the
   right trade for a preview of a file the agent just wrote. */
import { computed } from "vue";

import { markdown } from "../markdown";

const props = defineProps({ tab: { type: Object, required: true } });

const text = computed(() => props.tab.edited ?? props.tab.content ?? "");
const isHTML = computed(() => /\.html?$/i.test(props.tab.path));
</script>

<template>
  <iframe
    v-if="isHTML"
    class="h-full w-full border-0 bg-white"
    sandbox
    referrerpolicy="no-referrer"
    :title="tab.path"
    :srcdoc="text"
  ></iframe>
  <div v-else class="markdown mx-auto max-w-3xl px-5 py-5" v-html="markdown(text)"></div>
</template>
