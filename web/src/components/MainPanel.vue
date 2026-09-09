<script setup>
import { computed } from "vue";

import { S, closeTab, selectTab } from "../store";
import Transcript from "./Transcript.vue";
import ProjectView from "./ProjectView.vue";
import FileView from "./FileView.vue";
import PromptBar from "./PromptBar.vue";
import StatusDot from "./StatusDot.vue";

/* A session title can be a whole sentence, and the strip has to stay
   readable with a dozen of them open. */
const MAX_LABEL = 22;

const items = computed(() =>
  S.tabs.map((t, i) => ({
    label: t.label.length > MAX_LABEL ? t.label.slice(0, MAX_LABEL - 1) + "…" : t.label,
    value: t.id,
    title: t.label + (i < 9 ? `  (Alt+${i + 1})` : ""),
    // Only the first nine are one chord away, so only those show a number.
    hint: i < 9 ? String(i + 1) : "",
    running: t.kind === "session" && t.detail.running,
  })),
);

const active = computed({
  get: () => S.activeTab,
  set: (v) => selectTab(v),
});

const current = computed(() => S.tab);
const running = computed(() => !!S.detail?.running);
</script>

<template>
  <main class="flex min-h-0 min-w-0 flex-col">
    <div class="flex shrink-0 items-center gap-2.5 border-b border-default pr-3">
      <UTabs
        v-if="items.length"
        v-model="active"
        :items="items"
        :content="false"
        variant="link"
        class="min-w-0 flex-1"
        :ui="{ list: 'overflow-x-auto' }"
      >
        <template #leading="{ item }">
          <span
            v-if="item.hint"
            class="-mr-0.5 font-mono text-[10px] text-dimmed tabular-nums"
            aria-hidden="true"
            >{{ item.hint }}</span
          >
          <StatusDot v-if="item.running" status="running" />
        </template>
        <template #default="{ item }">
          <span :title="item.title">{{ item.label }}</span>
        </template>
        <template #trailing="{ item }">
          <span
            class="-mr-1 inline-flex size-4 items-center justify-center rounded text-dimmed hover:bg-accented hover:text-highlighted"
            :title="`Close ${item.label}`"
            @click.stop="closeTab(item.value)"
            >×</span
          >
        </template>
      </UTabs>
      <span v-else class="flex-1" />

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
    <FileView v-else-if="current?.kind === 'file'" :content="current.content" />
    <Transcript v-else />

    <PromptBar v-if="S.detail" />
  </main>
</template>
