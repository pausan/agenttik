<script setup>
/* A project in the centre: every task it has under Tasks, with project
   activity beside them under Stats.

   One list holds both states. Open tasks come first, in the order they were
   dragged into, drawn in the strongest text the theme has; archived ones
   follow, newest first, drawn grey — the colour is what says which is which,
   so a history and a working list read as one place rather than two. It is
   drawn a page at a time, 25 rows by default.

   Open rows are dragged with the browser's own drag and drop rather than
   pointer maths. The list reorders under the cursor as you go, so where the
   row is when you let go is where it lands, and the new order is sent once on
   drop. Archived rows are not dragged and are not ordered by anyone:
   `sessions.position` is the order someone chose for the work in front of
   them, and a history is ordered by the clock. */
import { computed, nextTick, onMounted, ref, watch } from "vue";

import {
  S,
  TASK_PAGE_SIZES,
  openSchedule,
  pickTask,
  removeTask,
  renameSchedule,
  renameTask,
  reorderTasks,
  resetOrchestratorPrompt,
  scheduleLabel,
  setProjectPrompt,
  setScheduleArchived,
  setSchedulePaused,
  setTaskArchived,
  setTaskPageSize,
  startTask,
  stopTask,
} from "../store";
import { ago, cost, duration, isoDate, isoLocal, nf, tokens, usageBreakdownRows } from "../api";
import { fuzzyAny } from "../fuzzy";
import { beginDrag } from "../drag";
import ScheduleRow from "./ScheduleRow.vue";
import TaskRow from "./TaskRow.vue";

const props = defineProps({ tab: { type: Object, required: true } });

const dragging = ref("");
const renaming = ref(""); // the task whose title is being edited
const filter = ref("");
const filterField = ref(null);
const page = ref(1);
const deleting = ref(null); // the task awaiting its delete confirmation

/* Which pane is open lives on the tab, not here: one ProjectView serves every
   project, so a pane kept locally would follow the reader into the next
   project. It also lets the transcript's injected-prompt chip open this page
   straight onto Prompt, whether or not the page was already built. */
const lower = computed({
  get: () => props.tab.pane || "tasks",
  set: (pane) => (props.tab.pane = pane),
});
const lowerTabs = [
  { label: "Tasks", value: "tasks" },
  { label: "Jobs", value: "jobs" },
  { label: "Prompt", value: "prompt" },
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

/* The prompt every new task in this project opens with. It is bound to the
   project only while it is not being typed in — a turn ending elsewhere
   refreshes the page, and that must not take the caret with it — and commits
   when it is left, as the schedule view's prompt does. Tasks already under way
   keep the copy they were given; only the next one follows an edit. */
const prompt = ref(props.tab.data.project.prompt || "");
const editingPrompt = ref(false);
const resettingPrompt = ref(false);
let promptSave = Promise.resolve();
watch(
  () => props.tab.data.project,
  (project) => {
    if (!editingPrompt.value) prompt.value = project.prompt || "";
  },
);

function commitPrompt() {
  editingPrompt.value = false;
  promptSave = setProjectPrompt(props.tab.data.project, prompt.value);
}

async function resetPrompt() {
  resettingPrompt.value = true;
  try {
    // Clicking reset blurs the editor first. Let that save finish before
    // resetting, so a slow save cannot overwrite the restored default.
    await promptSave;
    await resetOrchestratorPrompt();
  } finally {
    resettingPrompt.value = false;
  }
}

/* One ProjectView serves every project — switching to another one hands it a
   new tab rather than mounting a second component — so what belongs to the
   page in front is put back by hand. A filter or a page number carried into
   another project's tasks means nothing. */
watch(
  () => props.tab.data.project.id,
  () => {
    filter.value = "";
    page.value = 1;
    editingPrompt.value = false;
    prompt.value = props.tab.data.project.prompt || "";
    focusFilter();
  },
);

const statsRows = computed(() => {
  const stats = props.tab.data.stats;
  return [
    ["Tasks", nf.format(stats.sessions)],
    ["Running now", nf.format(stats.running)],
    ["Turns", nf.format(stats.turns)],
    ["Task input tokens", tokens(stats.input_tokens)],
    ["Task output tokens", tokens(stats.output_tokens)],
    ...usageBreakdownRows(stats),
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

/* One array of rows, each saying which state it is in, so the two lists page
   as one and the template stays a single loop. */
const rows = computed(() => [
  ...open.value.map((task) => ({ task, archived: false })),
  ...archived.value.map((task) => ({ task, archived: true })),
]);
const shown = computed(() => rows.value.length);

/* The project's scheduled jobs, read the way its tasks are: what is still
   scheduled first, in the order the sidebar was dragged into, then what has
   been archived, newest first and grey. The sidebar holds only the open ones,
   so this is the only place an archived job is seen — and the restore icon on
   its row is the only way back. See specs/028-scheduled-jobs.md. */
const jobs = computed(() => {
  const all = props.tab.data.schedules || [];
  return [
    ...all.filter((s) => !s.done_at).map((schedule) => ({ schedule, archived: false })),
    ...all
      .filter((s) => s.done_at)
      .sort((a, b) => b.done_at - a.done_at)
      .map((schedule) => ({ schedule, archived: true })),
  ];
});

const activeJob = computed(() => (S.owner?.kind === "schedule" ? S.owner.scheduleID : 0));

/* A job's clock, then what it is doing with it: when the next run is due, or
   that it is stopped — a paused job has no next run, and an archived one is
   dated by when it was put away. */
function jobSub({ schedule, archived }) {
  const clock = scheduleLabel(schedule);
  if (archived) return `${clock} · archived ${ago(schedule.done_at)}`;
  if (schedule.paused) return `${clock} · paused`;
  return `${clock} · next run ${isoLocal(schedule.next_run_at)}`;
}

const sizeItems = TASK_PAGE_SIZES.map((size) => ({ label: `${size} / page`, value: size }));
const size = computed({
  get: () => S.taskPageSize,
  set: setTaskPageSize,
});
const pages = computed(() => Math.max(1, Math.ceil(shown.value / size.value)));
const visible = computed(() => rows.value.slice((page.value - 1) * size.value, page.value * size.value));
const range = computed(() => {
  const first = (page.value - 1) * size.value;
  return `${first + 1}–${first + visible.value.length} of ${shown.value}`;
});

/* Narrowing the list, or making the pages bigger, is a new question: it is
   asked from the top. A list that shrinks under the page in front — a task
   archived off the last page — pulls it back to the last one there is. */
watch([filter, size], () => (page.value = 1));
watch(pages, (count) => {
  if (page.value > count) page.value = count;
});

/* Dragging moves a row within the whole open list, so it is only offered
   when the whole open list is on screen. */
const filtering = computed(() => !!filter.value.trim());

const subtitle = (s) => `${s.model}${s.effort ? " · " + s.effort : ""} · ${ago(s.last_active_at)}`;

function onStart(e, id) {
  dragging.value = id;
  beginDrag(e, id);
}

/* onOver moves the dragged row to where the pointer is, so the list shows the
   result before the drop rather than after it. */
function onOver(e, overID) {
  if (!dragging.value) return;
  e.preventDefault();
  if (overID === dragging.value) return;
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

async function doDelete() {
  const task = deleting.value;
  deleting.value = null;
  if (task) await removeTask(task);
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

      <!-- Every task of the project, a page at a time: what is open, then
           what it has archived. -->
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
          <USelect
            v-if="total > TASK_PAGE_SIZES[0]"
            v-model="size"
            :items="sizeItems"
            size="sm"
            class="shrink-0"
            title="Tasks per page"
          />
          <span class="shrink-0 text-xs text-dimmed tabular-nums">{{ shown }}/{{ total }}</span>
        </div>

        <p v-if="!total" class="px-3 py-5 text-center text-dimmed">No tasks in this project yet.</p>
        <p v-else-if="!shown" class="px-3 py-4 text-center text-dimmed">No task matches that.</p>

        <!-- An archived row is not dragged, and holds the grip column empty so
             both kinds line up. It carries the restore icon rather than the
             archive one, so this is also where a conversation comes back. -->
        <div
          v-for="row in visible"
          :key="row.task.id"
          class="flex items-center gap-1"
          :draggable="!row.archived && renaming !== row.task.id && !filtering"
          @dragstart="onStart($event, row.task.id)"
          @dragover="onOver($event, row.task.id)"
          @drop.prevent="onDrop"
          @dragend="onDrop"
        >
          <UIcon
            v-if="!row.archived"
            name="i-lucide-grip-vertical"
            class="size-3.5 shrink-0 text-dimmed"
            :class="filtering ? 'opacity-30' : 'cursor-grab'"
            :title="filtering ? 'Clear the filter to reorder' : 'Drag to reorder'"
          />
          <span v-else class="size-3.5 shrink-0" aria-hidden="true" />
          <TaskRow
            class="min-w-0 flex-1"
            :title="row.task.title"
            :prompt="row.task.prompt"
            :status="row.task.status"
            :queued="row.task.queue_count"
            :sub="subtitle(row.task)"
            :outcome="row.archived ? row.task.summary : ''"
            :job="row.task.schedule_id"
            :active="S.detail?.session.id === row.task.id"
            :archived="row.archived"
            :stoppable="row.task.status === 'running' || row.task.queue_count > 0"
            :archive="row.archived || (row.task.status !== 'running' && row.task.queue_count === 0)"
            deletable
            @stop="stopTask(row.task.id)"
            @select="pickTask(row.task.id)"
            @open-job="openSchedule(row.task.schedule_id)"
            @toggle-archive="setTaskArchived(row.task, !row.archived)"
            @rename="renameTask(row.task, $event)"
            @editing="renaming = $event ? row.task.id : ''"
            @delete="deleting = row.task"
          />
        </div>

        <div v-if="pages > 1" class="mt-3 flex items-center justify-between gap-3">
          <span class="text-xs text-dimmed tabular-nums">{{ range }}</span>
          <UPagination
            v-model:page="page"
            :total="shown"
            :items-per-page="size"
            :sibling-count="1"
            size="sm"
          />
        </div>
      </template>

      <!-- Every job of the project: what is still scheduled, then what it has
           archived. An archived job is paused by definition, so its row drops
           the pause control and carries the restore one. -->
      <template v-else-if="lower === 'jobs'">
        <p v-if="!jobs.length" class="px-3 py-5 text-center text-dimmed">
          No scheduled jobs in this project yet.
        </p>
        <ScheduleRow
          v-for="row in jobs"
          :key="row.schedule.id"
          :schedule="row.schedule"
          :sub="jobSub(row)"
          :archived="row.archived"
          :active="activeJob === row.schedule.id"
          @select="openSchedule(row.schedule.id)"
          @toggle-paused="setSchedulePaused(row.schedule, !row.schedule.paused)"
          @toggle-archive="setScheduleArchived(row.schedule, !row.archived)"
          @rename="renameSchedule(row.schedule, $event)"
        />
      </template>

      <template v-else-if="lower === 'prompt'">
        <div class="mb-2 flex items-start gap-3">
          <p class="min-w-0 flex-1 text-sm text-muted">
            Sent to the agent ahead of the first prompt of every task started here, so every
            conversation in this project begins knowing it. Leave it empty to send nothing.
          </p>
          <UButton
            v-if="tab.data.project.kind === 'orchestrator'"
            class="shrink-0"
            color="neutral"
            variant="subtle"
            icon="i-lucide-rotate-ccw"
            label="Reset prompt to default"
            :disabled="resettingPrompt"
            @click="resetPrompt"
          />
        </div>
        <UTextarea
          v-model="prompt"
          :disabled="resettingPrompt"
          :rows="tab.data.project.kind === 'orchestrator' ? 18 : 10"
          :autoresize="tab.data.project.kind !== 'orchestrator'"
          class="w-full"
          placeholder="e.g. Read AGENTS.md before you start. Never push."
          aria-label="Project prompt"
          :ui="{ base: 'resize-y font-mono text-xs' }"
          @focus="editingPrompt = true"
          @blur="commitPrompt"
        />
        <p class="mt-2 text-xs text-dimmed">
          Saved when you click away. Tasks already running keep the prompt they opened with.
        </p>
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

      <UModal :open="!!deleting" title="Delete task" @update:open="!$event && (deleting = null)">
        <template #body>
          <p class="text-muted">
            Delete <span class="font-medium text-highlighted">{{ deleting?.title || "Untitled task" }}</span>?
            Its transcript and history are deleted from agenttik. This cannot be undone.
          </p>
        </template>
        <template #footer>
          <div class="flex w-full justify-end gap-2">
            <UButton color="neutral" variant="ghost" label="Cancel" @click="deleting = null" />
            <UButton color="error" label="Delete" @click="doDelete" />
          </div>
        </template>
      </UModal>
    </div>
  </div>
</template>
