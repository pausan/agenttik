<script setup>
/* A prompt that would go into a running turn on another model than the one
   it is meant for.

   A turn waiting on background tasks takes prompts, but its process was
   started on one model, effort, subscription and provider and runs on no
   other, so a prompt meant for different ones would quietly run on the
   turn's. This asks first: send it on the turn's, leave it be, or, for a
   queued prompt, stop the turn to run it on its own. It is driven from the
   store, like the unsaved-changes dialog, because the prompt bar, the queue
   and an unprocessed prompt all ask it (specs/083-background-tasks.md). */
import { computed } from "vue";

import { sameGroup } from "../background-tasks";
import { S, modelGroupLabel, modelLabelOf, resolveMismatch } from "../store";

const ask = computed(() => S.mismatch);
const effort = (value) => (value ? value[0].toUpperCase() + value.slice(1) : "Default effort");
const full = (c) => (c ? [modelGroupLabel(c.provider, c.account_id), modelLabelOf(c.provider, c.model), effort(c.effort)].join(" · ") : "");
/* The buttons name the model and effort, and the subscription and provider
   too when those are what differ, so the two never read the same. */
const otherGroup = computed(() => !!ask.value && !sameGroup(ask.value.running, ask.value.wanted));
const short = (c) =>
  c
    ? [
        ...(otherGroup.value ? [modelGroupLabel(c.provider, c.account_id)] : []),
        modelLabelOf(c.provider, c.model),
        ...(c.effort ? [effort(c.effort)] : []),
      ].join(" · ")
    : "";
</script>

<template>
  <UModal
    :open="!!ask"
    title="Send on the running turn's model?"
    :ui="{ content: 'max-w-lg' }"
    @update:open="!$event && resolveMismatch('')"
  >
    <template #body>
      <p class="text-sm">
        This task is waiting on background tasks. Its turn takes {{ ask?.queued ? "this queued prompt" : "your prompt" }}
        now, but only on what it is running on.
      </p>
      <dl class="mt-3 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
        <dt class="text-muted">Running turn</dt>
        <dd aria-label="Running turn">{{ full(ask?.running) }}</dd>
        <dt class="text-muted">{{ ask?.queued ? "Queued prompt" : "Your prompt" }}</dt>
        <dd aria-label="Prompt's model">{{ full(ask?.wanted) }}</dd>
      </dl>
      <p class="mt-3 text-xs text-dimmed">
        <template v-if="ask?.queued">
          Force send runs it on its own model instead, but stops this turn and its background tasks.
          Cancel leaves it queued, to run on its own model once this turn ends.
        </template>
        <template v-else>
          To use your model, cancel and enqueue the prompt: it runs once this turn ends. From the queue,
          Force send runs it at once, but stops this turn and its background tasks.
        </template>
      </p>
    </template>

    <template #footer>
      <div class="flex w-full flex-wrap justify-end gap-2">
        <UButton color="neutral" variant="ghost" label="Cancel" @click="resolveMismatch('')" />
        <UButton
          v-if="ask?.queued"
          color="error"
          variant="subtle"
          :label="`Force send with ${short(ask?.wanted)}`"
          @click="resolveMismatch('stop')"
        />
        <UButton :label="`Send with ${short(ask?.running)}`" @click="resolveMismatch('join')" />
      </div>
    </template>
  </UModal>
</template>
