<script setup>
/* One bubble in the transcript.

   It is a component of its own for a reason beyond tidiness: a streaming turn
   grows the last message a few characters at a time, and if the bubbles were
   inlined in the transcript's v-for every one of them would be re-rendered —
   and re-parsed as markdown — on every delta. Here only the message whose
   content changed re-renders, and its computed markdown is what re-runs. */
import { computed } from "vue";

import { markdown } from "../markdown";

const props = defineProps({ message: { type: Object, required: true } });

const WHO = { user: "You", assistant: "Agent", tool: "Tool", thinking: "Thinking", error: "Error" };

/* What the agent writes is markdown, so it is rendered. What you typed is
   shown as you typed it, and a tool line is a literal command. */
const RENDERED = { assistant: true, thinking: true };

const BUBBLE = {
  user: "max-w-[85%] whitespace-pre-wrap rounded-[var(--ui-radius)] bg-elevated px-3 py-2 text-highlighted",
  assistant: "markdown text-highlighted",
  thinking: "markdown text-dimmed italic",
  tool: "max-h-[9em] overflow-auto whitespace-pre-wrap rounded-[var(--ui-radius)] bg-muted px-2.5 py-1.5 font-mono text-xs text-muted inset-ring inset-ring-default",
  error:
    "whitespace-pre-wrap rounded-[var(--ui-radius)] bg-error/8 px-2.5 py-2 text-error inset-ring inset-ring-error/25",
};

const rendered = computed(() => !!RENDERED[props.message.role]);
/* markdown() emits a fixed set of tags over escaped text, which is what makes
   v-html safe here: nothing an agent writes can reach the DOM as markup. */
const html = computed(() => (rendered.value ? markdown(props.message.content) : ""));
const classes = computed(() => [
  "wrap-anywhere",
  BUBBLE[props.message.role],
  props.message.streaming ? "streaming" : "",
]);
</script>

<template>
  <div class="mb-4" :class="message.role === 'user' ? 'flex flex-col items-end' : ''">
    <div class="mb-0.5 text-[11px] font-medium tracking-wider text-dimmed uppercase">
      {{ WHO[message.role] || message.role }}
    </div>
    <div v-if="rendered" :class="classes" v-html="html" />
    <div v-else :class="classes">{{ message.content }}</div>
  </div>
</template>
