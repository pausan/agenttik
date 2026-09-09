<script setup>
import { watch } from "vue";

import { S, fail, isStarred, loadProviders, setColor, toggleStar } from "../store";
import { ACCENTS, NEUTRALS } from "../theme";
import StatusDot from "./StatusDot.vue";

const open = defineModel("open", { type: Boolean, default: false });

/* Two rows of the same control, so the accent and the grey are picked the
   same way. Both apply on click: the dialog has no Apply button because
   seeing the colour is the only way to choose it. */
const COLORS = [
  { key: "accent", label: "Accent", palettes: ACCENTS },
  { key: "neutral", label: "Grey", palettes: NEUTRALS },
];

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
      <div class="max-h-[52vh] overflow-auto">
        <section>
          <div class="mb-0.5 font-semibold text-highlighted">Colours</div>
          <p class="mb-2 text-xs text-dimmed">
            The accent every control is drawn in, and the grey behind it. Both take effect at
            once and are remembered per browser.
          </p>

          <!-- py-1.5 leaves the selected swatch's ring room inside the
               scrolling body, which would otherwise clip it. -->
          <div v-for="row in COLORS" :key="row.key" class="flex items-center gap-2.5 py-1.5">
            <span class="w-24 shrink-0 text-[13px] text-highlighted">{{ row.label }}</span>
            <div class="flex flex-wrap gap-2">
              <button
                v-for="c in row.palettes"
                :key="c.name"
                type="button"
                :title="c.name"
                :aria-label="c.name"
                :aria-pressed="S.colors[row.key] === c.name"
                class="size-5 rounded-full ring-offset-2 ring-offset-bg"
                :class="[c.swatch, S.colors[row.key] === c.name && 'ring-2 ring-inverted']"
                @click="setColor(row.key, c.name)"
              />
            </div>
          </div>
        </section>

        <section class="mt-4 border-t border-default pt-3.5">
          <div class="mb-0.5 font-semibold text-highlighted">Models</div>
          <p class="mb-3.5 text-xs text-dimmed">
            Star the model and effort combinations you use. Starred ones come first in the model
            picker. agenttik runs the CLIs already signed in on this machine — it never handles
            your subscription credentials.
          </p>

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
            <p v-if="!p.available && p.reason" class="mb-2 text-xs text-dimmed">
              {{ p.reason }}
            </p>

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
