<script setup>
/* A request the running turn waits on: a tool call to allow, or questions to
   answer. Every window draws the same card, and the first answer from any of
   them settles it: the others see it go when the server says so. See
   specs/082-tool-approvals.md. */
import { computed, reactive } from "vue";

import { answersFor, emptyAnswers, pick, type } from "../questions.js";
import { answerApproval } from "../store";
import ApprovalTimer from "./ApprovalTimer.vue";

const props = defineProps({
  sessionId: { type: String, required: true },
  approval: { type: Object, required: true },
});

const questions = computed(() => props.approval.questions || []);
const state = reactive(emptyAnswers(questions.value));
const answers = computed(() => answersFor(questions.value, state));

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

function submit() {
  if (answers.value) answerApproval(props.sessionId, props.approval.id, true, answers.value);
}
</script>

<template>
  <div
    v-if="questions.length"
    class="mb-4 rounded-[var(--ui-radius)] border border-primary/40 bg-primary/5 p-4"
    role="group"
    aria-label="Agent question"
  >
    <div class="flex items-center gap-2 text-base">
      <UIcon name="i-lucide-message-circle-question" class="size-5 shrink-0 text-primary" aria-hidden="true" />
      <span class="font-medium text-highlighted">
        {{ questions.length > 1 ? "The agent has questions" : "The agent has a question" }}
      </span>
    </div>
    <p v-if="approval.description" class="mt-2 text-sm text-muted">{{ approval.description }}</p>
    <form class="mt-3 space-y-4" @submit.prevent="submit">
      <fieldset v-for="q in questions" :key="q.id" :disabled="approval.answering">
        <legend class="text-base text-default">
          <span v-if="q.header" class="mr-1.5 rounded bg-elevated px-1.5 py-0.5 text-[11px] text-muted">{{ q.header }}</span>
          {{ q.question }}
          <span v-if="q.multi" class="text-xs text-dimmed">(pick any)</span>
        </legend>
        <div v-if="q.options?.length" class="mt-1.5 flex flex-col gap-1">
          <button
            v-for="o in q.options"
            :key="o.label"
            type="button"
            class="flex items-start gap-3 rounded border px-3 py-2.5 text-left text-base"
            :class="state[q.id].picked.includes(o.label)
              ? 'border-primary bg-primary/10 text-highlighted'
              : 'border-default hover:bg-elevated'"
            :role="q.multi ? 'checkbox' : 'radio'"
            :aria-checked="state[q.id].picked.includes(o.label)"
            @click="pick(state, q, o.label)"
          >
            <UIcon
              :name="q.multi
                ? (state[q.id].picked.includes(o.label) ? 'i-lucide-square-check' : 'i-lucide-square')
                : (state[q.id].picked.includes(o.label) ? 'i-lucide-circle-dot' : 'i-lucide-circle')"
              class="mt-1 size-4 shrink-0"
              aria-hidden="true"
            />
            <span>
              <span class="block">{{ o.label }}</span>
              <span v-if="o.description" class="mt-0.5 block text-sm text-muted">{{ o.description }}</span>
            </span>
          </button>
        </div>
        <UInput
          v-if="q.other"
          class="mt-1.5 w-full"
          size="lg"
          :type="q.secret ? 'password' : 'text'"
          :placeholder="q.options?.length ? 'Or type an answer' : 'Type an answer'"
          :aria-label="`Answer: ${q.question}`"
          :model-value="state[q.id].typed"
          @update:model-value="type(state, q, $event)"
        />
      </fieldset>
      <div class="flex flex-wrap items-center gap-2">
        <UButton
          type="submit"
          size="sm"
          icon="i-lucide-send"
          label="Answer"
          :loading="approval.answering"
          :disabled="!answers || approval.answering"
        />
        <UButton
          size="sm"
          color="neutral"
          variant="outline"
          label="Skip"
          title="Decline to answer; the agent carries on without it"
          :disabled="approval.answering"
          @click="answerApproval(sessionId, approval.id, false)"
        />
        <ApprovalTimer :session-id="sessionId" :approval="approval" verb="Answering for you" />
      </div>
    </form>
  </div>
  <div
    v-else
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
    <div class="mt-2.5 flex flex-wrap items-center gap-2">
      <UButton
        size="sm"
        icon="i-lucide-check"
        label="Allow"
        :loading="approval.answering"
        :disabled="approval.answering"
        @click="answerApproval(sessionId, approval.id, true)"
      />
      <UButton
        v-if="approval.prefix"
        size="sm"
        variant="soft"
        icon="i-lucide-terminal"
        :label="`Allow ${approval.prefix} for this task`"
        :title="`Don't ask again for commands starting with “${approval.prefix}” until agenttik restarts, in this task only. Chained or piped commands still ask.`"
        :disabled="approval.answering"
        @click="answerApproval(sessionId, approval.id, true, undefined, 'task_command')"
      />
      <UButton
        size="sm"
        variant="soft"
        icon="i-lucide-wrench"
        :label="`Allow ${approval.tool || 'this tool'} for this task`"
        title="Don't ask again for this tool until agenttik restarts, in this task only"
        :disabled="approval.answering"
        @click="answerApproval(sessionId, approval.id, true, undefined, 'task')"
      />
      <UButton
        size="sm"
        variant="soft"
        icon="i-lucide-check-check"
        label="Allow all for this task"
        title="Don't ask again about any tool until agenttik restarts, in this task only"
        :disabled="approval.answering"
        @click="answerApproval(sessionId, approval.id, true, undefined, 'task_all')"
      />
      <UButton
        v-if="approval.remember?.includes('turn')"
        size="sm"
        variant="soft"
        label="Allow for this turn"
        title="Don't ask again for this until the agent finishes its reply"
        :disabled="approval.answering"
        @click="answerApproval(sessionId, approval.id, true, undefined, 'turn')"
      />
      <UButton
        v-if="approval.remember?.includes('always')"
        size="sm"
        variant="soft"
        label="Always allow"
        title="Don't ask again for this, in this task or any other"
        :disabled="approval.answering"
        @click="answerApproval(sessionId, approval.id, true, undefined, 'always')"
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
      <ApprovalTimer :session-id="sessionId" :approval="approval" verb="Allowing" />
    </div>
  </div>
</template>
