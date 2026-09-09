<script setup>
import { nextTick, ref, watch } from "vue";

import { S } from "../store";

const WHO = { user: "You", assistant: "Agent", tool: "Tool", thinking: "Thinking", error: "Error" };

const box = ref(null);

/* Follow the stream: every delta grows the last message, so watching the
   messages deeply is what tells us to scroll. */
watch(
  () => S.detail?.messages,
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
      <div
        v-for="(m, i) in S.detail.messages"
        :key="i"
        class="mb-4"
        :class="m.role === 'user' ? 'flex flex-col items-end' : ''"
      >
        <div class="mb-0.5 text-[11px] font-medium tracking-wider text-dimmed uppercase">
          {{ WHO[m.role] || m.role }}
        </div>
        <div
          class="wrap-anywhere whitespace-pre-wrap"
          :class="[
            {
              user: 'max-w-[85%] rounded-[var(--ui-radius)] bg-elevated px-3 py-2 text-highlighted',
              assistant: 'text-highlighted',
              thinking: 'text-dimmed italic',
              tool: 'max-h-[9em] overflow-auto rounded-[var(--ui-radius)] bg-muted px-2.5 py-1.5 font-mono text-xs text-muted inset-ring inset-ring-default',
              error: 'rounded-[var(--ui-radius)] bg-error/8 px-2.5 py-2 text-error inset-ring inset-ring-error/25',
            }[m.role],
            m.streaming ? 'streaming' : '',
          ]"
        >{{ m.content }}</div>
      </div>
    </div>
  </div>
</template>
