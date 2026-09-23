<script setup>
import { computed, ref, watch } from "vue";
import { S, currentProjectID, openFileForEdit } from "../store";
import { searchFiles } from "../file-search";
import { segments } from "../fuzzy";

const open = defineModel("open", { type: Boolean, default: false });
const query = ref("");
watch(currentProjectID, () => (query.value = ""));
const groups = computed(() => [{
  id: "files",
  label: "Project files",
  ignoreFilter: true,
  items: searchFiles(S.tree, query.value).map(({ path, hits }) => ({
    label: path,
    hits,
    icon: "i-lucide-file",
    onSelect: () => {
      open.value = false;
      openFileForEdit(path);
    },
  })),
}]);
</script>

<template>
  <UModal v-model:open="open" title="Go to file" :ui="{ content: 'max-w-xl' }">
    <template #content>
      <UCommandPalette
        v-model:search-term="query"
        :groups="groups"
        :fuse="{ resultLimit: 100 }"
        placeholder="Go to file…"
        close
        autofocus
        @update:open="open = $event"
      >
        <template #empty>{{ currentProjectID() ? (S.treeTruncated ? 'No files found in the part of this folder that is listed.' : 'No files found.') : 'Select a project to search its files.' }}</template>
        <template #item-label="{ item }">
          <span><span
            v-for="(seg, i) in segments(item.label, item.hits)"
            :key="i"
            :class="seg.hit ? 'font-semibold' : ''"
          >{{ seg.text }}</span></span>
        </template>
      </UCommandPalette>
    </template>
  </UModal>
</template>
