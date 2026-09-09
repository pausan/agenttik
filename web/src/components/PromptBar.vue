<script setup>
import { computed, ref } from "vue";

import { S, fail, isStarred, providerOf, send, setModel, stopTurn, toggleStar } from "../store";

const text = ref("");

/* The effort picker cannot carry an empty value, so "no effort" travels as a
   sentinel and is turned back into "" on the way to the API. */
const NONE = "__default";

const provider = computed(() => providerOf(S.detail?.session.provider));
const effort = computed(() => S.detail?.session.effort || "");

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
      setModel(rest, effort.value);
    }
  },
});

const effortItems = computed(() => [
  { label: "default effort", value: NONE },
  ...(provider.value?.efforts || []).map((e) => ({ label: e, value: e })),
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

function submit() {
  const prompt = text.value;
  text.value = "";
  send(prompt);
}
</script>

<template>
  <form class="shrink-0 p-3" @submit.prevent="submit">
    <div
      class="mx-auto max-w-[860px] rounded-[var(--ui-radius-lg,10px)] bg-default p-2 shadow-xs inset-ring inset-ring-accented focus-within:inset-ring-2 focus-within:inset-ring-primary"
    >
      <UTextarea
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
