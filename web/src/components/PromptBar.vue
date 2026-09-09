<script setup>
import { computed, nextTick, ref, watch } from "vue";

import { S, fail, isStarred, providerOf, send, setModel, stopTurn, toggleStar } from "../store";
import ContextPane from "./ContextPane.vue";

/* The unsent prompt belongs to the conversation, not to this bar: one bar
   serves every session, so text kept here would follow you between tabs. */
const text = computed({
  get: () => S.owner?.draft || "",
  set: (v) => {
    if (S.owner) S.owner.draft = v;
  },
});
const prompt = ref(null);

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

const labelOf = (id) => provider.value?.models.find((m) => m.id === id)?.label || id;

/* A favourite is a model and an effort together — an effort on its own is
   never starred — so starred combinations head the model picker as single
   entries that set both. The effort picker below stays free to change either
   way, starred or not. */
const combos = computed(() =>
  !provider.value
    ? []
    : S.stars.filter(
        (s) => s.provider === provider.value.name && provider.value.models.some((m) => m.id === s.model),
      ),
);

const onCombo = computed(() =>
  combos.value.some((c) => c.model === S.detail.session.model && (c.effort || "") === effort.value),
);

const modelItems = computed(() => {
  if (!provider.value) return [];
  const items = combos.value.map((c, i) => ({
    label: `★ ${labelOf(c.model)} · ${c.effort || "default"}`,
    value: "combo:" + i,
  }));
  if (items.length) items.push({ type: "separator" });
  return items.concat(provider.value.models.map((m) => ({ label: m.label, value: "model:" + m.id })));
});

const model = computed({
  get: () =>
    onCombo.value
      ? "combo:" +
        combos.value.findIndex(
          (c) => c.model === S.detail.session.model && (c.effort || "") === effort.value,
        )
      : "model:" + S.detail.session.model,
  set: (v) => {
    const [kind, rest] = [v.slice(0, v.indexOf(":")), v.slice(v.indexOf(":") + 1)];
    if (kind === "combo") {
      const c = combos.value[Number(rest)];
      setModel(c.model, c.effort || "");
    } else {
      const next = provider.value?.models.find((m) => m.id === rest);
      setModel(rest, effortsFor(next).includes(effort.value) ? effort.value : "");
    }
  },
});

const effortItems = computed(() => [
  { label: "default effort", value: NONE },
  ...selectedEfforts.value.map((e) => ({ label: e, value: e })),
]);

const effortValue = computed({
  get: () => effort.value || NONE,
  set: (v) => setModel(S.detail.session.model, v === NONE ? "" : v),
});

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
</script>

<template>
  <form class="shrink-0 p-3" @submit.prevent="submit">
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
        @keydown.enter.exact.prevent="submit"
      />
      <div class="flex items-center gap-1.5">
        <USelect v-model="model" :items="modelItems" size="sm" title="Model" />
        <USelect v-model="effortValue" :items="effortItems" size="sm" title="Effort" />
        <UButton
          color="neutral"
          variant="ghost"
          size="sm"
          :class="starred ? 'text-yellow-500' : ''"
          :title="`${starred ? 'Unstar' : 'Star'} ${labelOf(S.detail.session.model)} · ${effort || 'default'}`"
          :label="starred ? '★' : '☆'"
          @click="star"
        />
        <UPopover>
          <UButton
            color="neutral"
            variant="ghost"
            size="sm"
            icon="i-lucide-settings"
            title="Context and session totals"
            aria-label="Context and session totals"
          />
          <template #content><ContextPane /></template>
        </UPopover>
        <span class="flex-1" />
        <span v-if="S.detail.running" class="text-xs text-dimmed">running…</span>
        <UButton
          v-if="S.detail.running"
          color="neutral"
          variant="outline"
          size="sm"
          label="Stop"
          @click="stopTurn"
        />
        <UButton type="submit" size="sm" :disabled="S.detail.running" label="Send" />
      </div>
    </div>
    <p class="mx-auto mt-1.5 max-w-[860px] px-1 text-xs text-dimmed">
      <UKbd value="enter" /> to send · <UKbd value="shift" /><UKbd value="enter" /> for a newline
    </p>
  </form>
</template>
