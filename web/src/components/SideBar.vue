<script setup>
import { computed, nextTick, ref, watch } from "vue";

import {
  PROJECT_KEYS,
  S,
  TAB_CHORDS,
  isArchived,
  openProject,
  openSession,
  refreshSessions,
  renameSession,
  reorderProjects,
  reorderSidebarSessions,
  setSessionArchived,
  switchProject,
} from "../store";
import { ago } from "../api";
import { debounce } from "../debounce";
import { SEGMENTED } from "../ui";
import FileTree from "./FileTree.vue";
import SessionRow from "./SessionRow.vue";
import StatusDot from "./StatusDot.vue";

defineEmits(["add-project", "setup", "shortcuts"]);

const tab = ref("projects");
const sessionFilter = ref(null);
const fileTree = ref(null);

const tabs = [
  { label: "Projects", value: "projects" },
  { label: "Sessions", value: "sessions" },
  { label: "Tree", value: "tree" },
];

/* Sessions and Tree are each a filter over a list, so showing either one puts
   the cursor in that filter — the pane is opened to search it. Every way in
   goes through here, the strip's own clicks included, and repeating the chord
   on the pane already in front focuses it again rather than doing nothing. */
function show(which) {
  tab.value = which;
  if (which === "projects") return;
  nextTick(() =>
    which === "sessions" ? sessionFilter.value?.inputRef?.focus() : fileTree.value?.focus(),
  );
}

defineExpose({
  showProjects: () => show("projects"),
  showSessions: () => show("sessions"),
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
const renaming = ref(""); // the session whose title is being edited
const collapsedProjects = ref(new Set());

function toggleProject(id) {
  const collapsed = collapsedProjects.value;
  if (collapsed.has(id)) collapsed.delete(id);
  else collapsed.add(id);
}

/* The first eight rows answer to Alt and a letter, so the letter is where the
   row is, and dragging a project changes it. */
const projectKey = (i) => PROJECT_KEYS[i] || "";

/* A number in the sidebar means the same as the same number on the tab strip:
   Alt and that digit go there. So it is the session's place in the strip, not
   its place in this list — the strip also holds the project page and any open
   files, and a session that is not open has no number at all.

   One pass over the strip rather than a lookup per row, since the sidebar is
   redrawn whenever a tab changes. */
const numbers = computed(() => {
  const found = new Map();
  S.strip.slice(0, TAB_CHORDS).forEach((t, i) => {
    if (t.kind === "session") found.set(t.sessionID, i + 1);
  });
  return found;
});

/* Rows of the selected project keep the number column even when nothing
   reaches them, so the titles line up. Other projects have no strip. */
const sessionNumber = (project, session) =>
  S.activeProjectID === project.id ? numbers.value.get(session.id) || 0 : null;

/* A project is lit while any of its sessions is mid-turn, whether or not that
   conversation is the one on screen. */
const busy = (p) => p.recent_sessions.some((s) => s.status === "running");

/* Clicking a project selects it, which is what makes its tabs the ones on
   screen. Clicking the one already selected opens its page, since that is
   the only way back to it once conversations are open on top. */
function chooseProject(p) {
  if (S.activeProjectID === p.id) openProject(p.id);
  else switchProject(p.id);
}

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
      :model-value="tab"
      :items="tabs"
      :content="false"
      size="sm"
      class="mx-2.5 mb-2 shrink-0"
      :ui="SEGMENTED"
      @update:model-value="show(String($event))"
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
          v-for="(p, i) in S.projects"
          :key="p.id"
          class="px-2.5 pt-1 pb-2.5 not-first:mt-2.5 not-first:border-t not-first:border-default"
          @dragover="onProjectOver($event, p.id)"
          @drop.prevent="onProjectDrop"
        >
          <div
            draggable="true"
            class="mb-0.5 flex cursor-grab items-start gap-1 rounded-[var(--ui-radius)] px-1.5 py-1 active:cursor-grabbing"
            :class="[
              draggingProject === p.id ? 'opacity-40' : '',
              S.activeProjectID === p.id ? 'bg-primary/10' : 'hover:bg-elevated',
            ]"
            @dragstart="onProjectStart($event, p.id)"
            @dragend="onProjectDrop"
          >
            <button
              type="button"
              class="mt-0.5 shrink-0"
              :title="`${collapsedProjects.has(p.id) ? 'Expand' : 'Collapse'} sessions`"
              :aria-expanded="!collapsedProjects.has(p.id)"
              @click.stop="toggleProject(p.id)"
            >
              <UIcon
                :name="collapsedProjects.has(p.id) ? 'i-lucide-chevron-right' : 'i-lucide-chevron-down'"
                class="size-3.5 block text-dimmed"
              />
            </button>
            <button
              type="button"
              class="min-w-0 flex-1 text-left"
              :title="projectKey(i) ? `${p.name}  (Alt+${projectKey(i)})` : p.name"
              @click="chooseProject(p)"
            >
              <span class="flex items-center gap-1.5">
                <span
                  class="w-2.5 shrink-0 font-mono text-[10px] text-dimmed"
                  aria-hidden="true"
                  >{{ projectKey(i) }}</span
                >
                <span
                  class="min-w-0 flex-1 truncate font-semibold"
                  :class="S.activeProjectID === p.id ? 'text-primary' : 'text-highlighted'"
                >{{ p.name }}</span>
                <StatusDot v-if="busy(p)" status="running" />
              </span>
              <span class="block truncate pl-4 text-xs text-dimmed">{{ p.path }}</span>
            </button>
          </div>
          <div
            v-if="!collapsedProjects.has(p.id)"
            v-for="s in p.recent_sessions"
            :key="s.id"
            :class="[
              draggingSession?.sessionID === s.id ? 'opacity-40' : '',
              renaming === s.id ? '' : 'cursor-grab active:cursor-grabbing',
            ]"
            :draggable="renaming !== s.id"
            @dragstart.stop="onSessionStart($event, p.id, s.id)"
            @dragover="onSessionOver($event, p.id, s.id)"
            @drop="onSessionDrop"
            @dragend="onSessionDrop"
          >
            <SessionRow
              :title="s.title"
              :status="s.status"
              :queued="s.queue_count"
              :active="S.detail?.session.id === s.id"
              :number="sessionNumber(p, s)"
              archive
              @select="openSession(s.id)"
              @toggle-archive="setSessionArchived(s, true)"
              @rename="renameSession(s, $event)"
              @editing="renaming = $event ? s.id : ''"
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
          :title="s.title"
          :status="s.status"
          :queued="s.queue_count"
          :sub="`${s.project_name} · ${s.project_path} · ${ago(s.last_active_at)}`"
          :active="S.detail?.session.id === s.id"
          :archived="isArchived(s)"
          archive
          @select="openSession(s.id)"
          @toggle-archive="setSessionArchived(s, !isArchived(s))"
          @rename="renameSession(s, $event)"
        />
      </div>
    </template>

    <!-- Tree -->
    <template v-else>
      <div class="min-h-0 flex-1 overflow-auto p-2.5">
        <FileTree ref="fileTree" class="min-h-0 flex-1" />
      </div>
    </template>

    <footer class="flex shrink-0 items-center gap-1 border-t border-default p-2.5">
      <UButton
        block
        color="neutral"
        variant="ghost"
        icon="i-lucide-settings"
        label="Settings"
        class="min-w-0 flex-1"
        @click="$emit('setup')"
      />
      <UButton
        color="neutral"
        variant="ghost"
        icon="i-lucide-keyboard"
        title="Keyboard shortcuts"
        aria-label="Keyboard shortcuts"
        @click="$emit('shortcuts')"
      />
    </footer>
  </aside>
</template>
