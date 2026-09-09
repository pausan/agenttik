<script setup>
import StatusDot from "./StatusDot.vue";

defineProps({
  title: { type: String, required: true },
  status: { type: String, default: "idle" },
  sub: { type: String, default: "" },
  active: Boolean,
});
</script>

<template>
  <button
    type="button"
    class="mb-px block w-full rounded-[var(--ui-radius)] px-2 py-1 text-left"
    :class="active ? 'bg-primary/10' : 'hover:bg-elevated'"
  >
    <span class="flex items-center gap-2 overflow-hidden" :class="active ? 'text-primary' : 'text-highlighted'">
      <StatusDot :status="status" />
      <span class="truncate">{{ title }}</span>
    </span>
    <span v-if="sub" class="block truncate text-xs text-dimmed">{{ sub }}</span>
  </button>
</template>
