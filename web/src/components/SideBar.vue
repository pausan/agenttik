<script setup>
import { computed, nextTick, ref } from "vue";

import {
  PROJECT_KEYS,
  S,
  TAB_CHORDS,
  openProject,
  openSchedule,
  openTask,
  renameSchedule,
  renameTask,
  reorderProjects,
  reorderSidebarTasks,
  scheduleLabel,
  setScheduleArchived,
  setSchedulePaused,
  setTaskArchived,
  switchProject,
  stopTask,
  toggleProjectTasks,
} from "../store";
import { SEGMENTED } from "../ui";
import FileTree from "./FileTree.vue";
import ScheduleRow from "./ScheduleRow.vue";
import TaskRow from "./TaskRow.vue";
import StatusDot from "./StatusDot.vue";

defineEmits(["add-project", "setup", "shortcuts"]);

const tab = ref("projects");
const fileTree = ref(null);

const tabs = [
  { label: "Projects", value: "projects" },
  { label: "Tree", value: "tree" },
];

/* Opening Tree focuses its filter, including when it is already in front. */
function show(which) {
  tab.value = which;
  if (which === "projects") return;
  nextTick(() => fileTree.value?.focus());
}

defineExpose({
  showProjects: () => show("projects"),
  showTree: () => show("tree"),
});


const draggingProject = ref(0);
const draggingTask = ref(null);
const renaming = ref(""); // the task whose title is being edited

/* Folded projects live in the store, since Alt and a project's letter folds
   it as well as the chevron does. */
const folded = (p) => S.collapsedProjects.has(p.id);

/* The first eight rows answer to Alt and a letter, so the letter is where the
   row is, and dragging a project changes it. */
const projectKey = (i) => PROJECT_KEYS[i] || "";

/* A number in the sidebar is the task shortcut number:
   Alt and that digit go there. It is the task top-to-bottom position in
   this project list, whether or not its context is currently visible. Rows
   after the first nine keep the blank column and use Ctrl+PageUp/PageDown.

   One pass over the selected project keeps every sidebar row aligned. */
const numbers = computed(() => {
  const found = new Map();
  const project = S.projects.find((candidate) => candidate.id === S.activeProjectID);
  project?.recent_sessions.slice(0, TAB_CHORDS).forEach((task, i) => {
    found.set(task.id, i + 1);
  });
  return found;
});

/* Every row keeps the number column, whether or not a chord reaches it, so
   selecting another project does not slide its titles left as the numbers
   leave. */
const taskNumber = (project, task) =>
  S.activeProjectID === project.id ? numbers.value.get(task.id) || 0 : 0;

/* A project is lit while any of its tasks is mid-turn, whether or not that
   conversation is the one on screen. */
const busy = (p) =>
  p.recent_sessions.some((s) => s.status === "running") || p.schedules.some((s) => s.running);

/* The schedule tab in front, so its sidebar row is highlighted the way an
   open conversation's is. */
const activeSchedule = computed(() =>
  S.owner?.kind === "schedule" ? S.owner.scheduleID : 0,
);

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

function onTaskStart(e, projectID, taskID) {
  draggingTask.value = { projectID, taskID };
  beginDrag(e, taskID);
}

function onTaskOver(e, projectID, overID) {
  const dragged = draggingTask.value;
  if (!dragged || dragged.projectID !== projectID || dragged.taskID === overID) return;
  e.stopPropagation();
  e.preventDefault();
  const project = S.projects.find((p) => p.id === projectID);
  if (!project) return;
  const from = project.recent_sessions.findIndex((s) => s.id === dragged.taskID);
  const to = project.recent_sessions.findIndex((s) => s.id === overID);
  if (from < 0 || to < 0) return;
  project.recent_sessions.splice(to, 0, ...project.recent_sessions.splice(from, 1));
}

function onTaskDrop(e) {
  const dragged = draggingTask.value;
  if (!dragged) return;
  e.stopPropagation();
  e.preventDefault();
  draggingTask.value = null;
  const project = S.projects.find((p) => p.id === dragged.projectID);
  if (project) reorderSidebarTasks(project, project.recent_sessions.map((s) => s.id));
}
</script>

<template>
  <aside class="flex min-h-0 flex-col bg-muted">
    <UTabs
      :model-value="tab"
      :items="tabs"
      :content="false"
      size="sm"
      class="mx-2.5 mt-3 mb-2 shrink-0"
      :ui="SEGMENTED"
      @update:model-value="show(String($event))"
    >
      <template #default="{ item }">
        <template v-if="item.value === 'projects'"><span class="underline">P</span>rojects</template>
        <template v-else-if="item.value === 'sessions'"><span class="underline">S</span>asks</template>
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
          class="px-2.5 py-1 not-first:border-t not-first:border-muted"
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
              :title="`${folded(p) ? 'Expand' : 'Collapse'} tasks`"
              :aria-expanded="!folded(p)"
              @click.stop="toggleProjectTasks(p.id)"
            >
              <UIcon
                :name="folded(p) ? 'i-lucide-chevron-right' : 'i-lucide-chevron-down'"
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
          <ScheduleRow
            v-for="sched in folded(p) ? [] : p.schedules"
            :key="'sched' + sched.id"
            :schedule="sched"
            :active="activeSchedule === sched.id"
            @select="openSchedule(sched.id)"
            @toggle-paused="setSchedulePaused(sched, !sched.paused)"
            @toggle-archive="setScheduleArchived(sched, true)"
            @rename="renameSchedule(sched, $event)"
          />
          <div
            v-if="!folded(p)"
            v-for="s in p.recent_sessions"
            :key="s.id"
            :class="[
              draggingTask?.taskID === s.id ? 'opacity-40' : '',
              renaming === s.id ? '' : 'cursor-grab active:cursor-grabbing',
            ]"
            :draggable="renaming !== s.id"
            @dragstart.stop="onTaskStart($event, p.id, s.id)"
            @dragover="onTaskOver($event, p.id, s.id)"
            @drop="onTaskDrop"
            @dragend="onTaskDrop"
          >
            <TaskRow
              :title="s.title"
              :prompt="s.prompt"
              :status="s.status"
              :queued="s.queue_count"
              :active="S.detail?.session.id === s.id"
              :number="taskNumber(p, s)"
              :stoppable="s.status === 'running' || s.queue_count > 0"
              :archive="s.status !== 'running' && s.queue_count === 0"
              @stop="stopTask(s.id)"
              @select="openTask(s.id)"
              @toggle-archive="setTaskArchived(s, true)"
              @rename="renameTask(s, $event)"
              @editing="renaming = $event ? s.id : ''"
            />
          </div>
        </div>
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
