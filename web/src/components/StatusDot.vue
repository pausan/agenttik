<script setup>
/* The one-glance state of a session: running pulses, idle is quiet, and
   waiting is the amber in between — work accepted that a provider being away
   is holding back. See specs/045-provider-outage-retry.md. */
const props = defineProps({ status: { type: String, default: "idle" } });

const colors = {
  running: "bg-primary animate-pulse",
  error: "bg-error",
  waiting: "bg-warning animate-pulse [animation-duration:2.5s]",
  ok: "bg-success",
  idle: "bg-accented",
};
</script>

<template>
  <span class="size-1.5 shrink-0 rounded-full" :class="colors[props.status] || colors.idle" />
</template>
