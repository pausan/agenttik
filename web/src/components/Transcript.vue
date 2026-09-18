<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";

import {
  focusPrompt,
  forceQueued,
  openProjectPrompt,
  providerOf,
  S,
  updateQueuedModel,
  updateQueuedPrompt,
} from "../store";
import { tabScroll, vTabScroll } from "../tab-scroll";
import Message from "./Message.vue";
import ModelSelection from "./ModelSelection.vue";
import ToolGroup from "./ToolGroup.vue";

const emit = defineEmits(["start-tour"]);
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

/* A long conversation costs more to draw than opening it is worth waiting
   for: several hundred bubbles, each one a component and a markdown parse,
   is most of what a launch onto a busy task spends. The box opens scrolled to
   the bottom, so all but the last screenful of that is drawn where nobody is
   looking — TAIL rows go in first and the rest follows once they are on
   screen. Anything shorter than TAIL is drawn whole and never notices. */
const TAIL = 40;
const shown = ref(TAIL);
let reveal = 0;

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
  return out;
});

const rows = computed(() =>
  allRows.value.length > shown.value ? allRows.value.slice(-shown.value) : allRows.value,
);

/* First opening a task draws the tail and
   then the whole thing. The rest is drawn from a timeout inside a frame
   callback: the callback runs before the tail is painted, the timeout after
   it, which is the first moment the rest costs nobody anything.

   Returning to an open tab draws all rows immediately so its saved scroll
   offset is measured against the complete conversation. */
function revealTranscript() {
  const turn = ++reveal;
  shown.value = tabScroll(S.tab) ? Infinity : TAIL;
  if (shown.value === Infinity) return;
  if (allRows.value.length <= TAIL) return;
  requestAnimationFrame(() =>
    setTimeout(async () => {
      // A tab switch can happen between the frame and its timeout. Its old
      // callback must not reveal the new conversation before its tail paints.
      if (turn !== reveal) return;
      shown.value = Infinity;
      await nextTick();
      if (turn === reveal && box.value) box.value.scrollTop = box.value.scrollHeight;
    }, 0),
  );
}

watch(() => S.detail?.session.id, revealTranscript, { immediate: true });
// A short live conversation can cross the threshold without changing tabs.
// Keep the rows already on screen instead of briefly hiding them and
// moving a reader who has scrolled up.
watch(
  () => allRows.value.length > TAIL,
  (long, wasLong) => { if (long && !wasLong) shown.value = Infinity; },
);

/* Queued prompts are drawn under the transcript, each with how long it has
   been waiting, so opening a session shows the text that is going to run. */
const queued = computed(() => [
  ...(S.detail?.queued || []),
  ...(S.detail?.pendingQueued || []),
]);
const forcing = ref(0);
const changing = ref(0);
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
  if (q.pending || forcing.value || changing.value || editing.value) return;
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
  try {
    await forceQueued(q.id);
  } catch {
    /* forceQueued has already shown the failure */
  } finally {
    forcing.value = 0;
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
});
onUnmounted(() => {
  reveal++;
  window.clearInterval(clock);
});

/* Follow new output only while reading the bottom. A tab switch restores
   its saved position instead of being treated as new output. */
let follow = true;
function onScroll() {
  const el = box.value;
  follow = el.scrollHeight - el.clientHeight - el.scrollTop < 2;
}
watch(
  () => [S.activeTab, S.detail?.messages, queued.value.length],
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
  { deep: true, flush: "post", immediate: true },
);
</script>

<template>
  <div ref="box" v-tab-scroll="[S.tab, '', true]" class="min-h-0 flex-1 overflow-auto" @scroll="onScroll">
    <div v-if="!S.detail" class="pt-[18vh] text-center text-dimmed">
      <p>Pick a task, or a project to start one.</p>
      <UButton class="mt-4" label="Take the Quick Start Tour" icon="i-lucide-graduation-cap" @click="emit('start-tour')" />
    </div>
    <div v-else class="mx-auto max-w-[860px] px-6 pt-5 pb-2 max-md:px-3 max-md:pt-3">
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

      <template v-for="row in rows" :key="row.at">
        <ToolGroup v-if="row.tools" :tools="row.tools" />
        <Message v-else :message="row.message" />
      </template>
      <div v-if="S.detail.running" class="mb-3 flex items-center gap-2 text-xs text-dimmed" aria-label="Agent working">
        <span aria-hidden="true">{{ clockFace }}</span>
        <span>Working ({{ elapsedLabel }})</span>
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
          <span class="flex items-center gap-1 font-mono text-[11px] text-dimmed tabular-nums">
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
              :disabled="q.pending || forcing !== 0 || changing !== 0 || editing !== 0"
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
              :disabled="q.pending || forcing !== 0 || changing !== 0 || editing !== 0"
              :loading="changing === q.id"
              title="Change queued model"
              @change="chooseQueuedModel(q, $event)"
            />
            <UButton
              color="warning"
              variant="ghost"
              size="xs"
              :label="sendNowLabel"
              :loading="forcing === q.id"
              :disabled="q.pending || forcing !== 0 || changing !== 0 || editing !== 0"
              :title="sendNowHint"
              @click="force(q)"
            />
          </div>
        </template>
      </div>
    </div>
  </div>
</template>
