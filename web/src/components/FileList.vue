<script setup>
import { computed, ref } from "vue";

import TreeActionModal from "./TreeActionModal.vue";
import { useBatches } from "../batches";
import { ROW_BUTTON } from "../ui";
import { S, currentProjectID, openFile } from "../store";

const emit = defineEmits(["show-in-tree"]);

const props = defineProps({
  files: { type: Array, required: true }, // [{ path, status? }]
  allowRevert: { type: Boolean, default: false },
  allowRename: { type: Boolean, default: false },
  side: { type: String, default: "" }, // which diff a click opens, see openFile
  disabled: { type: Boolean, default: false },
  compactEmpty: { type: Boolean, default: false },
  empty: { type: String, required: true },
});

// A build can change thousands of files; rows past the first batch wait for a click.
const { visible, more, showMore } = useBatches(() => props.files, {
  whole: 250,
  reset: () => `${currentProjectID()}\0${S.repository}`,
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
  ...(props.allowRename ? [{
    label: "Rename",
    icon: "i-lucide-pencil",
    disabled: props.disabled || !aimed.value || props.files.find((f) => f.path === aimed.value)?.status === "D",
    onSelect: () => { action.value = { kind: "rename", path: aimed.value, projectID: currentProjectID() }; },
  }] : []),
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
        <div v-for="f in visible" :key="f.path" :data-path="f.path" class="flex items-center">
          <button
            type="button"
            class="flex min-w-0 flex-1 select-none items-center gap-1.5 rounded-[var(--ui-radius)] px-1.5 py-0.5 text-left font-mono text-xs hover:bg-elevated hover:text-highlighted"
            :data-path="f.path"
            @click="openFile(f.path, false, 'diff', side)"
            @dblclick="openFile(f.path, true, 'diff', side)"
          >
            <span v-if="f.status" class="w-5 shrink-0 text-primary">{{ f.status }}</span>
            <span class="path-clip truncate"><span>{{ f.path }}</span></span>
          </button>
          <button v-if="allowRevert" type="button" :class="ROW_BUTTON" :aria-label="`Revert changes to ${f.path}`" :title="`Revert changes to ${f.path}`" :disabled="disabled" @click="revert(f.path)">
            <UIcon name="i-lucide-undo-2" class="size-4 shrink-0" />
          </button>
          <slot name="actions" :file="f" />
        </div>
        <button
          v-if="more"
          type="button"
          class="w-full rounded-[var(--ui-radius)] px-1.5 py-1 text-left text-xs text-primary hover:bg-elevated"
          @click="showMore"
        >… See {{ more }} more …</button>
      </div>
    </UContextMenu>
    <TreeActionModal v-model:action="action" />
  </div>
</template>
