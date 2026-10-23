<script setup>
import { computed, nextTick, onMounted, onUnmounted, provide, ref, watch } from "vue";

import {
  deleteQueued,
  focusPrompt,
  forceQueued,
  openProjectPrompt,
  openTask,
  openThread,
  startThread,
  providerOf,
  S,
  updateQueuedModel,
  updateQueuedPrompt,
} from "../store";
import { turnSummaries } from "../api";
import { messageTime } from "../message-time";
import { tabScroll, vTabScroll } from "../tab-scroll";
import ApprovalCard from "./ApprovalCard.vue";
import ModelSelection from "./ModelSelection.vue";
import StickyPrompt from "./StickyPrompt.vue";
import TranscriptRows from "./TranscriptRows.vue";

defineProps({ findOpen: Boolean });
const emit = defineEmits(["start-tour"]);
const box = ref(null);
const content = ref(null);
const transcriptRows = ref(null);
const now = ref(Date.now());
provide("transcriptClock", now);
const fallbackStartedAt = ref(0);
let clock = null;

const CLOCK_FACES = ["🕛", "🕐", "🕑", "🕒", "🕓", "🕔", "🕕", "🕖", "🕗", "🕘", "🕙", "🕚"];
const approvals = computed(() => (S.detail ? S.approvals[S.detail.session.id] || [] : []));
const runningTurn = computed(() => S.detail?.turns?.findLast((turn) => turn.status === "running"));
const startedAt = computed(() => Number(runningTurn.value?.started_at) || fallbackStartedAt.value);
const elapsed = computed(() =>
  S.detail?.running && startedAt.value ? Math.max(0, Math.floor((now.value - startedAt.value) / 1000)) : 0,
);
const elapsedLabel = computed(() => `${elapsed.value} ${elapsed.value === 1 ? "second" : "seconds"}`);
const clockFace = computed(() => CLOCK_FACES[Math.floor(now.value / 250) % CLOCK_FACES.length]);

/* A run of consecutive tool calls is handed to one ToolGroup, which shows
   only its latest until asked for the rest. Grouping reads role and nothing
   else, so a streaming delta — which only grows a message's content — does
   not rebuild the list. */
const allRows = computed(() => {
  const messages = S.detail?.messages || [];
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
  // A finished turn closes with its time, tokens, cost and end time under its
  // last row, then the task's totals so far. Turns whose messages an edit removed still
  // count, as they do in Stats. Rows streamed before the turn ended carry no
  // turn id; the transcript is re-read when it ends, and that read supplies them.
  // The result files it presented go between its last row and that line.
  const summaries = turnSummaries(S.detail?.turns || [], messageTime);
  const outputs = new Map();
  for (const o of S.detail?.outputs || []) {
    if (!outputs.has(o.turn_id)) outputs.set(o.turn_id, []);
    outputs.get(o.turn_id).push(o);
  }
  const turnOf = (row) => (row.message || row.tools.at(-1)).turn_id;
  for (let i = 0; i < out.length; i++) {
    const id = turnOf(out[i]);
    if (summaries.has(id) && (i === out.length - 1 || turnOf(out[i + 1]) !== id)) {
      out[i].summary = summaries.get(id);
      out[i].outputs = outputs.get(id);
    }
  }
  return out;
});

/* Threads are drawn under the message they quote. */
const threadsByMessage = computed(() => {
  const out = new Map();
  for (const t of S.detail?.threads || []) {
    if (!out.has(t.anchor_message_id)) out.set(t.anchor_message_id, []);
    out.get(t.anchor_message_id).push(t);
  }
  return out;
});

/* Selecting text inside one stored message offers to ask about it in a
   thread. The button sits under the selection; the selection itself is the
   passage. Messages inside a thread are not offered: a thread has no threads. */
const picked = ref(null);
function elementOf(node) {
  return node?.nodeType === 1 ? node : node?.parentElement;
}
function readSelection() {
  const sel = window.getSelection();
  const text = sel?.toString().trim();
  if (!sel || sel.isCollapsed || !sel.rangeCount || !text || !content.value) return (picked.value = null);
  const range = sel.getRangeAt(0);
  const start = elementOf(range.startContainer)?.closest("[data-message-id]");
  if (!start || start !== elementOf(range.endContainer)?.closest("[data-message-id]") ||
      !content.value.contains(start) || start.closest("[data-threads]")) return (picked.value = null);
  const rect = range.getBoundingClientRect();
  const base = content.value.getBoundingClientRect();
  picked.value = {
    messageID: Number(start.dataset.messageId),
    quote: text,
    top: rect.bottom - base.top + 4,
    left: Math.max(0, Math.min(rect.left - base.left, base.width - 150)),
  };
}
function onSelectionChange() {
  if (picked.value && window.getSelection()?.isCollapsed) picked.value = null;
}
onMounted(() => document.addEventListener("selectionchange", onSelectionChange));
onUnmounted(() => document.removeEventListener("selectionchange", onSelectionChange));
watch(() => S.detail?.session.id, () => (picked.value = null));

function askInThread() {
  if (!picked.value) return;
  startThread(picked.value.messageID, picked.value.quote);
  picked.value = null;
  window.getSelection()?.removeAllRanges();
}

// The thread a spawned task came from, open under its message.
async function openOrigin() {
  const origin = S.detail?.origin;
  if (!origin) return;
  await openTask(origin.parent_id);
  openThread(origin.thread_id);
}

/* Keep the prompt for the replies being read once its whole bubble has left
   the viewport. Cache the human nodes when rows change; a scroll needs only
   a binary search, and never parses or copies message text. */
const pinned = ref(null);
let humanNodes = [];
let dismissedPrompt = null;
let pinFrame = 0;
let pinObserver;

function updatePinnedPrompt() {
  pinFrame = 0;
  const el = box.value;
  if (!el) return;
  const top = el.getBoundingClientRect().top + el.clientTop;
  let lo = 0;
  let hi = humanNodes.length;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (humanNodes[mid].getBoundingClientRect().top <= top + 2) lo = mid + 1;
    else hi = mid;
  }
  const node = humanNodes[lo - 1];
  const at = node ? Number(node.dataset.humanMessage) : null;
  const bottom = node?.getBoundingClientRect().bottom;
  // Expanding gives the original room to scroll out naturally, plus half a
  // viewport, before its compact copy can return.
  if (at === dismissedPrompt && bottom <= top - el.clientHeight / 2) dismissedPrompt = null;
  const message = node && bottom <= top && at !== dismissedPrompt ? S.detail?.messages[at] : null;
  if ((pinned.value?.at ?? null) !== (message ? at : null) || (pinned.value?.message ?? null) !== message) {
    pinned.value = message ? { at, message } : null;
  }
}

function schedulePinnedPrompt() {
  if (!pinFrame) pinFrame = requestAnimationFrame(updatePinnedPrompt);
}

watch(allRows, () => {
  humanNodes = [...(box.value?.querySelectorAll('[data-human-message]') || [])];
  if (content.value) pinObserver?.observe(content.value);
  schedulePinnedPrompt();
}, { flush: "post" });

async function revealPinnedPrompt() {
  const prompt = pinned.value;
  if (!prompt) return;
  dismissedPrompt = prompt.at;
  pinned.value = null;
  follow = false;
  const node = await transcriptRows.value?.reveal(prompt.at);
  if (!node || !box.value) return;
  const el = box.value;
  el.scrollTop += node.getBoundingClientRect().top - el.getBoundingClientRect().top - el.clientTop;
  navigationTarget = prompt.at;
  navigationTop = el.scrollTop;
  await nextTick();
  node.querySelector('button')?.focus({ preventScroll: true });
}

/* Queued prompts are drawn under the transcript, each with how long it has
   been waiting, so opening a session shows the text that is going to run. */
const queued = computed(() => [
  ...(S.detail?.queued || []),
  ...(S.detail?.pendingQueued || []),
]);
const forcing = ref(0);
/* The word on the button that was pressed. The turn it cancels ends before
   the new one starts, which would flip a busy button to "Send now". */
const forcingLabel = ref("");
const changing = ref(0);
const deleting = ref(0);
const editing = ref(0); // id of the queued prompt whose text is being edited, 0 for none
const editDraft = ref("");
const editBox = ref(null);
const saving = ref(false);
/* A prompt waiting behind *another* session starts beside it and interrupts
   nothing, so it says "Send now". One waiting behind its own session's turn
   cannot: a provider takes one prompt at a time, that turn is cancelled to
   make room, and the stronger word says so. */
const sendNowLabel = computed(() => (S.detail?.running ? "Force send" : "Send now"));
const sendNowHint = computed(() =>
  S.detail?.running
    ? "Stop this task's turn and send this queued prompt next"
    : "Send this prompt now, beside whatever else the project is running",
);

function waitLabel(since) {
  const secs = Math.max(0, Math.floor((now.value - Number(since || 0)) / 1000));
  return clockLabel(secs);
}

const clockLabel = (secs) => `${Math.floor(secs / 60)}:${String(secs % 60).padStart(2, "0")}`;

/* A prompt held back because the provider it needs was away says so where it
   waits: what failed, and how long until the next attempt. That countdown is
   the whole answer to "is this stuck?" — nothing was lost, the clock is what
   will start it, and Send now is right there for anyone done waiting. A
   prompt only waiting its turn has no retry_at and reads as before.
   See specs/045-provider-outage-retry.md. */
const heldLabel = (q) => (q.retry_at ? `Waiting for ${providerOf(q.provider)?.display_name || q.provider}` : "Queued");

function retryNote(q) {
  if (!q.retry_at) return "";
  const left = Math.max(0, Math.round((Number(q.retry_at) - now.value) / 1000));
  const when = left ? `retrying in ${clockLabel(left)}` : "retrying now";
  const tried = q.retry_count > 1 ? ` · ${q.retry_count} attempts so far` : "";
  const why = String(q.retry_error || "").trim().split("\n")[0];
  return `${why.length > 200 ? why.slice(0, 200) + "…" : why} — ${when}${tried}`;
}

async function setQueuedChoice(q, provider, model, effort, accountID) {
  if (q.pending) return;
  changing.value = q.id;
  try {
    await updateQueuedModel(q, provider, model, effort, accountID);
  } catch {
    /* updateQueuedModel has already shown the failure */
  } finally {
    changing.value = 0;
  }
}

/* A queued prompt has not started, so its text is as editable as its model
   choice — in place, the same shape as editing an already-sent one
   (Message.vue), just without that editor's warnings about rewinding the
   agent or the disk: nothing has run yet, so there is nothing to undo. */
async function startEdit(q) {
  if (q.pending || forcing.value || changing.value || deleting.value || editing.value) return;
  editing.value = q.id;
  editDraft.value = q.prompt;
  await nextTick();
  // editBox sits inside this v-for, so Vue collects it as an array — only one
  // item is ever being edited, so it holds at most the one textarea mounted.
  editBox.value?.[0]?.textareaRef?.focus();
}

/* Leaving the editor hands the caret back to the prompt box. The edit can be
   opened from the keyboard now, and a flow that ends with the focus on
   nothing is one that has to be finished with the mouse. */
function cancelEdit() {
  editing.value = 0;
  editDraft.value = "";
  focusPrompt();
}

function onEditKey(e) {
  if (e.key === "Escape") cancelEdit();
}

async function saveEdit(q) {
  if (saving.value) return;
  const prompt = editDraft.value.trim();
  if (!prompt) return;
  saving.value = true;
  try {
    await updateQueuedPrompt(q, prompt);
    cancelEdit();
  } catch {
    /* updateQueuedPrompt has already shown the failure */
  } finally {
    saving.value = false;
  }
}

/* The last queued prompt is the one the chord opens: the prompt just
   enqueued is where a typo is noticed. startEdit refuses a row the server has
   not answered for yet, so a press during that round trip does nothing rather
   than opening the row above it. See specs/012-task-queue.md. */
watch(
  () => S.queuedEdit,
  () => {
    const last = queued.value.at(-1);
    if (last) startEdit(last);
  },
);

function chooseQueuedModel(q, choice) {
  setQueuedChoice(q, choice.provider, choice.model, choice.effort, choice.accountID);
}

async function force(q) {
  if (q.pending) return;
  forcing.value = q.id;
  forcingLabel.value = sendNowLabel.value;
  try {
    await forceQueued(q.id);
  } catch {
    /* forceQueued has already shown the failure */
  } finally {
    forcing.value = 0;
  }
}

async function removeQueued(q) {
  if (q.pending || forcing.value || changing.value || deleting.value || editing.value) return;
  deleting.value = q.id;
  try {
    await deleteQueued(q);
    focusPrompt();
  } catch {
    /* deleteQueued has already shown the failure */
  } finally {
    deleting.value = 0;
  }
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
  clock = window.setInterval(() => (now.value = Date.now()), 250);
  humanNodes = [...box.value.querySelectorAll('[data-human-message]')];
  pinObserver = new ResizeObserver(schedulePinnedPrompt);
  pinObserver.observe(box.value);
  if (content.value) pinObserver.observe(content.value);
  schedulePinnedPrompt();
});
onUnmounted(() => {
  window.clearInterval(clock);
  pinObserver?.disconnect();
  cancelAnimationFrame(pinFrame);
});

/* Follow new output only while reading the bottom. A tab switch restores
   its saved position instead of being treated as new output. */
let follow = true;
const following = () => follow;
let navigationTarget = null;
let navigationTop = null;
watch(() => S.detail?.session.id, () => {
  navigationTarget = null;
  navigationTop = null;
  dismissedPrompt = null;
  pinned.value = null;
  pinObserver?.disconnect();
  if (box.value) pinObserver?.observe(box.value);
});

async function navigateMessage(direction) {
  const el = box.value;
  const tab = S.tab;
  if (!el || !S.detail?.messages.length) return;
  await nextTick();
  if (S.tab !== tab || box.value !== el) return;
  const humans = [...el.querySelectorAll('[data-human-message]')];
  const top = el.getBoundingClientRect().top + el.clientTop;
  let target;
  if (navigationTarget !== null) {
    target = direction < 0
      ? humans.findLast(node => Number(node.dataset.humanMessage) < navigationTarget) || humans[0]
      : humans.find(node => Number(node.dataset.humanMessage) > navigationTarget);
  } else {
    target = direction < 0
      ? (follow ? humans.at(-1) : humans.findLast(node => node.getBoundingClientRect().top < top - 2)) || humans[0]
      : follow ? undefined : humans.find(node => node.getBoundingClientRect().top > top + 2);
  }
  if (!target && direction < 0) return;
  navigationTarget = target ? Number(target.dataset.humanMessage) : Infinity;
  follow = !target;
  if (target) await transcriptRows.value?.reveal(navigationTarget);
  if (S.tab !== tab || box.value !== el) return;
  el.scrollTop = target ? el.scrollTop + target.getBoundingClientRect().top - top : el.scrollHeight;
  navigationTop = el.scrollTop;
}
defineExpose({ navigateMessage });
function onScroll() {
  const el = box.value;
  schedulePinnedPrompt();
  if (navigationTop !== null && Math.abs(el.scrollTop - navigationTop) < 2) return;
  navigationTarget = null;
  navigationTop = null;
  follow = el.scrollHeight - el.clientHeight - el.scrollTop < 2;
}
watch(
  () => [S.activeTab, S.detail?.messages, S.detail?.messages.length,
    S.detail?.messages.at(-1)?.content, queued.value.length, approvals.value.length],
  async (value, previous) => {
    const tab = S.tab;
    const switched = value[0] !== previous?.[0];
    if (switched && tabScroll(tab)) {
      follow = false;
      await nextTick();
      await nextTick();
      if (S.tab === tab && box.value) onScroll();
      return;
    }
    if (switched) follow = true;
    if (!follow) return;
    await nextTick();
    if (S.tab === tab && box.value) box.value.scrollTop = box.value.scrollHeight;
  },
  { flush: "post", immediate: true },
);
</script>

<template>
  <div ref="box" v-tab-scroll="[S.tab, '', true]" data-transcript class="min-h-0 flex-1 overflow-auto" @scroll="onScroll">
    <div class="sticky top-0 z-10 h-0" data-find-ignore>
      <StickyPrompt v-if="pinned" :message="pinned.message" @reveal="revealPinnedPrompt" />
    </div>
    <div v-if="!S.detail" class="pt-[18vh] text-center text-dimmed">
      <p>Pick a task, or a project to start one.</p>
      <UButton class="mt-4" label="Take the Quick Start Tour" icon="i-lucide-graduation-cap" @click="emit('start-tour')" />
    </div>
    <div
      v-else
      ref="content"
      class="relative mx-auto max-w-[860px] px-6 pt-5 pb-2 max-md:px-3 max-md:pt-3"
      @mouseup="readSelection"
      @keyup.shift="readSelection"
    >
      <UButton
        v-if="picked"
        class="absolute z-20 shadow"
        :style="{ top: `${picked.top}px`, left: `${picked.left}px` }"
        size="sm"
        icon="i-lucide-messages-square"
        label="Ask in thread"
        title="Ask about the selected text in a thread, without adding to this conversation"
        @mousedown.prevent
        @click="askInThread"
      />
      <!-- A task opened from a thread links back to it. -->
      <button
        v-if="S.detail.origin"
        type="button"
        class="mb-4 flex max-w-full items-center gap-1.5 text-left text-[11px] text-dimmed hover:text-primary"
        :title="S.detail.origin.quote"
        @click="openOrigin"
      >
        <UIcon name="i-lucide-messages-square" class="size-3.5 shrink-0" aria-hidden="true" />
        <span class="truncate underline decoration-dotted underline-offset-2">
          Continued from a thread in “{{ S.detail.origin.parent_title || "another task" }}”
        </span>
      </button>
      <!-- The project's own prompt went in ahead of the first thing asked
           here, so the conversation says so at the point it happened: what was
           added, on hover, and the page it is set on, on click. It is drawn
           from the copy the task holds, which is the text that actually went
           in rather than whatever the project says now.
           See specs/047-project-prompt.md. -->
      <UTooltip
        v-if="S.detail.session.project_prompt"
        :delay-duration="300"
        :content="{ side: 'bottom', align: 'start' }"
        :ui="{ content: 'block h-auto max-w-[600px] py-1.5 text-left' }"
      >
        <button
          type="button"
          class="mb-4 flex items-center gap-1.5 text-[11px] text-dimmed hover:text-primary"
          @click="openProjectPrompt(S.detail.session.project_id)"
        >
          <UIcon name="i-lucide-file-text" class="size-3.5 shrink-0" aria-hidden="true" />
          <span class="underline decoration-dotted underline-offset-2">Project prompt added</span>
        </button>
        <!-- Five lines, as the task list's prompt tooltip shows: the clamp
             puts the ellipsis on the last one it kept. -->
        <template #content>
          <div class="space-y-1">
            <div class="font-medium text-highlighted">Sent before the first prompt below</div>
            <div class="line-clamp-5 break-words whitespace-pre-wrap">
              {{ S.detail.session.project_prompt }}
            </div>
            <div class="text-dimmed">Click to open the project's prompt.</div>
          </div>
        </template>
      </UTooltip>

      <TranscriptRows
        :key="S.detail.session.id"
        ref="transcriptRows"
        :rows="allRows"
        :threads="threadsByMessage"
        :root="box"
        :tab="S.tab"
        :expanded="findOpen"
        :following="following"
        @layout="schedulePinnedPrompt"
      />
      <ApprovalCard v-for="a in approvals" :key="a.id" :session-id="S.detail.session.id" :approval="a" />
      <div v-if="S.detail.running" class="mb-3 flex items-center gap-2 text-xs text-dimmed" aria-label="Agent working" data-find-ignore>
        <span aria-hidden="true">{{ clockFace }}</span>
        <span>{{ !approvals.length ? "Working" : approvals.some((a) => a.questions?.length) ? "Waiting for your answer" : "Waiting for approval" }} ({{ elapsedLabel }})</span>
      </div>

      <!-- A prompt that is only waiting is still shown where it will run, so
           clicking into a session tells you what is coming. The dashed bubble
           and the timer are what say it has not started. -->
      <div v-for="q in queued" :key="q.id" class="mb-4 flex flex-col items-end" aria-label="Queued prompt">
        <div class="mb-0.5 flex flex-row-reverse items-center gap-1.5">
          <span
            class="text-[11px] font-medium tracking-wider uppercase"
            :class="q.retry_at ? 'text-warning' : 'text-dimmed'"
            >{{ q.pending ? "Sending…" : heldLabel(q) }}</span
          >
          <span class="flex items-center gap-1 font-mono text-[11px] text-dimmed tabular-nums" data-find-ignore>
            <span aria-hidden="true">{{ clockFace }}</span>{{ waitLabel(q.created_at) }}
          </span>
        </div>
        <!-- Editing replaces the bubble in place, same as an already-sent
             prompt (Message.vue): the text is rewritten where it sits rather
             than in a dialogue over it. -->
        <form v-if="editing === q.id" class="w-full max-w-[85%]" @submit.prevent="saveEdit(q)">
          <UTextarea
            ref="editBox"
            v-model="editDraft"
            :rows="3"
            autoresize
            class="w-full"
            @keydown="onEditKey"
            @keydown.enter.exact.prevent="saveEdit(q)"
          />
          <div class="mt-1.5 flex items-center gap-2">
            <UButton type="submit" size="xs" :loading="saving" :disabled="!editDraft.trim()" label="Save" />
            <UButton type="button" size="xs" color="neutral" variant="ghost" label="Cancel" @click="cancelEdit" />
          </div>
        </form>

        <template v-else>
          <div
            class="max-w-[85%] rounded-[var(--ui-radius)] border border-dashed bg-elevated/50 px-3 py-2 whitespace-pre-wrap text-muted wrap-anywhere"
            :class="q.retry_at ? 'border-warning/50' : 'border-default'"
          >
            {{ q.prompt }}
          </div>
          <p
            v-if="retryNote(q)"
            class="mt-1 max-w-[85%] text-right text-[11px] text-dimmed wrap-anywhere"
            aria-label="Why this prompt is waiting"
          >
            {{ retryNote(q) }}
          </p>
          <div class="queued-actions mt-1 flex items-center gap-1 max-md:flex-wrap max-md:justify-end">
            <UButton
              color="neutral"
              variant="ghost"
              size="xs"
              icon="i-lucide-pencil"
              :disabled="q.pending || forcing !== 0 || changing !== 0 || deleting !== 0 || editing !== 0"
              aria-label="Edit queued prompt"
              title="Edit this prompt"
              @click="startEdit(q)"
            />
            <ModelSelection
              :provider="q.provider"
              :account-id="q.account_id || 0"
              :model="q.model"
              :effort="q.effort || ''"
              size="xs"
              :disabled="q.pending || forcing !== 0 || changing !== 0 || deleting !== 0 || editing !== 0"
              :loading="changing === q.id"
              title="Change queued model"
              @change="chooseQueuedModel(q, $event)"
            />
            <UButton
              color="warning"
              variant="ghost"
              size="xs"
              :label="forcing === q.id ? forcingLabel : sendNowLabel"
              :loading="forcing === q.id"
              :disabled="q.pending || forcing !== 0 || changing !== 0 || deleting !== 0 || editing !== 0"
              :title="sendNowHint"
              @click="force(q)"
            />
            <UButton
              color="error"
              variant="ghost"
              size="xs"
              icon="i-lucide-trash-2"
              :loading="deleting === q.id"
              :disabled="q.pending || forcing !== 0 || changing !== 0 || deleting !== 0 || editing !== 0"
              aria-label="Delete queued prompt"
              title="Delete this queued prompt"
              @click="removeQueued(q)"
            />
          </div>
        </template>
      </div>
    </div>
  </div>
</template>
