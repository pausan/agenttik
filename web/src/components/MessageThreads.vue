<script setup>
/* The threads opened from one message: a chip each until opened, and the
   box where a new one's first question is written. See
   specs/092-threads.md. */
import { computed, nextTick, onMounted, ref, watch } from "vue";

import { createThread, openThread, S } from "../store";
import ThreadPanel from "./ThreadPanel.vue";

const props = defineProps({
  messageId: { type: Number, required: true },
  threads: { type: Array, default: () => [] },
});

const drafting = computed(() =>
  S.threadDraft?.messageID === props.messageId && S.threadDraft.parentID === S.detail?.session.id ? S.threadDraft : null,
);
const question = ref("");
const asking = ref(false);
const box = ref(null);

function focusBox() {
  nextTick(() => box.value?.textarea?.focus());
}
onMounted(() => drafting.value && focusBox());
watch(drafting, (draft) => {
  if (!draft) return;
  question.value = "";
  focusBox();
});

async function ask() {
  if (asking.value || !question.value.trim()) return;
  asking.value = true;
  if (await createThread(question.value)) question.value = "";
  asking.value = false;
}

function onKey(e) {
  if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
    e.preventDefault();
    ask();
  } else if (e.key === "Escape") {
    S.threadDraft = null;
  }
}

const label = (t) => `${t.replies} ${t.replies === 1 ? "reply" : "replies"}`;
</script>

<template>
  <div v-if="threads.length || drafting" class="-mt-2" aria-label="Threads" data-threads>
    <template v-for="t in threads" :key="t.id">
      <ThreadPanel v-if="S.threads[t.id]" :thread="S.threads[t.id]" />
      <button
        v-else
        type="button"
        class="mb-2 ml-4 flex max-w-[calc(100%-1rem)] items-center gap-2 rounded-[var(--ui-radius)] border border-default px-2 py-1 text-left text-xs text-muted hover:bg-elevated"
        :title="t.anchor_quote"
        :aria-label="`Open thread: ${t.title}`"
        @click="openThread(t.id)"
      >
        <UIcon
          :name="t.status === 'running' ? 'i-lucide-loader-circle' : 'i-lucide-messages-square'"
          class="shrink-0"
          :class="t.status === 'running' ? 'animate-spin' : ''"
        />
        <span class="truncate font-medium text-default">{{ t.title }}</span>
        <span class="shrink-0 text-dimmed">· {{ label(t) }}</span>
      </button>
    </template>

    <form
      v-if="drafting"
      class="mb-4 ml-4 rounded-[var(--ui-radius)] border border-default border-l-2 border-l-primary/60 bg-elevated/30 px-3 py-2"
      aria-label="New thread"
      @submit.prevent="ask"
    >
      <blockquote class="mb-2 line-clamp-3 border-l-2 border-default pl-2 text-xs whitespace-pre-wrap text-muted italic">
        {{ drafting.quote }}
      </blockquote>
      <UTextarea
        ref="box"
        v-model="question"
        :rows="1"
        autoresize
        class="w-full"
        placeholder="Ask about this passage… (Enter to send)"
        aria-label="Thread question"
        @keydown="onKey"
      />
      <p class="mt-1.5 mb-2 text-[11px] text-dimmed">
        Answered in a thread that knows this conversation. The conversation itself does not see it.
      </p>
      <div class="flex items-center gap-2">
        <UButton type="submit" size="xs" icon="i-lucide-send" label="Ask" :loading="asking" :disabled="!question.trim()" />
        <UButton type="button" size="xs" color="neutral" variant="ghost" label="Cancel" @click="S.threadDraft = null" />
      </div>
    </form>
  </div>
</template>
