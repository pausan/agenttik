<script setup>
/* One bubble in the transcript.

   It is a component of its own for a reason beyond tidiness: a streaming turn
   grows the last message a few characters at a time, and if the bubbles were
   inlined in the transcript's v-for every one of them would be re-rendered —
   and re-parsed as markdown — on every delta. Here only the message whose
   content changed re-renders, and its computed markdown is what re-runs.

   Every message can be copied. Only your own prompts can be edited, and only
   once they have been stored: a message still arriving over the stream has no
   id to rewrite. */
import { computed, nextTick, onUnmounted, ref, watch } from "vue";

import { S, copyText, editMessage, hit, queueUnprocessed, sendUnprocessed, setModel } from "../store";
import { promptState } from "../background-tasks";
import { messageDate, messageTime } from "../message-time";
import { promptImages, promptText } from "../prompt-images";
import ConversationImage from "./ConversationImage.vue";
import MarkdownContent from "./MarkdownContent.vue";
import ModelSelection from "./ModelSelection.vue";

const props = defineProps({
  message: { type: Object, required: true },
  // A thread's prompts are not the conversation's: editing one would rewrite
  // the conversation in front, so they are only copied.
  inThread: { type: Boolean, default: false },
});

const WHO = { user: "You", assistant: "Agent", tool: "Tool", thinking: "Thinking", error: "Error", question: "Question", answer: "Your answer", automatic_answer: "Automatic answer" };

/* What the agent writes is markdown, so it is rendered. What you typed is
   shown as you typed it, and a tool line is a literal command. */
const RENDERED = { assistant: true, thinking: true };

const BUBBLE = {
  user: "max-w-[85%] whitespace-pre-wrap rounded-[var(--ui-radius)] bg-elevated px-3 py-2 text-highlighted",
  assistant: "markdown text-highlighted",
  answer: "whitespace-pre-wrap rounded-[var(--ui-radius)] bg-elevated px-3 py-2 text-highlighted",
  question: "whitespace-pre-wrap rounded-[var(--ui-radius)] bg-elevated px-3 py-2 text-highlighted",
  automatic_answer: "whitespace-pre-wrap rounded-[var(--ui-radius)] bg-elevated px-3 py-2 text-highlighted",
  thinking: "markdown text-dimmed italic",
  tool: "max-h-[9em] overflow-auto whitespace-pre-wrap rounded-[var(--ui-radius)] bg-muted px-2.5 py-1.5 font-mono text-xs text-muted inset-ring inset-ring-default",
  error:
    "whitespace-pre-wrap rounded-[var(--ui-radius)] bg-error/8 px-2.5 py-2 text-error inset-ring inset-ring-error/25",
};

const rendered = computed(() => !!RENDERED[props.message.role]);

/* A streaming reply grows by a word every few milliseconds, and every delta
   would parse the whole reply as markdown and rebuild its DOM. While it
   streams, the rendered copy catches up at most every STREAM_RENDER_MS; any
   other change, the end of the stream included, shows at once. */
const STREAM_RENDER_MS = 100;
const markdownText = ref(props.message.content);
let renderTimer = 0;
watch(
  () => [props.message.content, props.message.streaming],
  ([content, streaming]) => {
    if (streaming) {
      renderTimer ||= window.setTimeout(() => {
        renderTimer = 0;
        markdownText.value = props.message.content;
      }, STREAM_RENDER_MS);
      return;
    }
    window.clearTimeout(renderTimer);
    renderTimer = 0;
    markdownText.value = content;
  },
);
onUnmounted(() => window.clearTimeout(renderTimer));
const images = computed(() => props.message.role === "user" ? promptImages(props.message.content) : []);
const text = computed(() => images.value.length ? promptText(props.message.content) : props.message.content);
const preview = ref(null);
const classes = computed(() => [
  "wrap-anywhere",
  BUBBLE[props.message.role],
  props.message.streaming ? "streaming" : "",
]);

/* Whether the agent has seen this prompt, said only when it has not
   (specs/083-background-tasks.md). */
const state = computed(() =>
  props.inThread ? "" : promptState(props.message, S.detail?.pending_follow_ups, S.detail?.session?.created_at),
);
const STATE = {
  waiting: {
    label: "Waiting for the agent",
    title: "The agent has this message and starts on it once it finishes what it is doing.",
  },
  unprocessed: {
    label: "Not processed",
    title: "The agent never processed this message: the task stopped before it got to it. Send or enqueue it to have it processed.",
  },
};

/* An unprocessed prompt goes again as it is, the way the prompt bar sends
   one: Send is out of reach while the agent is busy with something else. */
const resending = ref(""); // "send" or "queue" while that request is out
const busy = computed(() => !!S.detail?.running && !S.detail?.accepts_prompts);
async function resend(how) {
  if (resending.value) return;
  resending.value = how;
  try {
    await (how === "send" ? sendUnprocessed : queueUnprocessed)(props.message);
  } finally {
    resending.value = "";
  }
}

const editable = computed(
  () => !props.inThread && props.message.role === "user" && !!props.message.id && !props.message.streaming,
);

const copied = ref(false);
async function copy() {
  if (!(await copyText(props.message.content))) return;
  copied.value = true;
  window.setTimeout(() => (copied.value = false), 1200);
}

const draft = ref(null); // non-null while editing
const box = ref(null);
const sending = ref(false);
const choosing = ref(false);

async function chooseModel(choice) {
  choosing.value = true;
  try {
    await setModel(choice.provider, choice.model, choice.effort, choice.accountID);
  } finally {
    choosing.value = false;
  }
}

async function startEdit() {
  draft.value = props.message.content;
  await nextTick();
  box.value?.textareaRef?.focus();
}

async function submit(enqueue = false) {
  if (sending.value || choosing.value) return;
  sending.value = true;
  const sent = await editMessage(props.message, draft.value, enqueue);
  sending.value = false;
  // On success this bubble is about to be replaced by the truncation, so the
  // editor is only closed when it is still here to close.
  if (sent) draft.value = null;
}

/* Send and Enqueue answer the same chords as in the prompt bar, so a
   rebinding in Settings reaches the editor too. Anything else, plain Enter
   included when it is bound to neither, types into the box. */
function onKey(e) {
  if (e.isComposing) return;
  if (e.key === "Escape") {
    e.preventDefault();
    draft.value = null;
  } else if (hit(e, "prompt.send")) {
    e.preventDefault();
    if (draft.value.trim()) submit();
  } else if (hit(e, "prompt.enqueue")) {
    e.preventDefault();
    if (draft.value.trim()) submit(true);
  }
}
</script>

<template>
  <div class="group mb-4" :class="message.role === 'user' ? 'flex flex-col items-end' : ''" :data-message-id="message.id">
    <div
      class="mb-0.5 flex items-center gap-1.5"
      :class="message.role === 'user' ? 'flex-row-reverse' : ''"
    >
      <span class="text-[11px] text-dimmed">
        <span class="font-medium tracking-wider uppercase">{{ WHO[message.role] || message.role }}</span>
        <template v-if="message.created_at">
          ·
          <time
            class="tabular-nums"
            :datetime="new Date(message.created_at).toISOString()"
            :title="messageDate(message.created_at)"
          >{{ messageTime(message.created_at) }}</time>
        </template>
      </span>
      <span v-if="state" class="text-[11px] text-dimmed" :class="{ 'italic': state === 'waiting' }" :title="STATE[state].title">
        {{ STATE[state].label }}
      </span>
      <span v-if="state === 'unprocessed'" class="flex items-center gap-0.5">
        <UButton
          icon="i-lucide-send"
          size="xs"
          color="neutral"
          variant="ghost"
          label="Send"
          :loading="resending === 'send'"
          :disabled="!!resending || draft !== null || busy"
          :title="busy
            ? 'The agent is busy and takes no prompts until it finishes: enqueue this one, or send it then'
            : 'Send it now, as if you had just typed it. It moves to the end of the conversation.'"
          @click="resend('send')"
        />
        <UButton
          icon="i-lucide-clock"
          size="xs"
          color="neutral"
          variant="ghost"
          label="Enqueue"
          :loading="resending === 'queue'"
          :disabled="!!resending || draft !== null"
          title="Put it at the end of the task's queue, with the task's model. It runs when the queue reaches it."
          @click="resend('queue')"
        />
      </span>
      <!-- The actions stay in the layout and only appear on hover, so a
           bubble does not shift when the pointer reaches it. -->
      <span
        class="message-actions flex items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100 max-md:opacity-100"
      >
        <UButton
          :icon="copied ? 'i-lucide-check' : 'i-lucide-copy'"
          size="xs"
          color="neutral"
          variant="ghost"
          :aria-label="`Copy ${WHO[message.role] || message.role} message`"
          title="Copy"
          @click="copy"
        />
        <UButton
          v-if="editable"
          icon="i-lucide-pencil"
          size="xs"
          color="neutral"
          variant="ghost"
          :disabled="draft !== null || S.detail?.running"
          aria-label="Edit this prompt"
          title="Edit and continue from here"
          @click="startEdit"
        />
      </span>
    </div>

    <!-- Editing replaces the bubble in place, so the prompt is rewritten
         where it sits rather than in a dialogue over the transcript. -->
    <form v-if="draft !== null" aria-label="Edit prompt" class="w-full max-w-[85%] self-end" @submit.prevent="submit()">
      <UTextarea
        ref="box"
        v-model="draft"
        :rows="4"
        autoresize
        class="w-full"
        @keydown="onKey"
      />
      <div class="mt-1.5 flex flex-wrap items-center gap-2">
        <ModelSelection
          :provider="S.detail.session.provider"
          :account-id="S.detail.session.account_id || 0"
          :model="S.detail.session.model"
          :effort="S.detail.session.effort || ''"
          show-favourite
          size="xs"
          :disabled="sending || choosing"
          :loading="choosing"
          title="Change edited prompt model"
          @change="chooseModel"
        />
        <UButton type="submit" size="xs" :loading="sending" :disabled="!draft.trim() || choosing" label="Send" />
        <UButton type="button" size="xs" :disabled="!draft.trim() || sending || choosing" label="Enqueue" @click="submit(true)" />
        <UButton
          type="button"
          size="xs"
          color="neutral"
          variant="ghost"
          label="Cancel"
          @click="draft = null"
        />
        <span class="text-[11px] leading-snug text-dimmed">
          Replaces the transcript from here down. The agent keeps its own memory
          of the turns after this one, and files on disk are left as they are.
        </span>
      </div>
    </form>

    <template v-else>
      <MarkdownContent v-if="rendered" :class="classes" :text="markdownText" />
      <div v-else :class="classes">
        <span v-if="text">{{ text }}</span>
        <div v-if="images.length" class="mt-2 flex flex-wrap gap-2" aria-label="Message images">
          <button v-for="(image, i) in images" :key="i" type="button" :aria-label="`View attached image ${i + 1}`" class="max-w-full cursor-zoom-in rounded focus-visible:outline-2 focus-visible:outline-primary" @click="preview = image">
            <img :src="image.url" :alt="`Attached image ${i + 1}`" loading="lazy" class="max-h-80 max-w-full rounded object-contain" />
          </button>
        </div>
      </div>
    </template>
    <ConversationImage v-if="preview" :src="preview.url" source="the attached image"
      :message-id="!inThread && !message.streaming ? message.id || 0 : 0" @close="preview = null" />
  </div>
</template>
