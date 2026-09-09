<script setup>
/* The one-glance state of a session: running pulses, idle is quiet. */
const props = defineProps({ status: { type: String, default: "idle" } });

const colors = {
  running: "bg-primary animate-pulse",
  error: "bg-error",
  ok: "bg-success",
  idle: "bg-accented",
};
</script>

<template>
  <span class="size-1.5 shrink-0 rounded-full" :class="colors[props.status] || colors.idle" />
</template>
