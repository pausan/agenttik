<script setup>
/* One session in a list: the sidebar's, or a project's. Its archive control
   is separate from the row button, so opening a conversation stays one click
   away. */
import StatusDot from "./StatusDot.vue";

defineProps({
  title: { type: String, required: true },
  status: { type: String, default: "idle" },
  sub: { type: String, default: "" },
  active: Boolean,
  archived: Boolean,
  archive: Boolean,
});

defineEmits(["select", "toggle-archive"]);
</script>

<template>
  <div
    class="mb-px flex items-center rounded-[var(--ui-radius)]"
    :class="active ? 'bg-primary/10' : 'hover:bg-elevated'"
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
        <StatusDot :status="status" />
        <span class="truncate">{{ title }}</span>
      </span>
      <span v-if="sub" class="block truncate text-xs text-dimmed">{{ sub }}</span>
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
  </div>
</template>
