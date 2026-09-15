<script setup>
import { computed, watchEffect } from "vue";
import { S } from "../../store";
import { fuzzyAny } from "../../fuzzy";
import license from "../../../../LICENSE?raw";

const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count"]);
const shown = computed(() => fuzzyAny(["About", "agenttik", "Pau Sánchez", "version", S.instanceInfo.version, "copyright", "license", "MIT"], props.filter) !== null);
watchEffect(() => emit("count", Number(shown.value)));
</script>

<template>
  <section v-if="shown">
    <h2 class="mb-1 font-semibold text-highlighted">About agenttik</h2>
    <p class="mb-3 text-sm text-muted">Version {{ S.instanceInfo.version || "Unavailable" }}</p>
    <p class="text-sm">Created by Pau Sánchez</p>
    <p class="mb-4 text-xs text-muted">Copyright © 2026 Pau Sánchez</p>
    <h3 class="mb-2 text-sm font-medium">MIT License</h3>
    <p class="whitespace-pre-wrap text-xs text-muted">{{ license }}</p>
  </section>
</template>
