<script setup>
import { computed, onMounted, ref, watchEffect } from "vue";
import { api } from "../../api";
import { fileSize } from "../../file-info";
import { fuzzyAny } from "../../fuzzy";
import { S, fail, setActionModel } from "../../store";
import ModelSelection from "../ModelSelection.vue";

const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count"]);
const config = ref(null), saving = ref(false);
const visible = computed(() => fuzzyAny(["general", "speech", "microphone", "mic", "voice", "dictation", "transcription", "audio", "whistle", "whisper", "haiku", "luna", "correction"], props.filter) !== null);
watchEffect(() => emit("count", visible.value ? 1 : 0));
const items = computed(() => (config.value?.models || []).map((model) => ({ label: `${model.label} · ${fileSize(model.bytes)}`, value: model.id })));
const correction = computed(() => S.actionModels.find((row) => row.id === "transcription"));
onMounted(async () => {
  try { config.value = await api("GET", "/api/speech"); } catch (error) { fail(error); }
});
async function save(change) {
  saving.value = true;
  try { config.value = await api("PUT", "/api/speech", change); } catch (error) { fail(error); }
  finally { saving.value = false; }
}
async function chooseCorrection(choice) {
  saving.value = true;
  try { await setActionModel("transcription", choice); } catch (error) { fail(error); }
  finally { saving.value = false; }
}
</script>

<template>
  <section v-if="visible" class="mt-5" aria-label="Speech input">
    <div class="mb-0.5 font-semibold text-highlighted">Speech input</div>
    <p class="mb-2 text-xs text-dimmed">Tap a microphone to record, then tap again to transcribe the whole recording into your draft. Recordings can be up to 2 minutes.</p>
    <label class="mb-1 block text-sm" for="speech-model">Speech recognition model</label>
    <USelect id="speech-model" :model-value="config?.model || 'whistle'" :items="items" class="w-full"
      aria-label="Speech recognition model" :disabled="!config || saving" @update:model-value="save({ model: $event })" />
    <p class="mt-1 text-xs text-dimmed">The selected model downloads automatically the first time you use it. Speech recognition runs on the Agenttik host. Whisper adds languages such as Portuguese and Romanian.</p>
    <USwitch class="mt-3" label="Correct transcription" :model-value="config?.correct || false"
      :disabled="!config || saving" @update:model-value="save({ correct: $event })" />
    <p class="mt-1 text-xs text-dimmed">Optionally send the transcript to a lightweight model to correct recognition errors and punctuation. Uses your selected account; keeps the original language.</p>
    <div v-if="config?.correct && correction" class="mt-2 flex flex-wrap items-center gap-2">
      <span class="text-sm">Correction model</span>
      <ModelSelection :provider="correction.choice.provider" :account-id="correction.choice.account_id"
        :model="correction.choice.model" :effort="correction.choice.effort" :disabled="saving"
        aria-label="Transcription correction model" @change="chooseCorrection" />
      <UButton v-if="correction.custom" size="xs" color="neutral" variant="ghost" label="Use default"
        :disabled="saving" @click="chooseCorrection(null)" />
      <span v-else class="text-xs text-dimmed">Luna or Haiku, depending on your available accounts.</span>
    </div>
  </section>
</template>
