<script setup>
/* One session in a list: the sidebar's, or a project's. Its rename and archive
   controls are separate from the row button, so opening a conversation stays
   one click away.

   Renaming happens in the row: the title becomes a field, Enter or leaving it
   saves, Escape puts it back.

   A title is three to seven words the model chose, which is not always enough
   to tell two conversations apart, so hovering the row shows the prompt it was
   opened with. */
import { nextTick, ref } from "vue";

import StatusDot from "./StatusDot.vue";

const props = defineProps({
  title: { type: String, default: "" },
  // The prompt the session opened with, shown on hover. Empty draws no tooltip.
  prompt: { type: String, default: "" },
  status: { type: String, default: "idle" },
  sub: { type: String, default: "" },
  // null keeps the row out of a numbered list. 0 is a row in one that no
  // chord reaches, and holds the column so the titles still line up.
  number: { type: Number, default: null },
  active: Boolean,
  archived: Boolean,
  archive: Boolean,
  queued: { type: Number, default: 0 },
  stoppable: Boolean,
});

const emit = defineEmits(["select", "stop", "toggle-archive", "rename", "editing"]);

const editing = ref(false);
const draft = ref("");
const field = ref(null);

/* A row in a project list is a drag handle, and dragging inside a field there
   would move the row instead of selecting text, so the lists are told when
   the field is open and stop being draggable while it is. */
function setEditing(on) {
  editing.value = on;
  emit("editing", on);
}

function edit() {
  draft.value = props.title;
  setEditing(true);
  // Selected rather than just focused: a rename usually replaces the title.
  nextTick(() => field.value?.inputRef?.select());
}

/* Escape closes the field before the blur it causes reaches commit, so a
   cancelled rename is not saved on the way out. */
function commit() {
  if (!editing.value) return;
  setEditing(false);
  emit("rename", draft.value);
}
</script>

<template>
  <div
    class="mb-px flex items-center rounded-[var(--ui-radius)]"
    :class="active ? 'bg-primary/10' : 'hover:bg-elevated'"
  >
    <UInput
      v-if="editing"
      ref="field"
      v-model="draft"
      size="sm"
      placeholder="Untitled session"
      class="min-w-0 flex-1 cursor-text"
      @blur="commit"
      @keydown.enter.prevent="commit"
      @keydown.esc.prevent="setEditing(false)"
    />
    <template v-else>
      <UTooltip
        :disabled="!prompt"
        :delay-duration="400"
        :content="{ side: 'right', align: 'start' }"
        :ui="{ content: 'block h-auto max-w-[600px] py-1.5 text-left' }"
      >
        <button
          type="button"
          class="min-w-0 flex-1 px-2 py-1 text-left"
          @click="$emit('select')"
        >
          <span
            class="flex items-center gap-2 overflow-hidden"
            :class="active ? 'text-primary' : 'text-highlighted'"
          >
            <span
              v-if="number !== null"
              class="w-3 shrink-0 font-mono text-[10px] text-dimmed tabular-nums"
              aria-hidden="true"
              >{{ number || "" }}</span
            >
            <StatusDot :status="status" />
            <span v-if="queued" class="shrink-0" title="Queued prompt">🕒</span>
            <span class="truncate">{{ title || "Untitled session" }}</span>
          </span>
          <span v-if="sub" class="block truncate text-xs text-dimmed">{{ sub }}</span>
        </button>
        <!-- Five lines is as much as is worth reading in a hover; the clamp
             puts the ellipsis on the last one it kept. -->
        <template #content>
          <span class="line-clamp-5 break-words whitespace-pre-wrap">{{ prompt }}</span>
        </template>
      </UTooltip>
      <button
        type="button"
        class="mr-1 shrink-0 rounded p-1 text-dimmed hover:text-primary"
        title="Rename session"
        @click.stop="edit"
      >
        <UIcon name="i-lucide-pencil" class="size-3.5 block" />
      </button>
      <button
        v-if="stoppable"
        type="button"
        class="mr-1 shrink-0 rounded p-1 text-dimmed hover:text-primary"
        title="Stop session"
        aria-label="Stop session"
        @click.stop="$emit('stop')"
      >
        <UIcon name="i-lucide-square" class="size-3.5 block" />
      </button>
      <button
        v-if="archive"
        type="button"
        class="mr-1 shrink-0 rounded p-1 text-dimmed hover:text-primary"
        :class="archived ? 'text-primary' : ''"
        :aria-pressed="archived"
        :title="archived ? 'Unarchive session' : 'Archive session'"
        @click.stop="$emit('toggle-archive')"
      >
        <UIcon :name="archived ? 'i-lucide-archive-restore' : 'i-lucide-archive'" class="size-3.5 block" />
      </button>
    </template>
  </div>
</template>
