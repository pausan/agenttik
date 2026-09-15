<script setup>
import { ref, watch } from "vue";
import { api } from "../api";
import { S, isDirty } from "../store";

const open = defineModel("open", { type: Boolean, default: false });
const address = ref("");
const busy = ref(false);
const error = ref("");
const checked = ref(null);
watch(address, () => { checked.value = null; error.value = ""; });

async function connect() {
  if (busy.value || !address.value.trim()) return;
  if (S.tabs.some(isDirty)) {
    error.value = "Save or discard your file edits before connecting.";
    return;
  }
  busy.value = true;
  error.value = "";
  try {
    if (!checked.value) {
      const input = address.value.trim();
      const result = await api("POST", "/api/remote/check", { address: input });
      if (address.value.trim() === input) checked.value = result;
      busy.value = false;
      return;
    }
    const result = await api("POST", "/api/remote/connect", { address: address.value.trim() });
    window.location.assign(result.url);
  } catch (e) {
    error.value = e.message;
    busy.value = false;
  }
}
</script>

<template>
  <UModal v-model:open="open" title="Connect to remote server" :dismissible="!busy" :ui="{ content: 'max-w-md' }">
    <template #body>
      <form class="space-y-4" @submit.prevent="connect">
        <UFormField label="Server address" help="Host and port, or an HTTP(S) URL. The server will ask you to sign in if needed.">
          <UInput v-model="address" placeholder="host:7717" aria-label="Server address" autofocus :disabled="busy" class="w-full" />
        </UFormField>
        <p class="text-sm text-muted">Check the server’s name and version before connecting. Connecting reloads this window; send or save any draft first.</p>
        <div v-if="checked" class="rounded border border-default p-3">
          <p class="font-medium break-words">{{ checked.name || address.trim() }}</p>
          <p class="text-sm text-muted">agenttik {{ checked.version }}</p>
        </div>
        <p v-if="error" role="alert" class="text-sm text-error">{{ error }}</p>
        <div class="flex justify-end gap-2">
          <UButton label="Cancel" color="neutral" variant="ghost" :disabled="busy" @click="open = false" />
          <UButton type="submit" :label="busy ? 'Checking server…' : checked ? 'Connect' : 'Check server'" :loading="busy" :disabled="!address.trim()" />
        </div>
      </form>
    </template>
  </UModal>
</template>
