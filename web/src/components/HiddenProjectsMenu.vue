<script setup>
/* Hidden projects stay active in the database, but leave the main sidebar
   until the user restores them here. The command palette keeps this menu
   compact and gives it keyboard-friendly fuzzy autocomplete without needing a
   second large project list. */
import { computed, ref, watch } from "vue";

import { S, fail, loadHiddenProjects, setProjectHidden } from "../store";
import { fuzzy, segments } from "../fuzzy";

const open = ref(false);
const query = ref("");
const loading = ref(false);
const restoring = ref(false);

const items = computed(() => {
  const needle = query.value.trim();
  if (!needle) {
    return S.hiddenProjects.map((project) => ({
      label: project.name,
      value: project.id,
      project,
      hits: null,
    }));
  }

  const matches = [];
  for (const project of S.hiddenProjects) {
    const match = fuzzy(project.name, needle);
    if (match) matches.push({
      label: project.name,
      value: project.id,
      project,
      hits: match.hits,
      score: match.score,
    });
  }
  matches.sort((a, b) => a.score - b.score || a.label.length - b.label.length || a.label.localeCompare(b.label));
  return matches;
});

const groups = computed(() => [{
  id: "hidden-projects",
  label: "Hidden projects",
  ignoreFilter: true,
  items: items.value,
}]);

async function refresh() {
  loading.value = true;
  try {
    await loadHiddenProjects();
  } catch (e) {
    fail(e);
  } finally {
    loading.value = false;
  }
}

watch(open, (isOpen) => {
  if (isOpen) {
    query.value = "";
    refresh();
  } else {
    query.value = "";
  }
});

async function restore(id) {
  if (restoring.value) return;
  const item = items.value.find((candidate) => candidate.value === id);
  if (!item) return;
  await restoreProjects([item.project]);
}

async function restoreProjects(projects) {
  if (restoring.value) return;
  restoring.value = true;
  try {
    for (const project of [...projects]) {
      await setProjectHidden(project, false);
    }
  } finally {
    restoring.value = false;
  }
}
</script>

<template>
  <UPopover
    v-model:open="open"
    :content="{ side: 'top', sideOffset: 8, onFocusOutside: (event) => event.preventDefault() }"
  >
    <UButton
      color="neutral"
      variant="ghost"
      icon="i-lucide-eye-off"
      title="Hidden projects"
      aria-label="Hidden projects"
    />
    <template #content>
      <UCommandPalette
        v-model:search-term="query"
        class="w-72 max-w-[calc(100vw-2rem)]"
        :groups="groups"
        value-key="value"
        :loading="loading"
        :fuse="{ resultLimit: Math.max(S.hiddenProjects.length, 1) }"
        placeholder="Find hidden projects…"
        close
        preserve-group-order
        :ui="{ viewport: 'max-h-[min(20rem,50vh)]' }"
        @update:model-value="restore"
        @update:open="open = $event"
      >
        <template #item-label="{ item }">
          <span class="text-highlighted">
            <span
              v-for="(part, i) in segments(item.label, item.hits)"
              :key="i"
              :class="part.hit ? 'font-semibold text-primary' : ''"
            >{{ part.text }}</span>
          </span>
        </template>
        <template #empty="{ searchTerm }">
          {{ searchTerm ? `Nothing matches “${searchTerm}”.` : "No hidden projects." }}
        </template>
        <template #footer>
          <div class="p-2">
            <UButton
              color="neutral"
              variant="ghost"
              icon="i-lucide-eye"
              label="Restore all"
              :loading="restoring"
              :disabled="loading || restoring || !S.hiddenProjects.length"
              @click="restoreProjects(S.hiddenProjects)"
            />
          </div>
        </template>
      </UCommandPalette>
    </template>
  </UPopover>
</template>
