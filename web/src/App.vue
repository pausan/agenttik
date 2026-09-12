<script setup>
import { defineAsyncComponent, onMounted, onUnmounted, ref } from "vue";
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
// init() populates the sidebar, the open tabs and everything in them; a slow
// launch — a lot of restored tabs, a cold disk — would otherwise sit there
// looking empty rather than working. Delayed so a launch under a second,
// the common case, never flashes it.
const bootSpinner = ref(false);

/* Settings opens on the section that was asked for: the sidebar's keyboard
   button and the launcher's shortcut entry both land on Shortcuts, and
   everything else on General. */
function openSettings(id) {
  settingsSection.value = id;
  settings.value = true;
}

function openAddProject() {
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
  if (hit(e, "task.rename")) return run(e, () => sideBar.value?.editCurrent());
  if (hit(e, "tab.reopen")) return run(e, reopenClosedTab);
  if (hit(e, "goto")) return run(e, () => (goTo.value = true));
  if (hit(e, "panel.tree")) return run(e, () => sideBar.value?.showTree());
  if (hit(e, "panel.projects")) return run(e, () => sideBar.value?.showProjects());
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
});
onUnmounted(() => window.removeEventListener("keydown", onKey));
</script>

<template>
  <UApp>
    <div
      class="grid h-full bg-default text-default text-sm"
      :style="{
        gridTemplateColumns: hasInspector()
          ? `${S.layout.left}px 1px minmax(0,1fr) 1px ${S.layout.right}px`
          : `${S.layout.left}px 1px minmax(0,1fr)`,
      }"
    >
      <SideBar
        ref="sideBar"
        @add-project="openAddProject"
        @setup="openSettings('general')"
        @shortcuts="openSettings('shortcuts')"
      />
      <Splitter side="left" />
      <MainPanel />
      <template v-if="hasInspector()">
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
      @projects="sideBar?.showProjects()"
      @tree="sideBar?.showTree()"
      @add-project="openAddProject"
      @settings="openSettings('general')"
      @shortcuts="openSettings('shortcuts')"
    />
  </UApp>
</template>
