<script setup>
import { ref, watch } from "vue";

import { S, removeProject, renameProject, startSession } from "../store";

const name = ref("");
const confirming = ref(false);

watch(
  () => S.project?.project.name,
  (v) => (name.value = v || ""),
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

    <div class="mb-3 text-xs font-medium text-muted">
      Folder
      <div class="mt-1 rounded-[var(--ui-radius)] bg-elevated px-2 py-1.5 font-mono text-xs font-normal wrap-anywhere text-highlighted">
        {{ S.project.project.path }}
      </div>
    </div>

    <UButton block label="New session" @click="startSession(S.project.project)" />

    <p class="mt-3.5 mb-1.5 text-xs text-dimmed">
      Deleting removes its sessions and history from agenttik. The folder on disk is untouched.
    </p>
    <UButton block color="error" variant="soft" label="Delete project" @click="confirming = true" />

    <UModal v-model:open="confirming" title="Delete project">
      <template #body>
        <p class="text-muted">
          Delete <span class="font-medium text-highlighted">{{ S.project.project.name }}</span>?
          Its sessions and their history are deleted from agenttik. The folder on disk is untouched.
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
