<script setup>
/* Ctrl+P's fuzzy launcher keeps navigation and common actions in one place.
   Active tasks deliberately come from each project's open-task list:
   the Tasks pane is time-windowed and also includes archived sessions. */
import { computed } from "vue";

import {
  S,
  currentProjectID,
  openProject,
  openTask,
  providerOf,
  setModel,
  startCurrentTask,
} from "../store";

const open = defineModel("open", { type: Boolean, default: false });
const emit = defineEmits(["projects", "tree", "add-project", "settings", "shortcuts"]);

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
    icon: "i-lucide-chart-no-axes-combined",
    disabled: !S.detail,
    onSelect: () => choose(() => (S.inspector.active = "stats")),
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
    if (!provider || !model) return [];
    return [
      {
        label: `${provider.display_name} ${model.label} · ${star.effort || "default"}`,
        description: "Apply to the current task",
        icon: "i-lucide-star",
        disabled: !S.detail,
        onSelect: () => choose(() => setModel(star.provider, star.model, star.effort || "")),
      },
    ];
  }),
);

const actions = computed(() => [
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

const groups = computed(() => [
  { id: "navigation", label: "Go to", items: navigation.value },
  { id: "projects", label: "Projects", items: projectItems.value },
  { id: "tasks", label: "Active tasks", items: activeTasks.value },
  { id: "favourites", label: "Favourite models", items: favourites.value },
  { id: "actions", label: "Actions", items: actions.value },
]);
</script>

<template>
  <UModal v-model:open="open" title="Go to anywhere" :ui="{ content: 'max-w-xl' }">
    <template #content>
      <UCommandPalette
        :groups="groups"
        placeholder="Go to anywhere…"
        close
        autofocus
        @update:open="open = $event"
      />
    </template>
  </UModal>
</template>
