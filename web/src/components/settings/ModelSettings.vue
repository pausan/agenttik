<script setup>
/* Visibility and order for the shared model picker. */
import { computed, ref, watchEffect } from "vue";

import {
  S,
  fail,
  isModelChoiceHidden,
  modelAccountLabel,
  modelPickerGroups,
  providerOf,
  reorderStars,
  setModelChoiceHidden,
  toggleStar,
} from "../../store";
import { fuzzyAny } from "../../fuzzy";
import StatusDot from "../StatusDot.vue";

const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count"]);

const allGroups = computed(() => modelPickerGroups({ includeHidden: true }));
const groups = computed(() => allGroups.value.flatMap((group) => {
  const fields = [group.label, group.provider.display_name, group.provider.name,
    group.account.alias, "models"];
  if (fuzzyAny(fields, props.filter) !== null) return [group];
  const items = group.items.filter((item) =>
    fuzzyAny([item.label, item.model.label, item.model.id], props.filter) !== null,
  );
  return items.length ? [{ ...group, items }] : [];
}));

const favouriteRows = computed(() => S.stars.flatMap((star) => {
  const provider = providerOf(star.provider);
  const model = provider?.models.find((candidate) => candidate.id === star.model);
  if (!provider || !model) return [];
  const effort = star.effort
    ? star.effort[0].toUpperCase() + star.effort.slice(1)
    : "Default";
  const label = [modelAccountLabel(star.provider, star.account_id), model.label, effort].join(" · ");
  return fuzzyAny([label, provider.display_name, provider.name, model.id, "favourites"], props.filter) !== null
    ? [{ star, provider, model, label }]
    : [];
}));

watchEffect(() => emit("count", groups.value.length + favouriteRows.value.length));

async function setHidden(group, model, hidden) {
  try {
    await setModelChoiceHidden(group.provider.name, group.account.id, model, hidden);
  } catch (error) {
    fail(error);
  }
}

const dragging = ref(null);
const dropTarget = ref(null);

function endDrag() {
  dragging.value = null;
  dropTarget.value = null;
}

function startDrag(event, star) {
  if (props.filter) return event.preventDefault();
  dragging.value = star;
  event.dataTransfer.effectAllowed = "move";
  event.dataTransfer.setData("text/plain", star.model);
}

function dragOver(event, star) {
  if (!dragging.value || props.filter) return;
  event.preventDefault();
  event.dataTransfer.dropEffect = "move";
  dropTarget.value = star;
}

function dropFavourite(star) {
  const from = S.stars.indexOf(dragging.value);
  endDrag();
  return moveFavourite(from, S.stars.indexOf(star));
}

async function moveFavourite(index, to) {
  if (props.filter || index < 0 || to < 0 || to >= S.stars.length || index === to) return;
  const next = [...S.stars];
  next.splice(to, 0, next.splice(index, 1)[0]);
  try {
    await reorderStars(next);
  } catch (error) {
    fail(error);
  }
}

async function removeFavourite(star) {
  try {
    await toggleStar(star.provider, star.account_id, star.model, star.effort || "");
  } catch (error) {
    fail(error);
  }
}
</script>

<template>
  <section v-if="groups.length || favouriteRows.length">
    <div class="mb-0.5 font-semibold text-highlighted">Models</div>
    <p class="mb-3.5 text-xs text-dimmed">
      Hidden subscriptions and models stay out of every model picker. Add favourites from the
      main prompt, then set their order here.
    </p>

    <section v-if="favouriteRows.length" role="group" aria-label="Favourite models" class="mb-4">
      <h3 class="mb-1 font-semibold text-highlighted">Favourites</h3>
      <div
        v-for="{ star, provider, label } in favouriteRows"
        :key="`${star.provider}:${star.account_id}:${star.model}:${star.effort}`"
        class="mr-3 flex min-w-0 items-center gap-1 rounded py-0.5"
        :class="{ 'bg-elevated': dropTarget === star && dragging !== star, 'opacity-50': dragging === star }"
        @dragover="dragOver($event, star)"
        @dragleave="dropTarget === star && (dropTarget = null)"
        @drop.prevent="dropFavourite(star)"
      >
        <UButton
          size="xs"
          color="neutral"
          variant="ghost"
          icon="i-lucide-grip-vertical"
          :draggable="!filter"
          :disabled="!!filter"
          :aria-label="`Reorder favourite ${label}`"
          title="Drag to reorder, or use Up and Down arrow keys"
          @dragstart="startDrag($event, star)"
          @dragend="endDrag"
          @keydown.up.prevent="moveFavourite(S.stars.indexOf(star), S.stars.indexOf(star) - 1)"
          @keydown.down.prevent="moveFavourite(S.stars.indexOf(star), S.stars.indexOf(star) + 1)"
        />
        <span class="min-w-0 flex-1 truncate text-[13px] text-highlighted" :title="`${label} · ${provider.display_name}`">
          {{ label }}
        </span>
        <UButton
          size="xs"
          color="neutral"
          variant="ghost"
          icon="i-lucide-x"
          :aria-label="`Remove favourite ${label}`"
          title="Remove favourite"
          @click="removeFavourite(star)"
        />
      </div>
    </section>

    <section
      v-for="group in groups"
      :key="group.id"
      role="group"
      :aria-label="`${group.label} models`"
      class="not-first:mt-4 not-first:border-t not-first:border-default not-first:pt-3.5"
      :class="isModelChoiceHidden(group.provider.name, group.account.id) ? 'opacity-60' : ''"
    >
      <div class="mb-1 flex items-center gap-2">
        <UButton
          size="xs"
          color="neutral"
          variant="ghost"
          :icon="isModelChoiceHidden(group.provider.name, group.account.id) ? 'i-lucide-eye-off' : 'i-lucide-eye'"
          :aria-label="`${isModelChoiceHidden(group.provider.name, group.account.id) ? 'Show' : 'Hide'} ${group.label}`"
          :title="`${isModelChoiceHidden(group.provider.name, group.account.id) ? 'Show' : 'Hide'} every model in ${group.label}`"
          @click="setHidden(group, '', !isModelChoiceHidden(group.provider.name, group.account.id))"
        />
        <span class="font-semibold text-highlighted">{{ group.label }}</span>
        <UBadge :color="group.provider.available ? 'primary' : 'neutral'" variant="soft" size="sm">
          <StatusDot :status="group.provider.available ? 'ok' : 'idle'" />
          {{ group.provider.available ? "ready" : "unavailable" }}
        </UBadge>
      </div>
      <p v-if="!group.provider.available && group.provider.reason" class="mb-2 text-xs text-dimmed">
        {{ group.provider.reason }}
      </p>

      <div v-for="item in group.items" :key="item.model.id" class="flex min-w-0 items-center gap-2 py-0.5">
        <UButton
          size="xs"
          color="neutral"
          variant="ghost"
          :icon="isModelChoiceHidden(group.provider.name, group.account.id, item.model.id) ? 'i-lucide-eye-off' : 'i-lucide-eye'"
          :disabled="isModelChoiceHidden(group.provider.name, group.account.id)"
          :aria-label="`${isModelChoiceHidden(group.provider.name, group.account.id, item.model.id) ? 'Show' : 'Hide'} ${group.account.alias} · ${group.provider.display_name} · ${item.model.label}`"
          :title="`${isModelChoiceHidden(group.provider.name, group.account.id, item.model.id) ? 'Show' : 'Hide'} this model`"
          @click="setHidden(group, item.model.id, !isModelChoiceHidden(group.provider.name, group.account.id, item.model.id))"
        />
        <span class="min-w-0 truncate text-[13px] text-highlighted">
          {{ group.account.alias }} · {{ group.provider.display_name }} · {{ item.model.label }}
        </span>
      </div>
    </section>
  </section>
</template>
