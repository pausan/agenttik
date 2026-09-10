<script setup>
/* One schedule in a list. Pause and Resume sit here because that is the
   control a schedule needs most, and the archive icon only appears once it is
   paused: a hidden schedule that kept starting sessions is the one state
   nothing here would explain. */
import { nextTick, ref } from "vue";

const props = defineProps({
  schedule: { type: Object, required: true },
  sub: { type: String, default: "" },
  active: Boolean,
  archived: Boolean,
});

const emit = defineEmits(["select", "toggle-paused", "toggle-archive", "rename", "editing"]);

const editing = ref(false);
const draft = ref("");
const field = ref(null);

function setEditing(on) {
  editing.value = on;
  emit("editing", on);
}

function edit() {
  draft.value = props.schedule.title;
  setEditing(true);
  nextTick(() => field.value?.inputRef?.select());
}

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
      placeholder="Untitled schedule"
      class="min-w-0 flex-1 cursor-text"
      @blur="commit"
      @keydown.enter.prevent="commit"
      @keydown.esc.prevent="setEditing(false)"
    />
    <template v-else>
      <button type="button" class="min-w-0 flex-1 px-2 py-1 text-left" @click="$emit('select')">
        <span
          class="flex items-center gap-2 overflow-hidden"
          :class="active ? 'text-primary' : 'text-highlighted'"
        >
          <UIcon
            :name="schedule.paused ? 'i-lucide-pause' : 'i-lucide-repeat'"
            class="size-3.5 block shrink-0"
            :class="schedule.paused ? 'text-dimmed' : 'text-primary'"
          />
          <span class="truncate">{{ schedule.title || "Untitled schedule" }}</span>
          <span class="shrink-0 font-mono text-[10px] text-dimmed tabular-nums">
            {{ schedule.remaining < 0 ? "∞" : schedule.remaining }}
          </span>
        </span>
        <span v-if="sub" class="block truncate text-xs text-dimmed">{{ sub }}</span>
      </button>
      <button
        type="button"
        class="mr-1 shrink-0 rounded p-1 text-dimmed hover:text-primary"
        title="Rename schedule"
        @click.stop="edit"
      >
        <UIcon name="i-lucide-pencil" class="size-3.5 block" />
      </button>
      <button
        type="button"
        class="mr-1 shrink-0 rounded p-1 text-dimmed hover:text-primary"
        :title="schedule.paused ? 'Resume schedule' : 'Pause schedule'"
        :aria-label="schedule.paused ? 'Resume schedule' : 'Pause schedule'"
        @click.stop="$emit('toggle-paused')"
      >
        <UIcon :name="schedule.paused ? 'i-lucide-play' : 'i-lucide-pause'" class="size-3.5 block" />
      </button>
      <button
        v-if="schedule.paused || archived"
        type="button"
        class="mr-1 shrink-0 rounded p-1 text-dimmed hover:text-primary"
        :class="archived ? 'text-primary' : ''"
        :aria-pressed="archived"
        :title="archived ? 'Unarchive schedule' : 'Archive schedule'"
        @click.stop="$emit('toggle-archive')"
      >
        <UIcon
          :name="archived ? 'i-lucide-archive-restore' : 'i-lucide-archive'"
          class="size-3.5 block"
        />
      </button>
    </template>
  </div>
</template>
