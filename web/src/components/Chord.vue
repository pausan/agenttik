<script setup>
/* One chord — "Ctrl+Shift+T" — drawn as the keys it is made of. */
defineProps({ chord: { type: String, required: true } });
</script>

<template>
  <span class="inline-flex shrink-0 items-center gap-0.5">
    <template v-for="(key, i) in chord.split('+')" :key="i">
      <span v-if="i" class="text-dimmed">+</span>
      <UKbd :value="key" />
    </template>
  </span>
</template>
