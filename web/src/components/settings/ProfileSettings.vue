<script setup>
import { computed, ref, watchEffect } from "vue";
import { api, profileID } from "../../api";
import { profiles, loadProfiles, switchProfile } from "../../profiles";
import { fail } from "../../store";
import { fuzzyAny } from "../../fuzzy";

const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count"]);
const name = ref("");
const busy = ref(false);
const removing = ref(null);
const editing = ref(null);
const draft = ref("");
const show = computed(() => fuzzyAny(["profiles", "local", "add", "rename", "edit", "remove", "order", "reorder", ...profiles.items.map(p => p.name)], props.filter) !== null);
watchEffect(() => emit("count", show.value ? 1 : 0));

function startEdit(p) {
  editing.value = p.id;
  draft.value = p.name;
}
function cancelEdit() {
  editing.value = null;
  draft.value = "";
}
async function saveEdit(p) {
  if (!draft.value.trim() || busy.value) return;
  busy.value = true;
  try {
    await api("PATCH", `/api/profiles/${p.id}`, { name: draft.value });
    cancelEdit();
    await loadProfiles();
  } catch (e) { fail(e); }
  finally { busy.value = false; }
}

async function move(index, offset) {
  if (busy.value) return;
  const ids = profiles.items.map(p => p.id);
  const target = index + offset;
  if (target < 0 || target >= ids.length) return;
  [ids[index], ids[target]] = [ids[target], ids[index]];
  busy.value = true;
  try {
    await api("PUT", "/api/profiles/order", { ids });
    await loadProfiles();
  } catch (e) { fail(e); }
  finally { busy.value = false; }
}

async function add() {
  busy.value = true;
  try {
    await api("POST", "/api/profiles", { name: name.value });
    name.value = "";
    await loadProfiles();
  } catch (e) { fail(e); }
  finally { busy.value = false; }
}
async function remove() {
  busy.value = true;
  try {
    const id = removing.value.id;
    await api("DELETE", `/api/profiles/${id}`);
    removing.value = null;
    if (id === profileID) switchProfile("default");
    else await loadProfiles();
  } catch (e) { fail(e); }
  finally { busy.value = false; }
}
</script>

<template>
  <section v-if="show">
    <div class="mb-1 font-semibold text-highlighted">Local profiles</div>
    <p class="mb-4 text-xs text-dimmed">Each profile keeps its projects, tasks, settings, models, and favourites separately on this computer. Providers, subscriptions, and project files are shared. Switching profiles reloads the view; other profiles keep running.</p>
    <form class="mb-4 flex gap-2" @submit.prevent="add">
      <UInput v-model="name" placeholder="Profile name" aria-label="Profile name" maxlength="80" class="min-w-0 flex-1" />
      <UButton type="submit" label="Add profile" :disabled="!name.trim() || busy" :loading="busy" />
    </form>
    <div v-for="(p, index) in profiles.items" :key="p.id" class="flex items-center gap-2 border-t border-default py-2">
      <UButton color="neutral" variant="ghost" icon="i-lucide-arrow-up" :aria-label="`Move ${p.name} up`" :disabled="busy || index === 0" @click="move(index, -1)" />
      <UButton color="neutral" variant="ghost" icon="i-lucide-arrow-down" :aria-label="`Move ${p.name} down`" :disabled="busy || index === profiles.items.length - 1" @click="move(index, 1)" />
      <form v-if="editing === p.id" class="flex min-w-0 flex-1 items-center gap-2" @submit.prevent="saveEdit(p)">
        <UInput
          v-model="draft"
          :aria-label="`Rename ${p.name}`"
          maxlength="80"
          autofocus
          class="min-w-0 flex-1"
          @keydown.esc.prevent="cancelEdit"
        />
        <UButton type="submit" label="Save" :disabled="!draft.trim() || busy" :loading="busy" />
        <UButton type="button" color="neutral" variant="ghost" label="Cancel" :disabled="busy" @click="cancelEdit" />
      </form>
      <template v-else>
        <span class="min-w-0 flex-1 truncate">{{ p.name }}</span>
        <UButton
          color="neutral"
          variant="ghost"
          icon="i-lucide-pencil"
          :aria-label="`Rename ${p.name}`"
          :disabled="busy"
          @click="startEdit(p)"
        />
      </template>
      <span v-if="p.id === profileID" class="text-xs text-dimmed">Current</span>
      <UButton v-else color="neutral" variant="ghost" label="Switch" :aria-label="`Switch to ${p.name}`" @click="switchProfile(p.id)" />
      <UButton v-if="p.id !== 'default'" color="error" variant="ghost" icon="i-lucide-trash-2" :aria-label="`Remove ${p.name}`" :disabled="busy" @click="removing = p" />
    </div>
    <p class="mt-2 text-xs text-dimmed">The built-in profile cannot be removed.</p>
    <UModal :open="!!removing" title="Remove profile?" @update:open="v => { if (!v && !busy) removing = null; }">
      <template #body><p>Remove {{ removing?.name }} and all its tasks, history, and settings? Running tasks will stop. Project files stay on disk. This cannot be undone.</p></template>
      <template #footer>
        <UButton color="neutral" variant="ghost" label="Cancel" :disabled="busy" @click="removing = null" />
        <UButton color="error" label="Remove profile" :loading="busy" @click="remove" />
      </template>
    </UModal>
  </section>
</template>
