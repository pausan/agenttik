<script setup>
/* The shells and subagents a running turn has left working, under the
   progress line. Closed, it is one button counting them; open, one row each
   with when it started and how long it has run. See
   specs/083-background-tasks.md. */
import { computed, ref } from "vue";

import { durationLabel, taskSeconds, tasksLabel } from "../background-tasks.js";
import { openFileRef } from "../store";

const props = defineProps({
  tasks: { type: Array, required: true },
  // The transcript's clock, so running times tick with the progress line.
  now: { type: Number, required: true },
});

const open = ref(false);
const label = computed(() => tasksLabel(props.tasks));

const startedLabel = (task) =>
  new Date(task.started_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });

const endLabel = (task) => [task.status, task.summary].filter(Boolean).join(" · ");
</script>

<template>
  <div class="-mt-2 mb-3">
    <UButton
      class="-ml-1.5 text-dimmed"
      size="xs"
      color="neutral"
      variant="ghost"
      :icon="open ? 'i-lucide-chevron-down' : 'i-lucide-chevron-right'"
      :label="label"
      :aria-expanded="open"
      @click="open = !open"
    />
    <ul v-if="open" class="mt-1 flex flex-col gap-1.5 text-xs" aria-label="Background tasks">
      <li v-for="task in tasks" :key="task.id" class="flex items-start gap-2">
        <UIcon
          :name="task.kind === 'shell' ? 'i-lucide-square-terminal' : 'i-lucide-bot'"
          class="mt-0.5 size-3.5 shrink-0 text-dimmed"
        />
        <div class="min-w-0 flex-1">
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
      </li>
    </ul>
  </div>
</template>
