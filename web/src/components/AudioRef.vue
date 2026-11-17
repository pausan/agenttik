<script setup>
/* A small player under a message for a sound file it names. It shows only
   once the file is found, using the same lookup as a click on the link.
   It streams from the video endpoint, through the desktop media bridge where
   there is one. What the browser cannot decode is converted by ffmpeg, from
   the start, with no seeking. */
import { computed, onMounted, ref } from "vue";

import { locateFileRef, openExternal, openFileRef, videoURL } from "../store";
import { mediaBase } from "../video";

const props = defineProps({
  file: { type: String, required: true },
  hints: { type: Array, default: () => [] },
});

const found = ref(null);
const base = ref("");
const convert = ref(false);
const failed = ref(false);
const name = computed(() => found.value?.path.split("/").pop() || "");
const src = computed(() => found.value && base.value + videoURL(found.value, convert.value));

onMounted(async () => {
  try {
    const [at, b] = await Promise.all([locateFileRef(props.file, "", props.hints), mediaBase()]);
    if (at.dir) return;
    base.value = b;
    found.value = { kind: "file", projectID: at.projectID, path: at.path };
  } catch {
    // A file that is not there gets no player; its link still says why.
  }
});

function onError(e) {
  const code = e.target.error?.code;
  // 3 is a decode failure, 4 a format this browser does not play.
  if (!convert.value && (code === 3 || code === 4)) convert.value = true;
  else failed.value = true;
}

function openInBrowser() {
  const url = new URL(base.value + videoURL(found.value), window.location.href).href;
  if (!openExternal(url)) window.open(url, "_blank", "noopener,noreferrer");
}
</script>

<template>
  <div v-if="found" class="mt-2 flex max-w-xl flex-wrap items-center gap-2 rounded border border-default px-2 py-1.5 text-xs" :aria-label="`Audio ${name}`">
    <UIcon name="i-lucide-music" class="size-4 shrink-0 text-dimmed" />
    <span class="min-w-0 max-w-48 truncate font-mono" :title="found.path">{{ name }}</span>
    <audio v-if="!failed" class="h-8 min-w-48 flex-1" :src="src" controls preload="metadata" @error="onError"></audio>
    <span v-else class="flex-1 text-warning">Cannot play here.</span>
    <UButton icon="i-lucide-file-plus" size="xs" color="neutral" variant="ghost" aria-label="Open in tab" title="Open in tab" @click="openFileRef(file, 0, '', hints)" />
    <UButton icon="i-lucide-external-link" size="xs" color="neutral" variant="ghost" aria-label="Open in browser" title="Open in browser" @click="openInBrowser" />
  </div>
</template>
