<script setup>
/* A run of consecutive tool calls, drawn as one block.

   A turn can spend twenty calls reaching one answer, and scrolling past all of
   them to find the reply is the common case. So a run shows only its latest
   call — what the agent is doing now — behind a button that opens the rest and
   closes them again. A run of one has nothing to hide and is left alone.

   Grouping lives here rather than in the transcript so that a streaming turn
   still re-renders only the bubble that changed. */
import { computed, ref } from "vue";

import Message from "./Message.vue";

const props = defineProps({ tools: { type: Array, required: true } });

const open = ref(false);
/* Rows carry their position in the run so the key survives expanding: the
   collapsed bubble is the same message it was before the rest appeared. */
const rows = computed(() =>
  open.value
    ? props.tools.map((message, at) => ({ at, message }))
    : [{ at: props.tools.length - 1, message: props.tools.at(-1) }],
);
</script>

<template>
  <div v-if="tools.length > 1">
    <UButton
      class="mb-1 -ml-1.5 text-dimmed"
      size="xs"
      color="neutral"
      variant="ghost"
      :icon="open ? 'i-lucide-chevron-down' : 'i-lucide-chevron-right'"
      :label="`${tools.length} tool calls`"
      :aria-expanded="open"
      :title="open ? 'Show only the latest' : 'Show all of them'"
      @click="open = !open"
    />
    <Message v-for="row in rows" :key="row.at" :message="row.message" />
  </div>
  <Message v-else :message="tools[0]" />
</template>
