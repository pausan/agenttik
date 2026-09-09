<script setup>
import { onMounted, onUnmounted, ref } from "vue";
import { useToast } from "@nuxt/ui/composables";

import { S, init, selectTabAt, useErrors } from "./store";
import SideBar from "./components/SideBar.vue";
import MainPanel from "./components/MainPanel.vue";
import InspectorPanel from "./components/InspectorPanel.vue";
import AddProjectModal from "./components/AddProjectModal.vue";
import SetupModal from "./components/SetupModal.vue";
import Splitter from "./components/Splitter.vue";

const addProject = ref(false);
const setup = ref(false);

useErrors(useToast());

/* Alt+1 … Alt+9 goes straight to a tab, so any conversation is one chord
   away however many are open. The key is read from the physical code: on
   some layouts Alt and a digit produce a different character. */
function onKey(e) {
  if (!e.altKey || e.ctrlKey || e.metaKey) return;
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
      <SideBar @add-project="addProject = true" @setup="setup = true" />
      <Splitter side="left" />
      <MainPanel />
      <Splitter side="right" />
      <InspectorPanel />
    </div>

    <AddProjectModal v-model:open="addProject" />
    <SetupModal v-model:open="setup" />
  </UApp>
</template>
