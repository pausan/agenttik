<script setup>
/* A file's working-tree diff, unified or in two columns.

   Both come from one parse of the text git printed, so the toggle costs a
   re-render and nothing else. The unified view keeps the +/- markers because
   without them a line says nothing; the split view drops them, because which
   column a line is in already says which side it is on. */
import { computed, ref, watch } from "vue";

import { S, setDiffView, setDiffContext } from "../store";
import { MAX_LINES, classOf, pair, parseDiff } from "../diff";
import { api } from "../api";
import { SEGMENTED } from "../ui";

const props = defineProps({
  diff: { type: String, default: "" },
  tab: { type: Object, required: true },
});
const contexts = [
  { label: "Changes", value: "changes" },
  { label: "Whole file", value: "full" },
];
const full = ref(null);
const error = ref("");
watch(
  () => [S.diffContext, props.tab.projectID, props.tab.path, props.tab.commit, props.diff],
  async (_, __, onCleanup) => {
    let stale = false;
    onCleanup(() => { stale = true; });
    full.value = null;
    error.value = "";
    if (S.diffContext !== "full") return;
    const { projectID, path, commit } = props.tab;
    const endpoint = commit ? "commit/diff" : "diff";
    const query = new URLSearchParams({ path, context: "full" });
    if (commit) query.set("hash", commit);
    try {
      const result = await api("GET", `/api/projects/${projectID}/${endpoint}?${query}`);
      if (!stale) full.value = result;
    } catch (e) {
      if (!stale) error.value = e.message || "Could not load the whole file.";
    }
  },
  { immediate: true },
);
const loading = computed(() => S.diffContext === "full" && !full.value && !error.value);

const views = [
  { label: "Unified", value: "unified" },
  { label: "Split", value: "split" },
];

const parsed = computed(() => parseDiff(S.diffContext === "full" ? full.value?.diff : props.diff, MAX_LINES));

const unified = computed(() =>
  parsed.value.rows.map((r) => ({ text: r.text, cls: classOf(r.kind) })),
);

const split = computed(() => pair(parsed.value.rows));

/* The gutter is sized from the largest line number, so a four-digit file does
   not sit in the same column as a two-digit one. */
const gutter = computed(() => {
  let max = 0;
  for (const r of parsed.value.rows) max = Math.max(max, r.left || 0, r.right || 0);
  return String(max).length + 1 + "ch";
});
</script>

<template>
  <div>
    <div class="flex flex-wrap items-center justify-end gap-3 px-5 pt-3">
      <UTabs
        :model-value="S.diffContext"
        :items="contexts"
        :content="false"
        size="xs"
        :ui="SEGMENTED"
        @update:model-value="setDiffContext(String($event))"
      />
      <UTabs
        :model-value="S.diffView"
        :items="views"
        :content="false"
        size="xs"
        :ui="SEGMENTED"
        @update:model-value="setDiffView(String($event))"
      />
    </div>

    <p v-if="loading" class="px-5 py-4 text-dimmed">Loading…</p>
    <p v-else-if="error" role="alert" class="px-5 py-4 text-warning">{{ error }}</p>
    <template v-else>
      <pre
        v-if="S.diffView === 'unified'"
        class="m-0 px-5 py-4 font-mono text-xs"
      ><code><span v-for="(l, i) in unified" :key="i" class="block" :class="l.cls">{{ l.text || " " }}</span></code></pre>

      <div v-else class="px-5 py-4 font-mono text-xs">
        <div
          v-for="(row, i) in split"
          :key="i"
          class="grid"
          :class="row.kind === 'hunk' ? '' : 'grid-cols-2'"
        >
          <div v-if="row.kind === 'hunk'" class="truncate text-primary">{{ row.text }}</div>
          <template v-else>
            <div
              class="diff-cell border-r border-default"
              :class="row.l ? (row.kind === 'chg' ? 'bg-error/10' : '') : 'bg-elevated/50'"
            >
              <span class="diff-n" :style="{ width: gutter }">{{ row.l ? row.l.n : "" }}</span>
              <span :class="row.kind === 'chg' ? 'text-error' : 'text-muted'">{{
                row.l ? row.l.text || " " : " "
              }}</span>
            </div>
            <div
              class="diff-cell"
              :class="row.r ? (row.kind === 'chg' ? 'bg-success/10' : '') : 'bg-elevated/50'"
            >
              <span class="diff-n" :style="{ width: gutter }">{{ row.r ? row.r.n : "" }}</span>
              <span :class="row.kind === 'chg' ? 'text-success' : 'text-muted'">{{
                row.r ? row.r.text || " " : " "
              }}</span>
            </div>
          </template>
        </div>
      </div>

      <p v-if="parsed.truncated || (S.diffContext === 'full' && full?.partial)" class="px-5 pb-4 text-dimmed">… diff truncated</p>
    </template>
  </div>
</template>
