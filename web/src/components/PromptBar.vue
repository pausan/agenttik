<script setup>
import { computed, defineAsyncComponent, nextTick, ref, watch } from "vue";

import { S, createSchedule, accountLabel, contextWindow, enqueue, enterDoes, fail, hit, isStarred, limitsKey, modelPickerGroups, parseModelChoice, providerOf, refreshSubscriptionLimits, send, setModel, stopTurn, toggleStar } from "../store";
import { ago } from "../api";
import Chord from "./Chord.vue";
import ContextPane from "./ContextPane.vue";
import { useTextHistory } from "../text-history";
import { isMobile } from "../ui";
import { clipboardImages, promptImages, promptText, readClipboardImages, uploadImage, withImages } from "../prompt-images";

/* Everything below is reached by a click or a chord, never by the first
   paint, so its code is fetched from its own chunk the moment it is first
   needed instead of being parsed on the way in. The chunks are built into
   the binary beside the main one, so this is still a read off the local
   server and never a network call. */
const ScheduleModal = defineAsyncComponent(() => import("./ScheduleModal.vue"));

/* The unsent prompt belongs to the conversation, not to this bar: one bar
   serves every session, so text kept here would follow you between tabs. */
const text = computed({
  get: () => promptText(S.owner?.draft),
  set: (v) => {
    if (S.owner) S.owner.draft = withImages(v, promptImages(S.owner.draft));
  },
});
const history = useTextHistory(text, (value) => (text.value = value));
const prompt = ref(null);
const modelOpen = ref(false);
const queueOpen = ref(false);
const scheduling = ref(false);
const images = computed(() => promptImages(S.owner?.draft));
const empty = computed(() => !text.value.trim());
const uploading = computed(() => !!S.owner?.imageUploads);

async function onPaste(e) {
  let files = clipboardImages(e.clipboardData);
  const owner = S.owner;
  if (!owner) return;
  const readClipboard = !files.length && !e.clipboardData?.types?.length && navigator.clipboard?.read;
  if (!files.length && !readClipboard) return;
  e.preventDefault();
  const pastedText = e.clipboardData.getData("text/plain");
  if (pastedText) {
    const box = e.target;
    text.value = text.value.slice(0, box.selectionStart) + pastedText + text.value.slice(box.selectionEnd);
  }
  owner.imageUploads = (owner.imageUploads || 0) + 1;
  try {
    if (readClipboard) files = await readClipboardImages(navigator.clipboard);
    for (const file of files) {
      const image = await uploadImage(file);
      owner.draft = withImages(promptText(owner.draft), [...promptImages(owner.draft), image]);
    }
  } catch (error) {
    fail(error);
  } finally {
    owner.imageUploads -= 1;
  }
}

function removeImage(index) {
  S.owner.draft = withImages(text.value, images.value.filter((_, i) => i !== index));
}

/* New sessions can be made while another prompt bar is still mounted, so an
   explicit focus request is more reliable than the component's autofocus. */
watch(
  () => S.promptFocus,
  async () => {
    if (isMobile()) return;
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

/* Which subscription the next prompt would go on, named beside the model
   wherever the provider has more than one — otherwise there is nothing to
   tell it apart from and the name is noise. A task pointing at a
   subscription that has been removed says so here rather than looking
   ordinary and failing on send. */
const accountAlias = computed(() =>
  accountLabel(S.detail?.session.provider, S.detail?.session.account_id),
);
const modelButtonLabel = computed(() => {
  const model = labelOf(S.detail.session.provider, S.detail.session.model);
  return accountAlias.value ? `${model} · ${accountAlias.value}` : model;
});

/* A favourite is a model and an effort together — an effort on its own is
   never starred — so starred combinations head the model picker as single
   entries that set both. The effort picker below stays free to change either
   way, starred or not. */
const combos = computed(() =>
  S.stars.filter((s) => modelOf(s.provider, s.model)),
);

const modelSearch = ref("");
const collapsedProviders = ref(new Set());

function providerCollapsed(providerName) {
  return !modelSearch.value.trim() && collapsedProviders.value.has(providerName);
}

function toggleProvider(providerName) {
  const next = new Set(collapsedProviders.value);
  if (next.has(providerName)) next.delete(providerName);
  else next.add(providerName);
  collapsedProviders.value = next;
}

const onCombo = computed(() =>
  combos.value.some(
    (c) =>
      c.provider === S.detail.session.provider &&
      c.model === S.detail.session.model &&
      (c.effort || "") === effort.value,
  ),
);

/* One group per provider and subscription (050-subscription-accounts.md), so
   a model and the account it would run on are picked in one go. A favourite
   is a model and an effort only — never an account — so choosing one keeps
   the subscription the task is already on. */
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
  ...modelPickerGroups().map((group) =>
    providerCollapsed(group.id)
      ? {
          ...group,
          items: [{
            label: `Show ${group.items.length} models`,
            description: group.label,
            value: `expand:${group.id}`,
            icon: "i-lucide-chevron-right",
          }],
        }
      : group,
  ),
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
      : `model:${S.detail.session.provider}:${S.detail.session.account_id || 0}:${S.detail.session.model}`,
  set: (v) => {
    const [kind, rest] = [v.slice(0, v.indexOf(":")), v.slice(v.indexOf(":") + 1)];
    if (kind === "combo") {
      // A favourite carries no subscription, so the task keeps the one it is
      // on: that is the account whose allowance is on screen beside it.
      const c = combos.value[Number(rest)];
      setModel(c.provider, c.model, c.effort || "");
    } else {
      const { provider: providerName, accountID, model: modelID } = parseModelChoice(v);
      const nextProvider = providerOf(providerName);
      const next = nextProvider?.models.find((m) => m.id === modelID);
      const nextEfforts = next?.efforts || nextProvider?.efforts || [];
      setModel(providerName, modelID, nextEfforts.includes(effort.value) ? effort.value : "", accountID);
    }
  },
});

const effortItems = computed(() => [
  { label: "default effort", value: NONE },
  ...selectedEfforts.value.map((e) => ({ label: e, value: e })),
]);

const effortValue = computed({
  get: () => effort.value || NONE,
  set: (v) =>
    setModel(S.detail.session.provider, S.detail.session.model, v === NONE ? "" : v,
      S.detail.session.account_id || 0),
});


function pickModel(value) {
  // A cleared palette selection does not change the task's model.
  if (!value) return;
  if (value.startsWith("expand:")) {
    toggleProvider(value.slice("expand:".length));
    return;
  }
  model.value = value;
  modelOpen.value = false;
}

watch(modelOpen, (open) => {
  if (!open) modelSearch.value = "";
});

const contextUsed = computed(() => S.detail?.stats.context_tokens || 0);
const contextTotal = computed(() => contextWindow(S.detail?.session, S.detail?.stats));
const contextPct = computed(() => contextTotal.value ? Math.min(100, Math.round((contextUsed.value / contextTotal.value) * 100)) : 0);
const contextTone = computed(() => contextPct.value >= 90 ? "var(--color-red-500)" : contextPct.value >= 70 ? "var(--color-amber-500)" : "var(--ui-primary)");

/* One pair of thresholds for every allowance reading, the ring's own: amber
   from 70%, red from 90%. The bar carries it and so does the figure, so a
   bucket near its ceiling reads as one thing rather than a number to compare
   against a colour. */
const barTone = (percent) => (percent >= 90 ? "bg-error" : percent >= 70 ? "bg-warning" : "bg-primary");
const textTone = (percent) => (percent >= 90 ? "text-error" : percent >= 70 ? "text-warning" : "text-highlighted");

/* One bar per window the provider reports, across every bucket it sends:
   Codex packs its two windows into one bucket, Claude Code sends one bucket
   per window, and a plan can meter more than two. */
const subscriptionLimits = computed(
  () =>
    S.subscriptionLimits[
      limitsKey(S.detail?.session?.provider, S.detail?.session?.account_id)
    ] || [],
);
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

/* A reset is hours or days away, so the year is noise that pushed the line
   onto a third row; it is only drawn when the reset really is in another
   one. */
function resetAt(seconds) {
  const at = new Date(seconds * 1000);
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    year: at.getFullYear() === new Date().getFullYear() ? undefined : "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(at);
}

/* The reset date says when; this says how long, which is the number a 5-hour
   bucket is actually watched for. Days drop to the hour the same way resetAt
   drops the year: precision nobody reads a week out. Already-passed reads as
   nothing rather than a negative duration. */
function remaining(seconds) {
  const ms = seconds * 1000 - Date.now();
  if (ms <= 0) return "";
  const minutes = Math.round(ms / 60000);
  const days = Math.floor(minutes / 1440);
  const hours = Math.floor((minutes % 1440) / 60);
  if (days) return `${days}d ${hours}h remaining`;
  if (hours) return `${hours}h ${minutes % 60}min remaining`;
  return `${minutes}min remaining`;
}

/* An asked reading is current and carries no stamp. A remembered one — the
   bucket Claude Code named mid-turn — can be older than the panel it is shown
   in, so saying when it was taken is the honest alternative to presenting a
   stale figure as current. */
const reportedAgo = computed(() => {
  const stamps = subscriptionLimits.value.map((limit) => limit.reported_at).filter(Boolean);
  return stamps.length ? ago(Math.max(...stamps)) : "";
});

/* The bars answer for the model in the box, so the reading is taken again
   when the conversation changes, when the provider does, and when the model
   does — a provider can meter a single model on a window of its own
   (Claude Code's per-model weekly), so a different model is a different
   allowance, not just a different label. A provider that answers nothing
   keeps no bars. */
watch(
  [
    () => S.detail?.session?.id,
    () => S.detail?.session?.provider,
    () => S.detail?.session?.account_id,
    () => S.detail?.session?.model,
  ],
  ([, provider, account]) => {
    if (provider) refreshSubscriptionLimits(provider, account).catch(() => {});
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
  if (!empty.value && !uploading.value) send(S.owner?.draft || "");
}

function onPromptInput(e) {
  history.input(e);
}

function enqueuePrompt() {
  if (!empty.value && !uploading.value) enqueue(S.owner?.draft || "");
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

/* The fourth hint is only true while something is waiting, so it arrives with
   the queue rather than standing there naming a chord that does nothing. */
const queuedCount = computed(
  () => (S.detail?.queued?.length || 0) + (S.detail?.pendingQueued?.length || 0),
);

const hints = computed(() => [
  { chord: S.keys["prompt.send"][0], what: "to send" },
  { chord: S.keys["prompt.enqueue"][0], what: "to enqueue" },
  { chord: S.keys["prompt.newline"][0], what: "for a newline" },
  ...(queuedCount.value
    ? [{ chord: S.keys["prompt.editLast"][0], what: "to edit the last queued" }]
    : []),
]);

/* The button does what Enter does, so its label, icon and disabled state
   track the keybinding. The menu beside it is the Send menu (specs/012,
   specs/028, specs/062): Send, Enqueue, Schedule and Pinned task, in that order, so it never
   reshuffles when Settings moves Enter between Send and Enqueue. */
const actions = computed(() => ({
  send: { label: "Send", icon: "i-lucide-send", run: submit, disabled: empty.value || !!S.detail?.running || uploading.value },
  enqueue: { label: "Enqueue", icon: "i-lucide-clock", run: enqueuePrompt, disabled: empty.value || uploading.value },
  /* Schedule keeps the prompt rather than sending it: the dialog asks how
     often and how many times, and every run after that is a session of its
     own. See specs/028-scheduled-jobs.md. */
  schedule: { label: "Schedule…", icon: "i-lucide-repeat", run: () => (scheduling.value = true), disabled: empty.value || uploading.value },
  pin: { label: "Pinned task", icon: "i-lucide-pin", run: () => createSchedule({ every: "pinned", remaining: 0 }), disabled: empty.value || uploading.value },
}));
const primary = computed(() => actions.value[enterDoes() === "enqueue" ? "enqueue" : "send"]);
const menu = computed(() => [actions.value.send, actions.value.enqueue, actions.value.schedule, actions.value.pin]);

/* The menu closes itself: whichever action was chosen has just been used. */
function runMenuAction(action) {
  queueOpen.value = false;
  if (!action.disabled) action.run();
}
</script>

<template>
  <form class="prompt-bar shrink-0 p-3" @submit.prevent="primary.run()">
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
        @paste="onPaste"
      />
      <div v-if="images.length || uploading" class="flex flex-wrap gap-2 p-2" aria-label="Attached images">
        <div v-for="(image, i) in images" :key="i" class="relative">
          <a :href="image.url" target="_blank" rel="noopener"><img :src="image.url" :alt="`Attached image ${i + 1}`" class="h-20 w-24 rounded object-contain" /></a>
          <UButton type="button" size="xs" color="neutral" icon="i-lucide-x" :aria-label="`Remove image ${i + 1}`" class="absolute top-0 right-0" @click="removeImage(i)" />
        </div>
        <span v-if="uploading" role="status" class="text-sm text-muted">Saving images…</span>
      </div>
      <div class="prompt-controls flex items-center gap-1.5">
        <div class="prompt-model-controls contents">
          <UPopover v-model:open="modelOpen">
            <UButton
              type="button"
              color="neutral"
              variant="outline"
              size="sm"
              trailing-icon="i-lucide-chevron-down"
              :label="modelButtonLabel"
              class="prompt-model"
              title="Choose model and subscription"
            />
            <template #content>
              <UCommandPalette
                class="w-80 max-w-[calc(100vw-2rem)]"
                :groups="modelGroups"
                value-key="value"
                placeholder="Search models…"
                v-model:search-term="modelSearch"
                preserve-group-order
                :ui="{ viewport: 'max-h-[min(28rem,60vh)]' }"
                :fuse="{ fuseOptions: { keys: ['label', 'description'], threshold: 0.35, ignoreLocation: true }, resultLimit: 20 }"
                @update:model-value="pickModel"
              >
                <template #group-label="{ group, label }">
                  <button
                    v-if="group.id !== 'favourites'"
                    type="button"
                    class="flex w-full items-center gap-1 rounded px-1 py-0.5 text-left font-semibold text-highlighted hover:bg-elevated"
                    :aria-expanded="!providerCollapsed(group.id)"
                    :aria-label="`${providerCollapsed(group.id) ? 'Expand' : 'Collapse'} ${label}`"
                    @click.stop="toggleProvider(group.id)"
                  >
                    <UIcon :name="providerCollapsed(group.id) ? 'i-lucide-chevron-right' : 'i-lucide-chevron-down'" class="size-3.5" />
                    <span class="truncate">{{ label }}</span>
                  </button>
                  <span v-else class="block px-1 py-0.5 font-semibold text-highlighted">{{ label }}</span>
                </template>
              </UCommandPalette>
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
        </div>
        <span class="flex-1 max-md:hidden" />
        <div class="prompt-action-controls contents">
          <UPopover :content="{ side: 'top', sideOffset: 8 }">
            <button
              type="button"
              class="usage-summary"
              :aria-label="contextUsed && contextTotal ? `Main context ${contextPct}% and subscription usage` : 'Main context not reported and subscription usage'"
              :title="contextUsed && contextTotal ? `Main context: ${contextPct}%` : 'Main context not reported'"
            >
              <span
                class="context-ring"
                :style="{ '--context-pct': contextPct + '%', '--context-tone': contextTone }"
              >
                <span class="context-ring-value">{{ contextUsed && contextTotal ? contextPct + '%' : '—' }}</span>
              </span>
              <span v-if="subscriptionWindows.length" class="subscription-limits">
                <span v-for="(window, i) in subscriptionWindows" :key="window.label + i" class="subscription-limit-track">
                  <span
                    class="subscription-limit-fill"
                    :class="barTone(window.used_percent)"
                    :style="{ width: Math.min(100, window.used_percent) + '%' }"
                  />
                </span>
              </span>
            </button>
            <template #content>
              <div class="usage-details flex divide-x divide-default">
                <ContextPane />
                <!-- One window per row: its name and figure on a line, its bar
                     under them, its reset under that. A name is a plan's to
                     choose and can be long, so it truncates with the whole of
                     it on the hover; putting the reset on its own line is what
                     keeps the three from fighting over one. -->
                <div v-if="subscriptionWindows.length" class="w-64 p-3">
                  <p class="m-0 text-xs font-medium text-muted">
                    {{ planType ? planType + ' subscription' : 'Subscription allowance' }}
                  </p>
                  <!-- Which account these bars are the allowance of, drawn only
                       when the provider has more than one to confuse it with. -->
                  <p v-if="accountAlias" class="m-0 text-[11px] text-dimmed">{{ accountAlias }}</p>
                  <ul class="mt-2 mb-0 list-none space-y-2.5 p-0">
                    <li v-for="(window, i) in subscriptionWindows" :key="window.label + i">
                      <div class="flex items-baseline justify-between gap-2 text-xs">
                        <span class="truncate text-muted" :title="window.label">{{ window.label }}</span>
                        <span class="shrink-0 font-medium tabular-nums" :class="textTone(window.used_percent)">
                          {{ Math.round(window.used_percent) }}%
                        </span>
                      </div>
                      <span class="mt-1 block h-1.5 overflow-hidden rounded-full bg-accented">
                        <span
                          class="block h-full rounded-full"
                          :class="barTone(window.used_percent)"
                          :style="{ width: Math.min(100, window.used_percent) + '%' }"
                        />
                      </span>
                      <p class="mt-1 mb-0 text-[11px] text-dimmed tabular-nums">
                        {{ window.resets_at
                          ? 'resets ' + resetAt(window.resets_at) + (remaining(window.resets_at) ? ' · ' + remaining(window.resets_at) : '')
                          : 'reset time not reported' }}
                      </p>
                    </li>
                  </ul>
                  <dl v-if="reachedType || reportedAgo" class="mt-3 mb-0 border-t border-default pt-1">
                    <div v-if="reachedType" class="flex justify-between gap-2.5 py-0.5 text-xs">
                      <dt class="shrink-0 text-muted">Status</dt>
                      <dd class="m-0 truncate text-highlighted">{{ reachedType }}</dd>
                    </div>
                    <div v-if="reportedAgo" class="flex justify-between gap-2.5 py-0.5 text-xs">
                      <dt class="shrink-0 text-muted">Reported</dt>
                      <dd class="m-0 truncate text-highlighted tabular-nums">{{ reportedAgo }}</dd>
                    </div>
                  </dl>
                </div>
              </div>
            </template>
          </UPopover>
          <!-- No "running" label here: the header badge above the transcript
               already carries the state, with its own dot. -->
          <UButton
            v-if="S.detail.running"
            color="neutral"
            variant="outline"
            size="sm"
            label="Stop"
            @click="stopTurn"
          />
          <UFieldGroup size="sm">
            <UButton type="submit" :disabled="primary.disabled" :label="primary.label" :trailing-icon="primary.icon" />
            <UPopover v-model:open="queueOpen">
              <UButton type="button" :disabled="empty || uploading" icon="i-lucide-chevron-down" aria-label="More prompt actions" />
              <template #content>
                <div class="p-1">
                  <UButton
                    v-for="action in menu"
                    :key="action.label"
                    type="button"
                    color="neutral"
                    variant="ghost"
                    block
                    class="justify-start"
                    :disabled="action.disabled"
                    :label="action.label"
                    :trailing-icon="action.icon"
                    @click="runMenuAction(action)"
                  />
                </div>
              </template>
            </UPopover>
          </UFieldGroup>
        </div>
      </div>
    </div>
    <ScheduleModal v-if="scheduling" v-model:open="scheduling" />
    <p class="mx-auto mt-1.5 flex max-w-[860px] flex-wrap items-center gap-x-1.5 px-1 text-xs text-dimmed max-md:hidden">
      <template v-for="(hint, i) in hints" :key="hint.what">
        <span v-if="i">·</span>
        <Chord :chord="hint.chord" />
        <span>{{ hint.what }}</span>
      </template>
    </p>
  </form>
</template>
