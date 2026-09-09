<script setup>
/* An opened file, as its contents or as its diff.

   The toggle is remembered rather than reset per file: reading one diff
   usually means the next changed file wants one too, so the next file opens
   the way this one was left. */
import { computed } from "vue";

import { setFileMode } from "../store";
import { SEGMENTED } from "../ui";

const props = defineProps({ tab: { type: Object, required: true } });

const modes = [
  { label: "File", value: "file" },
  { label: "Diff", value: "diff" },
];

/* A diff is coloured a line at a time, so it is worth capping: a whole
   rewritten file is thousands of lines nobody reads to the end. */
const MAX_LINES = 4000;

function classOf(line) {
  if (line.startsWith("@@")) return "text-primary";
  if (line.startsWith("+++") || line.startsWith("---")) return "text-dimmed";
  if (line.startsWith("+")) return "text-success";
  if (line.startsWith("-")) return "text-error";
  if (line.startsWith("diff ") || line.startsWith("index ")) return "text-dimmed";
  return "text-muted";
}

const lines = computed(() => {
  const all = (props.tab.diff || "").split("\n");
  const shown = all.slice(0, MAX_LINES).map((text) => ({ text, cls: classOf(text) }));
  if (all.length > MAX_LINES) shown.push({ text: "… diff truncated", cls: "text-dimmed" });
  return shown;
});

/* The half being shown is null until it has been fetched, which is only ever
   visible when the toggle is switched: opening a file waits for it. */
const body = computed(() => {
  const value = props.tab.mode === "diff" ? props.tab.diff : props.tab.content;
  if (value === null) return "loading";
  if (props.tab.mode === "diff") return value ? "diff" : "empty";
  return "file";
});
</script>

<template>
  <div class="min-h-0 flex-1 overflow-auto">
    <div
      class="sticky top-0 z-10 flex items-center gap-3 border-b border-default bg-default px-5 py-1.5"
    >
      <span class="path-clip min-w-0 flex-1 truncate font-mono text-xs text-dimmed">
        <span>{{ tab.path }}</span>
      </span>
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

    <p v-if="body === 'loading'" class="px-5 py-5 text-center text-dimmed">Loading…</p>
    <p v-else-if="body === 'empty'" class="px-5 py-5 text-center text-dimmed">
      No changes to this file.
    </p>
    <pre
      v-else-if="body === 'diff'"
      class="m-0 px-5 py-4 font-mono text-xs"
    ><code><span v-for="(l, i) in lines" :key="i" class="block" :class="l.cls">{{ l.text || " " }}</span></code></pre>
    <pre
      v-else
      class="m-0 px-5 py-4 font-mono text-xs text-highlighted"
    ><code>{{ tab.content }}</code></pre>
  </div>
</template>
