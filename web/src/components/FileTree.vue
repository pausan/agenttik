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
   carrying the number of matches inside, and a click opens it.

   The tree is also where files are made, renamed and deleted: a right click
   offers the four, and F2 on the row under the cursor renames it. Each one
   asks first — a name, or in the case of a delete a yes — in
   TreeActionModal. The same menu opens the row on the desktop itself, which
   asks nothing at all. */
import { computed, ref, watch } from "vue";

import { S, openFile, openInSystem } from "../store";
import { buildTree, countFiles, filterTree } from "../tree";
import { segments } from "../fuzzy";
import TreeActionModal from "./TreeActionModal.vue";

/* The flat listing only changes when the project does, so the tree is built
   once and every keystroke only filters it. */
const full = computed(() => buildTree(S.tree, S.treeIgnored, S.treeDirs));
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

/* One menu for the whole pane, aimed by the event on its way to the trigger,
   the way LogsPane answers its rows: a menu component per row would be one
   per file in the project, and only the row under the pointer has anything to
   offer. A right click that lands beside the rows aims at the project folder
   itself, which is where a new file then goes and which has no name to rename
   and nothing to delete. */
const aimed = ref({ path: "", dir: true });

function aim(e) {
  const row = e.target.closest("[data-path]");
  aimed.value = row ? { path: row.dataset.path, dir: row.dataset.dir === "true" } : { path: "", dir: true };
}

const action = ref(null);

function ask(kind, node) {
  const at = node ? { path: node.path, dir: node.dir } : aimed.value;
  if (!at.path && kind !== "file" && kind !== "folder") return;
  action.value = { kind, ...at };
}

const menu = computed(() => [
  [
    // A click that missed every row aims at the project folder, which is
    // something to open even though it is nothing to rename or delete.
    {
      label: "Open in system browser",
      icon: "i-lucide-external-link",
      onSelect: () => openInSystem(aimed.value.path),
    },
  ],
  [
    { label: "New file", icon: "i-lucide-file-plus", onSelect: () => ask("file") },
    { label: "New folder", icon: "i-lucide-folder-plus", onSelect: () => ask("folder") },
  ],
  [
    {
      label: "Rename",
      icon: "i-lucide-pencil",
      kbds: ["F2"],
      disabled: !aimed.value.path,
      onSelect: () => ask("rename"),
    },
    {
      label: "Delete",
      icon: "i-lucide-trash-2",
      color: "error",
      disabled: !aimed.value.path,
      onSelect: () => ask("delete"),
    },
  ],
]);

/* What was just made has to be visible, so the folders above it are opened —
   a new file in a folder that had been collapsed is otherwise created into
   thin air. The listing has already been re-read by the time this runs. */
function reveal(path) {
  let at = full.value;
  for (const part of path.split("/").slice(0, -1)) {
    at = at.children?.find((c) => c.name === part);
    if (!at) return;
    if (!isOpen(at)) toggle(at.path);
  }
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

    <!-- The trigger is the pane itself, as-child so it adds no element: a
         right click that misses every row still offers New file, and the
         empty states are inside it for the same reason. -->
    <UContextMenu :items="menu" :ui="{ content: 'w-56' }">
      <div class="min-h-0 flex-1" @contextmenu="aim">
        <p v-if="!S.tree.length && !S.treeDirs.length" class="px-3 py-5 text-center text-dimmed">
          {{ S.owner ? "No files." : "Pick a project or task to browse its files." }}
        </p>
        <p v-else-if="!rows.length" class="px-3 py-5 text-center text-dimmed">
          Nothing matches “{{ S.treeFilter }}”.
        </p>
        <div v-else class="min-h-0">
          <div
            v-for="row in rows"
            :key="row.node.path"
            class="flex items-center font-mono text-xs"
            :data-path="row.node.path"
            :data-dir="row.node.dir"
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
              @keydown.f2.prevent.stop="ask('rename', row.node)"
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
              <span v-if="row.inside" class="ml-auto shrink-0 pl-1 tabular-nums">
                {{ row.inside }}
              </span>
            </button>
          </div>
        </div>
      </div>
    </UContextMenu>

    <TreeActionModal v-model:action="action" @done="reveal" />
  </div>
</template>
