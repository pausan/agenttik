<script setup>
/* Project folders start collapsed. Explicit expansion survives project changes
   and restarts; filtering temporarily reveals matching non-ignored paths. */
import { computed, nextTick, ref, watch } from "vue";

import { S, copyText, currentProjectID, focusPrompt, openFile, openInSystem, selectTab, startCurrentTask } from "../store";
import { buildTree, countFiles, filterTree } from "../tree";
import { segments } from "../fuzzy";
import { promptImages, promptText, withImages } from "../prompt-images";
import TreeActionModal from "./TreeActionModal.vue";

/* The flat listing only changes when the project does, so the tree is built
   once and every keystroke only filters it. */
const full = computed(() => buildTree(S.tree, S.treeIgnored, S.treeDirs));
const shown = computed(() => filterTree(full.value, S.treeFilter));
const matches = computed(() => countFiles(shown.value));

const selected = ref("");
const expanded = ref(new Set());
const filterToggled = ref(new Map());
const expansionKey = () => `agenttik.tree-expanded.${currentProjectID()}`;
watch(currentProjectID, () => {
  selected.value = "";
  filterToggled.value.clear();
  restoreExpansion();
}, { flush: "sync" });
watch(() => S.treeFilter, () => filterToggled.value.clear());

function restoreExpansion() {
  try {
    const saved = JSON.parse(localStorage.getItem(expansionKey()));
    expanded.value = new Set(Array.isArray(saved) ? saved.filter((p) => typeof p === "string") : []);
  } catch {
    expanded.value = new Set();
  }
}
restoreExpansion();

function saveExpansion() {
  if (!currentProjectID()) return;
  try {
    localStorage.setItem(expansionKey(), JSON.stringify([...expanded.value]));
  } catch { /* Tree navigation still works when storage is unavailable. */ }
}

const isOpen = (n) => S.treeFilter.trim()
  ? (filterToggled.value.get(n.path) ?? (!n.ig || expanded.value.has(n.path)))
  : expanded.value.has(n.path);

/* Opening the pane is asking to search it, so the sidebar puts the cursor
   here. */
const filter = ref(null);
const rowRefs = new Map();

function setRowRef(path, el) {
  if (el) rowRefs.set(path, el);
  else rowRefs.delete(path);
}

function toggle(node) {
  const open = !isOpen(node);
  if (S.treeFilter.trim()) filterToggled.value.set(node.path, open);
  open ? expanded.value.add(node.path) : expanded.value.delete(node.path);
  saveExpansion();
}

function expandRecursively() {
  const path = aimed.value.path;
  const walk = (node) => {
    if (!node.dir) return;
    if (node.path && (!path || node.path === path || node.path.startsWith(path + "/"))) {
      expanded.value.add(node.path);
      filterToggled.value.delete(node.path);
    }
    for (const child of node.children) walk(child);
  };
  walk(full.value);
  saveExpansion();
}

function focusRow(path) {
  if (!path) return;
  selected.value = path;
  const row = rowRefs.get(path);
  row?.querySelector("button")?.focus({ preventScroll: true });
  row?.scrollIntoView({ block: "nearest" });
}

function onKey(event, row) {
  if (!["ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight"].includes(event.key)) return;
  if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return;
  event.preventDefault();
  event.stopPropagation();
  const index = rows.value.findIndex((r) => r.node.path === row.node.path);
  if (event.key === "ArrowUp" || event.key === "ArrowDown") {
    focusRow(rows.value[index + (event.key === "ArrowUp" ? -1 : 1)]?.node.path);
  } else if (event.key === "ArrowRight") {
    if (row.node.dir && !row.open) toggle(row.node);
    else if (row.node.dir) focusRow(rows.value[index + 1]?.depth > row.depth ? rows.value[index + 1].node.path : "");
  } else if (row.node.dir && row.open) {
    toggle(row.node);
  } else {
    focusRow(row.node.path.split("/").slice(0, -1).join("/"));
  }
}

function onPaneKey(event) {
  if ((event.ctrlKey || event.metaKey) && event.code === "KeyF" && !event.altKey && !event.shiftKey) {
    event.preventDefault();
    event.stopPropagation();
    filter.value?.inputRef?.focus();
  }
}

function open(path, pin = false) {
  selected.value = path;
  return openFile(path, pin);
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
  action.value = { kind, ...at, projectID: currentProjectID(), repository: S.repository };
}

/* Closing the menu hands focus back to the row it was opened from, which
   would take it straight off the prompt this put the caret in. */
const keepCaret = ref(false);

function releaseFocus(e) {
  if (keepCaret.value) e.preventDefault();
  keepCaret.value = false;
}

async function sendPathToPrompt() {
  const path = aimed.value.path;
  const projectID = currentProjectID();
  if (!path) return;
  // Set before the task is started, since the menu closes while that waits.
  keepCaret.value = true;
  if (S.owner?.kind !== "session") await startCurrentTask();
  const owner = S.owner;
  if (owner?.kind !== "session" || currentProjectID() !== projectID) return;
  const text = promptText(owner.draft);
  owner.draft = withImages(text + (text && !/\s$/.test(text) ? "\n" : "") + path, promptImages(owner.draft));
  selectTab(owner.id);
  await nextTick();
  focusPrompt();
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
    { label: "Copy path", icon: "i-lucide-copy", disabled: !aimed.value.path, onSelect: () => copyText(aimed.value.path) },
    { label: "Send path to prompt", icon: "i-lucide-message-square-plus", disabled: !aimed.value.path, onSelect: sendPathToPrompt },
  ],
  [
    { label: "Expand recursively", icon: "i-lucide-list-tree", disabled: !aimed.value.dir, onSelect: expandRecursively },
  ],
  [
    { label: "New file", icon: "i-lucide-file-plus", onSelect: () => ask("file") },
    { label: "New folder", icon: "i-lucide-folder-plus", onSelect: () => ask("folder") },
  ],
  ...(!aimed.value.dir && S.changed.some((f) => f.path === aimed.value.path) ? [[{
    label: "Revert changes…",
    icon: "i-lucide-undo-2",
    color: "error",
    onSelect: () => ask("revert"),
  }]] : []),
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
    if (!isOpen(at)) toggle(at);
  }
}

async function select(path) {
  if (!path) return;
  selected.value = path;
  S.treeFilter = "";
  await nextTick();
  reveal(path);
  await nextTick();
  rowRefs.get(path)?.scrollIntoView({ block: "nearest" });
}

/* A request can arrive while the Tree listing is still being fetched. Repeat
   the reveal when that listing lands so the selected row becomes visible then
   too. */
watch(full, async () => {
  if (!selected.value) return;
  reveal(selected.value);
  await nextTick();
  rowRefs.get(selected.value)?.scrollIntoView({ block: "nearest" });
}, { flush: "post" });

defineExpose({
  focus: () => filter.value?.inputRef?.focus(),
  show: select,
});

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
  <div class="flex min-h-0 flex-col gap-2" @keydown="onPaneKey">
    <UInput
      ref="filter"
      v-model="S.treeFilter"
      icon="i-lucide-search"
      placeholder="Filter files"
      @keydown.esc.prevent.stop="S.treeFilter = ''"
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
    <UContextMenu :items="menu" :ui="{ content: 'w-56' }" :content="{ onCloseAutoFocus: releaseFocus }">
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
            :ref="(el) => setRowRef(row.node.path, el)"
            class="flex items-center font-mono text-xs"
            :data-path="row.node.path"
            :data-dir="row.node.dir"
            :style="{ paddingLeft: row.depth * 10 + 'px' }"
          >
            <button
              type="button"
              class="flex w-full min-w-0 select-none items-center gap-1 rounded-[var(--ui-radius)] px-1 py-0.5 text-left hover:bg-elevated hover:text-highlighted"
              :class="selected === row.node.path
                ? 'bg-primary/10 text-highlighted'
                : row.node.ig
                  ? 'text-dimmed hover:bg-elevated hover:text-highlighted'
                  : 'hover:bg-elevated hover:text-highlighted'"
              :title="row.node.ig ? row.node.path + ' — ignored by git' : row.node.path"
              :aria-expanded="row.node.dir ? row.open : undefined"
              :aria-current="selected === row.node.path ? 'true' : undefined"
              @focus="selected = row.node.path"
              @click="row.node.dir ? toggle(row.node) : open(row.node.path)"
              @keydown="onKey($event, row)"
              @dblclick="row.node.dir || open(row.node.path, true)"
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
