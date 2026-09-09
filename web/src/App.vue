<script setup>
import { onMounted, onUnmounted, ref } from "vue";
import { useToast } from "@nuxt/ui/composables";

import { S, closeTab, init, selectTabAt, startCurrentSession, useErrors } from "./store";
import SideBar from "./components/SideBar.vue";
import MainPanel from "./components/MainPanel.vue";
import InspectorPanel from "./components/InspectorPanel.vue";
import AddProjectModal from "./components/AddProjectModal.vue";
import SetupModal from "./components/SetupModal.vue";
import GoToModal from "./components/GoToModal.vue";
import Splitter from "./components/Splitter.vue";

const addProject = ref(false);
const setup = ref(false);
const goTo = ref(false);
const sideBar = ref(null);

useErrors(useToast());

/* Ctrl+N, Ctrl+P, and Ctrl+W create, find, and close conversations; Alt+P/S/T move the left strip.
   Alt+1 … Alt+9 goes straight to a tab. The key is read from the physical
   code: on some layouts Alt and a digit produce a different character. */
function onKey(e) {
  if (e.ctrlKey && !e.altKey && !e.metaKey) {
    if (e.code === "KeyN") {
      e.preventDefault();
      startCurrentSession();
    } else if (e.code === "KeyP") {
      e.preventDefault();
      goTo.value = true;
    } else if (e.code === "KeyW") {
      // A file opened from a session still belongs to that conversation.
      // Always consume the chord so it never closes the browser tab.
      e.preventDefault();
      if (S.owner?.kind === "session") closeTab(S.owner.id);
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
  if (!digit) return;
  e.preventDefault();
  selectTabAt(Number(digit[1]));
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
      <SideBar ref="sideBar" @add-project="addProject = true" @setup="setup = true" />
      <Splitter side="left" />
      <MainPanel />
      <Splitter side="right" />
      <InspectorPanel />
    </div>

    <AddProjectModal v-model:open="addProject" />
    <SetupModal v-model:open="setup" />
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
