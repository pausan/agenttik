<script setup>
import { computed, onMounted, onUnmounted, ref } from "vue";
import { api, nf } from "../../api";
import { profiles } from "../../profiles";
import { fail } from "../../store";

const props = defineProps({ busy: Boolean });
const emit = defineEmits(["connect"]);
const scan = ref(null);
const port = ref(7717);
const busy = ref(false);
const error = ref("");
const percent = computed(() => scan.value?.total ? 100 * scan.value.probed / scan.value.total : 0);
const canContinue = computed(() => scan.value?.total && !scan.value.complete && Number(port.value) === scan.value.port);
let poll;
let revision = 0;
let disposed = false;

function saved(id) { return profiles.remotes.some(r => r.id === id); }
function follow() {
  clearTimeout(poll);
  if (scan.value?.running && !disposed) poll = setTimeout(read, 500);
}
async function read() {
  const current = revision;
  try {
    const state = await api("GET", "/api/remotes/discovery");
    if (disposed) {
      if (state.running) await api("DELETE", "/api/remotes/discovery");
      return;
    }
    if (current !== revision) return;
    scan.value = state;
    if (state.port) port.value = state.port;
    follow();
  } catch (e) {
    if (!disposed && current === revision) error.value = e.message;
  }
}
async function search(restart = false) {
  if (busy.value) return;
  revision++;
  clearTimeout(poll);
  busy.value = true;
  error.value = "";
  try {
    const state = await api("POST", "/api/remotes/discovery", { port: Number(port.value), restart });
    if (disposed) {
      await api("DELETE", "/api/remotes/discovery");
      return;
    }
    scan.value = state;
    follow();
  } catch (e) { if (!disposed) error.value = e.message; }
  finally { busy.value = false; }
}
async function pause() {
  revision++;
  clearTimeout(poll);
  busy.value = true;
  error.value = "";
  try {
    scan.value = await api("DELETE", "/api/remotes/discovery");
    return true;
  } catch (e) { error.value = e.message; return false; }
  finally { busy.value = false; }
}
async function connect(machine) {
  if (await pause() && !disposed) emit("connect", machine);
}
onMounted(read);
onUnmounted(() => {
  disposed = true;
  revision++;
  clearTimeout(poll);
  if (scan.value?.running || busy.value) api("DELETE", "/api/remotes/discovery").catch(fail);
});
</script>

<template>
  <div class="mb-4 space-y-2">
    <div class="flex flex-wrap items-center gap-2">
      <UInput v-model="port" type="number" :min="1" :max="65535" aria-label="Discovery port" placeholder="7717" class="w-24" :disabled="scan?.running || busy" />
      <UButton v-if="scan?.running" color="neutral" variant="outline" label="Pause search" :disabled="busy" @click="pause" />
      <UButton v-else-if="canContinue" color="neutral" variant="outline" label="Continue search" :disabled="busy || props.busy" @click="search()" />
      <UButton v-else-if="!scan?.total" color="neutral" variant="outline" label="Search for machines" icon="i-lucide-search" :disabled="busy || props.busy || !(port >= 1 && port <= 65535)" @click="search()" />
      <UButton v-if="scan?.total" color="neutral" variant="ghost" label="Restart search" :disabled="busy || props.busy || !(port >= 1 && port <= 65535)" @click="search(true)" />
    </div>
    <p class="text-xs text-dimmed">Search private networks on this port. Large networks can take hours. Connecting or closing Settings pauses the search; continue later or restart to clear its results.</p>
    <div v-if="scan?.total" class="space-y-1" role="status">
      <UProgress :model-value="percent" />
      <p class="text-xs text-muted">{{ scan.running ? 'Searching' : scan.complete ? 'Search complete' : 'Search paused' }} · {{ nf.format(scan.probed) }} of {{ nf.format(scan.total) }} addresses · {{ nf.format(scan.machines.length) }} machines found</p>
    </div>
    <p v-if="error || scan?.error" role="alert" class="text-sm text-error">{{ error || scan.error }}</p>
    <div v-for="machine in scan?.machines || []" :key="machine.id" class="flex items-center gap-2 border-t border-default py-2">
      <div class="min-w-0 flex-1">
        <div class="truncate text-sm font-medium">{{ machine.name }}</div>
        <div class="truncate text-xs text-dimmed">{{ machine.address }} · agenttik {{ machine.version }}</div>
      </div>
      <span v-if="saved(machine.id)" class="text-xs text-dimmed">Saved</span>
      <UButton color="neutral" variant="outline" label="Connect" :aria-label="`Connect to ${machine.name}`" :disabled="busy || props.busy" @click="connect(machine)" />
    </div>
  </div>
</template>
