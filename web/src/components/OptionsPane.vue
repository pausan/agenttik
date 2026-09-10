<script setup>
import { ref, watch } from "vue";

import { S, removeProject, renameProject, startTask, updateProjectPath } from "../store";

const name = ref("");
const path = ref("");
const confirming = ref(false);

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

async function remove() {
  confirming.value = false;
  await removeProject(S.project.project);
}
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

    <label class="mb-3 block text-xs font-medium text-muted">
      Folder
      <UInput
        v-model="path"
        class="mt-1 w-full font-mono text-xs font-normal"
        @change="updateProjectPath(S.project.project, path)"
        @keydown.enter="$event.target.blur()"
      />
    </label>

    <UButton block label="New task" @click="startTask(S.project.project)" />

    <p class="mt-3.5 mb-1.5 text-xs text-dimmed">
      Deleting removes its tasks and history from agenttik. The folder on disk is untouched.
    </p>
    <UButton block color="error" variant="soft" label="Delete project" @click="confirming = true" />

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
