<script setup>
/* Go to file lists every match, but draws only the rows in view: a project
   can hold 200,000 paths, and a palette row per match cost more than the
   search. Rows have one fixed height, so the scroll offset says which. */
import { computed, ref, useTemplateRef, watch } from "vue";
import { S, currentProjectID, openFileForEdit } from "../store";
import { searchFiles } from "../file-search";
import { segments } from "../fuzzy";

const ROW = 32; // px, h-8
const PAD = 4; // px, the list's p-1
const OVERSCAN = 4;

const open = defineModel("open", { type: Boolean, default: false });
const query = ref("");
const active = ref(0);
const top = ref(0);
const list = useTemplateRef("list");
watch(currentProjectID, () => (query.value = ""));

const results = computed(() => searchFiles(S.tree, query.value));
watch(results, () => {
  active.value = 0;
  top.value = 0;
  if (list.value) list.value.scrollTop = 0;
});

// The list is never taller than the window, so the window bounds the rows
// in view without measuring the list.
const rows = computed(() => {
  const first = Math.max(0, Math.floor((top.value - PAD) / ROW) - OVERSCAN);
  const last = Math.min(results.value.length, first + Math.ceil(window.innerHeight / ROW) + 2 * OVERSCAN);
  const out = [];
  for (let rank = first; rank < last; rank++) {
    const { path, hits } = results.value.at(rank);
    out.push({ rank, path, segments: segments(path, hits) });
  }
  return out;
});

const empty = computed(() => {
  if (!currentProjectID()) return "Select a project to search its files.";
  return S.treeTruncated ? "No files found in the part of this folder that is listed." : "No files found.";
});

function choose(path) {
  open.value = false;
  openFileForEdit(path);
}

function onKey(e) {
  const count = results.value.length;
  if (e.key === "Enter" && !e.isComposing) {
    e.preventDefault();
    if (count) choose(results.value.at(active.value).path);
    return;
  }
  const page = Math.max(1, Math.floor((list.value?.clientHeight ?? 0) / ROW) - 1);
  const step = { ArrowDown: 1, ArrowUp: -1, PageDown: page, PageUp: -page }[e.key];
  if (!step || !count) return;
  e.preventDefault();
  active.value = Math.max(0, Math.min(count - 1, active.value + step));
  // Keep the active row in view.
  const el = list.value;
  const y = PAD + active.value * ROW;
  if (y < el.scrollTop + PAD) el.scrollTop = y - PAD;
  else if (y + ROW > el.scrollTop + el.clientHeight - PAD) el.scrollTop = y + ROW + PAD - el.clientHeight;
}
</script>

<template>
  <UModal v-model:open="open" title="Go to file" :ui="{ content: 'max-w-xl' }">
    <template #content>
      <div class="flex min-h-0 min-w-0 flex-col divide-y divide-default">
        <UInput
          v-model="query"
          variant="none"
          icon="i-lucide-search"
          placeholder="Go to file…"
          autofocus
          class="w-full [&>input]:h-12"
          role="combobox"
          aria-expanded="true"
          aria-controls="go-to-file-list"
          :aria-activedescendant="results.length ? `go-to-file-${active}` : undefined"
          @keydown="onKey"
        >
          <template #trailing>
            <UButton icon="i-lucide-x" color="neutral" variant="ghost" aria-label="Close" @click="open = false" />
          </template>
        </UInput>
        <div
          v-if="results.length"
          id="go-to-file-list"
          ref="list"
          role="listbox"
          aria-label="Project files"
          class="min-h-0 flex-1 overflow-y-auto p-1"
          @scroll="top = $event.target.scrollTop"
        >
          <div class="relative" :style="{ height: `${results.length * ROW}px` }">
            <div
              v-for="row in rows"
              :id="`go-to-file-${row.rank}`"
              :key="row.rank"
              role="option"
              :aria-selected="row.rank === active"
              class="absolute inset-x-0 flex h-8 cursor-pointer select-none items-center gap-1.5 rounded-md px-1.5 text-sm"
              :class="row.rank === active ? 'bg-elevated/50 text-highlighted' : 'text-default'"
              :style="{ top: `${row.rank * ROW}px` }"
              @mousemove="active = row.rank"
              @click="choose(row.path)"
            >
              <UIcon name="i-lucide-file" class="size-5 shrink-0" :class="row.rank === active ? 'text-default' : 'text-dimmed'" />
              <span class="truncate text-dimmed"><span
                v-for="(seg, i) in row.segments"
                :key="i"
                :class="seg.hit ? 'font-semibold' : ''"
              >{{ seg.text }}</span></span>
            </div>
          </div>
        </div>
        <p v-else class="py-6 text-center text-sm text-muted">{{ empty }}</p>
      </div>
    </template>
  </UModal>
</template>
