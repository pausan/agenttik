<script setup>
/* The Tree's New file, New folder, Rename and Delete, in one dialog.

   Three of them ask for a name and differ only in their wording; the fourth
   asks a question that should not be answered by accident. One component, so
   the four read as the four lines they are rather than as four modals.

   `action` is what is being asked — the kind, and the row it was aimed at —
   and clearing it closes the dialog. A failure leaves it open with the name
   still in the box, because "that file already exists" is answered by typing
   a different one, not by starting again. */
import { computed, nextTick, ref, watch } from "vue";

import { createEntry, deleteEntry, fail, openFile, renameEntry } from "../store";

const action = defineModel("action", { type: Object, default: null });
const emit = defineEmits(["done"]);

const KINDS = {
  file: { title: "New file", confirm: "Create", label: "Name" },
  folder: { title: "New folder", confirm: "Create", label: "Name" },
  rename: { title: "Rename", confirm: "Rename", label: "New name" },
  delete: { title: "Delete", confirm: "Delete" },
};

const name = ref("");
const busy = ref(false);
const field = ref(null);

/* The dialog is read through `at` rather than off the model: a modal keeps
   drawing its body while it closes, and the action is already null by then. */
const at = computed(() => action.value || {});
const kind = computed(() => KINDS[at.value.kind] || null);
const asking = computed(() => !!kind.value?.label);
// Where a new entry lands: inside the folder aimed at, beside the file aimed
// at, or at the top of the project when the right click missed every row.
const parent = computed(() => {
  const { path = "", dir } = at.value;
  if (dir) return path;
  return path.includes("/") ? path.slice(0, path.lastIndexOf("/")) : "";
});

/* The box starts on the current name for a rename and empty for a new file,
   and is selected rather than only focused: typing over it is what a rename
   box is for. */
watch(action, async (now) => {
  if (!now) return;
  name.value = now.kind === "rename" ? now.path.split("/").pop() : "";
  busy.value = false;
  await nextTick();
  field.value?.inputRef?.select();
});

async function submit() {
  const now = action.value;
  if (!now || busy.value) return;
  if (asking.value && !name.value.trim()) return;
  busy.value = true;
  try {
    let landed = now.path;
    if (now.kind === "delete") await deleteEntry(now.path);
    else if (now.kind === "rename") landed = await renameEntry(now.path, name.value);
    else {
      landed = await createEntry(parent.value, name.value, now.kind === "folder");
      // A new file is made to be typed into, so it opens — pinned, because a
      // temporary tab is taken over by the next file clicked.
      if (landed && now.kind === "file") await openFile(landed, true);
    }
    action.value = null;
    if (landed) emit("done", landed);
  } catch (e) {
    fail(e);
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <UModal
    :open="!!action"
    :title="kind?.title"
    :ui="{ content: 'max-w-md' }"
    @update:open="!$event && (action = null)"
  >
    <template #body>
      <template v-if="asking">
        <label class="block text-xs font-medium text-muted">
          {{ kind.label }}
          <UInput
            ref="field"
            v-model="name"
            class="mt-1 w-full font-normal"
            :ui="{ base: 'font-mono' }"
            @keydown.enter.prevent="submit"
          />
        </label>
        <p v-if="at.kind !== 'rename'" class="path-clip mt-2 truncate text-xs text-dimmed">
          in <span>{{ parent || "the project folder" }}</span>
        </p>
      </template>
      <template v-else>
        <p class="path-clip truncate font-mono text-xs text-highlighted">
          <span>{{ at.path }}</span>
        </p>
        <p class="mt-2 text-muted">
          {{ at.dir ? "This folder and everything in it" : "This file" }} will be deleted from disk.
          It cannot be undone.
        </p>
      </template>
    </template>

    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton color="neutral" variant="ghost" label="Cancel" @click="action = null" />
        <UButton
          :color="at.kind === 'delete' ? 'error' : 'primary'"
          :label="kind?.confirm"
          :loading="busy"
          @click="submit"
        />
      </div>
    </template>
  </UModal>
</template>
