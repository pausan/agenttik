<script setup>
import { watch } from "vue";

import { S, fail, isStarred, loadProviders, toggleStar } from "../store";
import StatusDot from "./StatusDot.vue";

const open = defineModel("open", { type: Boolean, default: false });

/* Re-read the providers every time it opens, so installing a CLI shows up
   without a restart. */
watch(open, async (on) => {
  if (!on) return;
  try {
    await loadProviders();
  } catch (e) {
    fail(e);
    open.value = false;
  }
});

async function star(provider, model, effort) {
  try {
    await toggleStar(provider, model.id, effort);
  } catch (e) {
    fail(e);
  }
}
</script>

<template>
  <UModal v-model:open="open" title="Settings" :ui="{ content: 'max-w-2xl' }">
    <template #body>
      <p class="mb-3.5 text-xs text-dimmed">
        Star the model and effort combinations you use. Starred ones come first in the model
        picker. agenttik runs the CLIs already signed in on this machine — it never handles your
        subscription credentials.
      </p>

      <div class="max-h-[52vh] overflow-auto">
        <section
          v-for="p in S.providers"
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
          <p v-if="!p.available && p.reason" class="mb-2 text-xs text-dimmed">{{ p.reason }}</p>

          <div v-for="m in p.models" :key="m.id" class="flex items-center gap-2.5 py-1">
            <span class="w-24 shrink-0 text-[13px] text-highlighted">{{ m.label }}</span>
            <div class="flex flex-wrap gap-1">
              <UButton
                v-for="e in ['', ...p.efforts]"
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
      </div>
    </template>
    <template #footer>
      <div class="flex w-full justify-end">
        <UButton label="Done" @click="open = false" />
      </div>
    </template>
  </UModal>
</template>
