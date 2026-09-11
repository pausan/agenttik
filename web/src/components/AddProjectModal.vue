<script setup>
import { nextTick, ref, watch } from "vue";

import { addProject } from "../store";
import FolderPicker from "./FolderPicker.vue";

const open = defineModel("open", { type: Boolean, default: false });

const path = ref("");
const name = ref("");
const error = ref("");
const picker = ref(null);

watch(open, async (on) => {
  if (!on) return;
  path.value = "";
  name.value = "";
  error.value = "";
  await nextTick();
  picker.value?.open();
});

/* A rejected folder — one already added, or one that is not a directory —
   leaves the dialog open on the field that has to change, so the reason
   belongs beside that field rather than in a toast behind the dialog. Moving
   anywhere in the picker clears it: the message was about the old folder. */
watch(path, () => (error.value = ""));

async function add() {
  if (!path.value.trim()) return;
  try {
    await addProject(path.value.trim(), name.value.trim());
    open.value = false;
  } catch (e) {
    error.value = e?.message || String(e);
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
      <p v-if="error" class="mt-3 text-xs text-error">{{ error }}</p>
    </template>
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton color="neutral" variant="ghost" label="Cancel" @click="open = false" />
        <UButton label="Add" @click="add" />
      </div>
    </template>
  </UModal>
</template>
