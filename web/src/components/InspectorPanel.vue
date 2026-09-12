<script setup>
import { computed, defineAsyncComponent } from "vue";

import { S } from "../store";
import FileList from "./FileList.vue";
import { SEGMENTED } from "../ui";

/* Everything below is reached by a click or a chord, never by the first
   paint, so its code is fetched from its own chunk the moment it is first
   needed instead of being parsed on the way in. The chunks are built into
   the binary beside the main one, so this is still a read off the local
   server and never a network call. */
const LogsPane = defineAsyncComponent(() => import("./LogsPane.vue"));
const OptionsPane = defineAsyncComponent(() => import("./OptionsPane.vue"));

const LABELS = { changed: "Changed", options: "Options", logs: "Commits" };

const items = computed(() => S.inspector.panes.map((p) => ({ label: LABELS[p], value: p })));

const active = computed({
  get: () => S.inspector.active,
  set: (v) => (S.inspector.active = v),
});
</script>

<template>
  <aside class="flex min-h-0 flex-col bg-muted">
    <UTabs
      v-model="active"
      :items="items"
      :content="false"
      size="sm"
      class="mx-2.5 mt-3 mb-2 shrink-0"
      :ui="SEGMENTED"
    />
    <div class="min-h-0 flex-1 p-2.5" :class="active === 'logs' ? 'flex flex-col overflow-hidden' : 'overflow-auto'">
      <LogsPane v-if="active === 'logs'" />
      <OptionsPane v-else-if="active === 'options'" />
      <FileList v-else-if="active === 'changed'" :files="S.changed" empty="No edited files." />
    </div>
  </aside>
</template>
