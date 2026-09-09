<script setup>
import { openFile } from "../store";

defineProps({
  files: { type: Array, required: true }, // [{ path, status? }]
  empty: { type: String, required: true },
});
</script>

<template>
  <div>
    <p v-if="!files.length" class="px-3 py-5 text-center text-dimmed">{{ empty }}</p>
    <button
      v-for="f in files"
      :key="f.path"
      type="button"
      class="flex w-full select-none items-center gap-1.5 rounded-[var(--ui-radius)] px-1.5 py-0.5 text-left font-mono text-xs hover:bg-elevated hover:text-highlighted"
      @click="openFile(f.path)"
      @dblclick="openFile(f.path, true)"
    >
      <span v-if="f.status" class="w-5 shrink-0 text-primary">{{ f.status }}</span>
      <span class="path-clip truncate"><span>{{ f.path }}</span></span>
    </button>
  </div>
</template>
