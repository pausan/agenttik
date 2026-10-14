<script setup>
/* One open thread, drawn under the message it quotes: the passage, the
   exchange, and a box to ask the next question. Its turns run in a session
   of their own that started from a copy of the conversation, so nothing
   here reaches the conversation above. It can be opened as a task, which
   starts from a copy of the thread and leaves this one where it is. See
   specs/092-threads.md. */
import { computed, nextTick, onMounted, ref } from "vue";

import { closeThread, deleteThread, S, sendThread, spawnThread, stopTask } from "../store";
import ApprovalCard from "./ApprovalCard.vue";
import Message from "./Message.vue";
import ToolGroup from "./ToolGroup.vue";

const props = defineProps({ thread: { type: Object, required: true } });

const detail = computed(() => props.thread.detail);
const running = computed(() => !!detail.value?.running);
const approvals = computed(() => S.approvals[props.thread.sessionID] || []);
const quote = computed(() => detail.value?.session.anchor_quote || "");

/* Runs of tool calls collapse to their latest, as in the transcript. */
const rows = computed(() => {
  const messages = detail.value?.messages || [];
  const out = [];
  for (let i = 0; i < messages.length; i++) {
    if (messages[i].role !== "tool") {
      out.push({ at: i, message: messages[i] });
      continue;
    }
    const at = i;
    const tools = [];
    while (i < messages.length && messages[i].role === "tool") tools.push(messages[i++]);
    i--;
    out.push({ at, tools });
  }
  return out;
});

const box = ref(null);
onMounted(() => nextTick(() => box.value?.textarea?.focus({ preventScroll: true })));

function send() {
  sendThread(props.thread, props.thread.draft || "");
}

function onKey(e) {
  if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
    e.preventDefault();
    send();
  } else if (e.key === "Escape") {
    closeThread(props.thread.sessionID);
  }
}

// Deleting asks twice, in place.
const confirming = ref(false);
function remove() {
  if (!confirming.value) {
    confirming.value = true;
    return;
  }
  deleteThread(props.thread);
}
</script>

<template>
  <section
    class="mb-4 ml-4 rounded-[var(--ui-radius)] border border-default border-l-2 border-l-primary/60 bg-elevated/30 px-3 pt-2 pb-3"
    :aria-label="`Thread: ${detail?.session.title || 'loading'}`"
  >
    <header class="mb-2 flex items-start gap-2">
      <UIcon name="i-lucide-messages-square" class="mt-0.5 shrink-0 text-dimmed" />
      <blockquote class="line-clamp-3 min-w-0 flex-1 border-l-2 border-default pl-2 text-xs whitespace-pre-wrap text-muted italic" :title="quote">
        {{ quote }}
      </blockquote>
      <div class="flex shrink-0 items-center gap-0.5">
        <UButton
          size="xs"
          color="neutral"
          variant="ghost"
          icon="i-lucide-square-arrow-out-up-right"
          label="Open as task"
          title="Continue this thread as a task of its own; the thread stays here"
          :disabled="running || !detail"
          @click="spawnThread(thread)"
        />
        <UButton
          size="xs"
          :color="confirming ? 'error' : 'neutral'"
          :variant="confirming ? 'soft' : 'ghost'"
          icon="i-lucide-trash-2"
          :label="confirming ? 'Delete thread?' : undefined"
          :aria-label="confirming ? 'Confirm deleting this thread' : 'Delete thread'"
          title="Delete this thread"
          @click="remove"
          @mouseleave="confirming = false"
        />
        <UButton
          size="xs"
          color="neutral"
          variant="ghost"
          icon="i-lucide-chevron-up"
          aria-label="Close thread"
          title="Close thread"
          @click="closeThread(thread.sessionID)"
        />
      </div>
    </header>

    <p v-if="!detail" class="text-xs text-dimmed">Loading…</p>
    <template v-else>
      <template v-for="row in rows" :key="row.at">
        <ToolGroup v-if="row.tools" :tools="row.tools" />
        <Message v-else :message="row.message" in-thread />
      </template>
      <ApprovalCard v-for="a in approvals" :key="a.id" :session-id="thread.sessionID" :approval="a" />
      <p v-if="running" class="mb-2 text-xs text-dimmed" aria-label="Thread working">Working…</p>

      <form class="flex items-end gap-2" aria-label="Reply in thread" @submit.prevent="send">
        <UTextarea
          ref="box"
          v-model="thread.draft"
          :rows="1"
          autoresize
          class="min-w-0 flex-1"
          placeholder="Ask a follow-up… (Enter to send, Shift+Enter for a new line)"
          aria-label="Thread reply"
          @keydown="onKey"
        />
        <UButton
          v-if="running"
          type="button"
          size="sm"
          color="neutral"
          variant="outline"
          icon="i-lucide-square"
          label="Stop"
          @click="stopTask(thread.sessionID)"
        />
        <UButton v-else type="submit" size="sm" icon="i-lucide-send" label="Send" :disabled="!thread.draft?.trim()" />
      </form>
    </template>
  </section>
</template>
