<script setup>
/* Settings is a rail and a pane. The rail filters: one box fuzzy-matches
   every row in every section — a colour, a model, a shortcut — and each
   section reports how many it kept, so the counts say where the answer is
   before you click.

   Every pane stays mounted, which is what keeps those counts live while
   the filter changes. They are small enough that this costs nothing. */
import { computed, reactive, ref, watch } from "vue";

import { S, fail, loadActionModels, loadArchivedProjects, loadOrchestratorConfig, loadProviders, loadServerConfig, openProject } from "../store";
import { loadProfiles } from "../profiles";
import ProfileSettings from "./settings/ProfileSettings.vue";
import AppearanceSettings from "./settings/AppearanceSettings.vue";
import GeneralSettings from "./settings/GeneralSettings.vue";
import ModelSettings from "./settings/ModelSettings.vue";
import OrchestratorSettings from "./settings/OrchestratorSettings.vue";
import ProjectSettings from "./settings/ProjectSettings.vue";
import ServerSettings from "./settings/ServerSettings.vue";
import ShortcutSettings from "./settings/ShortcutSettings.vue";
import SubscriptionSettings from "./settings/SubscriptionSettings.vue";
import AboutSettings from "./settings/AboutSettings.vue";
import HelpSettings from "./settings/HelpSettings.vue";
import { isMobile } from "../ui";
import { SETTINGS_SECTIONS, visibleSettings } from "../settings-navigation";
import APIProviderSettings from "./settings/APIProviderSettings.vue";

const open = defineModel("open", { type: Boolean, default: false });
defineProps({ tourActive: Boolean });
const emit = defineEmits(["start-tour"]);
const section = defineModel("section", { type: String, default: "general" });

const filter = ref("");
const counts = reactive({});
const expanded = reactive({ general: true, providers: true });
const shown = computed(() => visibleSettings(filter.value, counts));
const matchingSections = computed(() => shown.value.flatMap((s) => [s, ...(s.children || [])]).filter((s) => !s.group && counts[s.id]));
function selectSection(node) {
  if (node.group || (filter.value && !counts[node.id])) {
    expanded[node.id] = true;
    section.value = node.children?.[0]?.id || section.value;
  } else section.value = node.id;
}
watch(section, (id) => {
  const parent = SETTINGS_SECTIONS.find((s) => s.children?.some((child) => child.id === id));
  if (parent) expanded[parent.id] = true;
}, { immediate: true });

/* A filter that empties the section in front moves to one that has answers,
   so typing never leaves the pane blank while a match sits one click away. */
watch([filter, counts], () => {
  if (!filter.value || counts[section.value]) return;
  if (matchingSections.value.length) section.value = matchingSections.value[0].id;
});

/* Re-read the providers every time it opens, so installing a CLI shows up
   without a restart, and the archived projects with them — this is the only
   place they are shown, so nothing else has to keep them current. A filter
   left behind from last time would hide most of what the dialog is for. */
watch(open, async (on) => {
  if (!on) return;
  filter.value = "";
  try {
    await Promise.all([loadActionModels(), loadProfiles(), loadProviders(), loadArchivedProjects(), loadOrchestratorConfig(), loadServerConfig()]);
  } catch (e) {
    fail(e);
    open.value = false;
  }
}, { immediate: true });

async function showOrchestrator() {
  open.value = false;
  await openProject(S.orchestrator.project_id);
}
</script>

<template>
  <UModal v-model:open="open" title="Settings" :modal="!tourActive" :overlay="!tourActive" :ui="{ content: 'max-w-3xl' }" :content="{ onOpenAutoFocus: (e) => { if (isMobile()) e.preventDefault(); }, onInteractOutside: (e) => { if (tourActive) e.preventDefault(); } }">
    <template #body>
      <div class="settings-layout flex h-[58vh] min-h-0 gap-3">
        <nav class="settings-nav flex w-52 shrink-0 flex-col gap-2 border-r border-default pr-3">
          <UInput
            v-model="filter"
            size="sm"
            icon="i-lucide-search"
            placeholder="Filter settings…"
            :autofocus="!isMobile()"
          />
          <ul class="settings-sections min-h-0 flex-1 space-y-0.5 overflow-auto" aria-label="Settings sections">
            <li v-for="s in shown" :key="s.id" :data-section="s.id">
              <div class="flex items-center">
                <button v-if="s.children?.length" type="button" class="shrink-0 rounded p-1 text-muted hover:bg-elevated"
                  :aria-label="`${expanded[s.id] || filter ? 'Collapse' : 'Expand'} ${s.label}`"
                  :aria-expanded="!!(expanded[s.id] || filter)" :disabled="!!filter" @click="expanded[s.id] = !expanded[s.id]">
                  <UIcon :name="expanded[s.id] || filter ? 'i-lucide-chevron-down' : 'i-lucide-chevron-right'" class="size-3.5" />
                </button>
                <span v-else class="w-5.5 shrink-0" />
                <button type="button" class="flex min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-[var(--ui-radius)] px-1.5 py-1.5 text-left"
                  :class="section === s.id ? 'bg-elevated text-highlighted' : 'text-muted hover:bg-elevated/60'"
                  :aria-current="section === s.id ? 'page' : undefined" @click="selectSection(s)">
                  <UIcon :name="s.icon" class="size-4 shrink-0" />
                  <span class="min-w-0 flex-1 truncate">{{ s.label }}</span>
                  <span v-if="filter" class="shrink-0 text-xs text-dimmed">{{ s.count }}</span>
                </button>
              </div>
              <ul v-if="s.children?.length && (expanded[s.id] || filter)" :aria-label="`${s.label} sections`" class="ml-2.5 border-l border-default pl-3">
                <li v-for="child in s.children" :key="child.id">
                  <button type="button" class="flex w-full cursor-pointer items-center gap-2 rounded-[var(--ui-radius)] px-2 py-1.5 text-left"
                    :class="section === child.id ? 'bg-elevated text-highlighted' : 'text-muted hover:bg-elevated/60'"
                    :aria-current="section === child.id ? 'page' : undefined" @click="section = child.id">
                    <UIcon :name="child.icon" class="size-4 shrink-0" />
                    <span class="min-w-0 flex-1 truncate">{{ child.label }}</span>
                    <span v-if="filter" class="shrink-0 text-xs text-dimmed">{{ counts[child.id] }}</span>
                  </button>
                </li>
              </ul>
            </li>
            <li v-if="!shown.length" class="px-2 py-1.5 text-xs text-dimmed">Nothing matches.</li>
          </ul>
        </nav>

        <div class="min-h-0 min-w-0 flex-1 overflow-auto pr-1">
          <div v-show="section === 'help'">
            <HelpSettings :filter="filter" @count="counts.help = $event" @start-tour="emit('start-tour')" />
          </div>
          <div v-show="section === 'about'">
            <AboutSettings :filter="filter" @count="counts.about = $event" />
          </div>
          <div v-show="section === 'general'">
            <GeneralSettings
              :filter="filter"
              @count="counts.general = $event"
            />
          </div>
          <div v-show="section === 'profiles'">
            <ProfileSettings :filter="filter" @count="counts.profiles = $event" />
          </div>
          <div v-show="section === 'projects'">
            <ProjectSettings
              :filter="filter"
              @count="counts.projects = $event"
            />
          </div>
          <div v-show="section === 'orchestrator'">
            <OrchestratorSettings
              :filter="filter"
              @count="counts.orchestrator = $event"
              @open-project="showOrchestrator"
            />
          </div>
          <div v-show="section === 'appearance'">
            <AppearanceSettings
              :filter="filter"
              @count="counts.appearance = $event"
            />
          </div>
          <div v-show="section === 'models'">
            <ModelSettings
              :filter="filter"
              @count="counts.models = $event"
            />
          </div>
          <div v-show="section === 'subscriptions'">
            <SubscriptionSettings
              :filter="filter"
              @count="counts.subscriptions = $event"
            />
          </div>
          <div v-show="section === 'api-providers'">
            <APIProviderSettings :filter="filter" @count="counts['api-providers'] = $event" />
          </div>
          <div v-show="section === 'server'">
            <ServerSettings
              :active="open && section === 'server'"
              :filter="filter"
              @count="counts.server = $event"
            />
          </div>
          <div v-show="section === 'shortcuts'">
            <ShortcutSettings
              :filter="filter"
              @count="counts.shortcuts = $event"
            />
          </div>
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
