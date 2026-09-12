<script setup>
import { computed, nextTick, ref, watch } from "vue";

import {
  S,
  removeProject,
  renameProject,
  setProjectArchived,
  startTask,
  updateProjectPath,
} from "../store";
import FolderPicker from "./FolderPicker.vue";

const name = ref("");
const path = ref("");
const confirming = ref(false);
const browsing = ref(false);
const picked = ref("");
const picker = ref(null);

watch(
  () => S.project?.project.name,
  (v) => (name.value = v || ""),
  { immediate: true },
);

watch(
  () => S.project?.project.path,
  (v) => (path.value = v || ""),
  { immediate: true },
);

/* The picker opens on the folder the project points at now, so the common
   case — it moved next door — is a click or two rather than a walk from
   home. */
watch(browsing, async (on) => {
  if (!on) return;
  picked.value = "";
  await nextTick();
  picker.value?.open(S.project?.project.path);
});

async function choose() {
  if (!picked.value.trim()) return;
  browsing.value = false;
  await updateProjectPath(S.project.project, picked.value);
}

async function remove() {
  confirming.value = false;
  await removeProject(S.project.project);
}

/* Archiving takes the project's rows out of the sidebar, and with them the
   only way to reach a turn and stop it. So it waits for the work to finish,
   the same rule a running task's own archive icon follows. */
const busy = computed(() => {
  const p = S.projects.find((candidate) => candidate.id === S.project?.project.id);
  if (!p) return false;
  return (
    p.recent_sessions.some((task) => task.status === "running" || task.queue_count > 0) ||
    p.schedules.some((schedule) => schedule.running)
  );
});
</script>

<template>
  <div v-if="S.project">
    <label class="mb-3 block text-xs font-medium text-muted">
      Name
      <UInput
        v-model="name"
        class="mt-1 w-full font-normal"
        @change="renameProject(S.project.project, name)"
        @keydown.enter="$event.target.blur()"
      />
    </label>

    <div class="mb-3 text-xs font-medium text-muted">
      <label for="project-folder">Folder</label>
      <div class="mt-1 flex gap-1">
        <UInput
          id="project-folder"
          v-model="path"
          class="min-w-0 flex-1 font-mono text-xs font-normal"
          @change="updateProjectPath(S.project.project, path)"
          @keydown.enter="$event.target.blur()"
        />
        <UButton
          color="neutral"
          variant="subtle"
          icon="i-lucide-folder-open"
          title="Browse for a folder"
          aria-label="Browse for a folder"
          @click="browsing = true"
        />
      </div>
    </div>

    <UButton block label="New task" @click="startTask(S.project.project)" />

    <p class="mt-3.5 mb-1.5 text-xs text-dimmed">
      Archiving takes the project out of the sidebar and stops its schedules. Its tasks and
      history are kept — Settings › Projects brings it back.
    </p>
    <UButton
      block
      color="neutral"
      variant="soft"
      icon="i-lucide-archive"
      label="Archive project"
      :disabled="busy"
      :title="busy ? 'Wait for its tasks to finish, or stop them first.' : ''"
      @click="setProjectArchived(S.project.project, true)"
    />

    <p class="mt-3.5 mb-1.5 text-xs text-dimmed">
      Deleting removes its tasks and history from agenttik. The folder on disk is untouched.
      <template v-if="S.project.project.kind === 'orchestrator'">
        Settings › Orchestrator can re-enable it with its saved name, folder and prompt.
      </template>
    </p>
    <UButton block color="error" variant="soft" label="Delete project" @click="confirming = true" />

    <UModal v-model:open="browsing" title="Project folder" :ui="{ content: 'max-w-lg' }">
      <template #body>
        <FolderPicker ref="picker" v-model="picked" @submit="choose" />
      </template>
      <template #footer>
        <div class="flex w-full justify-end gap-2">
          <UButton color="neutral" variant="ghost" label="Cancel" @click="browsing = false" />
          <UButton label="Select" @click="choose" />
        </div>
      </template>
    </UModal>

    <UModal v-model:open="confirming" title="Delete project">
      <template #body>
        <p class="text-muted">
          Delete <span class="font-medium text-highlighted">{{ S.project.project.name }}</span>?
          Its tasks and their history are deleted from agenttik. The folder on disk is untouched.
        </p>
      </template>
      <template #footer>
        <div class="flex w-full justify-end gap-2">
          <UButton color="neutral" variant="ghost" label="Cancel" @click="confirming = false" />
          <UButton color="error" label="Delete" @click="remove" />
        </div>
      </template>
    </UModal>
  </div>
</template>
