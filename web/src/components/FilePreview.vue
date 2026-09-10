<script setup>
/* What a file renders as, edits included: the preview reads the tab's current
   text, so it follows the editor rather than the disk.

   Markdown goes through the transcript's own renderer, which emits a fixed
   list of tags and escapes everything else. HTML cannot be rendered that way
   — it *is* markup — so it goes in a fully sandboxed iframe: no scripts, no
   same-origin, no navigation, nothing that can reach back into the app. The
   cost is that relative images and stylesheets do not resolve, which is the
   right trade for a preview of a file the agent just wrote.

   An SVG is markup too, and an <img> draws the same boundary for less work:
   a document there cannot run a script or fetch anything. Its source is the
   tab's own text, so an SVG being edited redraws on the keystroke — which is
   why the file keeps its editor and this is a second view of it rather than a
   replacement.

   A raster image has no text to follow, and is the one preview that reads
   from disk instead of from the tab: the bytes come from the raw endpoint. */
import { computed } from "vue";

import { markdown } from "../markdown";
import { isImage, rawURL } from "../store";
import ImageFrame from "./ImageFrame.vue";

const props = defineProps({ tab: { type: Object, required: true } });

const text = computed(() => props.tab.edited ?? props.tab.content ?? "");
const isHTML = computed(() => /\.html?$/i.test(props.tab.path));
const isSVG = computed(() => /\.svg$/i.test(props.tab.path));
const image = computed(() => isImage(props.tab.path));

/* A data URL keeps the markup exact without a base64 round trip, and costs
   one encode per keystroke on a file small enough to be drawn as a picture. */
const svg = computed(() => "data:image/svg+xml;charset=utf-8," + encodeURIComponent(text.value));
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

  <div v-else-if="isSVG || image" class="flex h-full min-h-0 px-5 py-4">
    <ImageFrame v-if="isSVG" :src="svg" fit missing="This is not an SVG a browser can draw." />
    <ImageFrame v-else :src="rawURL(tab)" />
  </div>

  <div v-else class="markdown mx-auto max-w-3xl px-5 py-5" v-html="markdown(text)"></div>
</template>
