<script setup>
/* The centre: the tab strip over whatever the tab in front is.

   The strip is the active project's tabs only. Each kind has its own colour,
   tabs can be dragged within their kind. Session shortcuts use the sidebar
   order, so file tabs never consume an Alt number. */
import { computed, defineAsyncComponent, ref, watch } from "vue";

import {
  S,
  closeTab,
  moveTab,
  persistTabOrder,
  selectTab,
  startCurrentTask,
} from "../store";
import { beginDrag } from "../drag";
import { primaryChord } from "../platform";
import Transcript from "./Transcript.vue";
import PromptBar from "./PromptBar.vue";
import StatusDot from "./StatusDot.vue";

const emit = defineEmits(["start-tour"]);

/* Everything below is reached by a click or a chord, never by the first
   paint, so its code is fetched from its own chunk the moment it is first
   needed instead of being parsed on the way in. The chunks are built into
   the binary beside the main one, so this is still a read off the local
   server and never a network call. */
const ProjectView = defineAsyncComponent(() => import("./ProjectView.vue"));
const PinnedPromptView = defineAsyncComponent(() => import("./PinnedPromptView.vue"));
const ScheduleView = defineAsyncComponent(() => import("./ScheduleView.vue"));
const FileView = defineAsyncComponent(() => import("./FileView.vue"));
const StatsPane = defineAsyncComponent(() => import("./StatsPane.vue"));

/* A session title can be a whole sentence, and a job names itself exactly as
   a task does, so both are cut to keep the strip readable with a dozen of them
   open. Projects and files are named by a folder and a file, which are already
   as short as they are going to get. */
const MAX_LABEL = 32;
const named = (kind) => kind === "session" || kind === "schedule";
const short = (label) =>
  label.length > MAX_LABEL ? label.slice(0, MAX_LABEL - 3) + "..." : label;

/* One colour per kind, so what a tab is reads before its label does. */
const KINDS = {
  project: { text: "text-sky-500", edge: "border-sky-500" },
  session: { text: "text-primary", edge: "border-primary" },
  file: { text: "text-amber-500", edge: "border-amber-500" },
  schedule: { text: "text-violet-500", edge: "border-violet-500" },
};

const items = computed(() =>
  S.strip.map((t) => ({
    id: t.id,
    kind: t.kind,
    // The tab is named by its whole label even when the strip shows less of
    // it, so a truncated tab is still addressable.
    name: t.label,
    label: named(t.kind) ? short(t.label) : t.label,
    title:
      t.label +
      (t.temp ? "  (temporary — double click the file to keep it)" : ""),
    running: t.kind === "session" && t.detail.running,
    // Italic says "this one goes when the next file is clicked".
    temp: !!t.temp,
    colors: KINDS[t.kind],
  })),
);

const dragging = ref("");

/* The strip reorders under the pointer, so where a tab is when it is let go
   is where it stays. moveTab refuses to mix kinds. */
function onOver(e, id) {
  if (!dragging.value) return;
  e.preventDefault();
  if (dragging.value === id) return;
  moveTab(dragging.value, id);
}

function onStart(e, id) {
  dragging.value = id;
  beginDrag(e, id);
}

function onEnd() {
  const id = dragging.value;
  dragging.value = "";
  persistTabOrder(id);
}

const current = computed(() => S.tab);
const running = computed(() => !!S.detail?.running);

/* Stats belongs to the task in front. Leaving that task closes its popover so
   it cannot reopen unexpectedly on a later conversation. The shared flag
   also lets Command Palette open the same control. */
watch(
  () => S.detail?.session.id || "",
  () => (S.taskStatsOpen = false),
);

/* The badge says what the task is doing. "waiting" is its own state rather
   than a quiet idle: the task has a prompt it cannot run because the provider
   it needs is away, which is worth seeing without opening the transcript.
   See specs/045-provider-outage-retry.md. */
const badge = computed(() => {
  if (running.value) return { status: "running", color: "primary", label: "running" };
  if (S.detail?.session.status === "waiting") return { status: "waiting", color: "warning", label: "waiting" };
  return { status: "idle", color: "neutral", label: "idle" };
});
</script>

<template>
  <main class="flex min-h-0 min-w-0 flex-col">
    <div class="main-tab-bar flex shrink-0 items-center gap-2.5 border-b border-default pr-3">
      <div role="tablist" class="flex min-w-0 flex-1 items-center overflow-x-auto">
        <div
          v-for="item in items"
          :key="item.id"
          role="tab"
          draggable="true"
          :aria-selected="item.id === S.activeTab"
          :aria-label="item.name"
          :title="item.title"
          class="flex shrink-0 cursor-grab items-center gap-1.5 border-b-2 px-2.5 py-2 active:cursor-grabbing"
          :class="item.id === S.activeTab ? item.colors.edge : 'border-transparent'"
          @click="selectTab(item.id)"
          @dragstart="onStart($event, item.id)"
          @dragover="onOver($event, item.id)"
          @dragend="onEnd"
          @drop.prevent="onEnd"
        >
          <StatusDot v-if="item.running" status="running" />
          <span
            class="truncate"
            :class="[
              item.colors.text,
              item.id === S.activeTab ? 'font-medium' : 'opacity-75',
              item.temp ? 'italic' : '',
            ]"
            >{{ item.label }}</span
          >
          <button
            type="button"
            class="tab-close inline-flex size-4 shrink-0 cursor-pointer items-center justify-center rounded text-dimmed hover:bg-accented hover:text-highlighted"
            :aria-label="`Close ${item.name}`"
            title="Close"
            @click.stop="closeTab(item.id)"
            >×</button
          >
        </div>
      </div>

      <!-- Sits outside the scrolling strip so it stays reachable with a
           dozen tabs open. -->
      <UButton
        v-if="S.activeProjectID"
        icon="i-lucide-plus"
        color="neutral"
        variant="ghost"
        size="xs"
        class="shrink-0 max-md:hidden"
        :title="`New task (${primaryChord('N')})`"
        aria-label="New task"
        @click="startCurrentTask()"
      />

      <div class="flex shrink-0 items-center gap-1.5">
        <template v-if="S.project?.stats">
          <UBadge
            :color="S.project.stats.running ? 'primary' : 'neutral'"
            variant="soft"
            size="sm"
          >
            <StatusDot :status="S.project.stats.running ? 'running' : 'idle'" />
            {{ S.project.stats.running ? `${S.project.stats.running} running` : "idle" }}
          </UBadge>
          <UBadge color="neutral" variant="soft" size="sm" class="max-md:hidden">
            {{ S.project.stats.sessions }} {{ S.project.stats.sessions === 1 ? "task" : "tasks" }}
          </UBadge>
        </template>
        <template v-else-if="S.detail">
          <UBadge :color="badge.color" variant="soft" size="sm">
            <StatusDot :status="badge.status" />
            {{ badge.label }}
          </UBadge>
          <UBadge color="neutral" variant="soft" size="sm" class="max-md:hidden">{{ S.detail.session.model }}</UBadge>
          <UBadge color="neutral" variant="soft" size="sm" class="max-md:hidden">
            {{ S.detail.session.effort || "default" }}
          </UBadge>
          <UPopover
            v-model:open="S.taskStatsOpen"
            :content="{ side: 'bottom', align: 'end', sideOffset: 8 }"
          >
            <UButton
              icon="i-lucide-chart-pie"
              color="neutral"
              variant="ghost"
              size="xs"
              title="Task stats"
              aria-label="Task stats"
            />
            <template #content>
              <div class="max-h-[min(32rem,80dvh)] w-80 max-w-[calc(100vw-2rem)] overflow-auto p-3">
                <h3 class="m-0 mb-2 text-sm font-semibold text-highlighted">Task stats</h3>
                <StatsPane />
              </div>
            </template>
          </UPopover>
        </template>
      </div>
    </div>

    <ProjectView v-if="current?.kind === 'project'" :tab="current" />
    <PinnedPromptView v-else-if="current?.kind === 'schedule' && current.data.schedule.every === 'pinned'" :key="current.id" :tab="current" />
    <ScheduleView v-else-if="current?.kind === 'schedule'" :tab="current" />
    <FileView v-else-if="current?.kind === 'file'" :tab="current" />
    <Transcript v-else @start-tour="emit('start-tour')" />

    <!-- A file carries its own bar. The prompt box belongs to a conversation,
         and under a file it is only in the way. -->
    <PromptBar v-if="S.detail && current?.kind !== 'file'" />
  </main>
</template>
