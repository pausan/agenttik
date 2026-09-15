<script setup>
import { computed, ref, watchEffect } from "vue";
import { fuzzyAny } from "../../fuzzy";

import { diagnosticStorage } from "../../api";
import { diagnosticRevision, readDiagnostics, clearDiagnostics, diagnosticReport } from "../../diagnostics.js";
import { copyText } from "../../store";

const errors = computed(() => { diagnosticRevision.value; return readDiagnostics(diagnosticStorage); });
const copyStatus = ref("");
async function copy(entries) {
  copyStatus.value = await copyText(diagnosticReport(entries)) ? "Copied report" : "Could not copy report";
}
function clear() {
  try { clearDiagnostics(diagnosticStorage); copyStatus.value = "Errors cleared"; }
  catch { copyStatus.value = "Could not clear errors"; }
}
const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count", "start-tour"]);
const shown = computed(() => fuzzyAny(["Help", "Quick Start Tour", "getting started", "walkthrough", "tutorial", "connect", "agent"], props.filter) !== null);
const troubleshooting = computed(() => fuzzyAny(["Help", "Troubleshooting", "Last Errors", "bug report", "trace"], props.filter) !== null);
watchEffect(() => emit("count", Number(shown.value) + Number(troubleshooting.value)));
</script>

<template>
  <section v-if="shown">
    <div class="mb-1 font-semibold text-highlighted">Quick Start Tour</div>
    <p class="mb-3 text-xs text-muted">Connect your first agent, clone a practice project, and build a Python calculator with queued tasks and local commits. Go at your own pace and close the tour anytime.</p>
    <UButton label="Start Quick Start Tour" icon="i-lucide-graduation-cap" @click="emit('start-tour')" />
  </section>
  <section v-if="troubleshooting" class="mt-6">
    <div class="mb-1 font-semibold text-highlighted">Troubleshooting</div>
    <h3 class="mb-2 text-sm font-medium">Last Errors</h3>
    <p class="mb-3 text-xs text-muted">Up to 100 errors from the last five app launches with errors, stored on this device. Reports include error categories, request types and trace locations. Messages, prompts, paths and personal data are excluded. Private windows keep errors only until closed.</p>
    <div v-if="errors.length" class="mb-3 flex gap-2">
      <UButton label="Copy all errors" icon="i-lucide-copy" @click="copy(errors)" />
      <UButton label="Clear errors" color="neutral" variant="outline" @click="clear" />
    </div>
    <p role="status" class="text-xs text-muted">{{ copyStatus }}</p>
    <p v-if="!errors.length" class="text-xs text-muted">No errors recorded.</p>
    <ul v-else class="max-h-96 space-y-3 overflow-y-auto">
      <li v-for="(error, index) in errors" :key="index" class="rounded border border-default p-3 text-xs">
        <div class="flex items-center justify-between gap-2">
          <span class="font-medium">{{ error.kind }} · {{ error.source }}</span>
          <UButton label="Copy report" size="xs" color="neutral" variant="ghost" @click="copy([error])" />
        </div>
        <div class="text-muted">{{ new Date(error.time).toLocaleString() }}</div>
        <div v-if="error.method">{{ error.method }} {{ error.resource }} · {{ error.status ? `HTTP ${error.status}` : 'Request failed' }}</div>
        <details v-if="error.trace.length" class="mt-2">
          <summary>Trace locations</summary>
          <pre class="mt-1 whitespace-pre-wrap">{{ error.trace.join("\n") }}</pre>
        </details>
      </li>
    </ul>
  </section>
</template>
