<script setup>
/* The dialog behind the Send menu's Schedule action. It asks for the two
   things that are not already on screen — how often, and how many times — and
   nothing else: the prompt, the project and the model are whatever the box
   was set to. See specs/028-scheduled-jobs.md. */
import { ref, watch } from "vue";

import { EVERY, createSchedule, loadScheduleDefaults } from "../store";

const open = defineModel("open", { type: Boolean, default: false });

/* Whatever was entered last is what the next one opens on, so a quarter of an
   hour is typed once rather than every time. */
const form = ref(loadScheduleDefaults());
const forever = ref(true);

watch(open, (on) => {
  if (!on) return;
  form.value = loadScheduleDefaults();
  forever.value = form.value.remaining === -1;
});

/* -1 is forever, and the counter reaches 0 on its own; 0 is not something to
   be asked for, so the toggle switches between forever and a count. */
function submit() {
  const remaining = forever.value ? -1 : Math.max(1, Math.trunc(Number(form.value.remaining) || 1));
  createSchedule({ ...form.value, remaining });
  open.value = false;
}
</script>

<template>
  <UModal v-model:open="open" title="Schedule this prompt" :ui="{ content: 'max-w-lg' }">
    <template #body>
      <label class="block text-xs font-medium text-muted">
        Repeat every
        <USelectMenu
          v-model="form.every"
          value-key="value"
          :items="EVERY"
          :search-input="false"
          class="mt-1 w-full font-normal"
        />
      </label>

      <div v-if="form.every === 'interval'" class="mt-3 flex gap-3">
        <label class="block flex-1 text-xs font-medium text-muted">
          Hours
          <UInput v-model="form.hours" type="number" min="0" class="mt-1 w-full font-normal" />
        </label>
        <label class="block flex-1 text-xs font-medium text-muted">
          Minutes
          <UInput v-model="form.minutes" type="number" min="0" class="mt-1 w-full font-normal" />
        </label>
      </div>
      <label v-else class="mt-3 block text-xs font-medium text-muted">
        At
        <UInput v-model="form.at" type="time" class="mt-1 w-full font-normal" />
        <span class="mt-1 block font-normal text-dimmed">
          Weekly and monthly repeat on the day you create them.
        </span>
      </label>

      <div class="mt-4 border-t border-default pt-3">
        <USwitch v-model="forever" label="Run forever" />
        <label v-if="!forever" class="mt-2 block text-xs font-medium text-muted">
          Number of runs
          <UInput
            v-model="form.remaining"
            type="number"
            min="1"
            class="mt-1 w-full font-normal"
          />
        </label>
      </div>
    </template>
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton color="neutral" variant="ghost" label="Cancel" @click="open = false" />
        <UButton label="Schedule" @click="submit" />
      </div>
    </template>
  </UModal>
</template>
