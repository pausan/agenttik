<script setup>
import { computed, nextTick, ref } from "vue";

import {
  PROJECT_KEYS,
  S,
  TAB_CHORDS,
  openInSystem,
  openProject,
  openSchedule,
  pickTask,
  renameSchedule,
  renameTask,
  reorderProjects,
  reorderSidebarTasks,
  scheduleLabel,
  setScheduleArchived,
  setSchedulePaused,
  setTaskArchived,
  stopTask,
  toggleProjectTasks,
} from "../store";
import { SEGMENTED } from "../ui";
import { beginDrag } from "../drag";
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

// One TaskRow per open task and one ScheduleRow per job, so F2 can reach the
// row for whatever is current without a dialog of its own.
const taskRows = new Map();
function setTaskRow(id, el) {
  if (el) taskRows.set(id, el);
  else taskRows.delete(id);
}

const scheduleRows = new Map();
function setScheduleRow(id, el) {
  if (el) scheduleRows.set(id, el);
  else scheduleRows.delete(id);
}

/* F2 edits the row of whatever is in front — a task, or a scheduled job,
   which is named and renamed exactly as a task is. The row may be behind
   Tree, or folded away under its project, so both are opened first: the same
   way a click would have to. */
async function editCurrent() {
  const schedule = S.owner?.kind === "schedule" ? S.owner : null;
  const session = schedule ? null : S.detail?.session;
  const projectID = schedule ? schedule.projectID : session?.project_id;
  const project = S.projects.find((p) => p.id === projectID);
  if (!project) return;
  show("projects");
  if (folded(project)) toggleProjectTasks(project.id);
  await nextTick();
  if (schedule) scheduleRows.get(schedule.scheduleID)?.edit();
  else taskRows.get(session.id)?.edit();
}

defineExpose({
  showProjects: () => show("projects"),
  showTree: () => show("tree"),
  editCurrent,
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

/* A menu per project row, unlike the Tree's one menu for every file: a
   sidebar holds a handful of projects, and each one already knows which it
   is without a click having to be aimed at it. */
const projectMenu = (p) => [
  {
    label: "Open in system browser",
    icon: "i-lucide-external-link",
    onSelect: () => openInSystem("", p.id),
  },
];

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

function onProjectStart(e, id) {
  if (S.projects.find((p) => p.id === id)?.kind === "orchestrator") {
    e.preventDefault();
    return;
  }
  draggingProject.value = id;
  beginDrag(e, id);
}

function onProjectOver(e, overID) {
  if (!draggingProject.value) return;
  if (S.projects.find((p) => p.id === overID)?.kind === "orchestrator") return;
  e.preventDefault();
  if (draggingProject.value === overID) return;
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
  if (!dragged || dragged.projectID !== projectID) return;
  e.stopPropagation();
  e.preventDefault();
  if (dragged.taskID === overID) return;
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
          <!-- The menu wraps the project row itself, not the block around
               it: what follows are the project's jobs and tasks, and a right
               click on one of those is not aimed at the project. -->
          <UContextMenu :items="projectMenu(p)" :ui="{ content: 'w-56' }">
            <div
              :draggable="p.kind !== 'orchestrator'"
              class="mb-0.5 flex items-start gap-1 rounded-[var(--ui-radius)] px-1.5 py-1"
              :class="[S.activeProjectID === p.id ? 'bg-primary/10' : 'hover:bg-elevated', p.kind === 'orchestrator' ? 'cursor-default' : 'cursor-grab active:cursor-grabbing']"
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
              <!-- A click on the row opens the project's own page, selected or
                   not: it is the overview, and the only way back to it once a
                   conversation is open on top. Alt and the project's letter is
                   the way to the work instead. -->
              <button
                type="button"
                class="min-w-0 flex-1 text-left"
                :title="projectKey(i) ? `${p.name}  (Alt+${projectKey(i)})` : p.name"
                @click="openProject(p.id)"
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
                  <UIcon
                    v-if="p.kind === 'orchestrator'"
                    name="i-lucide-pin"
                    class="size-3 shrink-0 text-dimmed"
                    title="Orchestrator · pinned first"
                    aria-label="Orchestrator · pinned first"
                  />
                  <StatusDot v-if="busy(p)" status="running" />
                </span>
                <span class="block truncate pl-4 text-xs text-dimmed">{{ p.path }}</span>
              </button>
            </div>
          </UContextMenu>
          <ScheduleRow
            v-for="sched in folded(p) ? [] : p.schedules"
            :key="'sched' + sched.id"
            :ref="(el) => setScheduleRow(sched.id, el)"
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
            :class="renaming === s.id ? '' : 'cursor-grab active:cursor-grabbing'"
            :draggable="renaming !== s.id"
            @dragstart.stop="onTaskStart($event, p.id, s.id)"
            @dragover="onTaskOver($event, p.id, s.id)"
            @drop="onTaskDrop"
            @dragend="onTaskDrop"
          >
            <TaskRow
              :ref="(el) => setTaskRow(s.id, el)"
              :title="s.title"
              :prompt="s.prompt"
              :status="s.status"
              :queued="s.queue_count"
              :active="S.detail?.session.id === s.id"
              :number="taskNumber(p, s)"
              :job="s.schedule_id"
              :stoppable="s.status === 'running' || s.queue_count > 0"
              :archive="s.status !== 'running' && s.queue_count === 0"
              @stop="stopTask(s.id)"
              @select="pickTask(s.id)"
              @open-job="openSchedule(s.schedule_id)"
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
        color="primary"
        variant="ghost"
        icon="i-lucide-folder-plus"
        title="Add project"
        aria-label="Add project"
        @click="$emit('add-project')"
      />
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
