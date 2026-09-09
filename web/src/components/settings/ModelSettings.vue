<script setup>
/* Star the model and effort combinations worth keeping at the top of the
   prompt bar's picker. */
import { computed, watchEffect } from "vue";

import { S, fail, isStarred, toggleStar } from "../../store";
import { fuzzyAny } from "../../fuzzy";
import StatusDot from "../StatusDot.vue";

const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count"]);

/* A provider the filter names keeps all its models; otherwise the models
   themselves are matched. */
const providers = computed(() =>
  S.providers.flatMap((p) => {
    if (fuzzyAny([p.display_name, p.name, "models"], props.filter) !== null) return [p];
    const models = p.models.filter((m) => fuzzyAny([m.label, m.id], props.filter) !== null);
    return models.length ? [{ ...p, models }] : [];
  }),
);

watchEffect(() => emit("count", providers.value.length));

async function star(provider, model, effort) {
  try {
    await toggleStar(provider, model.id, effort);
  } catch (e) {
    fail(e);
  }
}
</script>

<template>
  <section v-if="providers.length">
    <div class="mb-0.5 font-semibold text-highlighted">Models</div>
    <p class="mb-3.5 text-xs text-dimmed">
      Star the model and effort combinations you use. Starred ones come first in the model picker.
      agenttik runs the CLIs already signed in on this machine — it never handles your subscription
      credentials.
    </p>

    <section
      v-for="p in providers"
      :key="p.name"
      class="not-first:mt-4 not-first:border-t not-first:border-default not-first:pt-3.5"
    >
      <div class="mb-0.5 flex items-center gap-2">
        <span class="font-semibold text-highlighted">{{ p.display_name }}</span>
        <UBadge :color="p.available ? 'primary' : 'neutral'" variant="soft" size="sm">
          <StatusDot :status="p.available ? 'ok' : 'idle'" />
          {{ p.available ? "ready" : "unavailable" }}
        </UBadge>
      </div>
      <p v-if="!p.available && p.reason" class="mb-2 text-xs text-dimmed">
        {{ p.reason }}
      </p>

      <div v-for="m in p.models" :key="m.id" class="flex items-center gap-2.5 py-1">
        <span class="w-24 shrink-0 text-[13px] text-highlighted">{{ m.label }}</span>
        <div class="flex flex-wrap gap-1">
          <UButton
            v-for="e in ['', ...(m.efforts || p.efforts)]"
            :key="e || 'default'"
            size="xs"
            :color="isStarred(p.name, m.id, e) ? 'primary' : 'neutral'"
            :variant="isStarred(p.name, m.id, e) ? 'soft' : 'subtle'"
            :title="`Star ${m.label} · ${e || 'default'}`"
            class="rounded-full"
            :label="(isStarred(p.name, m.id, e) ? '★ ' : '') + (e || 'default')"
            @click="star(p.name, m, e)"
          />
        </div>
      </div>
    </section>
  </section>
</template>
