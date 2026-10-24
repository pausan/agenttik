<script setup>
import { computed } from "vue";
import { dictation, dictationLabel, toggleDictation } from "../dictation";

defineProps({ large: Boolean });
const busy = computed(() => !["idle", "recording"].includes(dictation.phase));
const tooltip = computed(() => `${dictationLabel.value}${busy.value ? ' · Click to cancel' : ''}. Change speech options in Settings → General.`);
</script>

<template>
  <UTooltip :text="tooltip">
    <UButton type="button" :aria-label="dictationLabel" :aria-pressed="dictation.phase === 'recording'"
      :aria-busy="busy" :title="tooltip" :color="dictation.phase === 'recording' ? 'error' : 'neutral'"
      :variant="large ? 'soft' : 'ghost'" :size="large ? 'xl' : 'sm'"
      :icon="busy ? 'i-lucide-loader-circle' : dictation.phase === 'recording' ? 'i-lucide-square' : 'i-lucide-mic'"
      :ui="{ leadingIcon: [busy && 'animate-spin', large && 'size-10'] }"
      :class="large && 'size-28 justify-center rounded-full max-md:size-32'"
      @click="toggleDictation" />
  </UTooltip>
</template>
