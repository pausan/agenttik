<script setup>
import { computed, ref } from "vue";

import TreeActionModal from "./TreeActionModal.vue";
import { S, currentProjectID, openFile } from "../store";

const emit = defineEmits(["show-in-tree"]);

const props = defineProps({
  files: { type: Array, required: true }, // [{ path, status? }]
  allowRevert: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
  compactEmpty: { type: Boolean, default: false },
  empty: { type: String, required: true },
});

const action = ref(null);
const aimed = ref("");
function aim(e) {
  aimed.value = e.target.closest("[data-path]")?.dataset.path || "";
}

function revert(path) {
  action.value = { kind: "revert", path, projectID: currentProjectID(), repository: S.repository };
}

const menu = computed(() => [
  ...(props.allowRevert && S.changed.some((f) => f.path === aimed.value) ? [{
    label: "Revert changes…",
    icon: "i-lucide-undo-2",
    color: "error",
    disabled: props.disabled,
    onSelect: () => revert(aimed.value),
  }] : []),
  {
    label: "Show in tree",
    icon: "i-lucide-folder-tree",
    disabled: !aimed.value,
    onSelect: () => emit("show-in-tree", aimed.value),
  },
]);
</script>

<template>
  <div>
    <p v-if="!files.length" class="px-3 text-center text-dimmed" :class="compactEmpty ? 'py-3' : 'py-5'">{{ empty }}</p>
    <UContextMenu v-else :items="menu" :ui="{ content: 'w-56' }">
      <div @contextmenu="aim">
        <div v-for="f in files" :key="f.path" :data-path="f.path" class="flex items-center">
          <button
            type="button"
            class="flex min-w-0 flex-1 select-none items-center gap-1.5 rounded-[var(--ui-radius)] px-1.5 py-0.5 text-left font-mono text-xs hover:bg-elevated hover:text-highlighted"
            :data-path="f.path"
            @click="openFile(f.path)"
            @dblclick="openFile(f.path, true)"
          >
            <span v-if="f.status" class="w-5 shrink-0 text-primary">{{ f.status }}</span>
            <span class="path-clip truncate"><span>{{ f.path }}</span></span>
          </button>
          <UButton v-if="allowRevert" icon="i-lucide-undo-2" :aria-label="`Revert changes to ${f.path}`" :title="`Revert changes to ${f.path}`" size="xs" variant="ghost" color="neutral" :disabled="disabled" @click="revert(f.path)" />
          <slot name="actions" :file="f" />
        </div>
      </div>
    </UContextMenu>
    <TreeActionModal v-model:action="action" />
  </div>
</template>
