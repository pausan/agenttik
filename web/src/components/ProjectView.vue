<script setup>
/* A project in the centre: every task it has under Tasks, with project
   activity beside them under Stats.

   One list holds both states. Open tasks come first, in the order they were
   dragged into, drawn in the strongest text the theme has; archived ones
   follow, newest first, drawn grey — the colour is what says which is which,
   so a history and a working list read as one place rather than two.

   Open rows are dragged with the browser's own drag and drop rather than
   pointer maths. The list reorders under the cursor as you go, so where the
   row is when you let go is where it lands, and the new order is sent once on
   drop. Archived rows are not dragged and are not ordered by anyone:
   `sessions.position` is the order someone chose for the work in front of
   them, and a history is ordered by the clock. */
import { computed, nextTick, onMounted, ref, watch } from "vue";

import {
  S,
  openTask,
  renameTask,
  reorderTasks,
  setTaskArchived,
  startTask,
  stopTask,
} from "../store";
import { ago, cost, duration, isoDate, nf, tokens } from "../api";
import { fuzzyAny } from "../fuzzy";
import TaskRow from "./TaskRow.vue";

const props = defineProps({ tab: { type: Object, required: true } });

const dragging = ref("");
const renaming = ref(""); // the task whose title is being edited
const filter = ref("");
const filterField = ref(null);

const lower = ref("tasks");
const lowerTabs = [
  { label: "Tasks", value: "tasks" },
  { label: "Stats", value: "stats" },
];

/* Opening a project is asking which task, so the cursor starts in the filter
   — on the page itself and on every return to the tab. */
function focusFilter() {
  nextTick(() => filterField.value?.inputRef?.focus());
}
onMounted(focusFilter);
watch(lower, (which) => {
  if (which === "tasks") focusFilter();
});

const statsRows = computed(() => {
  const stats = props.tab.data.stats;
  return [
    ["Tasks", nf.format(stats.sessions)],
    ["Running now", nf.format(stats.running)],
    ["Turns", nf.format(stats.turns)],
    ["Input tokens", tokens(stats.input_tokens)],
    ["Output tokens", tokens(stats.output_tokens)],
    ["Cost", cost(stats.cost_usd)],
    ["Agent time", duration(stats.duration_ms)],
    ["Last used", isoDate(stats.last_active_at)],
  ];
});

const chart = computed(() => (props.tab.data.metrics || []).slice(-30));
const maxTasks = computed(() => Math.max(1, ...chart.value.map((point) => point.sessions)));
const maxDuration = computed(() => Math.max(1, ...chart.value.map((point) => point.duration_ms)));
const charts = computed(() => [
  { label: "Tasks created", key: "sessions", max: maxTasks.value, color: "bg-primary/70" },
  { label: "Agent time", key: "duration_ms", max: maxDuration.value, color: "bg-sky-500/70" },
]);

/* The filter reads titles, not prompts: a prompt is up to 600 characters and
   a subsequence match against one of those matches nearly anything typed.
   The prompt is still a hover away on every row.

   Matches keep each list's own order rather than moving the best one to the
   top — the question being asked of a task list is which one, and of a
   history when, and a best-match row jumping the queue loses both. */
const matching = (tasks) =>
  tasks.filter((s) => fuzzyAny([s.title || "Untitled task"], filter.value) !== null);

const open = computed(() => matching(props.tab.data.sessions));
const archived = computed(() => matching(props.tab.data.archived || []));
const total = computed(() => props.tab.data.sessions.length + (props.tab.data.archived?.length || 0));
const shown = computed(() => open.value.length + archived.value.length);

/* Dragging moves a row within the whole open list, so it is only offered
   when the whole open list is on screen. */
const filtering = computed(() => !!filter.value.trim());

const subtitle = (s) => `${s.model}${s.effort ? " · " + s.effort : ""} · ${ago(s.last_active_at)}`;

function onStart(e, id) {
  dragging.value = id;
  e.dataTransfer.effectAllowed = "move";
  // Firefox starts no drag at all without data on the transfer.
  e.dataTransfer.setData("text/plain", id);
}

/* onOver moves the dragged row to where the pointer is, so the list shows the
   result before the drop rather than after it. */
function onOver(e, overID) {
  if (!dragging.value || overID === dragging.value) return;
  e.preventDefault();
  const tasks = props.tab.data.sessions;
  const from = tasks.findIndex((s) => s.id === dragging.value);
  const to = tasks.findIndex((s) => s.id === overID);
  if (from < 0 || to < 0) return;
  tasks.splice(to, 0, ...tasks.splice(from, 1));
}

function onDrop() {
  if (!dragging.value) return;
  dragging.value = "";
  reorderTasks(props.tab, props.tab.data.sessions.map((s) => s.id));
}
</script>

<template>
  <div class="min-h-0 flex-1 overflow-auto">
    <div class="mx-auto max-w-[860px] px-6 py-5">
      <div class="mb-3.5 flex items-start gap-3 border-b border-default pb-3">
        <div class="min-w-0">
          <h2 class="m-0 text-[17px] tracking-tight text-highlighted">{{ tab.data.project.name }}</h2>
          <div class="truncate text-xs text-dimmed">{{ tab.data.project.path }}</div>
        </div>
        <UButton class="ml-auto shrink-0" label="New task" @click="startTask(tab.data.project)" />
      </div>

      <UTabs v-model="lower" :items="lowerTabs" :content="false" size="sm" class="mb-4" />

      <!-- Every task of the project: what is open, then what it has
           archived. -->
      <template v-if="lower === 'tasks'">
        <div class="mb-2 flex items-center gap-3">
          <UInput
            ref="filterField"
            v-model="filter"
            type="search"
            icon="i-lucide-search"
            placeholder="Filter tasks"
            class="min-w-0 flex-1"
          />
          <span class="shrink-0 text-xs text-dimmed tabular-nums">{{ shown }}/{{ total }}</span>
        </div>

        <p v-if="!total" class="px-3 py-5 text-center text-dimmed">No tasks in this project yet.</p>
        <p v-else-if="!shown" class="px-3 py-4 text-center text-dimmed">No task matches that.</p>

        <div
          v-for="s in open"
          :key="s.id"
          class="flex items-center gap-1"
          :class="dragging === s.id ? 'opacity-40' : ''"
          :draggable="renaming !== s.id && !filtering"
          @dragstart="onStart($event, s.id)"
          @dragover="onOver($event, s.id)"
          @drop.prevent="onDrop"
          @dragend="onDrop"
        >
          <UIcon
            name="i-lucide-grip-vertical"
            class="size-3.5 shrink-0 text-dimmed"
            :class="filtering ? 'opacity-30' : 'cursor-grab'"
            :title="filtering ? 'Clear the filter to reorder' : 'Drag to reorder'"
          />
          <TaskRow
            class="min-w-0 flex-1"
            :title="s.title"
            :prompt="s.prompt"
            :status="s.status"
            :queued="s.queue_count"
            :sub="subtitle(s)"
            :active="S.detail?.session.id === s.id"
            :stoppable="s.status === 'running' || s.queue_count > 0"
            :archive="s.status !== 'running' && s.queue_count === 0"
            @stop="stopTask(s.id)"
            @select="openTask(s.id)"
            @toggle-archive="setTaskArchived(s, true)"
            @rename="renameTask(s, $event)"
            @editing="renaming = $event ? s.id : ''"
          />
        </div>

        <!-- Archived rows carry the restore icon rather than the archive one,
             so this is also where a conversation comes back. The grip column
             is held empty so both kinds line up. -->
        <div v-for="s in archived" :key="s.id" class="flex items-center gap-1">
          <span class="size-3.5 shrink-0" aria-hidden="true" />
          <TaskRow
            class="min-w-0 flex-1"
            :title="s.title"
            :prompt="s.prompt"
            :status="s.status"
            :sub="subtitle(s)"
            :active="S.detail?.session.id === s.id"
            archived
            archive
            @select="openTask(s.id)"
            @toggle-archive="setTaskArchived(s, false)"
            @rename="renameTask(s, $event)"
          />
        </div>
      </template>

      <template v-else>
        <dl class="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <div v-for="[label, value] in statsRows" :key="label" class="rounded border border-default px-3 py-2">
            <dt class="text-xs text-dimmed">{{ label }}</dt>
            <dd class="m-0 mt-1 truncate font-medium text-highlighted tabular-nums">{{ value }}</dd>
          </div>
        </dl>

        <div class="mt-6 grid gap-6 sm:grid-cols-2">
          <section v-for="metric in charts" :key="metric.key">
            <div class="mb-2 flex items-baseline justify-between gap-2">
              <h3 class="m-0 text-xs font-semibold tracking-wide text-dimmed uppercase">{{ metric.label }}</h3>
              <span class="text-xs text-dimmed">Last 30 active days</span>
            </div>
            <div v-if="chart.length" class="flex h-28 items-end gap-px border-b border-default pb-px">
              <div v-for="point in chart" :key="point.date" :title="`${point.date}: ${point[metric.key]}`" class="flex h-full min-w-1 flex-1 items-end">
                <div class="w-full rounded-t-sm" :class="metric.color" :style="{ height: `${(point[metric.key] / metric.max) * 100}%` }" />
              </div>
            </div>
            <p v-else class="py-8 text-center text-dimmed">No activity yet.</p>
          </section>
        </div>
      </template>
    </div>
  </div>
</template>
