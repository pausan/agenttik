<script setup>
import { ref, watch } from "vue";

import { S, isDone, markDone, openProject, openSession, refreshSessions } from "../store";
import { ago } from "../api";
import { debounce } from "../debounce";
import { SEGMENTED } from "../ui";
import SessionRow from "./SessionRow.vue";

defineEmits(["add-project", "setup"]);

const tab = ref("projects");

const tabs = [
  { label: "Projects", value: "projects" },
  { label: "Sessions", value: "sessions" },
];

const windows = [
  { label: "Last day", value: "1d" },
  { label: "Last 3 days", value: "3d" },
  { label: "Last week", value: "7d" },
  { label: "Last month", value: "1mo" },
  { label: "All", value: "all" },
];

const reload = debounce(() => refreshSessions().catch(() => {}), 150);
watch(() => S.query, reload);
watch(() => S.window, () => refreshSessions().catch(() => {}));
</script>

<template>
  <aside class="flex min-h-0 flex-col bg-muted">
    <header class="flex items-center gap-2 px-3.5 pt-3 pb-2.5">
      <span class="size-4.5 rounded-[5px] bg-linear-[140deg] from-primary to-sky-500" aria-hidden="true" />
      <span class="font-semibold tracking-tight text-highlighted">agenttik</span>
    </header>

    <UTabs
      v-model="tab"
      :items="tabs"
      :content="false"
      size="sm"
      class="mx-2.5 mb-2 shrink-0"
      :ui="SEGMENTED"
    />

    <!-- Projects -->
    <template v-if="tab === 'projects'">
      <div class="shrink-0 px-2.5 pb-2">
        <UButton block color="primary" variant="soft" label="+ Add project" @click="$emit('add-project')" />
      </div>
      <div class="min-h-0 flex-1 overflow-auto">
        <p v-if="!S.projects.length" class="px-3 py-5 text-center text-dimmed">
          No projects yet. Add the folder you want to work in.
        </p>
        <div
          v-for="p in S.projects"
          :key="p.id"
          class="px-2.5 pt-1 pb-2.5 not-first:mt-2.5 not-first:border-t not-first:border-default"
        >
          <button
            type="button"
            title="Project stats, files and options"
            class="mb-0.5 block w-full rounded-[var(--ui-radius)] px-2 py-1 text-left hover:bg-elevated"
          @click="openProject(p.id)"
          >
            <span
              class="block truncate font-semibold"
              :class="S.project?.project.id === p.id ? 'text-primary' : 'text-highlighted'"
            >{{ p.name }}</span>
            <span class="block truncate text-xs text-dimmed">{{ p.path }}</span>
          </button>
          <SessionRow
            v-for="s in p.recent_sessions"
            :key="s.id"
            :title="s.title || 'Untitled session'"
            :status="s.status"
            :active="S.detail?.session.id === s.id"
            @select="openSession(s.id)"
          />
        </div>
      </div>
    </template>

    <!-- Sessions -->
    <template v-else>
      <div class="shrink-0 px-2.5 pb-2">
        <UInput v-model="S.query" type="search" placeholder="Filter sessions" class="w-full" />
      </div>
      <div class="shrink-0 px-2.5 pb-2">
        <USelect v-model="S.window" :items="windows" class="w-full" />
      </div>
      <div class="min-h-0 flex-1 overflow-auto px-2.5">
        <p v-if="!S.sessions.length" class="px-3 py-5 text-center text-dimmed">
          No sessions in this window.
        </p>
        <SessionRow
          v-for="s in S.sessions"
          :key="s.id"
          :title="s.title || 'Untitled session'"
          :status="s.status"
          :sub="`${s.project_name} · ${s.project_path} · ${ago(s.last_active_at)}`"
          :active="S.detail?.session.id === s.id"
          :done="isDone(s)"
          tick
          @select="openSession(s.id)"
          @toggle-done="markDone(s, !isDone(s))"
        />
      </div>
    </template>

    <footer class="shrink-0 border-t border-default p-2.5">
      <UButton
        block
        color="neutral"
        variant="ghost"
        icon="i-lucide-settings"
        label="Setup"
        @click="$emit('setup')"
      />
    </footer>
  </aside>
</template>
