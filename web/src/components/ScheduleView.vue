<script setup>
/* A schedule in the centre: the prompt it repeats, the clock it repeats on,
   and every run it has spawned or skipped. It looks like a project page
   because that is what it is — a list of work with a header over it — and it
   is the only place the forty runs of a busy schedule are kept, which is why
   the sidebar holds the schedule and not its runs.

   See specs/028-scheduled-jobs.md. */
import { computed, ref, watch } from "vue";

import {
  S,
  accountLabel,
  effortsFor,
  enterDoes,
  EVERY,
  modelPickerGroups,
  openTask,
  parseModelChoice,
  providerOf,
  removeSchedule,
  runScheduleNow,
  scheduleForm,
  scheduleLabel,
  setScheduleArchived,
  setSchedulePaused,
  setScheduleFrequency,
  setScheduleModel,
  setSchedulePrompt,
  setScheduleRemaining,
} from "../store";
import { isoLocal } from "../api";

const props = defineProps({ tab: { type: Object, required: true } });

const schedule = computed(() => props.tab.data.schedule);
const runs = computed(() => props.tab.data.runs);

/* Archived is the stronger of the two stopped states: the job has been put
   away, so the header offers the way back instead of a Resume that would
   schedule nothing and a Run that would start work from something shelved.
   Everything it repeats stays editable — the project page's Jobs tab is the
   other place it can be restored from. */
const archived = computed(() => !!schedule.value.done_at);

/* Run now: the same Send/Enqueue choice the prompt bar's Send menu offers,
   minus Schedule — scheduling a schedule does not mean anything. It runs even
   if a previous fire is still going and never spends the counter, both of
   which are the server's doing (runner.RunScheduleNow); this menu only picks
   Send or Enqueue and closes itself once one is chosen. */
const runMenuOpen = ref(false);
const runActions = computed(() => ({
  send: { label: "Send", icon: "i-lucide-send", run: () => runScheduleNow(schedule.value, false) },
  enqueue: { label: "Enqueue", icon: "i-lucide-clock", run: () => runScheduleNow(schedule.value, true) },
}));
const runPrimary = computed(() => runActions.value[enterDoes() === "enqueue" ? "enqueue" : "send"]);
const runMenu = computed(() => [runActions.value.send, runActions.value.enqueue]);

function runMenuAction(action) {
  runMenuOpen.value = false;
  action.run();
}

/* The fields are only bound to the schedule while they are not being typed
   in: a fire arriving mid-edit would otherwise take the caret with it. */
const editingCount = ref(false);
const editingFreq = ref(false);
const editingPrompt = ref(false);
const count = ref(String(schedule.value.remaining));
const freq = ref(scheduleForm(schedule.value));
const prompt = ref(schedule.value.prompt);
watch(schedule, (s) => {
  if (!editingCount.value) count.value = String(s.remaining);
  if (!editingFreq.value) freq.value = scheduleForm(s);
  if (!editingPrompt.value) prompt.value = s.prompt;
});

/* The prompt is what the schedule repeats, and it is as changeable as the
   clock beside it: a schedule worth keeping is not worth remaking over a
   typo. It commits when it is left, as the other fields do — the runs
   already spawned keep the text they were started with. */
function commitPrompt() {
  editingPrompt.value = false;
  setSchedulePrompt(schedule.value, prompt.value);
}

/* So is the model. The picker is the prompt bar's, less the favourites a
   schedule has no bar to star from: the providers, their models, and the
   efforts of whichever model is chosen. Provider, model and effort travel
   together, so switching to a model whose provider has no such effort drops
   it rather than sending one the provider would reject. */
const NONE = "__default";
const modelOpen = ref(false);
const modelSearch = ref("");
const provider = computed(() => providerOf(schedule.value.provider));
const selectedModel = computed(() =>
  provider.value?.models.find((m) => m.id === schedule.value.model),
);
/* The subscription is named beside the model wherever the provider has more
   than one: every run this job spawns spends that account's allowance. */
const modelLabel = computed(() => {
  const alias = accountLabel(schedule.value.provider, schedule.value.account_id);
  const model = selectedModel.value?.label || schedule.value.model;
  return alias ? `${model} · ${alias}` : model;
});
const efforts = computed(() => effortsFor(schedule.value.provider, schedule.value.model));

const modelGroups = computed(modelPickerGroups);

function pickModel(value) {
  modelOpen.value = false;
  const { provider: providerName, accountID, model } = parseModelChoice(value);
  const nextEfforts = effortsFor(providerName, model);
  const effort = schedule.value.effort || "";
  setScheduleModel(schedule.value, providerName, model,
    nextEfforts.includes(effort) ? effort : "", accountID);
}

const effortItems = computed(() => [
  { label: "default effort", value: NONE },
  ...efforts.value.map((e) => ({ label: e, value: e })),
]);

const effortValue = computed({
  get: () => schedule.value.effort || NONE,
  set: (v) =>
    setScheduleModel(schedule.value, schedule.value.provider, schedule.value.model,
      v === NONE ? "" : v),
});

watch(modelOpen, (open) => {
  if (!open) modelSearch.value = "";
});

function commitCount() {
  editingCount.value = false;
  setScheduleRemaining(schedule.value, count.value);
}

/* The clock is as changeable as the counter, and changing it restarts the
   cadence from now — the server books the next run, as a resume does. */
function commitFreq() {
  editingFreq.value = false;
  setScheduleFrequency(schedule.value, freq.value);
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
/* A run of skips is written as one row with a count (schedules.go,
   SkipScheduleRun), so the label pluralises rather than the list repeating
   itself once per fire the schedule waited out. */
function status(run) {
  const s = STATUS[run.status] || { label: run.status, class: "text-dimmed" };
  if (run.status === "skipped" && run.count > 1) {
    return { ...s, label: `Skipped ${run.count} times — the previous run was still going` };
  }
  return s;
}

/* A single fire is one timestamp; a collapsed run of skips is the range it
   spans. */
function runTime(run) {
  if (run.count > 1) return `${isoLocal(run.started_at)} – ${isoLocal(run.ended_at)}`;
  return isoLocal(run.started_at);
}
</script>

<template>
  <div class="min-h-0 flex-1 overflow-auto">
    <div class="mx-auto max-w-[860px] px-6 py-5">
      <div class="mb-3.5 flex items-start gap-3 border-b border-default pb-3">
        <div class="min-w-0">
          <h2 class="m-0 flex items-center gap-2 text-[17px] tracking-tight text-highlighted">
            <UIcon
              :name="archived ? 'i-lucide-archive' : schedule.paused ? 'i-lucide-pause' : 'i-lucide-repeat'"
              class="size-4 block shrink-0"
              :class="archived || schedule.paused ? 'text-dimmed' : 'text-primary'"
            />
            {{ schedule.title }}
            <!-- The job's number. Every task it spawns carries the same one,
                 which is how forty runs that share a name are told from
                 another job's. F2 renames the job from its sidebar row. -->
            <span
              class="font-mono text-xs font-normal text-dimmed tabular-nums"
              title="Job number — its runs carry it too. F2 renames the job."
              >#{{ schedule.id }}</span
            >
          </h2>
          <div class="text-xs text-dimmed">
            {{ scheduleLabel(schedule) }} ·
            {{
              archived
                ? "archived " + isoLocal(schedule.done_at)
                : schedule.paused
                  ? "paused"
                  : "next run " + isoLocal(schedule.next_run_at)
            }}
          </div>
        </div>
        <div class="ml-auto flex shrink-0 items-center gap-2">
          <template v-if="!archived">
            <UFieldGroup>
              <UButton :icon="runPrimary.icon" :label="runPrimary.label" @click="runPrimary.run()" />
              <UPopover v-model:open="runMenuOpen">
                <UButton icon="i-lucide-chevron-down" aria-label="More run actions" />
                <template #content>
                  <div class="p-1">
                    <UButton
                      v-for="action in runMenu"
                      :key="action.label"
                      color="neutral"
                      variant="ghost"
                      block
                      class="justify-start"
                      :label="action.label"
                      :trailing-icon="action.icon"
                      @click="runMenuAction(action)"
                    />
                  </div>
                </template>
              </UPopover>
            </UFieldGroup>
            <UButton
              :color="schedule.paused ? 'primary' : 'neutral'"
              :variant="schedule.paused ? 'solid' : 'outline'"
              :icon="schedule.paused ? 'i-lucide-play' : 'i-lucide-pause'"
              :label="schedule.paused ? 'Resume' : 'Pause'"
              @click="setSchedulePaused(schedule, !schedule.paused)"
            />
          </template>
          <!-- Put away: the one control it needs is the way back, and it
               comes back paused rather than mid-burst. -->
          <UButton
            v-else
            icon="i-lucide-archive-restore"
            label="Unarchive"
            @click="setScheduleArchived(schedule, false)"
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
        <UTextarea
          v-model="prompt"
          :rows="3"
          autoresize
          class="w-full"
          aria-label="Prompt"
          :ui="{ base: 'resize-y' }"
          @focus="editingPrompt = true"
          @blur="commitPrompt"
        />
        <div class="pt-1.5 text-muted">Model</div>
        <div class="flex flex-wrap items-center gap-2">
          <UPopover v-model:open="modelOpen">
            <UButton
              type="button"
              color="neutral"
              variant="outline"
              trailing-icon="i-lucide-chevron-down"
              :label="modelLabel"
              title="Choose model"
            />
            <template #content>
              <UCommandPalette
                class="w-80"
                :groups="modelGroups"
                value-key="value"
                placeholder="Search models…"
                v-model:search-term="modelSearch"
                preserve-group-order
                :ui="{ viewport: 'max-h-[min(28rem,60vh)]' }"
                :fuse="{ fuseOptions: { keys: ['label', 'description'], threshold: 0.35, ignoreLocation: true }, resultLimit: 20 }"
                @update:model-value="pickModel"
              />
            </template>
          </UPopover>
          <USelect v-model="effortValue" :items="effortItems" title="Effort" />
        </div>
        <div class="pt-1.5 text-muted">Repeats</div>
        <div class="flex flex-wrap items-center gap-2">
          <USelectMenu
            v-model="freq.every"
            value-key="value"
            :items="EVERY"
            :search-input="false"
            class="w-44"
            aria-label="Repeat every"
            @update:model-value="commitFreq"
          />
          <template v-if="freq.every === 'interval'">
            <UInput
              v-model="freq.hours"
              type="number"
              min="0"
              class="w-20"
              aria-label="Hours"
              @focus="editingFreq = true"
              @blur="commitFreq"
              @keydown.enter="$event.target.blur()"
            />
            <span class="text-xs text-dimmed">h</span>
            <UInput
              v-model="freq.minutes"
              type="number"
              min="0"
              class="w-20"
              aria-label="Minutes"
              @focus="editingFreq = true"
              @blur="commitFreq"
              @keydown.enter="$event.target.blur()"
            />
            <span class="text-xs text-dimmed">m</span>
          </template>
          <template v-else>
            <UInput
              v-model="freq.at"
              type="time"
              class="w-32"
              aria-label="Time of day"
              @focus="editingFreq = true"
              @blur="commitFreq"
              @keydown.enter="$event.target.blur()"
            />
            <span class="text-xs text-dimmed">
              on the {{ freq.every === "week" ? "weekday" : "day" }} it was set
            </span>
          </template>
        </div>
        <div class="pt-1.5 text-muted">Runs left</div>
        <div class="flex items-center gap-2">
          <UInput
            v-model="count"
            type="number"
            min="-1"
            class="w-28"
            aria-label="Runs left"
            @focus="editingCount = true"
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
          {{ runTime(run) }}
        </span>
        <span class="truncate text-sm" :class="status(run).class">{{ status(run).label }}</span>
        <span v-if="run.title" class="ml-auto truncate text-xs text-dimmed">{{ run.title }}</span>
      </div>
    </div>
  </div>
</template>
