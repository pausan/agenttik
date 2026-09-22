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
  modelGroupLabel,
  modelLabelOf,
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

/* Both are hoisted out of the template: an object literal there is a new
   object on every keystroke, which rebuilds the palette's search index and is
   what makes typing stutter on a phone with a long API catalogue. */
const FUSE = {
  fuseOptions: { keys: ["label", "search"], threshold: 0.35, ignoreLocation: true },
  resultLimit: 100,
};
const PALETTE_UI = {
  // The software keyboard shrinks the visual viewport, not the layout one, so
  // the list is bounded by the height App.vue measures rather than by vh.
  viewport: "max-h-[min(30rem,calc(var(--mobile-height,100dvh)*0.62))]",
  itemLabelBase: "truncate",
};

const open = ref(false);
const search = ref("");
/* Only whether something is being searched reaches the group list, so typing
   never rebuilds it — the palette does its own filtering. */
const searching = computed(() => search.value.trim().length > 0);
const expansion = ref(new Map());

const modelOf = (provider, model) =>
  providerOf(provider)?.models.find((candidate) => candidate.id === model);
const effortLabel = (effort) => effort
  ? effort[0].toUpperCase() + effort.slice(1)
  : "Default";
const choiceLabel = (provider, accountID, model, effort) => [
  modelAccountLabel(provider, accountID),
  modelLabelOf(provider, model),
  ...(effort !== undefined ? [effortLabel(effort)] : []),
].join(" · ");
/* A favourite is the whole combination, so it names all four parts. */
const favouriteLabel = (star) => [
  modelGroupLabel(star.provider, star.account_id),
  modelLabelOf(star.provider, star.model),
  effortLabel(star.effort),
].join(" · ");

const currentLabel = computed(() =>
  props.provider && props.model ? choiceLabel(props.provider, props.accountId, props.model) : "Choose model",
);
const currentTitle = computed(() =>
  props.provider && props.model
    ? `${props.title}: ${modelGroupLabel(props.provider, props.accountId)} · ${modelLabelOf(props.provider, props.model)}`
    : props.title,
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

/* With favourites at the top the rest of the list starts as one line per
   subscription, so the choices worth keeping are not buried under every model
   of every provider. Without them there is nothing to bury, and the groups
   open as before. Expanding or collapsing one is remembered while the popover
   is open. */
function isCollapsed(id) {
  return expansion.value.get(id) ?? favourites.value.length > 0;
}

/* A search ignores collapsing altogether: what matches is what shows. */
function groupCollapsed(id) {
  return !searching.value && isCollapsed(id);
}

function toggleGroup(id) {
  const next = new Map(expansion.value);
  next.set(id, !isCollapsed(id));
  expansion.value = next;
}

const currentValue = computed(() =>
  `model:${props.provider}:${props.accountId || 0}:${props.model}`);

const pickerGroups = computed(() => modelPickerGroups());

const groups = computed(() => [
  ...(favourites.value.length ? [{
    id: "favourites",
    label: "Favourites",
    items: favourites.value.map((star, index) => ({
      label: favouriteLabel(star),
      search: `${favouriteLabel(star)} · ${star.provider} · ${star.model}`,
      value: `favourite:${index}`,
      disabled: !providerOf(star.provider)?.available,
      icon: star.provider === props.provider && star.account_id === (props.accountId || 0) &&
        star.model === props.model && (star.effort || "") === (props.effort || "")
        ? "i-lucide-check" : undefined,
    })),
  }] : []),
  ...pickerGroups.value.map((group) => groupCollapsed(group.id)
    /* A collapsed group is one row and no heading of its own: the row is the
       heading. It carries no `label`, and its `slot` names a group-label slot
       that does not exist, which is what keeps the palette from drawing an
       empty heading above it. */
    ? {
        ...group,
        label: "",
        slot: "collapsed",
        items: [{
          label: group.label,
          count: group.items.length,
          current: group.items.some((item) => item.value === currentValue.value),
          search: `${group.label} · ${group.provider.display_name} · ${group.provider.name}`,
          value: `expand:${group.id}`,
          icon: "i-lucide-chevron-right",
        }],
      }
    : {
        ...group,
        items: group.items.map((item) => ({
          ...item,
          icon: item.value === currentValue.value ? "i-lucide-check" : undefined,
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
  close();
}

/* Escape, a click outside and a pick all land here, so the next opening
   starts on a clean search with every group back at its default. */
function close() {
  open.value = false;
}

watch(open, (isOpen) => {
  if (isOpen) return;
  search.value = "";
  expansion.value = new Map();
});

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
        :title="currentTitle"
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
          :ui="PALETTE_UI"
          :fuse="FUSE"
          @update:model-value="choose"
        >
          <!-- A collapsed group says how many models it holds, and whether the
               chosen one is among them. -->
          <template #collapsed-trailing="{ item }">
            <UIcon v-if="item.current" name="i-lucide-check" class="size-4 shrink-0" />
            <span v-else aria-hidden="true" class="text-xs text-dimmed">{{ item.count }}</span>
          </template>

          <!-- Collapsed groups name a `collapsed` slot that is deliberately
               absent, so only expanded groups and Favourites draw a heading. -->
          <template #group-label="{ group, label }">
            <button
              v-if="group.id !== 'favourites'"
              type="button"
              class="flex w-full items-center gap-1 rounded px-1 py-0.5 text-left font-semibold text-highlighted hover:bg-elevated max-md:py-2"
              aria-expanded="true"
              :title="`Collapse ${label}`"
              @click.stop="toggleGroup(group.id)"
            >
              <UIcon name="i-lucide-chevron-down" class="size-3.5 shrink-0" />
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
      :ui="{ leadingIcon: starred ? 'fill-current' : '' }"
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
