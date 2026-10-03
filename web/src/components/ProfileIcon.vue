<script setup>
// A remote machine's profile is the person with a small connection badge.
defineProps({ remote: { type: Boolean, default: false } });
</script>

<template>
  <span class="relative inline-flex shrink-0" :data-remote="remote || undefined">
    <UIcon name="i-lucide-user-round" class="size-5" />
    <UIcon v-if="remote" name="i-lucide-wifi" class="absolute -right-1 -bottom-0.5 size-3 rounded-full bg-default p-px text-primary" />
  </span>
</template>
