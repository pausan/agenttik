<script setup>
import { nextTick, ref, watch } from "vue";

import { addProject, fail } from "../store";
import FolderPicker from "./FolderPicker.vue";

const open = defineModel("open", { type: Boolean, default: false });

const path = ref("");
const name = ref("");
const picker = ref(null);

watch(open, async (on) => {
  if (!on) return;
  path.value = "";
  name.value = "";
  await nextTick();
  picker.value?.open();
});

async function add() {
  if (!path.value.trim()) return;
  try {
    await addProject(path.value.trim(), name.value.trim());
    open.value = false;
  } catch (e) {
    fail(e);
  }
}
</script>

<template>
  <UModal v-model:open="open" title="Add project" :ui="{ content: 'max-w-lg' }">
    <template #body>
      <FolderPicker ref="picker" v-model="path" @submit="add" />
      <label class="mt-3 block text-xs font-medium text-muted">
        Name
        <UInput
          v-model="name"
          class="mt-1 w-full font-normal"
          placeholder="defaults to the folder name"
        />
      </label>
    </template>
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton color="neutral" variant="ghost" label="Cancel" @click="open = false" />
        <UButton label="Add" @click="add" />
      </div>
    </template>
  </UModal>
</template>
