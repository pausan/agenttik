<script setup>
/* The mode library: reusable prompts kept apart from any project. A project
   links to one from its Prompt page and a task picks one in the prompt bar
   before its first prompt; this is where they are written. One mode is shown
   at a time, picked from a select, since a library of long prompts stacked
   one under another is a page of scrolling. Editing reaches every project
   linked to the mode, for conversations yet to start. Deleting leaves those
   projects with none. See specs/101-modes.md. */
import { computed, ref, watch, watchEffect } from "vue";

import { S, createMode, deleteMode, notice, updateMode } from "../../store";
import { fuzzyAny } from "../../fuzzy";
import { overPromptLimit, promptLimitNotice, promptSize, promptSizeTone } from "../../prompt-size";

const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count"]);

const rows = computed(() =>
  fuzzyAny(["Modes", "mode", "prompt", "subagent"], props.filter) !== null
    ? S.modes
    : S.modes.filter((m) => fuzzyAny([m.title, m.body], props.filter) !== null),
);
watchEffect(() => emit("count", rows.value.length));

/* 0 is the form for a new mode. A filter or a mode going away moves the
   selection to the first mode still listed. S.settingsMode is a mode another
   page asked Settings to open on; it is taken once. */
const NEW = 0;
const asked = rows.value.find((m) => m.id === S.settingsMode);
S.settingsMode = 0;
const selected = ref(asked?.id || rows.value[0]?.id || NEW);
watch(rows, (list) => {
  if (selected.value !== NEW && !list.some((m) => m.id === selected.value)) selected.value = list[0]?.id || NEW;
});
const mode = computed(() => S.modes.find((m) => m.id === selected.value));
const choices = computed(() => rows.value.map((m) => ({ label: m.title, value: m.id })));

const users = computed(() => S.projects.filter((p) => mode.value && p.mode_id === mode.value.id).length);

/* Cancel goes back to the mode shown before, or the first if it is gone. */
let before = NEW;
function startNew() {
  before = selected.value;
  selected.value = NEW;
}
function cancelNew() {
  title.value = "";
  body.value = "";
  selected.value = rows.value.some((m) => m.id === before) ? before : rows.value[0]?.id || NEW;
}

const title = ref("");
const body = ref("");

/* The size under the field follows what is typed, not what is saved: the mode
   commits on blur, so the two differ while writing. */
const typed = ref(null);
watch(selected, () => (typed.value = null));
const bodySize = computed(() => {
  const text = typed.value ?? mode.value?.body ?? "";
  return promptSize(text, text.trim() === mode.value?.body);
});
async function add() {
  if (!title.value.trim() || !body.value.trim()) return;
  const created = await createMode(title.value, body.value);
  if (created) {
    title.value = "";
    body.value = "";
    selected.value = created.id;
  }
}

/* An empty field is put back rather than saved: a mode needs both. Text over
   the limit is left in the field instead, to shorten rather than retype; the
   size under it says it is not saved. */
function commit(field, event) {
  const value = event.target.value.trim();
  if (!value || value === mode.value[field]) {
    event.target.value = mode.value[field];
    return;
  }
  if (field === "body" && overPromptLimit(value)) {
    notice("Mode text too long to save", promptLimitNotice(value));
    return;
  }
  updateMode(mode.value, { [field]: value });
}

/* Delete asks for a second click on the same button; leaving it stands it down. */
const armed = ref(false);
watch(selected, () => (armed.value = false));
function remove() {
  if (!armed.value) {
    armed.value = true;
    return;
  }
  armed.value = false;
  deleteMode(mode.value);
}

const examples = "e.g. You review code and do not edit files. List problems by severity, each with the file and line.";
</script>

<template>
  <section v-if="rows.length || !filter">
    <!-- New mode is the library's action, so it sits by the section's title,
         apart from the row that acts on the mode shown. -->
    <div class="mb-0.5 flex items-center gap-2">
      <span class="flex-1 font-semibold text-highlighted">Modes</span>
      <UButton v-if="selected !== NEW" size="sm" color="neutral" variant="subtle" icon="i-lucide-plus"
        label="New mode" @click="startNew" />
    </div>
    <p class="mb-1.5 text-xs text-dimmed">
      A mode is a reusable prompt that sets how a task works: a role, rules to follow, context about
      the domain, a process, a style of answer — anything you would otherwise paste at the start. A
      project linked to one sends it ahead of its own prompt, and a new task can pick another before
      its first prompt. Claude Code can also launch a mode as a subagent when you ask for it by name.
    </p>
    <p class="mb-3.5 text-xs text-dimmed">
      A mode is only words for the agent: it does not change tool access or permissions. Edits reach
      new tasks; tasks already started keep what they were given.
    </p>

    <div v-if="mode" class="mb-3 flex flex-wrap items-center gap-2">
      <USelect
        :model-value="selected"
        :items="choices"
        size="sm"
        class="min-w-40 flex-1"
        placeholder="Choose a mode"
        aria-label="Mode to edit"
        @update:model-value="selected = $event"
      />
      <span class="text-xs text-dimmed">
        {{ users === 1 ? "Linked to 1 project" : `Linked to ${users} projects` }}
      </span>
      <UButton
        size="sm"
        :color="armed ? 'error' : 'neutral'"
        variant="subtle"
        icon="i-lucide-trash-2"
        :label="armed ? 'Click again to delete' : 'Delete'"
        :aria-label="`Delete ${mode.title}`"
        @click="remove"
        @blur="armed = false"
      />
    </div>

    <form v-if="selected === NEW" class="space-y-2" @submit.prevent="add">
      <UInput v-model="title" size="sm" class="w-full" placeholder="Title, e.g. Code review, TDD, Explain for newcomers"
        aria-label="New mode title" />
      <UTextarea v-model="body" :rows="10" autoresize :maxrows="24" class="w-full" :placeholder="examples"
        aria-label="New mode text" :ui="{ base: 'resize-y font-mono text-xs' }" />
      <div class="flex items-center gap-2">
        <UButton type="submit" size="sm" icon="i-lucide-plus" label="Add mode"
          :disabled="!title.trim() || !body.trim() || overPromptLimit(body)" />
        <UButton v-if="rows.length" size="sm" color="neutral" variant="ghost" label="Cancel" @click="cancelNew" />
        <span class="flex-1 text-right text-xs tabular-nums" :class="promptSizeTone(body)">{{ promptSize(body) }}</span>
      </div>
    </form>

    <!-- Keyed by mode, so switching never carries one mode's half-typed text
         into another's field. -->
    <div v-else-if="mode" :key="mode.id" class="space-y-2">
      <UInput :model-value="mode.title" size="sm" class="w-full" :aria-label="`Title of ${mode.title}`"
        @blur="commit('title', $event)" @keydown.enter="$event.target.blur()" />
      <UTextarea :model-value="mode.body" :rows="10" autoresize :maxrows="24" class="w-full" :aria-label="`Text of ${mode.title}`"
        :ui="{ base: 'resize-y font-mono text-xs' }" @update:model-value="typed = $event" @blur="commit('body', $event)" />
      <div class="flex flex-wrap items-center gap-2 text-xs text-dimmed">
        <p class="m-0 flex-1">Saved when you click away.</p>
        <span class="tabular-nums" :class="promptSizeTone(typed ?? mode.body)">{{ bodySize }}</span>
      </div>
    </div>
  </section>
</template>
