<script setup>
/* Mode, accent and grey apply on click: the dialog has no Apply
   button because seeing the colour is the only way to choose it. */
import { computed, watchEffect } from "vue";

import { colorPreference, COLOR_MODES } from "../../color-mode";
import { S, setColor } from "../../store";
import { ACCENTS, NEUTRALS } from "../../theme";
import { fuzzyAny } from "../../fuzzy";

const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count"]);
const showMode = computed(() =>
  fuzzyAny(["color mode", "light", "dark", "night", "day", "system", "appearance", "theme"], props.filter) !== null,
);

const ROWS = [
  { key: "accent", label: "Accent", palettes: ACCENTS },
  { key: "neutral", label: "Grey", palettes: NEUTRALS },
];

/* A row the filter names keeps all its colours; otherwise the colours
   themselves are matched, so "teal" narrows the accents to one swatch. */
const rows = computed(() =>
  ROWS.flatMap((row) => {
    if (fuzzyAny([row.label, "colours", "appearance", "theme"], props.filter) !== null) return [row];
    const palettes = row.palettes.filter((c) => fuzzyAny([c.name], props.filter) !== null);
    return palettes.length ? [{ ...row, palettes }] : [];
  }),
);

watchEffect(() => emit("count", rows.value.length + Number(showMode.value)));
</script>

<template>
  <section v-if="showMode" class="mb-6">
    <label for="color-mode" class="mb-0.5 block font-semibold text-highlighted">Color mode</label>
    <p class="mb-2 text-xs text-dimmed">
      Choose Light, Dark, or System to follow your device’s appearance, including scheduled
      changes. Takes effect immediately and is remembered for this profile in this browser.
    </p>
    <USelectMenu v-model="colorPreference" :items="COLOR_MODES" value-key="value" :search-input="false" id="color-mode" aria-label="Color mode" class="w-40" />
  </section>
  <section v-if="rows.length">
    <div class="mb-0.5 font-semibold text-highlighted">Colours</div>
    <p class="mb-2 text-xs text-dimmed">
      The accent every control is drawn in, and the grey behind it. Both take effect at once and
      are remembered for this profile in this browser.
    </p>

    <!-- py-1.5 leaves the selected swatch's ring room inside the scrolling
         body, which would otherwise clip it. -->
    <div v-for="row in rows" :key="row.key" class="flex items-center gap-2.5 py-1.5">
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
</template>
