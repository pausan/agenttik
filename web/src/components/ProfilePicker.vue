<script setup>
import { computed, onMounted, onUnmounted } from "vue";
import { remoteID } from "../api";
import { allProfiles, hasProfileWork, isCurrentProfile, profiles, loadProfiles, switchProfile } from "../profiles";

import ProfileIcon from "./ProfileIcon.vue";
import StatusDot from "./StatusDot.vue";

function item(p, remote = "") {
  return { label: p.name, busy: p.busy, slot: "profile", remote, current: isCurrentProfile(p.id, remote), onSelect: () => switchProfile(p.id, remote) };
}
// With remote machines saved, each machine heads its own profiles.
const items = computed(() => {
  const local = profiles.items.map(p => item(p));
  if (!profiles.remotes.length) return local;
  return [
    [{ type: "label", label: profiles.name || "This computer" }, ...local],
    ...profiles.remotes.map(r => [{ type: "label", label: r.name }, ...r.profiles.map(p => item(p, r.id))]),
  ];
});
const current = computed(() => allProfiles().find(p => isCurrentProfile(p.id, p.remote)));
const name = computed(() => current.value?.name || "Profile");
const title = computed(() => `Profile: ${current.value?.machine ? `${current.value.machine} › ` : ""}${name.value}`);
const profileBusy = computed(hasProfileWork);
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
  <UDropdownMenu v-if="allProfiles().length > 1" :items="items" :content="{ side: 'top', align: 'end' }">
    <UButton color="neutral" variant="ghost" :label="name" class="max-w-28" :ui="{ label: 'truncate' }" :title="title" :aria-label="title">
      <template #leading>
        <ProfileIcon :remote="!!remoteID" />
      </template>
      <template #trailing>
        <StatusDot v-if="profileBusy" status="running" title="Profiles are working" aria-label="Profiles are working" />
      </template>
    </UButton>
    <template #profile-leading="{ item }">
      <UIcon v-if="item.current" name="i-lucide-check" class="size-5 shrink-0" />
      <ProfileIcon v-else :remote="!!item.remote" />
    </template>
    <template #profile-trailing="{ item }">
      <StatusDot v-if="item.busy" status="running" title="Working" aria-label="Working" />
    </template>
  </UDropdownMenu>
</template>
