<script setup>
/* The countdown to agenttik answering a request for the user, which Settings ›
   General turns on, and the button that stops it. Pausing says "I will answer
   this, wait for me": it stops the countdown in every window, and the card
   says so from then on. See specs/082-tool-approvals.md. */
import { computed, onUnmounted, ref, watch } from "vue";

import { holdApproval } from "../store";

const props = defineProps({
  sessionId: { type: String, required: true },
  approval: { type: Object, required: true },
  // What happens when it runs out: "Allowing" or "Answering for you".
  verb: { type: String, required: true },
});

// The clock only ticks while there is a countdown.
const now = ref(Date.now());
let ticker = null;
watch(
  () => props.approval.deadline,
  (deadline) => {
    clearInterval(ticker);
    ticker = deadline ? setInterval(() => (now.value = Date.now()), 250) : null;
    now.value = Date.now();
  },
  { immediate: true },
);
onUnmounted(() => clearInterval(ticker));
const secondsLeft = computed(() => Math.max(0, Math.ceil((props.approval.deadline - now.value) / 1000)));

const pausing = ref(false);
async function pause() {
  pausing.value = true;
  try {
    await holdApproval(props.sessionId, props.approval.id);
  } finally {
    pausing.value = false;
  }
}
</script>

<template>
  <div
    v-if="approval.deadline"
    class="ml-auto flex items-center gap-2 text-xs text-muted"
    role="timer"
    aria-label="Automatic answer"
  >
    <UIcon name="i-lucide-timer" class="size-3.5 shrink-0" aria-hidden="true" />
    <span class="tabular-nums">{{ verb }} in {{ secondsLeft }}s</span>
    <UButton
      size="xs"
      color="neutral"
      variant="outline"
      icon="i-lucide-timer-off"
      label="Pause timer"
      title="Stop the countdown and wait for your answer, however long it takes"
      :loading="pausing"
      @click="pause"
    />
  </div>
  <div v-else-if="approval.held" class="ml-auto flex items-center gap-1.5 text-xs text-muted" aria-label="Timer paused">
    <UIcon name="i-lucide-timer-off" class="size-3.5 shrink-0" aria-hidden="true" />
    <span>Timer paused · waiting for your answer</span>
  </div>
</template>
