<script setup>
import { computed } from "vue";

import { S, closeTab, selectTab } from "../store";
import Transcript from "./Transcript.vue";
import ProjectView from "./ProjectView.vue";
import FileView from "./FileView.vue";
import PromptBar from "./PromptBar.vue";
import StatusDot from "./StatusDot.vue";

const items = computed(() =>
  S.tabs.map((t) => ({ label: t.label, value: t.id, closable: t.closable })),
);

const active = computed({
  get: () => S.activeTab,
  set: (v) => selectTab(v),
});

const current = computed(() => S.tabs.find((t) => t.id === S.activeTab));
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
      >
        <template #trailing="{ item }">
          <span
            v-if="item.closable"
            class="-mr-1 inline-flex size-4 items-center justify-center rounded text-dimmed hover:bg-accented hover:text-highlighted"
            @click.stop="closeTab(item.value)"
          >×</span>
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

    <ProjectView v-if="current?.id === 'project'" />
    <FileView v-else-if="current?.content !== undefined" :content="current.content" />
    <Transcript v-else />

    <PromptBar v-if="S.detail" />
  </main>
</template>
