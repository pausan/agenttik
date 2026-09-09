<script setup>
import { nextTick, ref, watch } from "vue";

import { S } from "../store";
import Message from "./Message.vue";

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
      <Message v-for="(m, i) in S.detail.messages" :key="i" :message="m" />
    </div>
  </div>
</template>
