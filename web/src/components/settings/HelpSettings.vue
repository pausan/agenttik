<script setup>
import { computed, watchEffect } from "vue";
import { fuzzyAny } from "../../fuzzy";

const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count", "start-tour"]);
const shown = computed(() => fuzzyAny(["Help", "Quick Start Tour", "getting started", "walkthrough", "tutorial", "connect", "agent"], props.filter) !== null);
watchEffect(() => emit("count", Number(shown.value)));
</script>

<template>
  <section v-if="shown">
    <div class="mb-1 font-semibold text-highlighted">Quick Start Tour</div>
    <p class="mb-3 text-xs text-muted">Connect your first agent, clone a practice project, and build a Python calculator with queued tasks and local commits. Go at your own pace and close the tour anytime.</p>
    <UButton label="Start Quick Start Tour" icon="i-lucide-graduation-cap" @click="emit('start-tour')" />
  </section>
</template>
