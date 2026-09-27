<script setup>
import { computed, onMounted, onUnmounted } from "vue";
import { profileID } from "../api";
import { profiles, loadProfiles, switchProfile } from "../profiles";

import StatusDot from "./StatusDot.vue";

const items = computed(() => profiles.items.map(p => ({
  label: p.name,
  busy: p.busy,
  icon: p.id === profileID ? "i-lucide-check" : "i-lucide-user-round",
  onSelect: () => switchProfile(p.id),
})));
const name = computed(() => profiles.items.find(p => p.id === profileID)?.name || "Profile");
const otherBusy = computed(() => profiles.items.some(p => p.id !== profileID && p.busy));
let timer;
let disposed = false;
async function refresh() {
  try {
    if (!document.hidden) await loadProfiles();
  } catch {
    // Keep the last known state while disconnected and retry on the next tick.
  } finally {
    if (!disposed) timer = setTimeout(refresh, 3000);
  }
}
onMounted(() => { timer = setTimeout(refresh, 3000); });
onUnmounted(() => { disposed = true; clearTimeout(timer); });
</script>

<template>
  <span v-if="profiles.private" class="text-xs text-muted" title="Temporary instance; app data is removed on exit">Private</span>
  <UDropdownMenu v-if="profiles.items.length > 1" :items="items" :content="{ side: 'top', align: 'end' }">
    <UButton color="neutral" variant="ghost" icon="i-lucide-user-round" :label="name" class="max-w-28" :ui="{ label: 'truncate' }" :title="`Profile: ${name}`" :aria-label="`Profile: ${name}`">
      <template #trailing>
        <StatusDot v-if="otherBusy" status="running" title="Other profiles are working" aria-label="Other profiles are working" />
      </template>
    </UButton>
    <template #item-trailing="{ item }">
      <StatusDot v-if="item.busy" status="running" title="Working" aria-label="Working" />
    </template>
  </UDropdownMenu>
</template>
