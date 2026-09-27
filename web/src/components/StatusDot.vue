<script setup>
/* The one-glance state of a session: running pulses, idle is quiet, and
   waiting is the amber in between — work accepted that a provider being away
   is holding back. See specs/045-provider-outage-retry.md. */
const props = defineProps({ status: { type: String, default: "idle" } });

const colors = {
  running: "bg-primary status-pulse",
  error: "bg-error",
  unread: "bg-warning",
  "unread-running": "bg-warning status-pulse",
  waiting: "bg-warning status-pulse status-waiting",
  ok: "bg-success",
  idle: "bg-accented",
};
</script>

<template>
  <span :title="status === 'unread-running' ? 'Unread completion · work running' : status === 'unread' ? 'Unread completion' : status === 'error' ? 'Task failed' : undefined" class="size-1.5 shrink-0 rounded-full" :class="colors[props.status] || colors.idle" />
</template>

<style scoped>
.status-pulse {
  animation: status-pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite;
}
.status-waiting { animation-duration: 2.5s; }
@keyframes status-pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.5; transform: scale(0.6); }
}
</style>
