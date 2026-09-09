<script setup>
/* The centre: the tab strip over whatever the tab in front is.

   The strip is the active project's tabs only. Each kind has its own colour,
   tabs can be dragged within their kind, and the numbers are simply where a
   tab now sits, so rearranging changes what Alt+1 … Alt+9 reach. */
import { computed, ref } from "vue";

import { S, closeTab, moveTab, persistTabOrder, selectTab, startCurrentSession } from "../store";
import Transcript from "./Transcript.vue";
import ProjectView from "./ProjectView.vue";
import FileView from "./FileView.vue";
import PromptBar from "./PromptBar.vue";
import StatusDot from "./StatusDot.vue";

/* A session title can be a whole sentence, and the strip has to stay
   readable with a dozen of them open. */
const MAX_SESSION_LABEL = 20;

/* One colour per kind, so what a tab is reads before its label does. */
const KINDS = {
  project: { text: "text-sky-500", edge: "border-sky-500" },
  session: { text: "text-primary", edge: "border-primary" },
  file: { text: "text-amber-500", edge: "border-amber-500" },
};

const items = computed(() =>
  S.strip.map((t, i) => ({
    id: t.id,
    kind: t.kind,
    // The tab is named by its whole label even when the strip shows less of
    // it, so a truncated tab is still addressable.
    name: t.label,
    label:
      t.kind === "session" && t.label.length > MAX_SESSION_LABEL
        ? t.label.slice(0, MAX_SESSION_LABEL - 3) + "..."
        : t.label,
    title: t.label + (i < 9 ? `  (Alt+${i + 1})` : ""),
    // Only the first nine are one chord away, so only those show a number.
    hint: i < 9 ? String(i + 1) : "",
    running: t.kind === "session" && t.detail.running,
    colors: KINDS[t.kind],
  })),
);

const dragging = ref("");

/* The strip reorders under the pointer, so where a tab is when it is let go
   is where it stays. moveTab refuses to mix kinds. */
function onOver(e, id) {
  if (!dragging.value || dragging.value === id) return;
  e.preventDefault();
  moveTab(dragging.value, id);
}

function onStart(e, id) {
  dragging.value = id;
  e.dataTransfer.effectAllowed = "move";
  // Firefox starts no drag at all without data on the transfer.
  e.dataTransfer.setData("text/plain", id);
}

function onEnd() {
  const id = dragging.value;
  dragging.value = "";
  persistTabOrder(id);
}

const current = computed(() => S.tab);
const running = computed(() => !!S.detail?.running);
</script>

<template>
  <main class="flex min-h-0 min-w-0 flex-col">
    <div class="flex shrink-0 items-center gap-2.5 border-b border-default pr-3">
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
          :class="[
            item.id === S.activeTab ? item.colors.edge : 'border-transparent',
            dragging === item.id ? 'opacity-40' : '',
          ]"
          @click="selectTab(item.id)"
          @dragstart="onStart($event, item.id)"
          @dragover="onOver($event, item.id)"
          @dragend="onEnd"
          @drop.prevent="onEnd"
        >
          <span
            v-if="item.hint"
            class="font-mono text-[10px] text-dimmed tabular-nums"
            aria-hidden="true"
            >{{ item.hint }}</span
          >
          <StatusDot v-if="item.running" status="running" />
          <span
            class="truncate"
            :class="[item.colors.text, item.id === S.activeTab ? 'font-medium' : 'opacity-75']"
            >{{ item.label }}</span
          >
          <span
            class="inline-flex size-4 cursor-pointer items-center justify-center rounded text-dimmed hover:bg-accented hover:text-highlighted"
            title="Close"
            @click.stop="closeTab(item.id)"
            >×</span
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
        class="shrink-0"
        title="New session  (Ctrl+N)"
        aria-label="New session"
        @click="startCurrentSession()"
      />

      <div class="flex shrink-0 items-center gap-1.5">
        <template v-if="S.project">
          <UBadge
            :color="S.project.stats.running ? 'primary' : 'neutral'"
            variant="soft"
            size="sm"
          >
            <StatusDot :status="S.project.stats.running ? 'running' : 'idle'" />
            {{ S.project.stats.running ? `${S.project.stats.running} running` : "idle" }}
          </UBadge>
          <UBadge color="neutral" variant="soft" size="sm">
            {{ S.project.stats.sessions }} {{ S.project.stats.sessions === 1 ? "session" : "sessions" }}
          </UBadge>
        </template>
        <template v-else-if="S.detail">
          <UBadge :color="running ? 'primary' : 'neutral'" variant="soft" size="sm">
            <StatusDot :status="running ? 'running' : 'idle'" />
            {{ running ? "running" : "idle" }}
          </UBadge>
          <UBadge color="neutral" variant="soft" size="sm">{{ S.detail.session.model }}</UBadge>
          <UBadge color="neutral" variant="soft" size="sm">
            {{ S.detail.session.effort || "default" }}
          </UBadge>
        </template>
      </div>
    </div>

    <ProjectView v-if="current?.kind === 'project'" :tab="current" />
    <FileView v-else-if="current?.kind === 'file'" :tab="current" />
    <Transcript v-else />

    <!-- A file carries its own bar. The prompt box belongs to a conversation,
         and under a file it is only in the way. -->
    <PromptBar v-if="S.detail && current?.kind !== 'file'" />
  </main>
</template>
