<script setup>
/* Ctrl+Shift+P's fuzzy launcher keeps navigation and common actions in one place.
   Active tasks deliberately come from each project's open-task list:
   the Tasks pane is time-windowed and also includes archived sessions. */
import { computed, ref, watch } from "vue";

import { fuzzy, fuzzyAny, segments } from "../fuzzy";

import {
  S,
  currentProjectID,
  isModelChoiceVisible,
  modelAccountLabel,
  openProject,
  openTask,
  providerOf,
  setModel,
  startCurrentTask,
} from "../store";

const open = defineModel("open", { type: Boolean, default: false });
const emit = defineEmits(["projects", "tree", "add-project", "settings", "shortcuts", "remote"]);

function choose(action) {
  open.value = false;
  action();
}

const navigation = computed(() => [
  {
    label: "Projects",
    icon: "i-lucide-folder-kanban",
    onSelect: () => choose(() => emit("projects")),
  },
  {
    label: "Tree",
    icon: "i-lucide-folder-tree",
    onSelect: () => choose(() => emit("tree")),
  },
  {
    label: "Changed",
    description: "Current task's changed files",
    icon: "i-lucide-file-diff",
    disabled: !S.detail,
    onSelect: () => choose(() => (S.inspector.active = "changed")),
  },
  {
    label: "Task stats",
    description: "Current task",
    icon: "i-lucide-chart-pie",
    disabled: !S.detail,
    onSelect: () => choose(() => (S.taskStatsOpen = true)),
  },
]);

const projectItems = computed(() =>
  S.projects.map((project) => ({
    label: project.name,
    description: project.path,
    icon: "i-lucide-folder",
    onSelect: () => choose(() => openProject(project.id)),
  })),
);

const activeTasks = computed(() =>
  S.projects.flatMap((project) =>
    project.recent_sessions.map((session) => ({
      label: session.title || "Untitled task",
      description: project.name,
      icon: "i-lucide-message-square",
      onSelect: () => choose(() => openTask(session.id)),
    })),
  ),
);

const favourites = computed(() =>
  S.stars.flatMap((star) => {
    const provider = providerOf(star.provider);
    const model = provider?.models.find((candidate) => candidate.id === star.model);
    if (!provider || !model || !isModelChoiceVisible(star.provider, star.account_id, star.model)) return [];
    return [
      {
        label: `${modelAccountLabel(star.provider, star.account_id)} · ${model.label} · ${star.effort || "Default"}`,
        description: "Apply to the current task",
        icon: "i-lucide-star",
        disabled: !S.detail,
        onSelect: () => choose(() => setModel(star.provider, star.model, star.effort || "", star.account_id || 0)),
      },
    ];
  }),
);

const actions = computed(() => [
  {
    label: "Connect to remote server",
    description: "Open an agenttik instance by host and port",
    icon: "i-lucide-network",
    onSelect: () => choose(() => emit("remote")),
  },
  {
    label: "New task",
    description: "In the current project",
    icon: "i-lucide-square-pen",
    disabled: !currentProjectID(),
    onSelect: () => choose(startCurrentTask),
  },
  {
    label: "Add project",
    icon: "i-lucide-folder-plus",
    onSelect: () => choose(() => emit("add-project")),
  },
  {
    label: "Settings",
    icon: "i-lucide-settings",
    onSelect: () => choose(() => emit("settings")),
  },
  {
    label: "Keyboard shortcuts",
    description: "Settings, and where to change them",
    icon: "i-lucide-keyboard",
    onSelect: () => choose(() => emit("shortcuts")),
  },
]);

/* The palette's own search is a fuse threshold: "gts" finds nothing. Ours is
   the subsequence match the tree and the folder picker already use, so every
   list in the app narrows the same way and shows which letters matched. */
const query = ref("");
watch(open, (on) => on || (query.value = ""));

/* The label is highlighted on its own, so letters that only matched the
   description do not light up arbitrary characters in the title. */
function match(items, needle) {
  if (!needle) return items;
  const out = [];
  for (const item of items) {
    const score = fuzzyAny([item.label, item.description], needle);
    if (score === null) continue;
    out.push({ ...item, hits: fuzzy(item.label, needle)?.hits || null, score });
  }
  out.sort((a, b) => a.score - b.score || a.label.length - b.label.length);
  return out;
}

const groups = computed(() =>
  [
    { id: "navigation", label: "Go to", items: navigation.value },
    { id: "projects", label: "Projects", items: projectItems.value },
    { id: "tasks", label: "Active tasks", items: activeTasks.value },
    { id: "favourites", label: "Favourite models", items: favourites.value },
    { id: "actions", label: "Actions", items: actions.value },
  ].map((group) => ({
    ...group,
    ignoreFilter: true, // the palette leaves these alone; match() already filtered
    items: match(group.items, query.value.trim()),
  })),
);
</script>

<template>
  <UModal v-model:open="open" title="Command Palette" :ui="{ content: 'max-w-xl' }">
    <template #content>
      <UCommandPalette
        v-model:search-term="query"
        :groups="groups"
        :fuse="{ resultLimit: 100 }"
        placeholder="Command Palette…"
        preserve-group-order
        close
        autofocus
        @update:open="open = $event"
      >
        <!-- One wrapper: the palette spaces the label's children apart, which
             would put a gap around every matched letter. -->
        <template #item-label="{ item }">
          <span class="text-highlighted"
            ><span
              v-for="(seg, i) in segments(item.label, item.hits)"
              :key="i"
              :class="seg.hit ? 'font-semibold' : ''"
              >{{ seg.text }}</span
            ></span
          >
        </template>
      </UCommandPalette>
    </template>
  </UModal>
</template>
