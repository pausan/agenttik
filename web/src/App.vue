<script setup>
import { onMounted, onUnmounted, ref } from "vue";
import { useToast } from "@nuxt/ui/composables";

import {
  S,
  closeTab,
  hasInspector,
  hit,
  init,
  reopenClosedTab,
  selectAdjacentTab,
  saveActiveFile,
  selectProjectAt,
  selectTabAt,
  startCurrentSession,
  useErrors,
} from "./store";
import SideBar from "./components/SideBar.vue";
import MainPanel from "./components/MainPanel.vue";
import InspectorPanel from "./components/InspectorPanel.vue";
import AddProjectModal from "./components/AddProjectModal.vue";
import SettingsModal from "./components/SettingsModal.vue";
import GoToModal from "./components/GoToModal.vue";
import Splitter from "./components/Splitter.vue";
import UnsavedModal from "./components/UnsavedModal.vue";

const addProject = ref(false);
const settings = ref(false);
const settingsSection = ref("general");
const goTo = ref(false);
const sideBar = ref(null);

/* Settings opens on the section that was asked for: the sidebar's keyboard
   button and the launcher's shortcut entry both land on Shortcuts, and
   everything else on General. */
function openSettings(id) {
  settingsSection.value = id;
  settings.value = true;
}

useErrors(useToast());

/* The chords come from Settings, so this reads as a list of actions rather
   than of keys; shortcuts.js does the comparing. Alt+1 … Alt+9 and
   Alt+A … Alt+H are families rather than single chords and stay here. The key
   is read from the physical code: on some layouts Alt and a digit produce a
   different character. */
function onKey(e) {
  if (hit(e, "tab.prev")) return run(e, () => selectAdjacentTab(-1));
  if (hit(e, "tab.next")) return run(e, () => selectAdjacentTab(1));
  if (hit(e, "session.new")) return run(e, startCurrentSession);
  if (hit(e, "tab.reopen")) return run(e, reopenClosedTab);
  if (hit(e, "goto")) return run(e, () => (goTo.value = true));
  if (hit(e, "panel.tree")) return run(e, () => sideBar.value?.showTree());
  if (hit(e, "panel.sessions")) return run(e, () => sideBar.value?.showSessions());
  if (hit(e, "panel.projects")) return run(e, () => sideBar.value?.showProjects());
  if (hit(e, "file.save")) {
    // The editor handles its own save; this is the same chord with the caret
    // anywhere else on a file tab.
    if (S.tab?.kind === "file") run(e, saveActiveFile);
    return;
  }
  if (hit(e, "tab.close")) {
    // A file opened from a session still belongs to that conversation.
    // Always consume the chord so it never closes the browser tab.
    return run(e, () => {
      if (S.owner?.kind === "session") closeTab(S.owner.id);
    });
  }

  if (!e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
  const digit = /^(?:Digit|Numpad)([1-9])$/.exec(e.code);
  if (digit) return run(e, () => selectTabAt(Number(digit[1])));
  // A … H are the first eight projects, in sidebar order.
  const letter = /^Key([A-H])$/.exec(e.code);
  if (letter) run(e, () => selectProjectAt(letter[1].charCodeAt(0) - 65));
}

function run(e, action) {
  e.preventDefault();
  action();
}

onMounted(() => {
  init();
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
        @add-project="addProject = true"
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

    <UnsavedModal />
    <AddProjectModal v-model:open="addProject" />
    <SettingsModal v-model:open="settings" v-model:section="settingsSection" />
    <GoToModal
      v-model:open="goTo"
      @projects="sideBar?.showProjects()"
      @sessions="sideBar?.showSessions()"
      @tree="sideBar?.showTree()"
      @add-project="addProject = true"
      @settings="openSettings('general')"
      @shortcuts="openSettings('shortcuts')"
    />
  </UApp>
</template>
