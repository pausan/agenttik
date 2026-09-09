<script setup>
/* An opened file: its text, its diff, or what it renders as.

   The view is remembered rather than reset per file — reading one diff
   usually means the next changed file wants one too — so the next file opens
   the way this one was left. A file that renders as nothing has no Preview to
   offer and falls back to the editor.

   The bar underneath is the file's own: a conversation's prompt box has no
   business under a file, so MainPanel leaves it out and this takes its
   place. */
import { computed } from "vue";

import { canPreview, isDirty, saveFile, setFileMode } from "../store";
import { langOf } from "../highlight";
import { nf } from "../api";
import { SEGMENTED } from "../ui";
import FileDiff from "./FileDiff.vue";
import FileEditor from "./FileEditor.vue";
import FilePreview from "./FilePreview.vue";

const props = defineProps({ tab: { type: Object, required: true } });

const modes = computed(() => {
  const items = [
    { label: "Edit", value: "edit" },
    { label: "Diff", value: "diff" },
  ];
  if (canPreview(props.tab.path)) items.push({ label: "Preview", value: "preview" });
  return items;
});

const dirty = computed(() => isDirty(props.tab));
const text = computed(() => props.tab.edited ?? props.tab.content ?? "");

/* The view being shown is null until it has been fetched, which is only ever
   visible when the toggle is switched: opening a file waits for it. */
const body = computed(() => {
  const value = props.tab.mode === "diff" ? props.tab.diff : props.tab.content;
  if (value === null) return "loading";
  if (props.tab.mode === "diff") return value ? "diff" : "empty";
  return props.tab.mode === "preview" ? "preview" : "edit";
});

/* An iframe scrolls itself; everything else scrolls in the pane. */
const fills = computed(() => body.value === "preview" && /\.html?$/i.test(props.tab.path));

const lines = computed(() => text.value.split("\n").length);

/* One extra pass over a diff that has already been fetched, and only when it
   changes — cheaper than threading the count back out of the parse. */
const stat = computed(() => {
  if (props.tab.mode !== "diff" || !props.tab.diff) return null;
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
    <div class="flex shrink-0 items-center gap-3 border-b border-default bg-default px-5 py-1.5">
      <span class="path-clip min-w-0 flex-1 truncate font-mono text-xs text-dimmed">
        <span>{{ tab.path }}</span>
      </span>
      <span
        v-if="dirty"
        class="shrink-0 font-mono text-base leading-none text-primary"
        title="Unsaved changes"
        aria-label="Unsaved changes"
        >*</span
      >
      <UButton
        v-if="!tab.readOnly"
        icon="i-lucide-save"
        size="xs"
        color="neutral"
        variant="ghost"
        :disabled="!dirty"
        :loading="tab.saving"
        title="Save (Ctrl+S)"
        @click="saveFile(tab)"
        >Save</UButton
      >
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
      <p v-if="body === 'loading'" class="px-5 py-5 text-center text-dimmed">Loading…</p>
      <p v-else-if="body === 'empty'" class="px-5 py-5 text-center text-dimmed">
        No changes to this file.
      </p>
      <FileDiff v-else-if="body === 'diff'" :diff="tab.diff" />
      <FilePreview v-else-if="body === 'preview'" :tab="tab" />
      <FileEditor v-else :tab="tab" />
    </div>

    <div
      class="flex shrink-0 items-center gap-3 border-t border-default px-5 py-1 text-xs text-dimmed"
    >
      <span>{{ langOf(tab.path) || "text" }}</span>
      <span v-if="tab.readOnly" class="text-warning">read only</span>
      <span v-else-if="dirty" class="text-primary">unsaved</span>
      <span class="flex-1"></span>
      <template v-if="stat">
        <span class="text-success">+{{ nf.format(stat.add) }}</span>
        <span class="text-error">−{{ nf.format(stat.del) }}</span>
      </template>
      <span v-else>{{ nf.format(lines) }} {{ lines === 1 ? "line" : "lines" }}</span>
    </div>
  </div>
</template>
