<script setup>
/* Settings is a rail and a pane. The rail filters: one box fuzzy-matches
   every row in every section — a colour, a model, a shortcut — and each
   section reports how many it kept, so the counts say where the answer is
   before you click.

   Every pane stays mounted, which is what keeps those counts live while
   the filter changes. They are small enough that this costs nothing. */
import { computed, reactive, ref, watch } from "vue";

import { fail, loadArchivedProjects, loadProviders, loadServerConfig } from "../store";
import AppearanceSettings from "./settings/AppearanceSettings.vue";
import GeneralSettings from "./settings/GeneralSettings.vue";
import ModelSettings from "./settings/ModelSettings.vue";
import ProjectSettings from "./settings/ProjectSettings.vue";
import ServerSettings from "./settings/ServerSettings.vue";
import ShortcutSettings from "./settings/ShortcutSettings.vue";

const open = defineModel("open", { type: Boolean, default: false });
const section = defineModel("section", { type: String, default: "general" });

const SECTIONS = [
  { id: "general", label: "General", icon: "i-lucide-settings" },
  { id: "projects", label: "Projects", icon: "i-lucide-archive" },
  { id: "appearance", label: "Appearance", icon: "i-lucide-palette" },
  { id: "models", label: "Models", icon: "i-lucide-sparkles" },
  { id: "server", label: "Server", icon: "i-lucide-server" },
  { id: "shortcuts", label: "Shortcuts", icon: "i-lucide-keyboard" },
];

const filter = ref("");
const counts = reactive({ general: 0, projects: 0, appearance: 0, models: 0, server: 0, shortcuts: 0 });

const shown = computed(() => (filter.value ? SECTIONS.filter((s) => counts[s.id]) : SECTIONS));

/* A filter that empties the section in front moves to one that has answers,
   so typing never leaves the pane blank while a match sits one click away. */
watch([filter, counts], () => {
  if (!filter.value || counts[section.value]) return;
  if (shown.value.length) section.value = shown.value[0].id;
});

/* Re-read the providers every time it opens, so installing a CLI shows up
   without a restart, and the archived projects with them — this is the only
   place they are shown, so nothing else has to keep them current. A filter
   left behind from last time would hide most of what the dialog is for. */
watch(open, async (on) => {
  if (!on) return;
  filter.value = "";
  try {
    await Promise.all([loadProviders(), loadArchivedProjects(), loadServerConfig()]);
  } catch (e) {
    fail(e);
    open.value = false;
  }
});
</script>

<template>
  <UModal v-model:open="open" title="Settings" :ui="{ content: 'max-w-3xl' }">
    <template #body>
      <div class="flex h-[58vh] min-h-0 gap-3">
        <nav class="flex w-44 shrink-0 flex-col gap-2 border-r border-default pr-3">
          <UInput
            v-model="filter"
            size="sm"
            icon="i-lucide-search"
            placeholder="Filter settings…"
            autofocus
          />
          <div class="min-h-0 flex-1 space-y-0.5 overflow-auto">
            <button
              v-for="s in shown"
              :key="s.id"
              type="button"
              class="flex w-full cursor-pointer items-center gap-2 rounded-[var(--ui-radius)] px-2 py-1.5 text-left"
              :class="
                section === s.id ? 'bg-elevated text-highlighted' : 'text-muted hover:bg-elevated/60'
              "
              @click="section = s.id"
            >
              <UIcon :name="s.icon" class="size-4 shrink-0" />
              <span class="min-w-0 flex-1 truncate">{{ s.label }}</span>
              <span v-if="filter" class="shrink-0 text-xs text-dimmed">{{ counts[s.id] }}</span>
            </button>
            <p v-if="!shown.length" class="px-2 py-1.5 text-xs text-dimmed">Nothing matches.</p>
          </div>
        </nav>

        <div class="min-h-0 flex-1 overflow-auto pr-1">
          <GeneralSettings
            v-show="section === 'general'"
            :filter="filter"
            @count="counts.general = $event"
          />
          <ProjectSettings
            v-show="section === 'projects'"
            :filter="filter"
            @count="counts.projects = $event"
          />
          <AppearanceSettings
            v-show="section === 'appearance'"
            :filter="filter"
            @count="counts.appearance = $event"
          />
          <ModelSettings
            v-show="section === 'models'"
            :filter="filter"
            @count="counts.models = $event"
          />
          <ServerSettings
            v-show="section === 'server'"
            :filter="filter"
            @count="counts.server = $event"
          />
          <ShortcutSettings
            v-show="section === 'shortcuts'"
            :filter="filter"
            @count="counts.shortcuts = $event"
          />
        </div>
      </div>
    </template>
    <template #footer>
      <div class="flex w-full justify-end">
        <UButton label="Done" @click="open = false" />
      </div>
    </template>
  </UModal>
</template>
