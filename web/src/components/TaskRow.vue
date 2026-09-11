<script setup>
/* One task in a list: the sidebar's, or a project's. Its rename and archive
   controls are separate from the row button, so opening a conversation stays
   one click away.

   Renaming happens in the row: the title becomes a field, Enter or leaving it
   saves, Escape puts it back.

   A title is three to seven words the model chose, which is not always enough
   to tell two tasks apart, so hovering the row shows its opening prompt. */
import { nextTick, ref } from "vue";

import StatusDot from "./StatusDot.vue";

const props = defineProps({
  title: { type: String, default: "" },
  // The prompt the task opened with, shown on hover. Empty draws no tooltip.
  prompt: { type: String, default: "" },
  status: { type: String, default: "idle" },
  sub: { type: String, default: "" },
  // What the task came to, written when it was archived. Empty draws nothing,
  // which is every task archived before there was a summary to write.
  // See specs/051-task-outcomes.md.
  outcome: { type: String, default: "" },
  // null keeps the row out of a numbered list. 0 is a row in one that no
  // chord reaches, and holds the column so the titles still line up.
  number: { type: Number, default: null },
  // The scheduled job that spawned this task, or 0 for one started by hand.
  // Forty runs of a job share a name, so the number is what says which job
  // this is a run of. See specs/028-scheduled-jobs.md.
  job: { type: Number, default: 0 },
  active: Boolean,
  archived: Boolean,
  archive: Boolean,
  // Only the project page's Tasks tab offers this — the sidebar row does
  // not pass it, so nothing there grows a destructive button unasked.
  deletable: Boolean,
  queued: { type: Number, default: 0 },
  stoppable: Boolean,
});

const emit = defineEmits(["select", "stop", "toggle-archive", "delete", "rename", "editing", "open-job"]);

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

// Let a parent start the rename from outside the row, e.g. the F2 shortcut
// aimed at whichever task is current.
defineExpose({ edit });
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
      placeholder="Untitled task"
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
          <!-- Grey says archived: on a list holding both, the colour is what
               separates a task still in play from one that is history. -->
          <span
            class="flex items-center gap-2 overflow-hidden"
            :class="active ? 'text-primary' : archived ? 'text-muted' : 'text-highlighted'"
          >
            <span
              v-if="number !== null"
              class="w-3 shrink-0 font-mono text-[10px] text-dimmed tabular-nums"
              aria-hidden="true"
              >{{ number || "" }}</span
            >
            <StatusDot :status="status" />
            <span v-if="queued" class="shrink-0" title="Queued prompt">🕒</span>
            <span class="truncate">{{ title || "Untitled task" }}</span>
          </span>
          <!-- The outcome reads in the same lighter voice the transcript
               gives thinking: it is the row's own note on itself, not part of
               the list's structure. Two lines at most — a summary is written
               to a sentence, and a row is not a paragraph. -->
          <span v-if="outcome" class="mt-0.5 block line-clamp-2 text-xs text-dimmed italic">{{ outcome }}</span>
          <span v-if="sub" class="block truncate text-xs text-dimmed">{{ sub }}</span>
        </button>
        <!-- Five lines is as much as is worth reading in a hover; the clamp
             puts the ellipsis on the last one it kept. -->
        <template #content>
          <div class="space-y-1">
            <div class="font-medium text-highlighted">{{ title || "Untitled task" }}</div>
            <div class="line-clamp-5 break-words whitespace-pre-wrap">{{ prompt }}</div>
          </div>
        </template>
      </UTooltip>
      <!-- The job this run came from. It is a control rather than a label
           because a finished run is archived, so its job's page is the only
           place its history is kept — and this is the way back to it. It sits
           in the action strip since a button inside the row's own button is
           not valid markup. -->
      <button
        v-if="job"
        type="button"
        class="mr-1 shrink-0 rounded px-1 py-0.5 font-mono text-[10px] text-dimmed tabular-nums hover:bg-elevated hover:text-primary"
        :title="`Run of scheduled job #${job} — open it`"
        @click.stop="$emit('open-job')"
      >
        #{{ job }}
      </button>
      <button
        type="button"
        class="mr-1 shrink-0 rounded p-1 text-dimmed hover:text-primary"
        title="Rename task"
        @click.stop="edit"
      >
        <UIcon name="i-lucide-pencil" class="size-3.5 block" />
      </button>
      <button
        v-if="stoppable"
        type="button"
        class="mr-1 shrink-0 rounded p-1 text-dimmed hover:text-primary"
        title="Stop task"
        aria-label="Stop task"
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
        :title="archived ? 'Unarchive task' : 'Archive task'"
        @click.stop="$emit('toggle-archive')"
      >
        <UIcon :name="archived ? 'i-lucide-archive-restore' : 'i-lucide-archive'" class="size-3.5 block" />
      </button>
      <button
        v-if="deletable"
        type="button"
        class="mr-1 shrink-0 rounded p-1 text-dimmed hover:text-error"
        title="Delete task"
        aria-label="Delete task"
        @click.stop="$emit('delete')"
      >
        <UIcon name="i-lucide-trash-2" class="size-3.5 block" />
      </button>
    </template>
  </div>
</template>
