<script setup>
import { ref } from "vue";
import { api } from "../api";
import { S, isDirty } from "../store";

const open = defineModel("open", { type: Boolean, default: false });
const address = ref("");
const busy = ref(false);
const error = ref("");

async function connect() {
  if (busy.value || !address.value.trim()) return;
  if (S.tabs.some(isDirty)) {
    error.value = "Save or discard your file edits before connecting.";
    return;
  }
  busy.value = true;
  error.value = "";
  try {
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
        <p class="text-sm text-muted">We check /api/version before opening the server. Connecting reloads this window; send or save any draft first.</p>
        <p v-if="error" role="alert" class="text-sm text-error">{{ error }}</p>
        <div class="flex justify-end gap-2">
          <UButton label="Cancel" color="neutral" variant="ghost" :disabled="busy" @click="open = false" />
          <UButton type="submit" :label="busy ? 'Checking server…' : 'Connect'" :loading="busy" :disabled="!address.trim()" />
        </div>
      </form>
    </template>
  </UModal>
</template>
