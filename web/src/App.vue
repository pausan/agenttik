<script setup>
import { onMounted, onUnmounted, ref } from "vue";
import { useToast } from "@nuxt/ui/composables";

import {
  S,
  closeTab,
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
import SetupModal from "./components/SetupModal.vue";
import ShortcutsModal from "./components/ShortcutsModal.vue";
import GoToModal from "./components/GoToModal.vue";
import Splitter from "./components/Splitter.vue";
import UnsavedModal from "./components/UnsavedModal.vue";

const addProject = ref(false);
const setup = ref(false);
const shortcuts = ref(false);
const goTo = ref(false);
const sideBar = ref(null);

useErrors(useToast());

/* Ctrl+N/T, Ctrl+P, Ctrl+W, and Ctrl+Shift+T create, find, archive, and
   restore conversations; Ctrl+PageUp/PageDown walk the tab strip and Alt+P/S/T move the
   left one. Alt+1 … Alt+9 goes straight to a tab and Alt+A … Alt+H to a project.
   The key is read from the physical code: on some layouts Alt and a digit
   produce a different character. */
function onKey(e) {
  if (e.ctrlKey && !e.altKey && !e.metaKey) {
    if (e.code === "PageUp") {
      e.preventDefault();
      selectAdjacentTab(-1);
    } else if (e.code === "PageDown") {
      e.preventDefault();
      selectAdjacentTab(1);
    } else if (e.code === "KeyN") {
      e.preventDefault();
      startCurrentSession();
    } else if (e.code === "KeyP") {
      e.preventDefault();
      goTo.value = true;
    } else if (e.code === "KeyS") {
      // The editor handles its own Ctrl+S; this is the same chord with the
      // caret anywhere else on a file tab.
      if (S.tab?.kind === "file") {
        e.preventDefault();
        saveActiveFile();
      }
    } else if (e.code === "KeyW") {
      // A file opened from a session still belongs to that conversation.
      // Always consume the chord so it never closes the browser tab.
      e.preventDefault();
      if (S.owner?.kind === "session") closeTab(S.owner.id);
    } else if (e.code === "KeyT") {
      e.preventDefault();
      if (e.shiftKey) reopenClosedTab();
      else startCurrentSession();
    }
    return;
  }

  if (!e.altKey || e.ctrlKey || e.metaKey) return;
  if (e.code === "KeyT") {
    e.preventDefault();
    sideBar.value?.showTree();
    return;
  }
  if (e.code === "KeyS") {
    e.preventDefault();
    sideBar.value?.showSessions();
    return;
  }
  if (e.code === "KeyP") {
    e.preventDefault();
    sideBar.value?.showProjects();
    return;
  }
  const digit = /^(?:Digit|Numpad)([1-9])$/.exec(e.code);
  if (digit) {
    e.preventDefault();
    selectTabAt(Number(digit[1]));
    return;
  }
  // A … H are the first eight projects, in sidebar order.
  const letter = /^Key([A-H])$/.exec(e.code);
  if (!letter) return;
  e.preventDefault();
  selectProjectAt(letter[1].charCodeAt(0) - 65);
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
        gridTemplateColumns: `${S.layout.left}px 1px minmax(0,1fr) 1px ${S.layout.right}px`,
      }"
    >
      <SideBar
        ref="sideBar"
        @add-project="addProject = true"
        @setup="setup = true"
        @shortcuts="shortcuts = true"
      />
      <Splitter side="left" />
      <MainPanel />
      <Splitter side="right" />
      <InspectorPanel />
    </div>

    <UnsavedModal />
    <AddProjectModal v-model:open="addProject" />
    <SetupModal v-model:open="setup" />
    <ShortcutsModal v-model:open="shortcuts" />
    <GoToModal
      v-model:open="goTo"
      @projects="sideBar?.showProjects()"
      @sessions="sideBar?.showSessions()"
      @tree="sideBar?.showTree()"
      @add-project="addProject = true"
      @settings="setup = true"
    />
  </UApp>
</template>
