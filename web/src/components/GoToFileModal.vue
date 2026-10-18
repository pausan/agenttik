<script setup>
/* Go to file lists every match, but draws only the rows in view: a project
   can hold 200,000 paths, and a palette row per match cost more than the
   search. Rows have one fixed height, so the scroll offset says which. The
   search runs in a worker (see file-search.js); until its answer arrives the
   last one stays on screen, and Enter waits for it. */
import { computed, ref, shallowRef, toRaw, useTemplateRef, watch } from "vue";
import { S, currentProjectID, openFileForEdit } from "../store";
import { matchHits } from "../file-search";
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

// The answer on screen: the query and listing it is for, how many paths
// match, and the places of the best in order, or null for listing order.
const answer = shallowRef({ query: "", paths: [], length: 0, ranked: null });
const waiting = ref(false); // for the answer to the newest query
let worker = null;
let held = null; // the listing the worker holds
let asked = null; // the newest request
let lastID = 0;
let enter = false; // Enter was pressed while waiting

function ask(all = false) {
  // Raw, as a proxy cannot be posted: an empty listing is not stored raw.
  const paths = toRaw(S.tree);
  worker ||= startWorker();
  if (held !== paths) {
    // One string crosses to the worker far faster than 200,000 do: 19 ms
    // against up to 84 ms in WebKit. No path holds a NUL.
    worker.postMessage({ listing: paths.join("\0"), ignoredFrom: paths.length - S.treeIgnored.length });
    held = paths;
  }
  if (!query.value.trim()) {
    asked = null;
    waiting.value = false;
    answer.value = { query: "", paths, length: paths.length, ranked: null };
    return;
  }
  // The rest of an answer is only asked for once, and never ahead of a
  // newer query, whose answer would then be dropped.
  if (all && (waiting.value || asked?.all)) return;
  asked = { id: ++lastID, query: query.value, paths, all };
  waiting.value = !all;
  worker.postMessage({ id: asked.id, query: asked.query, all });
}

function startWorker() {
  const w = new Worker(new URL("../file-search-worker.js", import.meta.url), { type: "module" });
  w.onmessage = ({ data }) => {
    if (data.id !== asked?.id) return;
    answer.value = { query: asked.query, paths: asked.paths, length: data.length, ranked: data.ranked };
    waiting.value = false;
    if (enter && open.value) pick();
    enter = false;
  };
  return w;
}

watch([open, query, () => S.tree], () => open.value && ask(), { immediate: true });
watch(answer, (now, before) => {
  if (now.query === before.query && now.paths === before.paths) return;
  active.value = 0;
  top.value = 0;
  if (list.value) list.value.scrollTop = 0;
});

/* placeOf is the listing place of the path at rank, or undefined when that
   rank is past those ranked so far. */
function placeOf(rank) {
  const { ranked } = answer.value;
  return ranked ? ranked[rank] : rank;
}

// The list is never taller than the window, so the window bounds the rows
// in view without measuring the list.
const view = computed(() => {
  const first = Math.max(0, Math.floor((top.value - PAD) / ROW) - OVERSCAN);
  return { first, last: Math.min(answer.value.length, first + Math.ceil(window.innerHeight / ROW) + 2 * OVERSCAN) };
});
const rows = computed(() => {
  const { paths, query: q } = answer.value;
  const out = [];
  for (let rank = view.value.first; rank < view.value.last; rank++) {
    const place = placeOf(rank);
    if (place === undefined) break;
    const path = paths[place];
    out.push({ rank, path, segments: segments(path, q && matchHits(path, q)) });
  }
  return out;
});
// Only the best rows come ranked; scrolling past them asks for the rest.
watch(() => answer.value.ranked && view.value.last > answer.value.ranked.length, (more) => more && ask(true));

const empty = computed(() => {
  if (!currentProjectID()) return "Select a project to search its files.";
  return S.treeTruncated ? "No files found in the part of this folder that is listed." : "No files found.";
});

function choose(path) {
  open.value = false;
  openFileForEdit(path);
}

function pick() {
  if (!answer.value.length) return;
  const place = placeOf(active.value);
  if (place === undefined) {
    enter = true;
    ask(true);
  } else choose(answer.value.paths[place]);
}

function onKey(e) {
  const count = answer.value.length;
  if (e.key === "Enter" && !e.isComposing) {
    e.preventDefault();
    if (waiting.value) enter = true;
    else pick();
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
          :aria-activedescendant="answer.length ? `go-to-file-${active}` : undefined"
          @keydown="onKey"
        >
          <template #trailing>
            <UButton icon="i-lucide-x" color="neutral" variant="ghost" aria-label="Close" @click="open = false" />
          </template>
        </UInput>
        <div
          v-if="answer.length"
          id="go-to-file-list"
          ref="list"
          role="listbox"
          aria-label="Project files"
          class="min-h-0 flex-1 overflow-y-auto p-1"
          @scroll="top = $event.target.scrollTop"
        >
          <div class="relative" :style="{ height: `${answer.length * ROW}px` }">
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
        <p v-else-if="!waiting" class="py-6 text-center text-sm text-muted">{{ empty }}</p>
      </div>
    </template>
  </UModal>
</template>
