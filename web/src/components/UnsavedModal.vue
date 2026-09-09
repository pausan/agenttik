<script setup>
/* Closing a file that has been typed into asks first.

   Three answers, because two would be a trap: Save writes and closes, Don't
   save closes and throws the edits away, and Continue editing puts you back
   in the file. It is driven from the store rather than opened by a caller,
   because the close it interrupts can come from the tab's ×, from Ctrl+W, or
   from a conversation being closed with files open beneath it. */
import { computed } from "vue";

import { S, resolveClosing } from "../store";

const open = computed(() => !!S.closing);
const files = computed(() => S.closing?.tabs || []);
const saving = computed(() => files.value.some((t) => t.saving));
</script>

<template>
  <UModal
    :open="open"
    title="Unsaved changes"
    :dismissible="false"
    :close="false"
    :ui="{ content: 'max-w-md' }"
    @update:open="!$event && resolveClosing('cancel')"
  >
    <template #body>
      <p class="text-muted">
        {{ files.length === 1 ? "This file has" : "These files have" }} changes that have not been
        written to disk.
      </p>
      <ul class="mt-2.5 space-y-0.5">
        <li v-for="f in files" :key="f.id" class="path-clip truncate font-mono text-xs text-dimmed">
          <span>{{ f.path }}</span>
        </li>
      </ul>
    </template>

    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton color="neutral" variant="ghost" @click="resolveClosing('cancel')">
          Continue editing
        </UButton>
        <UButton color="neutral" variant="subtle" @click="resolveClosing('discard')">
          Don't save
        </UButton>
        <UButton color="primary" :loading="saving" @click="resolveClosing('save')">Save</UButton>
      </div>
    </template>
  </UModal>
</template>
