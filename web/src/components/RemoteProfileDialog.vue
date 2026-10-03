<script setup>
import { computed, onUnmounted, ref } from "vue";
import { api, nf } from "../api";
import { connectRemote, connected, profiles, profileURL } from "../profiles";

/* Opening a remote profile waits here while its machine is checked, asks for
   the remote's password and code when it wants them, and, when the saved
   address no longer reaches the same machine, can search this computer's
   networks for it by its machine ID. */
const c = computed(() => profiles.connecting);
const remote = computed(() => profiles.remotes.find(r => r.id === c.value?.remote));
const name = computed(() => remote.value?.name || "Remote machine");
const status = computed(() => c.value?.state?.status || "checking");
const path = computed(() => `/api/remotes/${encodeURIComponent(c.value.remote)}`);
const password = ref("");
const code = ref("");
const error = ref("");
const busy = ref(false);
const scan = ref(null);
let poll;

async function signIn() {
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  try {
    const state = await api("POST", `${path.value}/login`, { password: password.value, code: code.value });
    password.value = "";
    code.value = "";
    connected(c.value, state);
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}

function retry() {
  error.value = "";
  connectRemote();
}

async function find() {
  error.value = "";
  try {
    scan.value = await api("POST", `${path.value}/find`);
    poll = setTimeout(follow, 500);
  } catch (e) {
    error.value = e.message;
  }
}

async function follow() {
  try {
    scan.value = await api("GET", `${path.value}/find`);
  } catch (e) {
    error.value = e.message;
    scan.value = null;
    return;
  }
  if (scan.value.running) {
    poll = setTimeout(follow, 500);
  } else if (scan.value.found) {
    scan.value = null;
    connectRemote();
  } else if (scan.value.error) {
    error.value = scan.value.error;
  }
}

async function stopFind() {
  clearTimeout(poll);
  await api("DELETE", `${path.value}/find`).catch(() => {});
  scan.value = null;
}

// A window already on this remote has nothing behind the dialog to go back to.
function close() {
  if (scan.value?.running) stopFind();
  if (c.value?.current) window.location.assign(profileURL("default"));
  else profiles.connecting = null;
}
onUnmounted(() => clearTimeout(poll));

const percent = computed(() => scan.value?.total ? Math.min(100, Math.round(100 * scan.value.probed / scan.value.total)) : 0);
</script>

<template>
  <UModal :open="true" :title="name" :dismissible="false" :close="false" :ui="{ content: 'max-w-md' }">
    <template #body>
      <div class="space-y-4">
        <p v-if="remote" class="text-sm text-muted break-words">{{ remote.address }}<template v-if="remote.version"> · agenttik {{ remote.version }}</template></p>

        <p v-if="status === 'checking'" class="flex items-center gap-2 text-sm">
          <UIcon name="i-lucide-loader-circle" class="size-4 animate-spin" /> Connecting…
        </p>

        <form v-else-if="status === 'signin'" class="space-y-3" @submit.prevent="signIn">
          <p class="text-sm">This machine asks for its password<template v-if="c.state.code_required"> and the code from its authenticator</template>. The session is kept on this computer.</p>
          <UFormField label="Password">
            <UInput v-model="password" type="password" aria-label="Remote password" autocomplete="current-password" autofocus class="w-full" :disabled="busy" />
          </UFormField>
          <UFormField v-if="c.state.code_required" label="Code">
            <UInput v-model="code" aria-label="Remote code" inputmode="numeric" autocomplete="one-time-code" maxlength="6" class="w-full" :disabled="busy" />
          </UFormField>
          <div class="flex justify-end">
            <UButton type="submit" label="Sign in" :loading="busy" :disabled="!password" />
          </div>
        </form>

        <template v-else-if="status !== 'ok'">
          <p class="text-sm">
            <template v-if="status === 'moved'">Another agenttik now answers at this address.</template>
            <template v-else>This machine is not answering.</template>
            <template v-if="remote"> If it moved to another address on your network, search for it.</template>
          </p>
          <p v-if="c.state.error" class="text-xs text-dimmed break-words">{{ c.state.error }}</p>
          <div v-if="scan?.running" class="space-y-2" role="status">
            <UProgress :model-value="percent" />
            <p class="text-xs text-muted">Searched {{ nf.format(scan.probed) }} of {{ nf.format(scan.total) }} addresses. A large network can take hours.</p>
          </div>
          <p v-else-if="scan && !scan.found && !scan.error" class="text-sm text-muted">Not found on this computer's networks.</p>
        </template>

        <p v-if="error" role="alert" class="text-sm text-error break-words">{{ error }}</p>

        <div class="flex flex-wrap justify-end gap-2">
          <UButton color="neutral" variant="ghost" :label="c.current ? 'Use a local profile' : 'Cancel'" @click="close" />
          <template v-if="status === 'unreachable' || status === 'moved'">
            <UButton v-if="scan?.running" color="neutral" variant="outline" label="Stop searching" @click="stopFind" />
            <UButton v-else-if="remote" color="neutral" variant="outline" label="Find on network" @click="find" />
            <UButton label="Retry" :disabled="scan?.running" @click="retry" />
          </template>
        </div>
      </div>
    </template>
  </UModal>
</template>
