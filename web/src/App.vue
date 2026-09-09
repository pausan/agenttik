<script setup>
import { onMounted, ref } from "vue";
import { useToast } from "@nuxt/ui/composables";

import { init, useErrors } from "./store";
import SideBar from "./components/SideBar.vue";
import MainPanel from "./components/MainPanel.vue";
import InspectorPanel from "./components/InspectorPanel.vue";
import AddProjectModal from "./components/AddProjectModal.vue";
import SetupModal from "./components/SetupModal.vue";

const addProject = ref(false);
const setup = ref(false);

useErrors(useToast());
onMounted(init);
</script>

<template>
  <UApp>
    <div class="grid h-full grid-cols-[272px_minmax(0,1fr)_312px] bg-default text-default text-sm">
      <SideBar @add-project="addProject = true" @setup="setup = true" />
      <MainPanel />
      <InspectorPanel />
    </div>

    <AddProjectModal v-model:open="addProject" />
    <SetupModal v-model:open="setup" />
  </UApp>
</template>
