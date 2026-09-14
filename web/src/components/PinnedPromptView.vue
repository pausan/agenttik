<script setup>
import { computed, ref, watch } from "vue";
import { patchSchedule, removeSchedule, runPinnedPrompt, setScheduleArchived, setScheduleModel } from "../store";
import ModelSelection from "./ModelSelection.vue";

const props = defineProps({ tab: { type: Object, required: true } });
const saved = computed(() => props.tab.data.schedule);
const prompt = ref(saved.value.prompt);
const busy = ref(false);
const customizing = ref(false);
const customPrompt = ref("");
const dirty = computed(() => prompt.value !== saved.value.prompt);
watch(saved, (value, old) => {
  if (prompt.value === old.prompt) prompt.value = value.prompt;
});
async function save() {
  if (!prompt.value.trim() || busy.value) return;
  busy.value = true;
  try {
    const updated = await patchSchedule(saved.value, { prompt: prompt.value });
    if (updated) prompt.value = updated.prompt;
  } finally { busy.value = false; }
}
function customize() {
  customPrompt.value = saved.value.prompt;
  customizing.value = true;
}
async function play(custom) {
  if (busy.value || (custom && !customPrompt.value.trim())) return;
  busy.value = true;
  try {
    if (await runPinnedPrompt(saved.value, custom ? customPrompt.value : undefined)) customizing.value = false;
  } finally { busy.value = false; }
}
function chooseModel(choice) {
  setScheduleModel(saved.value, choice.provider, choice.model, choice.effort, choice.accountID);
}
</script>

<template>
  <div class="min-h-0 flex-1 overflow-auto">
    <div class="mx-auto max-w-[860px] px-6 py-5">
      <div class="mb-4 flex flex-wrap items-center gap-2 border-b border-default pb-3">
        <UIcon name="i-lucide-pin" class="size-4 text-primary" />
        <h2 class="mr-auto text-lg text-highlighted">{{ saved.title }}</h2>
        <template v-if="!saved.done_at">
          <UButton icon="i-lucide-play" label="Play" :disabled="busy" @click="play(false)" />
          <UButton label="Customize and run…" color="neutral" variant="outline" :disabled="busy" @click="customize" />
        </template>
        <UButton v-else label="Unarchive" @click="setScheduleArchived(saved, false)" />
        <UButton label="Delete" icon="i-lucide-trash-2" color="error" variant="ghost" @click="removeSchedule(saved)" />
      </div>
      <div class="mb-3 flex flex-wrap items-center gap-2">
        <span class="text-sm text-muted">Pinned task</span>
        <ModelSelection
          :provider="saved.provider"
          :account-id="saved.account_id || 0"
          :model="saved.model"
          :effort="saved.effort || ''"
          size="xs"
          @change="chooseModel"
        />
      </div>
      <label class="block text-sm text-muted">
        Prompt
        <UTextarea v-model="prompt" aria-label="Pinned prompt" :rows="8" class="mt-1 w-full" :ui="{ base: 'resize-y' }" />
      </label>
      <div class="mt-3 flex justify-end gap-2">
        <UButton label="Discard" color="neutral" variant="outline" :disabled="!dirty || busy" @click="prompt = saved.prompt" />
        <UButton label="Save changes" :disabled="!dirty || !prompt.trim() || busy" @click="save" />
      </div>
      <UModal v-model:open="customizing" title="Customize and run" :ui="{ content: 'max-w-2xl' }">
        <template #body>
          <UTextarea v-model="customPrompt" aria-label="One-off prompt" :rows="8" class="w-full" />
        </template>
        <template #footer>
          <div class="flex w-full justify-end gap-2">
            <UButton label="Cancel" color="neutral" variant="ghost" @click="customizing = false" />
            <UButton label="Send" icon="i-lucide-send" :disabled="!customPrompt.trim() || busy" @click="play(true)" />
          </div>
        </template>
      </UModal>
    </div>
  </div>
</template>
