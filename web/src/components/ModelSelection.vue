<script setup>
/* The one model control used by prompts, edited messages, queued prompts and
   saved jobs. It keeps model and effort separate while every model choice
   still carries its subscription and provider. */
import { computed, ref, watch } from "vue";

import {
  S,
  effortsFor,
  fail,
  isModelChoiceVisible,
  isStarred,
  modelAccountLabel,
  modelPickerGroups,
  parseModelChoice,
  providerOf,
  toggleStar,
} from "../store";

defineOptions({ inheritAttrs: false });

const props = defineProps({
  provider: { type: String, required: true },
  accountId: { type: Number, default: 0 },
  model: { type: String, required: true },
  effort: { type: String, default: "" },
  size: { type: String, default: "sm" },
  disabled: { type: Boolean, default: false },
  loading: { type: Boolean, default: false },
  showFavourite: { type: Boolean, default: false },
  title: { type: String, default: "Choose model and subscription" },
});
const emit = defineEmits(["change"]);

const NONE = "__default";
const open = ref(false);
const search = ref("");
const collapsed = ref(new Set());

const modelOf = (provider, model) =>
  providerOf(provider)?.models.find((candidate) => candidate.id === model);
const modelLabel = (provider, model) => modelOf(provider, model)?.label || model;
const effortLabel = (effort) => effort
  ? effort[0].toUpperCase() + effort.slice(1)
  : "Default";
const choiceLabel = (provider, accountID, model, effort) => [
  modelAccountLabel(provider, accountID),
  modelLabel(provider, model),
  ...(effort !== undefined ? [effortLabel(effort)] : []),
].join(" · ");

const currentLabel = computed(() =>
  choiceLabel(props.provider, props.accountId, props.model),
);
const currentEfforts = computed(() => effortsFor(props.provider, props.model));
const effortItems = computed(() => [
  { label: "Default", value: NONE },
  ...currentEfforts.value.map((effort) => ({
    label: effortLabel(effort), value: effort,
  })),
]);

const favourites = computed(() => S.stars.filter((star) =>
  modelOf(star.provider, star.model) &&
  isModelChoiceVisible(star.provider, star.account_id, star.model),
));

function groupCollapsed(id) {
  return !search.value.trim() && collapsed.value.has(id);
}

function toggleGroup(id) {
  const next = new Set(collapsed.value);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  collapsed.value = next;
}

const groups = computed(() => [
  ...(favourites.value.length ? [{
    id: "favourites",
    label: "Favourites",
    items: favourites.value.map((star, index) => ({
      label: choiceLabel(star.provider, star.account_id, star.model, star.effort || ""),
      search: `${modelAccountLabel(star.provider, star.account_id)} · ${providerOf(star.provider)?.display_name || star.provider} · ${modelLabel(star.provider, star.model)} · ${effortLabel(star.effort)}`,
      value: `favourite:${index}`,
      disabled: !providerOf(star.provider)?.available,
      icon: star.provider === props.provider && star.account_id === (props.accountId || 0) &&
        star.model === props.model && (star.effort || "") === (props.effort || "")
        ? "i-lucide-check" : undefined,
    })),
  }] : []),
  ...modelPickerGroups().map((group) => groupCollapsed(group.id)
    ? {
        ...group,
        items: [{
          label: `Show ${group.items.length} models`,
          search: group.label,
          value: `expand:${group.id}`,
          icon: "i-lucide-chevron-right",
        }],
      }
    : {
        ...group,
        items: group.items.map((item) => ({
          ...item,
          icon: item.value === `model:${props.provider}:${props.accountId || 0}:${props.model}`
            ? "i-lucide-check" : undefined,
        })),
      }),
]);

function choose(value) {
  if (!value) return;
  if (value.startsWith("expand:")) {
    toggleGroup(value.slice("expand:".length));
    return;
  }
  if (value.startsWith("favourite:")) {
    const star = favourites.value[Number(value.slice("favourite:".length))];
    if (star) emit("change", {
      provider: star.provider,
      accountID: star.account_id || 0,
      model: star.model,
      effort: star.effort || "",
    });
  } else {
    const choice = parseModelChoice(value);
    const nextEfforts = effortsFor(choice.provider, choice.model);
    emit("change", {
      ...choice,
      effort: nextEfforts.includes(props.effort) ? props.effort : "",
    });
  }
  open.value = false;
}

function chooseEffort(value) {
  emit("change", {
    provider: props.provider,
    accountID: props.accountId || 0,
    model: props.model,
    effort: value === NONE ? "" : value,
  });
}

const starred = computed(() =>
  isStarred(props.provider, props.accountId, props.model, props.effort),
);

async function favourite() {
  try {
    await toggleStar(props.provider, props.accountId, props.model, props.effort);
  } catch (error) {
    fail(error);
  }
}

watch(open, (isOpen) => {
  if (!isOpen) search.value = "";
});
</script>

<template>
  <div class="model-selection contents">
    <UPopover v-model:open="open">
      <UButton
        v-bind="$attrs"
        type="button"
        color="neutral"
        variant="outline"
        :size="size"
        trailing-icon="i-lucide-chevron-down"
        :label="currentLabel"
        class="model-selection-button min-w-0 max-w-72"
        :title="title"
        :disabled="disabled"
        :loading="loading"
      />
      <template #content>
        <UCommandPalette
          class="model-selection-palette w-96 max-w-[calc(100vw-2rem)]"
          :class="{ 'has-favourites': favourites.length }"
          :groups="groups"
          value-key="value"
          placeholder="Search models…"
          v-model:search-term="search"
          preserve-group-order
          :ui="{ viewport: 'max-h-[min(30rem,65vh)]', itemLabelBase: 'truncate' }"
          :fuse="{ fuseOptions: { keys: ['label', 'search'], threshold: 0.35, ignoreLocation: true }, resultLimit: 100 }"
          @update:model-value="choose"
        >
          <template #group-label="{ group, label }">
            <button
              v-if="group.id !== 'favourites'"
              type="button"
              class="flex w-full items-center gap-1 rounded px-1 py-0.5 text-left font-semibold text-highlighted hover:bg-elevated"
              :aria-expanded="!groupCollapsed(group.id)"
              :aria-label="`${groupCollapsed(group.id) ? 'Expand' : 'Collapse'} ${label}`"
              @click.stop="toggleGroup(group.id)"
            >
              <UIcon :name="groupCollapsed(group.id) ? 'i-lucide-chevron-right' : 'i-lucide-chevron-down'" class="size-3.5" />
              <span class="truncate">{{ label }}</span>
            </button>
            <span v-else class="block px-1 py-0.5 font-semibold text-highlighted">{{ label }}</span>
          </template>
        </UCommandPalette>
      </template>
    </UPopover>

    <USelect
      :model-value="effort || NONE"
      :items="effortItems"
      :size="size"
      :disabled="disabled"
      title="Effort"
      @update:model-value="chooseEffort"
    />

    <UButton
      v-if="showFavourite"
      type="button"
      color="neutral"
      variant="ghost"
      :size="size"
      :class="starred ? 'text-yellow-500' : ''"
      :title="`${starred ? 'Remove from' : 'Add to'} favourites: ${choiceLabel(provider, accountId, model, effort)}`"
      :aria-label="`${starred ? 'Remove from' : 'Add to'} favourites: ${choiceLabel(provider, accountId, model, effort)}`"
      icon="i-lucide-star"
      @click="favourite"
    />
  </div>
</template>

<style scoped>
/* A long favourite list must not push provider groups out of the picker. */
.model-selection-palette.has-favourites :deep([data-slot="group"]:first-child) {
  max-height: 11rem;
  overflow-y: auto;
  overscroll-behavior: contain;
}
</style>
