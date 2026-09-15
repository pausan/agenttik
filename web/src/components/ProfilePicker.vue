<script setup>
import { computed } from "vue";
import { profileID } from "../api";
import { profiles, switchProfile } from "../profiles";

const items = computed(() => profiles.items.map(p => ({
  label: p.name,
  icon: p.id === profileID ? "i-lucide-check" : "i-lucide-user-round",
  onSelect: () => switchProfile(p.id),
})));
const name = computed(() => profiles.items.find(p => p.id === profileID)?.name || "Profile");
</script>

<template>
  <span v-if="profiles.private" class="text-xs text-muted" title="Temporary instance; app data is removed on exit">Private</span>
  <UDropdownMenu v-if="profiles.items.length > 1" :items="items" :content="{ side: 'top', align: 'end' }">
    <UButton color="neutral" variant="ghost" icon="i-lucide-user-round" :label="name" class="max-w-28" :ui="{ label: 'truncate' }" :title="`Profile: ${name}`" :aria-label="`Profile: ${name}`" />
  </UDropdownMenu>
</template>
