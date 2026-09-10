<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";

import { forceQueued, providerOf, S, updateQueuedModel } from "../store";
import Message from "./Message.vue";
import ToolGroup from "./ToolGroup.vue";

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

/* A run of consecutive tool calls is handed to one ToolGroup, which shows
   only its latest until asked for the rest. Grouping reads role and nothing
   else, so a streaming delta — which only grows a message's content — does
   not rebuild the list. */
const rows = computed(() => {
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

/* Queued prompts are drawn under the transcript, each with how long it has
   been waiting, so opening a session shows the text that is going to run. */
const queued = computed(() => [
  ...(S.detail?.queued || []),
  ...(S.detail?.pendingQueued || []),
]);
const forcing = ref(0);
const changing = ref(0);
const NONE = "__default";

const modelOf = (provider, model) => providerOf(provider)?.models.find((candidate) => candidate.id === model);
const labelOf = (provider, model) => modelOf(provider, model)?.label || model;
const effortsFor = (q) => modelOf(q.provider, q.model)?.efforts || providerOf(q.provider)?.efforts || [];
const effortItems = (q) => [
  { label: "default effort", value: NONE },
  ...effortsFor(q).map((effort) => ({ label: effort, value: effort })),
];
const modelGroups = computed(() => S.providers.map((provider) => ({
  id: provider.name,
  label: provider.display_name,
  items: provider.models.map((model) => ({
    label: model.label,
    description: model.id,
    value: ["model", provider.name, model.id].join(":"),
    disabled: !provider.available,
  })),
})));

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

async function setQueuedChoice(q, provider, model, effort) {
  if (q.pending) return;
  changing.value = q.id;
  try {
    await updateQueuedModel(q.id, provider, model, effort);
  } catch {
    /* updateQueuedModel has already shown the failure */
  } finally {
    changing.value = 0;
  }
}

function pickQueuedModel(q, value) {
  const [, provider, model] = value.split(":", 3);
  const next = modelOf(provider, model);
  const efforts = next?.efforts || providerOf(provider)?.efforts || [];
  setQueuedChoice(q, provider, model, efforts.includes(q.effort) ? q.effort : "");
}

function pickQueuedEffort(q, effort) {
  setQueuedChoice(q, q.provider, q.model, effort === NONE ? "" : effort);
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
      Pick a task, or a project to start one.
    </p>
    <div v-else class="mx-auto max-w-[860px] px-6 pt-5 pb-2">
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
        <div class="mt-1 flex items-center gap-1">
          <UPopover>
            <UButton
              color="neutral"
              variant="ghost"
              size="xs"
              trailing-icon="i-lucide-chevron-down"
              :label="labelOf(q.provider, q.model)"
              :loading="changing === q.id"
              :disabled="q.pending || forcing !== 0 || changing !== 0"
              title="Change queued model"
            />
            <template #content>
              <UCommandPalette
                class="w-80"
                :groups="modelGroups"
                value-key="value"
                placeholder="Search models…"
                :fuse="{ fuseOptions: { keys: ['label', 'description'], threshold: 0.35, ignoreLocation: true }, resultLimit: 20 }"
                @update:model-value="pickQueuedModel(q, $event)"
              />
            </template>
          </UPopover>
          <USelect
            :model-value="q.effort || NONE"
            :items="effortItems(q)"
            size="xs"
            :disabled="q.pending || forcing !== 0 || changing !== 0"
            title="Change queued effort"
            @update:model-value="pickQueuedEffort(q, $event)"
          />
          <UButton
            color="warning"
            variant="ghost"
            size="xs"
            :label="sendNowLabel"
            :loading="forcing === q.id"
            :disabled="q.pending || forcing !== 0 || changing !== 0"
            :title="sendNowHint"
            @click="force(q)"
          />
        </div>
      </div>
    </div>
  </div>
</template>
