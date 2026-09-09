<script setup>
/* General is what is neither a colour, a model nor a chord. For now that is
   one question: does the plain Enter send the prompt or queue it?

   Picking one writes both bindings rather than setting a flag beside them, so
   this pane and Shortcuts are the same fact shown twice — see store.js. A
   binding moved off the pair in Shortcuts leaves neither option ticked, which
   is the honest answer, and says where it went. */
import { computed, watchEffect } from "vue";

import { PROMPT_CHORDS, S, enterDoes, setEnterDoes } from "../../store";
import { fuzzyAny } from "../../fuzzy";
import Chord from "../Chord.vue";

const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count"]);

const CHOICES = [
  { value: "send", plain: "Send", modified: "Enqueue" },
  { value: "enqueue", plain: "Enqueue", modified: "Send" },
];

/* One row either way: the filter keeps the pair or drops it. */
const rows = computed(() =>
  fuzzyAny(
    ["Prompt", "general", "send", "enqueue", PROMPT_CHORDS.plain, PROMPT_CHORDS.modified],
    props.filter,
  ) !== null
    ? CHOICES
    : [],
);

watchEffect(() => emit("count", rows.value.length));

const chosen = computed({
  get: () => enterDoes(),
  set: (what) => setEnterDoes(what),
});
</script>

<template>
  <section v-if="rows.length">
    <div class="mb-0.5 font-semibold text-highlighted">Prompt</div>
    <p class="mb-2 text-xs text-dimmed">
      What the prompt box does when you press Enter. The other of the two takes
      <Chord :chord="PROMPT_CHORDS.modified" />, and Shortcuts can move either one elsewhere.
    </p>

    <URadioGroup v-model="chosen" :items="rows" size="sm" :ui="{ item: 'py-1' }">
      <template #label="{ item }">
        <span class="flex items-center gap-1.5">
          <Chord :chord="PROMPT_CHORDS.plain" />
          <span>{{ item.plain }}</span>
        </span>
      </template>
      <template #description="{ item }">
        <span class="flex items-center gap-1.5 text-xs">
          <Chord :chord="PROMPT_CHORDS.modified" />
          <span>{{ item.modified }}</span>
        </span>
      </template>
    </URadioGroup>

    <p v-if="chosen === 'custom'" class="mt-2 text-xs text-warning">
      Neither, for now: Shortcuts sends with
      <Chord :chord="S.keys['prompt.send'][0]" /> and enqueues with
      <Chord :chord="S.keys['prompt.enqueue'][0]" />. Pick one above to go back to the pair.
    </p>
  </section>
</template>
