<script setup>
import { computed, onMounted, ref, watchEffect } from "vue";

import { PROMPT_CHORDS, S, copyText, enterDoes, fail, setEnterDoes, setFoldOthers, resetPreferences } from "../../store";
import { fuzzyAny } from "../../fuzzy";
import { fileSize } from "../../file-info.js";
import { api } from "../../api";
import { colorPreference } from "../../color-mode";
import Chord from "../Chord.vue";
import { smartSearch, setSmartSearch } from "../../smart-search.js";
import DesktopSettings from "./DesktopSettings.vue";
import SmartSearchProgress from "../SmartSearchProgress.vue";

const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count"]);

const confirmingReset = ref(false);
const resetting = ref(false);
const resetDone = ref(false);
const desktopVersion = ref(0);
const showReset = computed(() => fuzzyAny(["general", "reset", "defaults", "colors", "choices", "favourites", "shortcuts"], props.filter) !== null);
async function resetDefaults() {
  resetting.value = true;
  try {
    await resetPreferences();
    colorPreference.value = "auto";
    position.value = "top";
    desktopVersion.value++;
    confirmingReset.value = false;
    resetDone.value = true;
  } catch (e) { fail(e); }
  finally { resetting.value = false; }
}
const database = ref(null);
const copied = ref(false);
async function copyDatabasePath() {
  copied.value = await copyText(database.value.database_path);
}
const showDatabase = computed(() => fuzzyAny(["general", "database", "path", "size", "storage", "copy"], props.filter) !== null);

const desktopCount = ref(0);
const position = ref("top");
const positionReady = ref(false);
const savingPosition = ref(false);
onMounted(async () => {
  try {
    const config = await api("GET", "/api/general");
    position.value = config.new_item_position;
    database.value = config;
    positionReady.value = true;
  } catch (e) {
    fail(e);
  }
});
async function setPosition(value) {
  savingPosition.value = true;
  try {
    position.value = (await api("PUT", "/api/general", { new_item_position: value })).new_item_position;
  } catch (e) {
    fail(e);
  } finally {
    savingPosition.value = false;
  }
}
const positionRows = computed(() =>
  fuzzyAny(["general", "new", "tasks", "projects", "order", "top", "bottom", "insert"], props.filter) !== null
    ? [
        { value: "top", label: "At the top (default)", description: "New tasks and projects appear first" },
        { value: "bottom", label: "At the bottom", description: "New tasks and projects appear last" },
      ]
    : [],
);

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

const searchRows = computed(() => fuzzyAny(["general", "project", "tasks", "smart", "fuzzy", "search", "Bekko"], props.filter) !== null ? [
  { value: false, label: "Fuzzy Search (default)", description: "Match task titles as you type" },
  { value: true, label: "Smart Search", description: "Find related tasks by meaning, across languages" },
] : []);
watchEffect(() => emit("count", Number(showReset.value) + Number(showDatabase.value) + desktopCount.value + promptRows.value.length + foldRows.value.length + positionRows.value.length + searchRows.value.length));

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
  <DesktopSettings :key="desktopVersion" :filter="filter" @count="desktopCount = $event" />
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
  <section v-if="positionRows.length" :class="(promptRows.length || foldRows.length) && 'mt-5'">
    <div class="mb-0.5 font-semibold text-highlighted">New tasks and projects</div>
    <p class="mb-2 text-xs text-dimmed">
      Where new items are added. Existing items keep their order. Applies to all windows.
    </p>
    <URadioGroup
      :model-value="position"
      :items="positionRows"
      :disabled="!positionReady || savingPosition"
      size="sm"
      :ui="{ item: 'py-1' }"
      @update:model-value="setPosition"
    />
  </section>
  <section v-if="searchRows.length" class="mt-5">
    <div class="mb-0.5 font-semibold text-highlighted">Project task search</div>
    <p class="mb-2 text-xs text-dimmed">
      Smart Search downloads Bekko a8m (about 150 MB with its tokenizer), then indexes all tasks.
      Runs on the Agenttik host and shares the cached model and index across browsers. Only task search in project views changes;
      all other searches stay fuzzy.
    </p>
    <URadioGroup :model-value="smartSearch.enabled" :items="searchRows" size="sm" @update:model-value="setSmartSearch" />
    <SmartSearchProgress />
  </section>
  <section v-if="showDatabase" class="mt-5">
    <div class="mb-0.5 font-semibold text-highlighted">Database</div>
    <template v-if="database">
      <div class="text-xs text-dimmed">Path on the Agenttik host</div>
      <div class="mt-1 flex items-center gap-2">
        <output
          id="database-path"
          aria-label="Path on the Agenttik host"
          :title="database.database_path"
          class="min-w-0 flex-1 overflow-x-auto rounded-[var(--ui-radius)] bg-muted px-2 py-1.5 font-mono text-xs text-highlighted whitespace-nowrap"
        >{{ database.database_path }}</output>
        <UButton :icon="copied ? 'i-lucide-check' : 'i-lucide-copy'" color="neutral" variant="ghost" aria-label="Copy database path" @click="copyDatabasePath" />
      </div>
      <p class="mt-1 text-xs text-dimmed">Database file size: {{ fileSize(database.database_size) }}</p>
    </template>
    <p v-else class="text-xs text-dimmed">Database details unavailable.</p>
  </section>
  <section v-if="showReset" class="mt-5">
    <div class="mb-0.5 font-semibold text-highlighted">Reset defaults</div>
    <p class="mb-2 text-xs text-dimmed">Restore colors, choices, favourites, and shortcuts. You will stay signed in to all accounts.</p>
    <UButton color="warning" variant="soft" label="Reset defaults" @click="confirmingReset = true" />
    <p v-if="resetDone" role="status" class="mt-2 text-xs text-success">Defaults restored. Restart Agenttik to apply tray settings.</p>
  </section>
  <UModal v-model:open="confirmingReset" title="Reset defaults?" :dismissible="!resetting">
    <template #body>
      <p>Reset appearance, shortcuts, remembered choices, model favourites and visibility, item placement, and tray settings? This cannot be undone.</p>
      <p class="mt-2">Appearance and browser choices reset in this browser. Shared preferences reset for all windows. Accounts stay signed in. Server authentication, projects, tasks, open files, and drafts are preserved.</p>
    </template>
    <template #footer>
      <UButton color="neutral" variant="ghost" label="Cancel" :disabled="resetting" @click="confirmingReset = false" />
      <UButton color="warning" label="Reset defaults" :loading="resetting" @click="resetDefaults" />
    </template>
  </UModal>
</template>
