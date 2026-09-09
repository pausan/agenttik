<script setup>
import { nextTick, ref, watch } from "vue";

import {
  S,
  isArchived,
  openProject,
  openSession,
  refreshSessions,
  reorderProjects,
  reorderSidebarSessions,
  setSessionArchived,
} from "../store";
import { ago } from "../api";
import { debounce } from "../debounce";
import { SEGMENTED } from "../ui";
import FileTree from "./FileTree.vue";
import SessionRow from "./SessionRow.vue";

defineEmits(["add-project", "setup"]);

const tab = ref("projects");
const sessionFilter = ref(null);

const tabs = [
  { label: "Projects", value: "projects" },
  { label: "Sessions", value: "sessions" },
  { label: "Tree", value: "tree" },
];

function show(which, focus = false) {
  tab.value = which;
  if (focus) nextTick(() => sessionFilter.value?.inputRef?.focus());
}

defineExpose({
  showProjects: () => show("projects"),
  showSessions: () => show("sessions", true),
  showTree: () => show("tree"),
});

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

const draggingProject = ref(0);
const draggingSession = ref(null);

function beginDrag(e, value) {
  e.dataTransfer.effectAllowed = "move";
  // Firefox does not start a drag unless the transfer carries data.
  e.dataTransfer.setData("text/plain", String(value));
}

function onProjectStart(e, id) {
  draggingProject.value = id;
  beginDrag(e, id);
}

function onProjectOver(e, overID) {
  if (!draggingProject.value || draggingProject.value === overID) return;
  e.preventDefault();
  const from = S.projects.findIndex((p) => p.id === draggingProject.value);
  const to = S.projects.findIndex((p) => p.id === overID);
  if (from < 0 || to < 0) return;
  S.projects.splice(to, 0, ...S.projects.splice(from, 1));
}

function onProjectDrop() {
  if (!draggingProject.value) return;
  draggingProject.value = 0;
  reorderProjects(S.projects.map((p) => p.id));
}

function onSessionStart(e, projectID, sessionID) {
  draggingSession.value = { projectID, sessionID };
  beginDrag(e, sessionID);
}

function onSessionOver(e, projectID, overID) {
  const dragged = draggingSession.value;
  if (!dragged || dragged.projectID !== projectID || dragged.sessionID === overID) return;
  e.stopPropagation();
  e.preventDefault();
  const project = S.projects.find((p) => p.id === projectID);
  if (!project) return;
  const from = project.recent_sessions.findIndex((s) => s.id === dragged.sessionID);
  const to = project.recent_sessions.findIndex((s) => s.id === overID);
  if (from < 0 || to < 0) return;
  project.recent_sessions.splice(to, 0, ...project.recent_sessions.splice(from, 1));
}

function onSessionDrop(e) {
  const dragged = draggingSession.value;
  if (!dragged) return;
  e.stopPropagation();
  e.preventDefault();
  draggingSession.value = null;
  const project = S.projects.find((p) => p.id === dragged.projectID);
  if (project) reorderSidebarSessions(project, project.recent_sessions.map((s) => s.id));
}
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
    >
      <template #default="{ item }">
        <template v-if="item.value === 'projects'"><span class="underline">P</span>rojects</template>
        <template v-else-if="item.value === 'sessions'"><span class="underline">S</span>essions</template>
        <template v-else><span class="underline">T</span>ree</template>
      </template>
    </UTabs>

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
          @dragover="onProjectOver($event, p.id)"
          @drop.prevent="onProjectDrop"
        >
          <button
            type="button"
            draggable="true"
            title="Project stats, files and options"
            class="mb-0.5 block w-full cursor-grab rounded-[var(--ui-radius)] px-2 py-1 text-left active:cursor-grabbing hover:bg-elevated"
            :class="draggingProject === p.id ? 'opacity-40' : ''"
            @dragstart="onProjectStart($event, p.id)"
            @dragend="onProjectDrop"
            @click="openProject(p.id)"
          >
            <span
              class="block truncate font-semibold"
              :class="S.project?.project.id === p.id ? 'text-primary' : 'text-highlighted'"
            >{{ p.name }}</span>
            <span class="block truncate text-xs text-dimmed">{{ p.path }}</span>
          </button>
          <div
            v-for="s in p.recent_sessions"
            :key="s.id"
            class="cursor-grab active:cursor-grabbing"
            draggable="true"
            :class="draggingSession?.sessionID === s.id ? 'opacity-40' : ''"
            @dragstart.stop="onSessionStart($event, p.id, s.id)"
            @dragover="onSessionOver($event, p.id, s.id)"
            @drop="onSessionDrop"
            @dragend="onSessionDrop"
          >
            <SessionRow
              :title="s.title || 'Untitled session'"
              :status="s.status"
              :active="S.detail?.session.id === s.id"
              archive
              @select="openSession(s.id)"
              @toggle-archive="setSessionArchived(s, true)"
            />
          </div>
        </div>
      </div>
    </template>

    <!-- Sessions -->
    <template v-else-if="tab === 'sessions'">
      <div class="shrink-0 px-2.5 pb-2">
        <UInput
          ref="sessionFilter"
          v-model="S.query"
          type="search"
          icon="i-lucide-search"
          placeholder="Filter sessions"
          class="w-full"
        />
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
          :archived="isArchived(s)"
          archive
          @select="openSession(s.id)"
          @toggle-archive="setSessionArchived(s, !isArchived(s))"
        />
      </div>
    </template>

    <!-- Tree -->
    <template v-else>
      <div class="min-h-0 flex-1 overflow-auto p-2.5">
        <FileTree class="min-h-0 flex-1" />
      </div>
    </template>

    <footer class="shrink-0 border-t border-default p-2.5">
      <UButton
        block
        color="neutral"
        variant="ghost"
        icon="i-lucide-settings"
        label="Settings"
        @click="$emit('setup')"
      />
    </footer>
  </aside>
</template>
