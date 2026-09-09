<script setup>
/* A file's working-tree diff, unified or in two columns.

   Both come from one parse of the text git printed, so the toggle costs a
   re-render and nothing else. The unified view keeps the +/- markers because
   without them a line says nothing; the split view drops them, because which
   column a line is in already says which side it is on. */
import { computed } from "vue";

import { S, setDiffView } from "../store";
import { MAX_LINES, classOf, pair, parseDiff } from "../diff";
import { SEGMENTED } from "../ui";

const props = defineProps({ diff: { type: String, default: "" } });

const views = [
  { label: "Unified", value: "unified" },
  { label: "Split", value: "split" },
];

const parsed = computed(() => parseDiff(props.diff, MAX_LINES));

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
    <div class="flex items-center justify-end px-5 pt-3">
      <UTabs
        :model-value="S.diffView"
        :items="views"
        :content="false"
        size="xs"
        :ui="SEGMENTED"
        @update:model-value="setDiffView(String($event))"
      />
    </div>

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

    <p v-if="parsed.truncated" class="px-5 pb-4 text-dimmed">… diff truncated</p>
  </div>
</template>
