<script setup>
import { computed } from "vue";

import { S } from "../store";
import FileList from "./FileList.vue";
import StatsPane from "./StatsPane.vue";
import OptionsPane from "./OptionsPane.vue";
import { SEGMENTED } from "../ui";

const LABELS = { changed: "Changed", options: "Options", stats: "Stats" };

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
    <div class="min-h-0 flex-1 overflow-auto p-2.5">
      <StatsPane v-if="active === 'stats'" />
      <OptionsPane v-else-if="active === 'options'" />
      <FileList v-else-if="active === 'changed'" :files="S.changed" empty="No edited files." />
    </div>
  </aside>
</template>
