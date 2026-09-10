<script setup>
import { computed, nextTick, ref, watch } from "vue";

import { S, contextWindow, enqueue, enterDoes, fail, hit, isStarred, providerOf, refreshSubscriptionLimits, send, setModel, stopTurn, toggleStar } from "../store";
import { ago } from "../api";
import Chord from "./Chord.vue";
import ContextPane from "./ContextPane.vue";
import { useTextHistory } from "../text-history";

/* The unsent prompt belongs to the conversation, not to this bar: one bar
   serves every session, so text kept here would follow you between tabs. */
const text = computed({
  get: () => S.owner?.draft || "",
  set: (v) => {
    if (S.owner) S.owner.draft = v;
  },
});
const history = useTextHistory(text, (value) => (text.value = value));
const prompt = ref(null);
const modelOpen = ref(false);
const queueOpen = ref(false);

/* New sessions can be made while another prompt bar is still mounted, so an
   explicit focus request is more reliable than the component's autofocus. */
watch(
  () => S.promptFocus,
  async () => {
    await nextTick();
    prompt.value?.textareaRef?.focus();
  },
);

/* The effort picker cannot carry an empty value, so "no effort" travels as a
   sentinel and is turned back into "" on the way to the API. */
const NONE = "__default";

const provider = computed(() => providerOf(S.detail?.session.provider));
const effort = computed(() => S.detail?.session.effort || "");
const selectedModel = computed(() =>
  provider.value?.models.find((m) => m.id === S.detail?.session.model),
);
const effortsFor = (model) => model?.efforts || provider.value?.efforts || [];
const selectedEfforts = computed(() => effortsFor(selectedModel.value));

const modelOf = (providerName, id) => providerOf(providerName)?.models.find((m) => m.id === id);
const labelOf = (providerName, id) => modelOf(providerName, id)?.label || id;

/* A favourite is a model and an effort together — an effort on its own is
   never starred — so starred combinations head the model picker as single
   entries that set both. The effort picker below stays free to change either
   way, starred or not. */
const combos = computed(() =>
  S.stars.filter((s) => modelOf(s.provider, s.model)),
);

const onCombo = computed(() =>
  combos.value.some(
    (c) =>
      c.provider === S.detail.session.provider &&
      c.model === S.detail.session.model &&
      (c.effort || "") === effort.value,
  ),
);

const modelGroups = computed(() => [
  ...(combos.value.length
    ? [{
        id: "favourites",
        label: "Favourites",
        items: combos.value.map((c, i) => ({
          label: `★ ${labelOf(c.provider, c.model)} · ${c.effort || "default"}`,
          description: providerOf(c.provider)?.display_name || c.provider,
          value: "combo:" + i,
          disabled: !providerOf(c.provider)?.available,
        })),
      }]
    : []),
  ...S.providers.map((p) => ({
    id: p.name,
    label: p.display_name,
    items: p.models.map((m) => ({
      label: m.label,
      description: m.id,
      value: `model:${p.name}:${m.id}`,
      disabled: !p.available,
    })),
  })),
]);

const model = computed({
  get: () =>
    onCombo.value
      ? "combo:" +
        combos.value.findIndex(
          (c) =>
            c.provider === S.detail.session.provider &&
            c.model === S.detail.session.model &&
            (c.effort || "") === effort.value,
        )
      : `model:${S.detail.session.provider}:${S.detail.session.model}`,
  set: (v) => {
    const [kind, rest] = [v.slice(0, v.indexOf(":")), v.slice(v.indexOf(":") + 1)];
    if (kind === "combo") {
      const c = combos.value[Number(rest)];
      setModel(c.provider, c.model, c.effort || "");
    } else {
      const [providerName, modelID] = rest.split(":", 2);
      const nextProvider = providerOf(providerName);
      const next = nextProvider?.models.find((m) => m.id === modelID);
      const nextEfforts = next?.efforts || nextProvider?.efforts || [];
      setModel(providerName, modelID, nextEfforts.includes(effort.value) ? effort.value : "");
    }
  },
});

const effortItems = computed(() => [
  { label: "default effort", value: NONE },
  ...selectedEfforts.value.map((e) => ({ label: e, value: e })),
]);

const effortValue = computed({
  get: () => effort.value || NONE,
  set: (v) => setModel(S.detail.session.provider, S.detail.session.model, v === NONE ? "" : v),
});


function pickModel(value) {
  model.value = value;
  modelOpen.value = false;
}

const contextUsed = computed(() => S.detail?.stats.context_tokens || 0);
const contextTotal = computed(() => contextWindow(S.detail?.session, S.detail?.stats));
const contextPct = computed(() => contextTotal.value ? Math.min(100, Math.round((contextUsed.value / contextTotal.value) * 100)) : 0);
const contextTone = computed(() => contextPct.value >= 90 ? "var(--color-red-500)" : contextPct.value >= 70 ? "var(--color-amber-500)" : "var(--ui-primary)");

/* One bar per window the provider reports, across every bucket it sends:
   Codex packs its two windows into one bucket, Claude Code sends one bucket
   per window, and a plan can meter more than two. */
const subscriptionLimits = computed(() => S.subscriptionLimits[S.detail?.session?.provider] || []);
const subscriptionWindows = computed(() =>
  subscriptionLimits.value.flatMap((limit) =>
    [limit.primary, limit.secondary]
      .filter(Boolean)
      .map((window, index) => ({ ...window, label: allowanceLabel(window, index) })),
  ),
);
const planType = computed(() => subscriptionLimits.value.find((limit) => limit.plan_type)?.plan_type || "");
const reachedType = computed(() => subscriptionLimits.value.find((limit) => limit.reached_type)?.reached_type || "");

/* A provider that names its own buckets wins; one that only sizes them gets
   the duration turned into a name. */
function allowanceLabel(window, index) {
  if (window.label) return window.label;
  const minutes = window.window_duration_mins;
  if (minutes >= 6 * 24 * 60) return "Weekly";
  if (minutes >= 60 && minutes % 60 === 0) return `${minutes / 60}-hour`;
  return index === 0 ? "Hourly" : "Allowance";
}

function resetAt(seconds) {
  return seconds ? new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(seconds * 1000) : "not reported";
}

/* An asked reading is current and carries no stamp. A remembered one — the
   bucket Claude Code named mid-turn — can be older than the panel it is shown
   in, so saying when it was taken is the honest alternative to presenting a
   stale figure as current. */
const reportedAgo = computed(() => {
  const stamps = subscriptionLimits.value.map((limit) => limit.reported_at).filter(Boolean);
  return stamps.length ? ago(Math.max(...stamps)) : "";
});

watch(
  () => S.detail?.session?.provider,
  (provider) => {
    if (provider) refreshSubscriptionLimits(provider).catch(() => {});
  },
  { immediate: true },
);

const starred = computed(() =>
  isStarred(S.detail.session.provider, S.detail.session.model, effort.value),
);

async function star() {
  const { provider: p, model: m } = S.detail.session;
  try {
    await toggleStar(p, m, effort.value);
  } catch (e) {
    fail(e);
  }
}

/* send clears the draft itself, and only when it really sends. */
function submit() {
  send(text.value);
}

function onPromptInput(e) {
  history.input(e);
}

function enqueuePrompt() {
  enqueue(text.value);
}

/* Send and enqueue are bindings, so the box answers whatever Settings says.
   Anything Enter is not bound to falls through to the textarea, which makes
   the newline — including plain Enter, if send has been moved off it. */
function onPromptKey(e) {
  if (hit(e, "prompt.enqueue")) {
    e.preventDefault();
    enqueuePrompt();
    return;
  }
  if (hit(e, "prompt.send")) {
    e.preventDefault();
    submit();
    return;
  }
  history.keydown(e);
}

const hints = computed(() => [
  { chord: S.keys["prompt.send"][0], what: "to send" },
  { chord: S.keys["prompt.enqueue"][0], what: "to enqueue" },
  { chord: S.keys["prompt.newline"][0], what: "for a newline" },
]);

/* The button does what Enter does, so the two never disagree, and the menu
   beside it holds the other action — never a second copy of the same one. */
const actions = computed(() => ({
  send: { label: "Send", run: submit, disabled: !!S.detail?.running },
  enqueue: { label: "🕒 Enqueue", run: enqueuePrompt, disabled: false },
}));
const primary = computed(() => actions.value[enterDoes() === "enqueue" ? "enqueue" : "send"]);
const other = computed(() => actions.value[enterDoes() === "enqueue" ? "send" : "enqueue"]);

/* The menu closes itself: it holds one action and it has just been used. */
function runOther() {
  queueOpen.value = false;
  other.value.run();
}
</script>

<template>
  <form class="shrink-0 p-3" @submit.prevent="primary.run()">
    <div
      class="mx-auto max-w-[860px] rounded-[var(--ui-radius-lg,10px)] bg-default p-2 shadow-xs inset-ring inset-ring-accented focus-within:inset-ring-2 focus-within:inset-ring-primary"
    >
      <UTextarea
        ref="prompt"
        v-model="text"
        :rows="3"
        variant="none"
        placeholder="Ask the agent…"
        class="w-full"
        :ui="{ base: 'resize-y' }"
        @input="onPromptInput"
        @keydown="onPromptKey"
      />
      <div class="flex items-center gap-1.5">
        <UPopover v-model:open="modelOpen">
          <UButton
            type="button"
            color="neutral"
            variant="outline"
            size="sm"
            trailing-icon="i-lucide-chevron-down"
            :label="labelOf(S.detail.session.provider, S.detail.session.model)"
            title="Choose model"
          />
          <template #content>
            <UCommandPalette
              class="w-80"
              :groups="modelGroups"
              value-key="value"
              placeholder="Search models…"
              :fuse="{ fuseOptions: { keys: ['label', 'description'], threshold: 0.35, ignoreLocation: true }, resultLimit: 20 }"
              @update:model-value="pickModel"
            />
          </template>
        </UPopover>
        <USelect v-model="effortValue" :items="effortItems" size="sm" title="Effort" />
        <UButton
          color="neutral"
          variant="ghost"
          size="sm"
          :class="starred ? 'text-yellow-500' : ''"
          :title="`${starred ? 'Unstar' : 'Star'} ${labelOf(S.detail.session.provider, S.detail.session.model)} · ${effort || 'default'}`"
          :label="starred ? '★' : '☆'"
          @click="star"
        />
        <span class="flex-1" />
        <UPopover :content="{ side: 'top', sideOffset: 8 }">
          <button type="button" class="usage-summary" aria-label="Context and subscription usage">
            <span
              class="context-ring"
              :style="{ '--context-pct': contextPct + '%', '--context-tone': contextTone }"
            >
              <span class="context-ring-value">{{ contextTotal ? contextPct + '%' : '—' }}</span>
            </span>
            <span v-if="subscriptionWindows.length" class="subscription-limits">
              <span v-for="(window, i) in subscriptionWindows" :key="window.label + i" class="subscription-limit-track">
                <span
                  class="subscription-limit-fill"
                  :class="window.used_percent >= 90 ? 'bg-error' : window.used_percent >= 70 ? 'bg-warning' : 'bg-primary'"
                  :style="{ width: Math.min(100, window.used_percent) + '%' }"
                />
              </span>
            </span>
          </button>
          <template #content>
            <div class="flex divide-x divide-default">
              <ContextPane />
              <div v-if="subscriptionWindows.length" class="w-64 p-3">
                <p class="m-0 text-xs font-medium text-muted">
                  {{ planType ? planType + ' subscription' : 'Subscription allowance' }}
                </p>
                <dl class="mt-1.5 mb-0 space-y-1 text-xs tabular-nums">
                  <div v-for="(window, i) in subscriptionWindows" :key="window.label + i" class="flex justify-between gap-3">
                    <dt class="text-muted">{{ window.label }}</dt>
                    <dd class="m-0 text-highlighted">{{ Math.round(window.used_percent) }}% used · resets {{ resetAt(window.resets_at) }}</dd>
                  </div>
                  <div v-if="reachedType" class="flex justify-between gap-3">
                    <dt class="text-muted">Status</dt>
                    <dd class="m-0 text-highlighted">{{ reachedType }}</dd>
                  </div>
                  <div v-if="reportedAgo" class="flex justify-between gap-3">
                    <dt class="text-muted">Reported</dt>
                    <dd class="m-0 text-highlighted">{{ reportedAgo }}</dd>
                  </div>
                </dl>
              </div>
            </div>
          </template>
        </UPopover>
        <span v-if="S.detail.running" class="text-xs text-dimmed">running…</span>
        <UButton
          v-if="S.detail.running"
          color="neutral"
          variant="outline"
          size="sm"
          label="Stop"
          @click="stopTurn"
        />
        <UFieldGroup size="sm">
          <UButton type="submit" :disabled="primary.disabled" :label="primary.label" />
          <UPopover v-model:open="queueOpen">
            <UButton type="button" icon="i-lucide-chevron-down" aria-label="More prompt actions" />
            <template #content>
              <div class="p-1">
                <UButton
                  type="button"
                  color="neutral"
                  variant="ghost"
                  block
                  :disabled="other.disabled"
                  :label="other.label"
                  @click="runOther"
                />
              </div>
            </template>
          </UPopover>
        </UFieldGroup>
      </div>
    </div>
    <p class="mx-auto mt-1.5 flex max-w-[860px] flex-wrap items-center gap-x-1.5 px-1 text-xs text-dimmed">
      <template v-for="(hint, i) in hints" :key="hint.what">
        <span v-if="i">·</span>
        <Chord :chord="hint.chord" />
        <span>{{ hint.what }}</span>
      </template>
    </p>
  </form>
</template>
