<script setup>
/* A tool call the running turn waits on the user to allow. Every window
   draws the same card, and the first answer from any of them settles it:
   the others see it go when the server says so. See
   specs/082-tool-approvals.md. */
import { computed } from "vue";

import { answerApproval } from "../store";

const props = defineProps({
  sessionId: { type: String, required: true },
  approval: { type: Object, required: true },
});

/* The input is JSON. A shell command reads best as itself; anything else is
   indented so its fields can be told apart. */
const input = computed(() => {
  const raw = props.approval.input || "";
  try {
    const value = JSON.parse(raw);
    if (value && typeof value.command === "string" && Object.keys(value).length <= 2) return value.command;
    return JSON.stringify(value, null, 2);
  } catch {
    return raw;
  }
});
</script>

<template>
  <div
    class="mb-4 rounded-[var(--ui-radius)] border border-warning/50 bg-warning/5 px-3 py-2.5"
    role="group"
    aria-label="Tool approval"
  >
    <div class="flex items-center gap-2 text-sm">
      <UIcon name="i-lucide-shield-alert" class="size-4 shrink-0 text-warning" aria-hidden="true" />
      <span class="font-medium text-highlighted">Allow {{ approval.tool || "this tool" }}?</span>
    </div>
    <p v-if="approval.description" class="mt-1 text-xs text-muted">{{ approval.description }}</p>
    <pre
      v-if="input"
      class="mt-2 max-h-48 overflow-auto rounded bg-elevated px-2 py-1.5 font-mono text-xs whitespace-pre-wrap text-default wrap-anywhere"
    >{{ input }}</pre>
    <div class="mt-2.5 flex items-center gap-2">
      <UButton
        size="sm"
        icon="i-lucide-check"
        label="Allow"
        :loading="approval.answering"
        :disabled="approval.answering"
        @click="answerApproval(sessionId, approval.id, true)"
      />
      <UButton
        size="sm"
        color="neutral"
        variant="outline"
        icon="i-lucide-x"
        label="Deny"
        :disabled="approval.answering"
        @click="answerApproval(sessionId, approval.id, false)"
      />
    </div>
  </div>
</template>
