<script setup>
/* A schedule in the centre: the prompt it repeats, the clock it repeats on,
   and every run it has spawned or skipped. It looks like a project page
   because that is what it is — a list of work with a header over it — and it
   is the only place the forty runs of a busy schedule are kept, which is why
   the sidebar holds the schedule and not its runs.

   See specs/028-scheduled-jobs.md. */
import { computed, ref, watch } from "vue";

import {
  openTask,
  removeSchedule,
  scheduleLabel,
  setSchedulePaused,
  setScheduleRemaining,
} from "../store";
import { isoLocal } from "../api";

const props = defineProps({ tab: { type: Object, required: true } });

const schedule = computed(() => props.tab.data.schedule);
const runs = computed(() => props.tab.data.runs);

/* The field is only bound to the schedule while it is not being typed in:
   a fire arriving mid-edit would otherwise take the caret with it. */
const editing = ref(false);
const count = ref(String(schedule.value.remaining));
watch(schedule, (s) => {
  if (!editing.value) count.value = String(s.remaining);
});

function commitCount() {
  editing.value = false;
  setScheduleRemaining(schedule.value, count.value);
}

/* Skips have no session and no end; runs are told apart by their timestamps,
   since every one of them was started by the same prompt. */
const STATUS = {
  running: { label: "Running", class: "text-primary" },
  done: { label: "Done", class: "text-muted" },
  error: { label: "Failed", class: "text-error" },
  interrupted: { label: "Interrupted", class: "text-warning" },
  skipped: { label: "Skipped — the previous run was still going", class: "text-dimmed" },
};
const status = (run) => STATUS[run.status] || { label: run.status, class: "text-dimmed" };
</script>

<template>
  <div class="min-h-0 flex-1 overflow-auto">
    <div class="mx-auto max-w-[860px] px-6 py-5">
      <div class="mb-3.5 flex items-start gap-3 border-b border-default pb-3">
        <div class="min-w-0">
          <h2 class="m-0 flex items-center gap-2 text-[17px] tracking-tight text-highlighted">
            <UIcon
              :name="schedule.paused ? 'i-lucide-pause' : 'i-lucide-repeat'"
              class="size-4 block shrink-0"
              :class="schedule.paused ? 'text-dimmed' : 'text-primary'"
            />
            {{ schedule.title }}
          </h2>
          <div class="text-xs text-dimmed">
            {{ scheduleLabel(schedule) }} ·
            {{ schedule.paused ? "paused" : "next run " + isoLocal(schedule.next_run_at) }}
          </div>
        </div>
        <div class="ml-auto flex shrink-0 items-center gap-2">
          <UButton
            :color="schedule.paused ? 'primary' : 'neutral'"
            :variant="schedule.paused ? 'solid' : 'outline'"
            :icon="schedule.paused ? 'i-lucide-play' : 'i-lucide-pause'"
            :label="schedule.paused ? 'Resume' : 'Pause'"
            @click="setSchedulePaused(schedule, !schedule.paused)"
          />
          <UButton
            color="error"
            variant="ghost"
            icon="i-lucide-trash-2"
            label="Delete"
            @click="removeSchedule(schedule)"
          />
        </div>
      </div>

      <div class="mb-4 grid grid-cols-[auto_1fr] items-start gap-x-4 gap-y-2 text-sm">
        <div class="pt-1.5 text-muted">Prompt</div>
        <div class="rounded-[var(--ui-radius)] bg-elevated px-3 py-1.5 whitespace-pre-wrap">
          {{ schedule.prompt }}
        </div>
        <div class="pt-1.5 text-muted">Model</div>
        <div class="pt-1.5 text-dimmed">
          {{ schedule.provider }} · {{ schedule.model
          }}{{ schedule.effort ? " · " + schedule.effort : "" }}
        </div>
        <div class="pt-1.5 text-muted">Runs left</div>
        <div class="flex items-center gap-2">
          <UInput
            v-model="count"
            type="number"
            min="-1"
            class="w-28"
            aria-label="Runs left"
            @focus="editing = true"
            @blur="commitCount"
            @keydown.enter="$event.target.blur()"
          />
          <span class="text-xs text-dimmed">−1 runs forever</span>
        </div>
      </div>

      <h3 class="mb-1.5 text-xs font-medium tracking-wide text-muted uppercase">Runs</h3>
      <p v-if="!runs.length" class="px-3 py-5 text-center text-dimmed">
        Nothing has run yet.
      </p>
      <div
        v-for="run in runs"
        :key="run.id"
        class="flex items-center gap-3 rounded-[var(--ui-radius)] px-2 py-1"
        :class="run.session_id ? 'cursor-pointer hover:bg-elevated' : ''"
        @click="run.session_id && openTask(run.session_id)"
      >
        <span class="shrink-0 font-mono text-xs text-dimmed tabular-nums">
          {{ isoLocal(run.started_at) }}
        </span>
        <span class="truncate text-sm" :class="status(run).class">{{ status(run).label }}</span>
        <span v-if="run.title" class="ml-auto truncate text-xs text-dimmed">{{ run.title }}</span>
      </div>
    </div>
  </div>
</template>
