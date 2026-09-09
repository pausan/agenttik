<script setup>
defineProps({ content: { type: String, required: true } });
</script>

<template>
  <div class="min-h-0 flex-1 overflow-auto">
    <pre class="m-0 px-5 py-4 font-mono text-xs text-highlighted"><code>{{ content }}</code></pre>
  </div>
</template>
