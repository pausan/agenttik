<script setup>
/* One session in a list: the sidebar's, or a project's.

   The checkmark is only offered where ticking off makes sense — the Sessions
   list and a project's own view — and is a button of its own so the row stays
   one click away from opening the conversation. */
import StatusDot from "./StatusDot.vue";

defineProps({
  title: { type: String, required: true },
  status: { type: String, default: "idle" },
  sub: { type: String, default: "" },
  active: Boolean,
  done: Boolean,
  tick: Boolean,
});

defineEmits(["select", "toggle-done"]);
</script>

<template>
  <div
    class="mb-px flex items-center rounded-[var(--ui-radius)]"
    :class="active ? 'bg-primary/10' : 'hover:bg-elevated'"
  >
    <button
      v-if="tick"
      type="button"
      class="ml-1 shrink-0 rounded p-0.5 text-dimmed hover:text-primary"
      :class="done ? 'text-primary' : ''"
      :aria-pressed="done"
      :title="done ? 'Mark as not done' : 'Mark as done'"
      @click.stop="$emit('toggle-done')"
    >
      <UIcon :name="done ? 'i-lucide-square-check' : 'i-lucide-square'" class="size-3.5 block" />
    </button>
    <button
      type="button"
      class="min-w-0 flex-1 px-2 py-1 text-left"
      :class="done ? 'opacity-55' : ''"
      @click="$emit('select')"
    >
      <span
        class="flex items-center gap-2 overflow-hidden"
        :class="active ? 'text-primary' : 'text-highlighted'"
      >
        <StatusDot :status="status" />
        <span class="truncate" :class="done ? 'line-through' : ''">{{ title }}</span>
      </span>
      <span v-if="sub" class="block truncate text-xs text-dimmed">{{ sub }}</span>
    </button>
  </div>
</template>
