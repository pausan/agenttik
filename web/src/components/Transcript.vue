<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";

import { S } from "../store";
import Message from "./Message.vue";

const box = ref(null);
const now = ref(Date.now());
const fallbackStartedAt = ref(0);
let clock = null;

const CLOCK_FACES = ["🕛", "🕐", "🕑", "🕒", "🕓", "🕔", "🕕", "🕖", "🕗", "🕘", "🕙", "🕚"];
const runningTurn = computed(() => S.detail?.turns?.findLast((turn) => turn.status === "running"));
const startedAt = computed(() => Number(runningTurn.value?.started_at) || fallbackStartedAt.value);
const elapsed = computed(() =>
  S.detail?.running && startedAt.value ? Math.max(0, Math.floor((now.value - startedAt.value) / 1000)) : 0,
);
const elapsedLabel = computed(() => `${elapsed.value} ${elapsed.value === 1 ? "second" : "seconds"}`);
const clockFace = computed(() => CLOCK_FACES[Math.floor(now.value / 250) % CLOCK_FACES.length]);

/* Queued prompts are drawn under the transcript, each with how long it has
   been waiting, so opening a session shows the text that is going to run. */
const queued = computed(() => S.detail?.queued || []);
function waitLabel(since) {
  const secs = Math.max(0, Math.floor((now.value - Number(since || 0)) / 1000));
  return `${Math.floor(secs / 60)}:${String(secs % 60).padStart(2, "0")}`;
}

/* The POST response normally supplies the running turn right away. The local
   timestamp still covers the small gap before it does, and a running session
   restored after a reload instead uses its persisted turn start time. */
watch(
  () => [S.detail?.session.id, S.detail?.running],
  ([, running]) => {
    if (running) fallbackStartedAt.value = Date.now();
  },
  { immediate: true },
);

onMounted(() => {
  clock = window.setInterval(() => (now.value = Date.now()), 1000);
});
onUnmounted(() => window.clearInterval(clock));

/* Follow the stream: every delta grows the last message, so watching the
   messages deeply is what tells us to scroll. */
watch(
  () => [S.detail?.messages, queued.value.length],
  async () => {
    await nextTick();
    if (box.value) box.value.scrollTop = box.value.scrollHeight;
  },
  { deep: true, flush: "post" },
);
</script>

<template>
  <div ref="box" class="min-h-0 flex-1 overflow-auto">
    <p v-if="!S.detail" class="pt-[18vh] text-center text-dimmed">
      Pick a session, or a project to start one.
    </p>
    <div v-else class="mx-auto max-w-[860px] px-6 pt-5 pb-2">
      <Message v-for="(m, i) in S.detail.messages" :key="i" :message="m" />
      <div v-if="S.detail.running" class="mb-3 flex items-center gap-2 text-xs text-dimmed" aria-label="Agent working">
        <span aria-hidden="true">{{ clockFace }}</span>
        <span>Working ({{ elapsedLabel }})</span>
      </div>

      <!-- A prompt that is only waiting is still shown where it will run, so
           clicking into a session tells you what is coming. The dashed bubble
           and the timer are what say it has not started. -->
      <div v-for="q in queued" :key="q.id" class="mb-4 flex flex-col items-end" aria-label="Queued prompt">
        <div class="mb-0.5 flex flex-row-reverse items-center gap-1.5">
          <span class="text-[11px] font-medium tracking-wider text-dimmed uppercase">Queued</span>
          <span class="flex items-center gap-1 font-mono text-[11px] text-dimmed tabular-nums">
            <span aria-hidden="true">{{ clockFace }}</span>{{ waitLabel(q.created_at) }}
          </span>
        </div>
        <div
          class="max-w-[85%] rounded-[var(--ui-radius)] border border-dashed border-default bg-elevated/50 px-3 py-2 whitespace-pre-wrap text-muted wrap-anywhere"
        >
          {{ q.prompt }}
        </div>
      </div>
    </div>
  </div>
</template>
