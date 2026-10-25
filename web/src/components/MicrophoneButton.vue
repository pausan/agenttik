<script setup>
import { computed } from "vue";
import { dictation, dictationLabel, toggleDictation } from "../dictation";
import { LEVELS } from "../speech-recorder";

const props = defineProps({ large: Boolean });
const busy = computed(() => !["idle", "recording"].includes(dictation.phase));
const tooltip = computed(() => `${dictationLabel.value}${busy.value ? ' · Click to cancel' : ''}. Change speech options in Settings → General.`);
// Newest level on the right; pad with silence until enough have arrived.
const bars = computed(() => {
  const count = props.large ? LEVELS : 5;
  const levels = dictation.levels.slice(-count);
  return [...Array(count - levels.length).fill(0), ...levels];
});
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
  <!-- The large meter keeps its space so the button does not move under the finger. -->
  <span v-if="large || dictation.phase === 'recording'" class="voice-meter flex shrink-0 items-center"
    :class="[large ? 'h-10 gap-1' : 'h-5 gap-0.5', dictation.phase !== 'recording' && 'invisible']" aria-hidden="true">
    <span v-for="(level, i) in bars" :key="i" class="h-full rounded-full bg-info"
      :class="large ? 'w-1.5' : 'w-[3px]'" :style="{ transform: `scaleY(${Math.max(0.12, level)})` }" />
  </span>
</template>

<style scoped>
.voice-meter > span { transition: transform 100ms linear; }
</style>
