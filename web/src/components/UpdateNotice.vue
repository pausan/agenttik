<script setup>
import { onMounted, onUnmounted, ref } from "vue";
import { api } from "../api";

const status = ref({});
const error = ref("");
const dismissed = ref(false);
const submitting = ref(false);
let timer;
let stopped = false;
async function refresh() {
  try { status.value = await api("GET", "/api/updates"); } catch { /* Retry after reconnect. */ }
  if (!stopped) timer = setTimeout(refresh, status.value.busy ? 1000 : 60000);
}
async function act(action) {
  submitting.value = true;
  error.value = "";
  try {
    status.value = await api("POST", `/api/updates/${action}`, { version: status.value.available.version });
    clearTimeout(timer);
    if (!stopped) timer = setTimeout(refresh, 1000);
  } catch (err) { error.value = err.message; }
  finally { submitting.value = false; }
}
onMounted(() => { timer = setTimeout(refresh, 3000); });
onUnmounted(() => { stopped = true; clearTimeout(timer); });
</script>

<template>
  <section v-if="!dismissed && (status.available || status.ready || status.error)"
    role="status" aria-live="polite"
    class="fixed bottom-4 right-4 z-50 max-w-sm rounded-lg border border-default bg-default p-4 shadow-lg max-sm:left-4">
    <p class="font-semibold">{{ status.ready ? "Update ready" : status.available ? `Agenttik ${status.available.version} is available` : "Update failed" }}</p>
    <p class="mt-1 text-muted">
      {{ status.ready ? "Quit Agenttik to install, then reopen it. Use Quit from the tray if closing the window hides it." : status.busy ? "Preparing the update. Approve the system administrator prompt if one appears." : "Install when you quit. Your running tasks can finish first." }}
    </p>
    <p v-if="error || status.error" class="mt-2 text-error">{{ error || status.error }}</p>
    <div class="mt-3 flex justify-end gap-2">
      <UButton v-if="status.ready || !status.available" color="neutral" variant="ghost" @click="dismissed = true">Dismiss</UButton>
      <template v-else>
        <UButton color="neutral" variant="ghost" :disabled="status.busy || submitting" @click="act('ignore')">Ignore this version</UButton>
        <UButton :loading="status.busy || submitting" :disabled="status.busy || submitting" @click="act('install')">Update</UButton>
      </template>
    </div>
  </section>
</template>
