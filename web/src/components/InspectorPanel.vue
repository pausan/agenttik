<script setup>
import { computed, defineAsyncComponent } from "vue";

import { S, selectRepository } from "../store";
import FileList from "./FileList.vue";
import { SEGMENTED } from "../ui";

const emit = defineEmits(["show-in-tree"]);

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
    <header class="mx-2.5 mt-3 shrink-0">
      <h2 class="m-0 px-0.5 text-sm font-semibold tracking-tight text-highlighted">Workspace</h2>
      <USelect
        v-if="S.repositories.length > 1"
        :model-value="S.repository"
        :items="S.repositories.map((path) => ({ label: path === '.' ? 'Project root' : path, value: path }))"
        size="sm"
        class="mt-2 w-full"
        aria-label="Git repository"
        @update:model-value="selectRepository"
      />
    </header>
    <UTabs
      v-model="active"
      :items="items"
      :content="false"
      size="sm"
      class="mx-2.5 mt-2 mb-2 shrink-0"
      :ui="SEGMENTED"
    />
    <div class="min-h-0 flex-1 p-2.5" :class="active === 'logs' ? 'flex flex-col overflow-hidden' : 'overflow-auto'">
      <LogsPane v-if="active === 'logs'" @show-in-tree="emit('show-in-tree', $event)" />
      <OptionsPane v-else-if="active === 'options'" />
      <FileList
        v-else-if="active === 'changed'"
        :files="S.changed"
        empty="No edited files."
        @show-in-tree="emit('show-in-tree', $event)"
      />
    </div>
  </aside>
</template>
