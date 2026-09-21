<script setup>
import { computed, ref, watch } from "vue";
import { api } from "../api";
import { accountOf, openTask, providerOf } from "../store";
import { analyticsGroups, analyticsShare, analyticsTotals } from "../analytics";

const open = defineModel("open", { type: Boolean, default: false });
const days = ref(7);
const tab = ref("breakdown");
const grouping = ref("provider");
const metric = ref("cost_usd");
const data = ref(null);
const loading = ref(false);
const error = ref("");
const expanded = ref(new Set());
let request = 0;
const ranges = [1, 7, 14, 30, 90, 365].map((value) => ({ label: value === 1 ? "Last 24 hours" : `Last ${value} days`, value }));
const metrics = [{ label: "Reported cost", value: "cost_usd" }, { label: "Input tokens", value: "input_tokens" }, { label: "Output tokens", value: "output_tokens" }, { label: "Turns", value: "turns" }, { label: "Cache read tokens", value: "cache_read_tokens" }, { label: "Cache write tokens", value: "cache_write_tokens" }];
const groups = computed(() => analyticsGroups(data.value?.rows || [], grouping.value, metric.value));
const modelGroups = computed(() => analyticsGroups(data.value?.rows || [], "model_effort", metric.value));
const chartMax = computed(() => Math.max(0, ...modelGroups.value.map((group) => group[metric.value])));
const total = computed(() => analyticsTotals(data.value?.rows || []));
const nf = new Intl.NumberFormat();
const usd = new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", minimumFractionDigits: 2, maximumFractionDigits: 4 });
const number = (value) => nf.format(value || 0);
const dollars = (row) => row.cost_turns ? usd.format(row.cost_usd) : "—";
const providerLabel = (name) => providerOf(name)?.display_name || name;
const accountLabel = (provider, id) => accountOf(provider, id)?.alias || (id ? `Removed subscription #${id}` : "System");

const groupLabel = (group) => grouping.value === "model" ? `${group.model || "Unknown model"} · ${providerLabel(group.provider)}` : providerLabel(group.provider);

async function refresh() {
  const id = ++request;
  loading.value = true;
  error.value = "";
  data.value = null;
  try {
    const result = await api("GET", `/api/analytics?days=${days.value}`);
    if (id === request) data.value = result;
  } catch (err) {
    if (id === request) error.value = err.message;
  } finally {
    if (id === request) loading.value = false;
  }
}
watch(days, refresh, { immediate: true });
function goToTask(id) {
  open.value = false;
  openTask(id);
}
</script>

<template>
  <UModal v-model:open="open" title="Analytics" description="Usage across projects in this profile" fullscreen :ui="{ body: 'overflow-y-auto' }">
    <template #body>
      <div class="mx-auto max-w-6xl space-y-6 pb-8">
        <UTabs v-model="tab" :items="[{ label: 'Breakdown', value: 'breakdown' }, { label: 'Models & effort', value: 'models' }]" :content="false" aria-label="Analytics views" />
        <div class="flex flex-wrap items-end gap-3">
          <label class="space-y-1 text-sm"><span class="block text-muted">Period</span><USelect v-model="days" :items="ranges" aria-label="Analytics period" class="w-44" /></label>
          <label v-if="tab === 'breakdown'" class="space-y-1 text-sm"><span class="block text-muted">Break down by</span><USelect v-model="grouping" :items="[{ label: 'Provider', value: 'provider' }, { label: 'Model', value: 'model' }, { label: 'Subscription / connection', value: 'subscription' }]" aria-label="Analytics grouping" class="w-56" /></label>
          <label class="space-y-1 text-sm"><span class="block text-muted">Rank by</span><USelect v-model="metric" :items="metrics" aria-label="Analytics ranking" class="w-44" /></label>
          <UButton icon="i-lucide-refresh-cw" color="neutral" variant="outline" :loading="loading" @click="refresh">Refresh</UButton>
        </div>
        <p class="text-sm text-muted">Reported costs are usage figures, not subscription bills. Subscription fees and unreported costs are excluded. A dash means no positive cost was recorded. Token accounting varies by provider; cache counts are shown separately.</p>
        <p v-if="loading" role="status" class="text-muted">Loading analytics…</p>
        <div v-else-if="error" role="alert" class="text-error">{{ error }} <UButton color="neutral" variant="link" @click="refresh">Retry</UButton></div>
        <template v-else-if="data">
          <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <div v-for="card in [{ label: 'Reported cost', value: dollars(total) }, { label: 'Input tokens', value: number(total.input_tokens) }, { label: 'Output tokens', value: number(total.output_tokens) }, { label: 'Turns', value: number(total.turns) }]" :key="card.label" class="rounded-lg border border-default p-4">
              <div class="text-sm text-muted">{{ card.label }}</div><div class="mt-1 text-2xl font-semibold tabular-nums">{{ card.value }}</div>
            </div>
          </div>
          <p class="text-xs text-muted">{{ new Date(data.from).toLocaleString() }} – {{ new Date(data.to).toLocaleString() }} · By turn start time · Includes archived and hidden projects · Refresh to include new usage</p>
          <p v-if="total.inferred_turns" class="text-sm text-muted">{{ number(total.inferred_turns) }} older turns use the task's provider and subscription at upgrade time; earlier switches cannot be reconstructed.</p>
          <p v-if="!groups.length" class="py-12 text-center text-muted">No recorded usage in this period.</p>
          <div v-if="tab === 'models' && modelGroups.length" class="overflow-x-auto rounded-lg border border-default p-4">
            <p class="mb-4 text-sm text-muted">{{ metrics.find((item) => item.value === metric)?.label }} by model and effort · Highest first</p>
            <div class="flex min-w-full items-end gap-4" role="list" aria-label="Model and effort usage">
              <div v-for="group in modelGroups" :key="group.key" role="listitem" class="min-w-32 flex-1 text-center text-xs">
                <div class="flex h-64 flex-col justify-end">
                  <div class="mb-2 tabular-nums">{{ metric === 'cost_usd' ? dollars(group) : number(group[metric]) }}</div>
                  <div class="mx-auto w-16 rounded-t bg-primary" :style="{ height: chartMax > 0 ? `${group[metric] / chartMax * 220}px` : '0px' }"></div>
                </div>
                <div class="border-t border-default pt-2 font-medium">{{ group.model || 'Unknown model' }}</div>
                <div class="text-muted">{{ group.effort || 'Unspecified effort' }} · {{ providerLabel(group.provider) }}</div>
                <div class="mt-2 text-muted">{{ number(group.turns) }} turns · {{ dollars(group) }}</div>
              </div>
            </div>
          </div>
          <template v-if="tab === 'breakdown'">
            <section v-for="group in groups" :key="group.key" class="space-y-3" :aria-label="groupLabel(group)">
              <div class="flex flex-wrap items-baseline justify-between gap-2">
                <h2 class="text-lg font-semibold">{{ groupLabel(group) }}<span v-if="group.account_id !== null" class="font-normal text-muted"> · {{ accountLabel(group.provider, group.account_id) }}</span></h2>
                <span class="text-sm text-muted">{{ dollars(group) }} · {{ number(group.cost_turns) }}/{{ number(group.turns) }} turns with positive cost</span>
              </div>
              <div class="overflow-x-auto rounded-lg border border-default">
                <table class="w-full text-sm analytics-table">
                  <thead class="bg-elevated text-muted"><tr><th>Project / task</th><th>Reported cost</th><th>Input</th><th>Output</th><th>Cache read</th><th>Cache write</th><th>Turns</th><th>Share*</th></tr></thead>
                  <tbody v-for="project in group.projects" :key="project.id">
                    <tr class="font-medium"><td>{{ project.name }}</td><td>{{ dollars(project) }}</td><td>{{ number(project.input_tokens) }}</td><td>{{ number(project.output_tokens) }}</td><td>{{ number(project.cache_read_tokens) }}</td><td>{{ number(project.cache_write_tokens) }}</td><td>{{ number(project.turns) }}</td><td>{{ analyticsShare(project[metric], group[metric]) }}</td></tr>
                    <tr><td colspan="8" class="!pt-0"><details @toggle="$event.target.open ? expanded.add(`${group.key}:${project.id}`) : expanded.delete(`${group.key}:${project.id}`)"><summary class="cursor-pointer text-muted">Task breakdown ({{ project.task_count }})</summary>
                      <table v-if="expanded.has(`${group.key}:${project.id}`)" class="mt-2 w-full text-xs" aria-label="Task breakdown"><thead class="text-muted"><tr><th>Task / subscription</th><th>Reported cost</th><th>Input</th><th>Output</th><th>Cache read</th><th>Cache write</th><th>Turns</th><th>Share*</th></tr></thead><tbody>
                        <tr v-for="row in project.rows" :key="`${row.session_id}:${row.account_id}`"><td><button class="text-primary text-left hover:underline" @click="goToTask(row.session_id)">{{ row.title || 'Untitled task' }}</button><div class="text-muted">{{ accountLabel(row.provider, row.account_id) }}</div></td><td>{{ dollars(row) }}</td><td>{{ number(row.input_tokens) }}</td><td>{{ number(row.output_tokens) }}</td><td>{{ number(row.cache_read_tokens) }}</td><td>{{ number(row.cache_write_tokens) }}</td><td>{{ number(row.turns) }}</td><td>{{ analyticsShare(row[metric], group[metric]) }}</td></tr>
                      </tbody></table>
                    </details></td></tr>
                  </tbody>
                </table>
              </div>
            </section>
            <p v-if="groups.length" class="text-xs text-muted">* Share of the selected ranking metric within each {{ grouping === 'model' ? 'model' : grouping === 'provider' ? 'provider' : 'subscription / connection' }}. Projects and tasks are ordered highest first.</p>
          </template>
        </template>
      </div>
    </template>
  </UModal>
</template>

<style scoped>
.analytics-table th, .analytics-table td { padding: 0.65rem 0.75rem; text-align: right; white-space: nowrap; font-variant-numeric: tabular-nums; }
.analytics-table th:first-child, .analytics-table td:first-child { text-align: left; white-space: normal; min-width: 12rem; }
.analytics-table > tbody { border-top: 1px solid var(--ui-border); }
</style>
