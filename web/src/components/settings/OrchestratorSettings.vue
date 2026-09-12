<script setup>
import { computed, ref, watchEffect } from "vue";

import { S, resetOrchestratorPrompt, setOrchestratorEnabled } from "../../store";
import { fuzzyAny } from "../../fuzzy";

const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count", "open-project"]);
const saving = ref(false);
const shown = computed(() => fuzzyAny(
  ["Orchestrator", "enable", "meta project", "pinned", "default prompt", "reset", "tasks", S.orchestrator?.name || ""],
  props.filter,
) !== null);
watchEffect(() => emit("count", shown.value ? 2 : 0));

const busy = computed(() => {
  const project = S.projects.find((p) => p.kind === "orchestrator");
  return project?.recent_sessions.some((s) => s.status === "running" || s.queue_count > 0) ||
    project?.schedules.some((s) => s.running);
});

async function enable(value) {
  saving.value = true;
  try {
    await setOrchestratorEnabled(value);
  } finally {
    saving.value = false;
  }
}

async function reset() {
  saving.value = true;
  try {
    await resetOrchestratorPrompt();
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <section v-if="shown && S.orchestrator">
    <div class="mb-0.5 font-semibold text-highlighted">Orchestrator</div>
    <p class="mb-3.5 text-xs text-dimmed">
      A separate project, pinned above the others. Ask it about current work or have it
      start tasks and manage projects for you.
    </p>
    <USwitch
      :model-value="S.orchestrator.enabled"
      label="Enable orchestrator"
      :disabled="saving || !!busy"
      @update:model-value="enable"
    />
    <p class="mt-2 text-xs text-dimmed">
      Disabling keeps its tasks, files and options. Deleting the project clears its task
      history, keeps its files and options, and turns this setting off. Enable it again here.
    </p>
    <p v-if="busy" class="mt-2 text-xs text-muted">
      Finish or stop its active and queued tasks before disabling it.
    </p>

    <div class="mt-5 text-xs text-muted">
      <div class="font-medium text-highlighted">{{ S.orchestrator.name }}</div>
      <div class="mt-1 break-all font-mono text-dimmed">{{ S.orchestrator.path }}</div>
      <UButton
        v-if="S.orchestrator.enabled"
        class="mt-2"
        size="sm"
        color="neutral"
        variant="subtle"
        label="Open project"
        @click="$emit('open-project')"
      />
    </div>

    <div class="mt-5 border-t border-default pt-4">
      <div class="mb-1 font-semibold text-highlighted">Default prompt</div>
      <p class="mb-3 text-xs text-dimmed">
        The project’s Prompt tab holds its instructions. Reset replaces them with the
        default supplied by this version of agenttik. Existing conversations keep their
        original instructions.
      </p>
      <UButton
        color="neutral"
        variant="subtle"
        icon="i-lucide-rotate-ccw"
        label="Reset prompt to default"
        :disabled="saving"
        @click="reset"
      />
    </div>
  </section>
</template>
