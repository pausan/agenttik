<script setup>
/* An opened file: its text, its diff, or what it renders as. An image has no
   text, so it offers only the last two, and its diff is the two pictures
   rather than two columns of lines.

   Tree selections prefer Edit, with Preview for images, fonts and videos. Changes
   and commit file lists open Diff. The toggle changes the current tab's view.

   The bar underneath is the file's own: a conversation's prompt box has no
   business under a file, so MainPanel leaves it out and this takes its
   place. */
import { computed, inject, onMounted, onUnmounted, ref, watch } from "vue";
import { vTabScroll } from "../tab-scroll";

import { canPreview, isDirty, isFont, isImage, isMedia, isOutsideProject, isVideo, saveFile, setFileMode } from "../store";
import { langOf } from "../highlight";
import { api, nf } from "../api";
import { fileInfoLabel } from "../file-info";
import { largeFile, lineCount } from "../file-editor";
import { SEGMENTED } from "../ui";
import { primaryChord } from "../platform";
import FileDiff from "./FileDiff.vue";
import FileEditor from "./FileEditor.vue";
import FilePreview from "./FilePreview.vue";
import ImageDiff from "./ImageDiff.vue";
import HexEditor from "./HexEditor.vue";

const props = defineProps({ tab: { type: Object, required: true } });

const fileExpanded = inject("fileExpanded");
watch(() => props.tab.mode, () => { fileExpanded.value = false; });
function onExpandKey(event) {
  if (event.key !== "Escape" || !fileExpanded.value) return;
  event.preventDefault();
  event.stopImmediatePropagation();
  fileExpanded.value = false;
}
onMounted(() => window.addEventListener("keydown", onExpandKey, true));
onUnmounted(() => {
  window.removeEventListener("keydown", onExpandKey, true);
  fileExpanded.value = false;
});

const image = computed(() => isImage(props.tab.path));
const font = computed(() => isFont(props.tab.path));
const video = computed(() => isVideo(props.tab.path));
const media = computed(() => isMedia(props.tab.path));
const outside = computed(() => isOutsideProject(props.tab.path));

const info = ref(null);
const dimensions = ref(null);
const details = computed(() => fileInfoLabel(info.value, dimensions.value));
watch(() => props.tab.id, () => { dimensions.value = null; });
// A deleted diff describes the old file. A commit must never show today's
// metadata. Ignore a response if the reusable tab has moved to another file.
watch(
  () => [props.tab.id, props.tab.content, props.tab.diff],
  async (_, __, onCleanup) => {
    let stale = false;
    onCleanup(() => { stale = true; });
    info.value = null;
    const tab = props.tab;
    const deleted = /^deleted file mode /m.test(tab.diff || "");
    const rev = deleted ? (tab.commit ? tab.commit + "^" : "HEAD") : tab.commit;
    const query = `path=${encodeURIComponent(tab.path)}${rev ? `&rev=${encodeURIComponent(rev)}` : ""}`;
    try {
      const value = await api("GET", `/api/projects/${tab.projectID}/file-info?${query}`);
      if (!stale) info.value = value;
    } catch {
      // A missing file or unreadable header must not hide its diff or editor.
    }
  },
  { immediate: true },
);

/* A file opened from a commit has one view. There is nothing to edit in a
   revision that has already been made, and its text is not what is on disk,
   so offering Edit or Preview would only show the wrong thing. */
const modes = computed(() => {
  if (props.tab.commit) return [{ label: "Diff", value: "diff" }];
  if (props.tab.binary && !media.value) {
    const items = [{ label: "Hex", value: "hex" }];
    if (!outside.value) items.push({ label: "Diff", value: "diff" });
    return items;
  }
  // Media has no editable text.
  const items = media.value ? [] : [{ label: "Edit", value: "edit" }];
  // A file outside the project is in no repository of it: there is no diff.
  if (!outside.value) items.push({ label: "Diff", value: "diff" });
  if (canPreview(props.tab.path)) items.push({ label: "Preview", value: "preview" });
  if (media.value) items.push({ label: "Hex", value: "hex" });
  return items;
});

const expandLabel = computed(() => props.tab.mode === "edit" ? "editor" : props.tab.mode);

const dirty = computed(() => isDirty(props.tab));
const text = computed(() => props.tab.edited ?? props.tab.content ?? "");

/* Restored files show their tab immediately and fill in their contents later. */
const body = computed(() => {
  if (props.tab.loadError) return "error";
  if (props.tab.mode === "hex" && !props.tab.binary) return "loading";
  const value = props.tab.mode === "diff" ? props.tab.diff : props.tab.content;
  if (value === null) return "loading";
  // The diff of an image is still fetched, for its one useful word: git says
  // nothing at all when a file has not changed, and the pictures are what is
  // drawn from there on.
  if (props.tab.mode === "diff") return value ? (image.value ? "images" : "diff") : "empty";
  return props.tab.mode === "hex" ? "hex" : props.tab.mode === "preview" ? "preview" : "edit";
});

/* An iframe scrolls itself and a picture or video is fitted to its pane;
   everything else scrolls in the pane. */
const fills = computed(
  () =>
    body.value === "images" ||
    (body.value === "edit" && large.value) ||
    (body.value === "preview" && (image.value || video.value || /\.html?$/i.test(props.tab.path))),
);

const lines = computed(() => props.tab.binary ? 0 : lineCount(text.value));
const large = computed(() => !props.tab.binary && largeFile(text.value, lines.value));

/* One extra pass over a diff that has already been fetched, and only when it
   changes — cheaper than threading the count back out of the parse. */
const stat = computed(() => {
  if (props.tab.mode !== "diff" || !props.tab.diff || media.value || props.tab.binary) return null;
  let add = 0;
  let del = 0;
  for (const line of props.tab.diff.split("\n")) {
    if (line.startsWith("+++") || line.startsWith("---")) continue;
    if (line[0] === "+") add++;
    else if (line[0] === "-") del++;
  }
  return { add, del };
});
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <div class="flex shrink-0 flex-wrap items-center gap-3 border-b border-default bg-default px-5 py-1.5">
      <span class="path-clip min-w-0 flex-1 truncate font-mono text-xs text-dimmed">
        <span>{{ tab.path }}</span>
      </span>
      <span v-if="details" class="shrink-0 font-mono text-xs text-dimmed" aria-label="File information">{{ details }}</span>
      <span v-if="tab.commit" class="shrink-0 font-mono text-xs text-primary" title="Shown as this commit changed it">
        @ {{ tab.commit }}
      </span>
      <span
        v-if="dirty"
        class="shrink-0 font-mono text-base leading-none text-primary"
        title="Unsaved changes"
        aria-label="Unsaved changes"
        >*</span
      >
      <!-- Text and hex edits share Save and the unsaved-tab guard. -->
      <UButton
        v-if="(tab.mode === 'edit' || tab.mode === 'hex') && !tab.readOnly"
        icon="i-lucide-save"
        size="xs"
        color="neutral"
        variant="ghost"
        :disabled="!dirty"
        :loading="tab.saving"
        :title="`Save (${primaryChord('S')})`"
        @click="saveFile(tab)"
        >Save</UButton
      >
      <UButton
        :icon="fileExpanded ? 'i-lucide-minimize' : 'i-lucide-maximize'"
        size="xs"
        color="neutral"
        variant="ghost"
        :aria-label="`${fileExpanded ? 'Restore' : 'Expand'} ${expandLabel}`"
        :title="fileExpanded ? `Restore ${expandLabel} (Escape)` : `Expand ${expandLabel}`"
        :aria-pressed="fileExpanded"
        @click="fileExpanded = !fileExpanded"
      />
      <UTabs
        :model-value="tab.mode"
        :items="modes"
        :content="false"
        size="xs"
        class="shrink-0"
        :ui="SEGMENTED"
        @update:model-value="setFileMode(tab, String($event))"
      />
    </div>

    <div v-tab-scroll="[tab, tab.mode, false, body !== 'loading']" class="min-h-0 flex-1" :class="fills ? 'overflow-hidden' : 'overflow-auto'">
      <p v-if="body === 'error'" role="alert" class="px-5 py-5 text-warning">{{ tab.loadError }}</p>
      <p v-else-if="body === 'loading'" class="px-5 py-5 text-center text-dimmed">Loading…</p>
      <p v-else-if="body === 'empty'" class="px-5 py-5 text-center text-dimmed">
        No changes to this file.
      </p>
      <FileDiff v-else-if="body === 'diff'" :diff="tab.diff" :tab="tab" />
      <ImageDiff v-else-if="body === 'images'" :tab="tab" @dimensions="dimensions = $event" />
      <FilePreview v-else-if="body === 'preview'" :tab="tab" @dimensions="dimensions = $event" />
      <HexEditor v-else-if="body === 'hex'" :tab="tab" />
      <FileEditor v-else :tab="tab" :large="large" />
    </div>

    <div
      class="flex shrink-0 items-center gap-3 border-t border-default px-5 py-1 text-xs text-dimmed"
    >
      <span>{{ body === "hex" ? "binary" : image ? "image" : font ? "font" : video ? "video" : langOf(tab.path) || "text" }}</span>
      <span v-if="tab.commit" class="text-dimmed">committed</span>
      <span v-else-if="media && body !== 'hex'" class="text-dimmed">not editable</span>
      <span v-else-if="tab.readOnly" class="text-warning">read only</span>
      <span v-else-if="dirty" class="text-primary">unsaved</span>
      <span v-if="body === 'edit' && large">plain text · no wrap</span>
      <span class="flex-1"></span>
      <template v-if="stat">
        <span class="text-success">+{{ nf.format(stat.add) }}</span>
        <span class="text-error">−{{ nf.format(stat.del) }}</span>
      </template>
      <span v-else-if="body === 'hex'">{{ nf.format(tab.size) }} bytes</span>
      <span v-else-if="!media">{{ nf.format(lines) }} {{ lines === 1 ? "line" : "lines" }}</span>
    </div>
  </div>
</template>
