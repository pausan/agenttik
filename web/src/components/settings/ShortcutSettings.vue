<script setup>
/* Every shortcut the app answers to, and the recorder that changes one.

   The list is the same registry the handlers read, so a row and the chord it
   describes cannot drift apart. Families ("Alt+1…9") and the editing
   conventions the textareas answer to themselves are shown but not
   rebindable — see shortcuts.js. */
import { computed, onUnmounted, ref, watchEffect } from "vue";

import { S, bindKey, isDefaultKey, keyConflicts, resetAllKeys, resetKey } from "../../store";
import { ACTIONS, GROUPS, actionOf, chordOf, modifiersOf } from "../../shortcuts";
import { fuzzyAny } from "../../fuzzy";
import Chord from "../Chord.vue";

const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count"]);

const recording = ref(""); // the action the next keystroke is bound to
const held = ref([]); // the modifiers already down, so the row shows progress

const rows = computed(() =>
  ACTIONS.filter(
    (a) => fuzzyAny([a.what, a.group, "shortcuts", ...S.keys[a.id]], props.filter) !== null,
  ),
);

const groups = computed(() =>
  GROUPS.map((label) => ({ label, rows: rows.value.filter((a) => a.group === label) })).filter(
    (g) => g.rows.length,
  ),
);

watchEffect(() => emit("count", rows.value.length));

const conflicts = computed(() => keyConflicts());

/* A chord bound twice is worth saying out loud: which handler wins is
   whichever asks first, and that is not something to find out by accident. */
function clash(action) {
  for (const chord of S.keys[action.id]) {
    const ids = conflicts.value.get(chord);
    if (ids) return actionOf(ids.find((id) => id !== action.id)).what;
  }
  return "";
}

function record(action) {
  if (action.fixed) return;
  stop();
  recording.value = action.id;
  window.addEventListener("keydown", onRecord, true);
  window.addEventListener("keyup", onModifiers, true);
}

function stop() {
  recording.value = "";
  held.value = [];
  window.removeEventListener("keydown", onRecord, true);
  window.removeEventListener("keyup", onModifiers, true);
}

/* The recorder takes the keystroke before anything else sees it — capture
   phase, propagation stopped — so binding Ctrl+P records instead of opening
   the launcher, and Escape cancels instead of closing the dialog. */
function onRecord(e) {
  e.preventDefault();
  e.stopPropagation();
  if (e.repeat) return;
  if (e.code === "Escape") return stop();
  const chord = chordOf(e);
  if (!chord) {
    held.value = modifiersOf(e);
    return;
  }
  bindKey(recording.value, chord);
  stop();
}

function onModifiers(e) {
  e.preventDefault();
  e.stopPropagation();
  held.value = modifiersOf(e);
}

onUnmounted(stop);
</script>

<template>
  <section v-if="groups.length">
    <div class="mb-0.5 font-semibold text-highlighted">Shortcuts</div>
    <p class="mb-2 text-xs text-dimmed">
      Click a chord and press the keys you want. Escape cancels. Greyed chords belong to the text
      boxes themselves and stay as they are.
    </p>

    <section
      v-for="g in groups"
      :key="g.label"
      class="mt-3 border-t border-default pt-2.5 first:mt-0 first:border-0 first:pt-0"
    >
      <h3 class="mb-1 font-semibold text-highlighted">{{ g.label }}</h3>
      <div v-for="a in g.rows" :key="a.id" class="flex items-center gap-2 py-0.5">
        <span class="min-w-0 flex-1 text-muted">
          {{ a.what }}
          <span v-if="clash(a)" class="text-xs text-warning">· also {{ clash(a) }}</span>
        </span>

        <component
          :is="a.fixed ? 'span' : 'button'"
          :type="a.fixed ? undefined : 'button'"
          class="flex shrink-0 items-center gap-1 rounded-[var(--ui-radius)] px-1.5 py-1"
          :class="
            a.fixed
              ? 'opacity-70'
              : recording === a.id
                ? 'ring-1 ring-primary'
                : 'cursor-pointer hover:bg-elevated'
          "
          :title="a.fixed ? 'Not rebindable' : `Change: ${a.what}`"
          @click="record(a)"
        >
          <template v-if="recording === a.id">
            <Chord v-if="held.length" :chord="held.join('+')" />
            <span class="text-xs text-dimmed">press a key…</span>
          </template>
          <template v-for="(chord, i) in recording === a.id ? [] : S.keys[a.id]" :key="chord">
            <span v-if="i" class="text-dimmed">or</span>
            <Chord :chord="chord" />
          </template>
        </component>

        <UButton
          size="xs"
          color="neutral"
          variant="ghost"
          icon="i-lucide-rotate-ccw"
          :class="a.fixed || isDefaultKey(a.id) ? 'invisible' : ''"
          :title="`Restore the default for: ${a.what}`"
          :aria-label="`Restore the default for ${a.what}`"
          @click="resetKey(a.id)"
        />
      </div>
    </section>

    <UButton
      v-if="!filter"
      class="mt-4"
      size="xs"
      color="neutral"
      variant="subtle"
      label="Restore all defaults"
      @click="resetAllKeys()"
    />
  </section>
</template>
