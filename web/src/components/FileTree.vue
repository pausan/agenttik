<script setup>
/* The Tree pane: a filter box over the project's files as a real tree.

   Folders start expanded and stay that way — the point of the pane is to see
   where things are — and can be collapsed by hand. Typing filters on every
   keystroke against the whole path, so "wesst" finds web/src/store.js, and
   the letters that matched are highlighted wherever they fall along it.

   What git ignores is in the tree too, drawn grey and sorted below what is
   not. Those folders are the one exception to starting open: a dependency
   tree is thousands of files against a project's hundred, and it would bury
   both the pane and every filter typed into it. It stays one grey row
   carrying the number of matches inside, and a click opens it. */
import { computed, ref, watch } from "vue";

import { S, openFile } from "../store";
import { buildTree, countFiles, filterTree } from "../tree";
import { segments } from "../fuzzy";

/* The flat listing only changes when the project does, so the tree is built
   once and every keystroke only filters it. */
const full = computed(() => buildTree(S.tree, S.treeIgnored));
const shown = computed(() => filterTree(full.value, S.treeFilter));
const matches = computed(() => countFiles(shown.value));

/* Folders opened or closed by hand are remembered by path — a ref around a
   Set is reactive in its own right, so mutating it is enough. It means
   "closed" for a project folder and "open" for an ignored one, which is the
   whole of the difference between them: one is occasionally closed, the other
   occasionally opened. A filter is a search, so clearing the set puts each
   kind back the way it starts. */
const toggled = ref(new Set());
watch(() => S.treeFilter, () => toggled.value.clear());

const isOpen = (n) => toggled.value.has(n.path) === !!n.ig;

/* Opening the pane is asking to search it, so the sidebar puts the cursor
   here. */
const filter = ref(null);
defineExpose({ focus: () => filter.value?.inputRef?.focus() });

function toggle(path) {
  const seen = toggled.value;
  seen.has(path) ? seen.delete(path) : seen.add(path);
}

/* rows flattens the tree into the lines to draw, which keeps the template one
   loop instead of a recursive component. */
const rows = computed(() => {
  const out = [];
  const searching = !!S.treeFilter.trim();
  const walk = (node, depth) => {
    for (const child of node.children) {
      const open = isOpen(child);
      // A shut ignored folder is the one place a filter's hits are out of
      // sight, so it says how many are in there.
      const inside = searching && child.dir && child.ig && !open ? countFiles(child) : 0;
      out.push({ node: child, depth, open, inside });
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
      ref="filter"
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

    <p v-if="!S.tree.length" class="px-3 py-5 text-center text-dimmed">
      {{ S.owner ? "No files." : "Pick a project or session to browse its files." }}
    </p>
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
          class="flex w-full min-w-0 select-none items-center gap-1 rounded-[var(--ui-radius)] px-1 py-0.5 text-left hover:bg-elevated hover:text-highlighted"
          :class="row.node.ig ? 'text-dimmed' : ''"
          :title="row.node.ig ? row.node.path + ' — ignored by git' : row.node.path"
          :aria-expanded="row.node.dir ? row.open : undefined"
          @click="row.node.dir ? toggle(row.node.path) : openFile(row.node.path)"
          @dblclick="row.node.dir || openFile(row.node.path, true)"
        >
          <UIcon
            v-if="row.node.dir"
            :name="row.open ? 'i-lucide-chevron-down' : 'i-lucide-chevron-right'"
            class="size-3 shrink-0 text-dimmed"
          />
          <span v-else class="w-3 shrink-0" aria-hidden="true" />
          <span class="truncate" :class="row.node.dir && !row.node.ig ? 'text-muted' : ''">
            <span
              v-for="(seg, i) in segments(row.node.name, row.node.hits)"
              :key="i"
              :class="seg.hit ? 'font-semibold text-primary' : ''"
              >{{ seg.text }}</span
            >
          </span>
          <span v-if="row.inside" class="ml-auto shrink-0 pl-1 tabular-nums">{{ row.inside }}</span>
        </button>
      </div>
    </div>
  </div>
</template>
