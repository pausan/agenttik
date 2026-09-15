<script setup>
/* An opened file: its text, its diff, or what it renders as. An image has no
   text, so it offers only the last two, and its diff is the two pictures
   rather than two columns of lines.

   The view is remembered rather than reset per file — reading one diff
   usually means the next changed file wants one too — so the next file opens
   the way this one was left. A file that renders as nothing has no Preview to
   offer and falls back to the editor.

   The bar underneath is the file's own: a conversation's prompt box has no
   business under a file, so MainPanel leaves it out and this takes its
   place. */
import { computed, inject, onMounted, onUnmounted, ref, watch } from "vue";

import { canPreview, isDirty, isFont, isImage, isOutsideProject, saveFile, setFileMode } from "../store";
import { langOf } from "../highlight";
import { api, nf } from "../api";
import { fileInfoLabel } from "../file-info";
import { SEGMENTED } from "../ui";
import { primaryChord } from "../platform";
import FileDiff from "./FileDiff.vue";
import FileEditor from "./FileEditor.vue";
import FilePreview from "./FilePreview.vue";
import ImageDiff from "./ImageDiff.vue";

const props = defineProps({ tab: { type: Object, required: true } });

const diffExpanded = inject("diffExpanded");
watch(() => props.tab.mode, () => { diffExpanded.value = false; });
function onExpandKey(event) {
  if (event.key !== "Escape" || !diffExpanded.value) return;
  event.preventDefault();
  event.stopImmediatePropagation();
  diffExpanded.value = false;
}
onMounted(() => window.addEventListener("keydown", onExpandKey, true));
onUnmounted(() => {
  window.removeEventListener("keydown", onExpandKey, true);
  diffExpanded.value = false;
});

const image = computed(() => isImage(props.tab.path));
const font = computed(() => isFont(props.tab.path));
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
  // Images and fonts have no editable text.
  const items = image.value || font.value ? [] : [{ label: "Edit", value: "edit" }];
  // A file outside the project is in no repository of it: there is no diff.
  if (!outside.value) items.push({ label: "Diff", value: "diff" });
  if (canPreview(props.tab.path)) items.push({ label: "Preview", value: "preview" });
  return items;
});

const dirty = computed(() => isDirty(props.tab));
const text = computed(() => props.tab.edited ?? props.tab.content ?? "");

/* Restored files show their tab immediately and fill in their contents later. */
const body = computed(() => {
  if (props.tab.loadError) return "error";
  const value = props.tab.mode === "diff" ? props.tab.diff : props.tab.content;
  if (value === null) return "loading";
  // The diff of an image is still fetched, for its one useful word: git says
  // nothing at all when a file has not changed, and the pictures are what is
  // drawn from there on.
  if (props.tab.mode === "diff") return value ? (image.value ? "images" : "diff") : "empty";
  return props.tab.mode === "preview" ? "preview" : "edit";
});

/* An iframe scrolls itself and a picture is fitted to its pane; everything
   else scrolls in the pane. */
const fills = computed(
  () =>
    body.value === "images" ||
    (body.value === "preview" && (image.value || /\.html?$/i.test(props.tab.path))),
);

const lines = computed(() => text.value.split("\n").length);

/* One extra pass over a diff that has already been fetched, and only when it
   changes — cheaper than threading the count back out of the parse. */
const stat = computed(() => {
  if (props.tab.mode !== "diff" || !props.tab.diff || image.value || font.value) return null;
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
      <!-- Save belongs to the editor. A diff and a preview are not where text
           is typed, and a file with no text to type — an image, a binary, a
           commit's revision — never shows the editor at all. Ctrl+S still
           reaches the file from any view: the edits are there either way. -->
      <UButton
        v-if="tab.mode === 'edit' && !tab.readOnly"
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
        v-if="tab.mode === 'diff'"
        :icon="diffExpanded ? 'i-lucide-minimize' : 'i-lucide-maximize'"
        size="xs"
        color="neutral"
        variant="ghost"
        :aria-label="diffExpanded ? 'Restore diff' : 'Expand diff'"
        :title="diffExpanded ? 'Restore diff (Escape)' : 'Expand diff'"
        :aria-pressed="diffExpanded"
        @click="diffExpanded = !diffExpanded"
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

    <div class="min-h-0 flex-1" :class="fills ? 'overflow-hidden' : 'overflow-auto'">
      <p v-if="body === 'error'" role="alert" class="px-5 py-5 text-warning">{{ tab.loadError }}</p>
      <p v-else-if="body === 'loading'" class="px-5 py-5 text-center text-dimmed">Loading…</p>
      <p v-else-if="body === 'empty'" class="px-5 py-5 text-center text-dimmed">
        No changes to this file.
      </p>
      <FileDiff v-else-if="body === 'diff'" :diff="tab.diff" />
      <ImageDiff v-else-if="body === 'images'" :tab="tab" @dimensions="dimensions = $event" />
      <FilePreview v-else-if="body === 'preview'" :tab="tab" @dimensions="dimensions = $event" />
      <FileEditor v-else :tab="tab" />
    </div>

    <div
      class="flex shrink-0 items-center gap-3 border-t border-default px-5 py-1 text-xs text-dimmed"
    >
      <span>{{ image ? "image" : font ? "font" : langOf(tab.path) || "text" }}</span>
      <span v-if="tab.commit" class="text-dimmed">committed</span>
      <span v-else-if="image || font" class="text-dimmed">not editable</span>
      <span v-else-if="tab.readOnly" class="text-warning">read only</span>
      <span v-else-if="dirty" class="text-primary">unsaved</span>
      <span class="flex-1"></span>
      <template v-if="stat">
        <span class="text-success">+{{ nf.format(stat.add) }}</span>
        <span class="text-error">−{{ nf.format(stat.del) }}</span>
      </template>
      <span v-else-if="!image && !font">{{ nf.format(lines) }} {{ lines === 1 ? "line" : "lines" }}</span>
    </div>
  </div>
</template>
