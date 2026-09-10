<script setup>
/* A project in the centre: its open sessions, in the order you put them, with
   project history and activity below.

   Rows are dragged with the browser's own drag and drop rather than pointer
   maths. The list reorders under the cursor as you go, so where the row is
   when you let go is where it lands, and the new order is sent once on drop.

   The archived list is not dragged and is not ordered by anyone: it is
   history, newest at the top, behind a fuzzy filter. */
import { computed, ref } from "vue";

import {
  S,
  openSession,
  renameSession,
  reorderSessions,
  setSessionArchived,
  startSession,
  stopSession,
} from "../store";
import { ago, cost, duration, isoDate, nf, tokens } from "../api";
import { fuzzyAny } from "../fuzzy";
import SessionRow from "./SessionRow.vue";

const props = defineProps({ tab: { type: Object, required: true } });

const dragging = ref("");
const renaming = ref(""); // the session whose title is being edited
const filter = ref("");

const lower = ref("sessions");
const lowerTabs = [
  { label: "Sessions", value: "sessions" },
  { label: "Stats", value: "stats" },
];

const statsRows = computed(() => {
  const stats = props.tab.data.stats;
  return [
    ["Sessions", nf.format(stats.sessions)],
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
const maxSessions = computed(() => Math.max(1, ...chart.value.map((point) => point.sessions)));
const maxDuration = computed(() => Math.max(1, ...chart.value.map((point) => point.duration_ms)));
const charts = computed(() => [
  { label: "Sessions created", key: "sessions", max: maxSessions.value, color: "bg-primary/70" },
  { label: "Agent time", key: "duration_ms", max: maxDuration.value, color: "bg-sky-500/70" },
]);
/* The filter reads titles, not prompts: a prompt is up to 600 characters and
   a subsequence match against one of those matches nearly anything typed.
   The prompt is still a hover away on every row.

   Matches keep the server's order rather than moving the best one to the
   top — the point of the list is when a conversation happened. */
const matches = computed(() =>
  (props.tab.data.archived || []).filter(
    (s) => fuzzyAny([s.title || "Untitled session"], filter.value) !== null,
  ),
);

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
  const sessions = props.tab.data.sessions;
  const from = sessions.findIndex((s) => s.id === dragging.value);
  const to = sessions.findIndex((s) => s.id === overID);
  if (from < 0 || to < 0) return;
  sessions.splice(to, 0, ...sessions.splice(from, 1));
}

function onDrop() {
  if (!dragging.value) return;
  dragging.value = "";
  reorderSessions(props.tab, props.tab.data.sessions.map((s) => s.id));
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
        <UButton class="ml-auto shrink-0" label="New session" @click="startSession(tab.data.project)" />
      </div>

      <p v-if="!tab.data.sessions.length" class="px-3 py-5 text-center text-dimmed">
        Nothing open in this project.
      </p>
      <div
        v-for="s in tab.data.sessions"
        :key="s.id"
        class="flex items-center gap-1"
        :class="dragging === s.id ? 'opacity-40' : ''"
        :draggable="renaming !== s.id"
        @dragstart="onStart($event, s.id)"
        @dragover="onOver($event, s.id)"
        @drop.prevent="onDrop"
        @dragend="onDrop"
      >
        <UIcon
          name="i-lucide-grip-vertical"
          class="size-3.5 shrink-0 cursor-grab text-dimmed"
          title="Drag to reorder"
        />
        <SessionRow
          class="min-w-0 flex-1"
          :title="s.title"
          :status="s.status"
          :queued="s.queue_count"
          :sub="subtitle(s)"
          :active="S.detail?.session.id === s.id"
          :stoppable="s.status === 'running' || s.queue_count > 0"
          :archive="s.status !== 'running' && s.queue_count === 0"
          @stop="stopSession(s.id)"
          @select="openSession(s.id)"
          @toggle-archive="setSessionArchived(s, true)"
          @rename="renameSession(s, $event)"
          @editing="renaming = $event ? s.id : ''"
        />
      </div>

      <!-- Archived: newest at the top, oldest at the bottom. Only drawn once
           the project has archived something, so a new project is still just
           its open sessions. -->
      <section class="mt-6 border-t border-default pt-4">
        <UTabs v-model="lower" :items="lowerTabs" :content="false" size="sm" class="mb-4" />
        <template v-if="lower === 'sessions'">
          <div class="mb-2 flex items-center gap-3">
          <h3 class="m-0 shrink-0 text-xs font-semibold tracking-wide text-dimmed uppercase">
            Archived
          </h3>
          <UInput
            v-model="filter"
            type="search"
            icon="i-lucide-search"
            placeholder="Filter archived sessions"
            class="min-w-0 flex-1"
          />
          <span class="shrink-0 text-xs text-dimmed tabular-nums">
            {{ matches.length }}/{{ tab.data.archived.length }}
          </span>
        </div>

        <p v-if="!tab.data.archived.length" class="px-3 py-5 text-center text-dimmed">No archived sessions.</p>
        <p v-else-if="!matches.length" class="px-3 py-4 text-center text-dimmed">
          No archived session matches that.
        </p>
        <SessionRow
          v-for="s in matches"
          :key="s.id"
          :title="s.title"
          :prompt="s.prompt"
          :status="s.status"
          :sub="subtitle(s)"
          :active="S.detail?.session.id === s.id"
          archived
          archive
          @select="openSession(s.id)"
          @toggle-archive="setSessionArchived(s, false)"
          @rename="renameSession(s, $event)"
        />
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
      </section>
    </div>
  </div>
</template>
