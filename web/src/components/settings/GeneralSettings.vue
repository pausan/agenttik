<script setup>
/* General is what is neither a colour, a model nor a chord. For now that is
   two questions: does the plain Enter send the prompt or queue it, and does
   selecting a project in the sidebar fold the other projects away?

   Picking an Enter writes both bindings rather than setting a flag beside
   them, so this pane and Shortcuts are the same fact shown twice — see
   store.js. A binding moved off the pair in Shortcuts leaves neither option
   ticked, which is the honest answer, and says where it went. */
import { computed, watchEffect } from "vue";

import { PROMPT_CHORDS, S, enterDoes, setEnterDoes, setFoldOthers } from "../../store";
import { fuzzyAny } from "../../fuzzy";
import Chord from "../Chord.vue";

const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count"]);

const CHOICES = [
  { value: "send", plain: "Send", modified: "Enqueue" },
  { value: "enqueue", plain: "Enqueue", modified: "Send" },
];

const FOLDING = [
  {
    value: "keep",
    label: "Leave the others as they are",
    description: "Every project keeps the tasks it had open",
  },
  {
    value: "fold",
    label: "Fold the others away",
    description: "Only the selected project shows its tasks",
  },
];

/* One row either way: the filter keeps a pair or drops it. */
const promptRows = computed(() =>
  fuzzyAny(
    ["Prompt", "general", "send", "enqueue", PROMPT_CHORDS.plain, PROMPT_CHORDS.modified],
    props.filter,
  ) !== null
    ? CHOICES
    : [],
);

const foldRows = computed(() =>
  fuzzyAny(
    ["Sidebar", "general", "projects", "fold", "collapse", "expand", "selecting"],
    props.filter,
  ) !== null
    ? FOLDING
    : [],
);

watchEffect(() => emit("count", promptRows.value.length + foldRows.value.length));

const chosen = computed({
  get: () => enterDoes(),
  set: (what) => setEnterDoes(what),
});

const folding = computed({
  get: () => (S.foldOthers ? "fold" : "keep"),
  set: (what) => setFoldOthers(what === "fold"),
});
</script>

<template>
  <section v-if="promptRows.length">
    <div class="mb-0.5 font-semibold text-highlighted">Prompt</div>
    <p class="mb-2 text-xs text-dimmed">
      What the prompt box does when you press Enter. The other of the two takes
      <Chord :chord="PROMPT_CHORDS.modified" />, and Shortcuts can move either one elsewhere.
    </p>

    <URadioGroup v-model="chosen" :items="promptRows" size="sm" :ui="{ item: 'py-1' }">
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

  <section v-if="foldRows.length" :class="promptRows.length && 'mt-5'">
    <div class="mb-0.5 font-semibold text-highlighted">Sidebar</div>
    <p class="mb-2 text-xs text-dimmed">
      What selecting a project does to the rest of them. The chevron, and Alt with a project's
      letter, still fold the selected project away either way.
    </p>

    <URadioGroup v-model="folding" :items="foldRows" size="sm" :ui="{ item: 'py-1' }" />
  </section>
</template>
