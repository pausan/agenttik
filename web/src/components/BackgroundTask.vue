<script setup>
/* A shell or subagent stays where it was first reported in the transcript.
   Its state changes in place. See specs/083-background-tasks.md. */
import { computed } from "vue";

import { durationLabel, taskSeconds } from "../background-tasks.js";
import { openFileRef } from "../store";

const props = defineProps({
  message: { type: Object, required: true },
  // The transcript's clock, so running times tick with the progress line.
  now: { type: Number, required: true },
});

const task = computed(() => JSON.parse(props.message.content));

const startedLabel = (task) =>
  new Date(task.started_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });

const endLabel = (task) => [task.status, task.summary].filter(Boolean).join(" · ");
</script>

<template>
  <div class="mb-4 flex items-start gap-2 text-xs" aria-label="Background task">
    <UIcon
      :name="task.kind === 'shell' ? 'i-lucide-square-terminal' : 'i-lucide-bot'"
      class="mt-0.5 size-3.5 shrink-0 text-dimmed"
    />
    <div class="min-w-0 flex-1">
      <div class="mb-0.5 text-[11px] text-dimmed">Background {{ task.kind === "shell" ? "shell" : "agent" }}</div>
      <div class="truncate font-mono" :title="task.description">{{ task.description || task.id }}</div>
      <div class="flex flex-wrap gap-x-2 text-[11px] text-dimmed tabular-nums">
        <span>started {{ startedLabel(task) }}</span>
        <span>{{ task.status === "running" ? "running" : "ran" }} {{ durationLabel(taskSeconds(task, now)) }}</span>
        <span v-if="task.status !== 'running'" :class="{ 'text-error': task.status === 'failed' }">{{ endLabel(task) }}</span>
        <button
          v-if="task.output_file"
          type="button"
          class="text-primary hover:underline"
          :title="task.output_file"
          @click="openFileRef(task.output_file)"
        >
          Output
        </button>
      </div>
    </div>
  </div>
</template>
