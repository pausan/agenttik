<script setup>
/* The Tree pane: a filter box over the project's files as a real tree.

   Folders start expanded and stay that way — the point of the pane is to see
   where things are — and can be collapsed by hand. Typing filters on every
   keystroke against the whole path, so "wesst" finds web/src/store.js, and
   the letters that matched are highlighted wherever they fall along it. */
import { computed, ref, watch } from "vue";

import { S, openFile } from "../store";
import { buildTree, countFiles, filterTree } from "../tree";
import { segments } from "../fuzzy";

/* The flat listing only changes when the project does, so the tree is built
   once and every keystroke only filters it. */
const full = computed(() => buildTree(S.tree));
const shown = computed(() => filterTree(full.value, S.treeFilter));
const matches = computed(() => countFiles(shown.value));

/* Collapsed folders are remembered by path — a ref around a Set is reactive
   in its own right, so mutating it is enough. A filter is a search, so while
   one is typed everything it kept is open. */
const collapsed = ref(new Set());
watch(() => S.treeFilter, () => collapsed.value.clear());

function toggle(path) {
  const open = collapsed.value;
  open.has(path) ? open.delete(path) : open.add(path);
}

/* rows flattens the tree into the lines to draw, which keeps the template one
   loop instead of a recursive component. */
const rows = computed(() => {
  const out = [];
  const walk = (node, depth) => {
    for (const child of node.children) {
      const open = !collapsed.value.has(child.path);
      out.push({ node: child, depth, open });
      if (child.dir && open) walk(child, depth + 1);
    }
  };
  walk(shown.value, 0);
  return out;
});
</script>

<template>
  <div class="flex min-h-0 flex-col gap-2">
    <UInput
      v-model="S.treeFilter"
      icon="i-lucide-search"
      placeholder="Filter files"
      class="w-full shrink-0"
      :ui="{ base: 'text-xs' }"
    >
      <template v-if="S.treeFilter" #trailing>
        <span class="text-[10px] text-dimmed tabular-nums">{{ matches }}</span>
      </template>
    </UInput>

    <p v-if="!S.tree.length" class="px-3 py-5 text-center text-dimmed">No files.</p>
    <p v-else-if="!rows.length" class="px-3 py-5 text-center text-dimmed">
      Nothing matches “{{ S.treeFilter }}”.
    </p>
    <div v-else class="min-h-0">
      <div
        v-for="row in rows"
        :key="row.node.path"
        class="flex items-center font-mono text-xs"
        :style="{ paddingLeft: row.depth * 10 + 'px' }"
      >
        <button
          type="button"
          class="flex w-full min-w-0 items-center gap-1 rounded-[var(--ui-radius)] px-1 py-0.5 text-left hover:bg-elevated hover:text-highlighted"
          :title="row.node.path"
          :aria-expanded="row.node.dir ? row.open : undefined"
          @click="row.node.dir ? toggle(row.node.path) : openFile(row.node.path)"
        >
          <UIcon
            v-if="row.node.dir"
            :name="row.open ? 'i-lucide-chevron-down' : 'i-lucide-chevron-right'"
            class="size-3 shrink-0 text-dimmed"
          />
          <span v-else class="w-3 shrink-0" aria-hidden="true" />
          <span class="truncate" :class="row.node.dir ? 'text-muted' : ''">
            <span
              v-for="(seg, i) in segments(row.node.name, row.node.hits)"
              :key="i"
              :class="seg.hit ? 'font-semibold text-primary' : ''"
              >{{ seg.text }}</span
            >
          </span>
        </button>
      </div>
    </div>
  </div>
</template>
