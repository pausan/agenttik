<script setup>
import { computed, defineAsyncComponent, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { useToast } from "@nuxt/ui/composables";

import {
  S,
  closeTabOnly,
  editLastQueued,
  hasInspector,
  hit,
  init,
  reopenClosedTab,
  selectAdjacentSidebarRow,
  selectAdjacentTab,
  saveActiveFile,
  selectProjectAt,
  selectTaskAt,
  startCurrentTask,
  useErrors,
} from "./store";
import SideBar from "./components/SideBar.vue";
import MainPanel from "./components/MainPanel.vue";
import InspectorPanel from "./components/InspectorPanel.vue";
import Splitter from "./components/Splitter.vue";
import { MOBILE_QUERY } from "./ui";

/* Everything below is reached by a click or a chord, never by the first
   paint, so its code is fetched from its own chunk the moment it is first
   needed instead of being parsed on the way in. The chunks are built into
   the binary beside the main one, so this is still a read off the local
   server and never a network call. */
const AddProjectModal = defineAsyncComponent(() => import("./components/AddProjectModal.vue"));
const SettingsModal = defineAsyncComponent(() => import("./components/SettingsModal.vue"));
const GoToModal = defineAsyncComponent(() => import("./components/GoToModal.vue"));
const UnsavedModal = defineAsyncComponent(() => import("./components/UnsavedModal.vue"));

const addProject = ref(false);
// The add-project dialog can keep a clone going after it is minimized, so it
// stays mounted after its first use. The other dialogs have no work to keep
// alive and mount only while open.
const addProjectLoaded = ref(false);
const settings = ref(false);
const settingsSection = ref("general");
const goTo = ref(false);
const sideBar = ref(null);
const mobileSideBar = ref(null);
const media = window.matchMedia(MOBILE_QUERY);
const mobile = ref(media.matches);
const projectsOpen = ref(false);
const workspaceOpen = ref(false);
const viewportHeight = ref();
const activeProject = computed(() => S.projects.find((p) => p.id === S.activeProjectID));
const drawerUI = {
  content: "mobile-drawer w-[calc(100%-2rem)] max-w-sm",
  header: "min-h-14 p-3",
  body: "flex min-h-0 flex-col overflow-hidden p-0 sm:p-0",
  close: "static ml-auto size-11 justify-center",
};

function closePanels() {
  projectsOpen.value = false;
  workspaceOpen.value = false;
}

// Visual viewport height also follows the software keyboard on Safari.
function resizeViewport() {
  const viewport = window.visualViewport;
  viewportHeight.value = mobile.value && viewport?.scale === 1 ? `${viewport.height}px` : undefined;
}

function changeLayout() {
  mobile.value = media.matches;
  closePanels();
  resizeViewport();
}

async function showSidebar(which) {
  if (mobile.value) {
    workspaceOpen.value = false;
    projectsOpen.value = true;
    await nextTick();
  }
  const panel = mobile.value ? mobileSideBar.value : sideBar.value;
  if (which === "tree") panel?.showTree();
  else if (which === "rename") panel?.editCurrent();
  else panel?.showProjects();
}

// File selections from Tree or Changed also return to the main view.
watch([() => S.activeTab, () => S.activeProjectID], closePanels);
// init() populates the sidebar, the open tabs and everything in them; a slow
// launch — a lot of restored tabs, a cold disk — would otherwise sit there
// looking empty rather than working. Delayed so a launch under a second,
// the common case, never flashes it.
const bootSpinner = ref(false);

/* Settings opens on the section that was asked for: the sidebar's keyboard
   button and the launcher's shortcut entry both land on Shortcuts, and
   everything else on General. */
function openSettings(id) {
  closePanels();
  settingsSection.value = id;
  settings.value = true;
}

function openAddProject() {
  closePanels();
  addProjectLoaded.value = true;
  addProject.value = true;
}

useErrors(useToast());

/* The chords come from Settings, so this reads as a list of actions rather
   than of keys; shortcuts.js does the comparing. Alt+1 … Alt+9 and
   Alt+A … Alt+H are families rather than single chords and stay here. The key
   is read from the physical code: on some layouts Alt and a digit produce a
   different character. */
function onKey(e) {
  if (hit(e, "task.prev")) return run(e, () => selectAdjacentSidebarRow(-1));
  if (hit(e, "task.next")) return run(e, () => selectAdjacentSidebarRow(1));
  if (hit(e, "tab.prev")) return run(e, () => selectAdjacentTab(-1));
  if (hit(e, "tab.next")) return run(e, () => selectAdjacentTab(1));
  if (hit(e, "task.new")) return run(e, startCurrentTask);
  if (hit(e, "task.rename")) return run(e, () => showSidebar("rename"));
  if (hit(e, "tab.reopen")) return run(e, reopenClosedTab);
  if (hit(e, "goto")) return run(e, () => (goTo.value = true));
  if (hit(e, "panel.tree")) return run(e, () => showSidebar("tree"));
  if (hit(e, "panel.projects")) return run(e, () => showSidebar("projects"));
  if (hit(e, "file.save")) {
    // The editor handles its own save; this is the same chord with the caret
    // anywhere else on a file tab.
    if (S.tab?.kind === "file") run(e, saveActiveFile);
    return;
  }
  if (hit(e, "prompt.editLast")) {
    // The queue belongs to the conversation in front, so the chord reaches it
    // from the prompt box and from the transcript alike — and is left alone
    // over a file, where Ctrl+E is the editor's to answer.
    if (S.detail && S.tab?.kind !== "file") run(e, editLastQueued);
    return;
  }
  if (hit(e, "tab.close")) {
    // Ctrl+W closes the tab in front, not the task that owns a file.
    // Always consume the chord so it never closes the browser tab, and do
    // not let a held key walk through the rest of the strip.
    e.preventDefault();
    if (!e.repeat) closeTabOnly(S.tab?.id);
    return;
  }

  if (!e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
  const digit = /^(?:Digit|Numpad)([1-9])$/.exec(e.code);
  if (digit) return run(e, () => selectTaskAt(Number(digit[1])));
  // A … H are the first eight projects, in sidebar order. The letter is
  // always consumed but acts once: pressing it on the project already showing
  // folds its tasks, and a held key would only flap them.
  const letter = /^Key([A-H])$/.exec(e.code);
  if (letter) {
    e.preventDefault();
    if (!e.repeat) selectProjectAt(letter[1].charCodeAt(0) - 65);
  }
}

function run(e, action) {
  e.preventDefault();
  action();
}

onMounted(() => {
  const showSpinner = setTimeout(() => (bootSpinner.value = true), 1000);
  init().finally(() => {
    clearTimeout(showSpinner);
    bootSpinner.value = false;
  });
  window.addEventListener("keydown", onKey);
  media.addEventListener("change", changeLayout);
  window.visualViewport?.addEventListener("resize", resizeViewport);
  resizeViewport();
});
onUnmounted(() => {
  window.removeEventListener("keydown", onKey);
  media.removeEventListener("change", changeLayout);
  window.visualViewport?.removeEventListener("resize", resizeViewport);
});
</script>

<template>
  <UApp>
    <div
      class="app-shell grid h-full bg-default text-default text-sm"
      :style="{
        '--mobile-height': viewportHeight,
        gridTemplateRows: mobile ? 'auto minmax(0,1fr)' : undefined,
        gridTemplateColumns: mobile ? 'minmax(0,1fr)' : hasInspector()
          ? `${S.layout.left}px 1px minmax(0,1fr) 1px ${S.layout.right}px`
          : `${S.layout.left}px 1px minmax(0,1fr)`,
      }"
    >
      <header v-if="mobile" class="mobile-header flex min-w-0 items-center gap-2 border-b border-default bg-muted p-2">
        <USlideover v-model:open="projectsOpen" side="left" title="Projects and tasks" :ui="drawerUI" :content="{ style: { height: viewportHeight } }">
          <UButton
            icon="i-lucide-menu"
            color="neutral"
            variant="ghost"
            class="size-11 shrink-0 justify-center"
            aria-label="Projects and tasks"
          />
          <template #body>
            <SideBar
              ref="mobileSideBar"
              class="mobile-sidebar flex-1"
              @navigate="closePanels"
              @add-project="openAddProject"
              @setup="openSettings('general')"
              @shortcuts="openSettings('shortcuts')"
            />
          </template>
        </USlideover>
        <div class="min-w-0 flex-1">
          <p class="text-[10px] text-dimmed">{{ activeProject ? 'Project' : 'Workspace' }}</p>
          <p class="truncate font-semibold text-highlighted">{{ activeProject?.name || 'agenttik' }}</p>
        </div>
        <UButton
          v-if="activeProject"
          icon="i-lucide-plus"
          label="New task"
          class="min-h-11 shrink-0"
          @click="startCurrentTask()"
        />
        <UButton v-else icon="i-lucide-folder-plus" label="Add project" class="min-h-11 shrink-0" @click="openAddProject" />
        <USlideover v-if="activeProject && hasInspector()" v-model:open="workspaceOpen" title="Workspace" :ui="drawerUI" :content="{ style: { height: viewportHeight } }">
          <UButton
            icon="i-lucide-panels-top-left"
            color="neutral"
            variant="ghost"
            class="size-11 shrink-0 justify-center"
            aria-label="Workspace"
          />
          <template #body>
            <InspectorPanel class="mobile-inspector flex-1" />
          </template>
        </USlideover>
      </header>
      <SideBar
        v-if="!mobile"
        ref="sideBar"
        @add-project="openAddProject"
        @setup="openSettings('general')"
        @shortcuts="openSettings('shortcuts')"
      />
      <Splitter v-if="!mobile" side="left" />
      <MainPanel />
      <template v-if="!mobile && hasInspector()">
        <Splitter side="right" />
        <InspectorPanel />
      </template>
    </div>

    <div
      v-if="bootSpinner"
      class="fixed inset-0 z-50 flex items-center justify-center bg-default"
      aria-label="Loading"
    >
      <UIcon name="i-lucide-loader-circle" class="size-6 animate-spin text-muted" />
    </div>

    <UnsavedModal v-if="S.closing" />
    <AddProjectModal v-if="addProjectLoaded" v-model:open="addProject" />
    <SettingsModal v-if="settings" v-model:open="settings" v-model:section="settingsSection" />
    <GoToModal
      v-if="goTo"
      v-model:open="goTo"
      @projects="showSidebar('projects')"
      @tree="showSidebar('tree')"
      @add-project="openAddProject"
      @settings="openSettings('general')"
      @shortcuts="openSettings('shortcuts')"
    />
  </UApp>
</template>
