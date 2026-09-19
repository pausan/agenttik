<script setup>
import { computed, ref, watch } from "vue";
import { api, ago } from "../api";
import { S, TASK_WINDOWS, openSchedule, pickTask, renameTask, setTaskArchived, toggleTaskUnread } from "../store";
import TaskRow from "./TaskRow.vue";

const props = defineProps({ tab: { type: Object, required: true } });
defineEmits(["delete"]);
const query = ref("");
const window = ref("all");
const page = ref(1);
const rows = ref([]);
const loading = ref(false);
const error = ref("");
const retry = ref(0);
const size = computed(() => S.taskPageSize);
const visible = computed(() => rows.value.slice(0, size.value));
const more = computed(() => rows.value.length > size.value);

watch([query, window, size], () => { page.value = 1; }, { flush: "sync" });
watch([query, window, page, size, () => props.tab.data.revision, retry], (values, previous, onCleanup) => {
  let stale = false;
  loading.value = true;
  error.value = "";
  rows.value = [];
  // Fetch one extra row to answer Next without counting or retaining history.
  const params = new URLSearchParams({
    window: window.value, project_id: props.tab.projectID, only_done: "true",
    limit: size.value + 1, offset: (page.value - 1) * size.value, q: query.value.trim(),
  });
  const timer = setTimeout(async () => {
    try {
      const result = await api("GET", `/api/sessions?${params}`);
      if (stale) return;
      rows.value = result;
      if (!result.length && page.value > 1) page.value--;
    } catch (err) {
      if (!stale) error.value = err.message;
    } finally {
      if (!stale) loading.value = false;
    }
  }, previous && values[0] !== previous[0] ? 250 : 0);
  onCleanup(() => { stale = true; clearTimeout(timer); });
}, { immediate: true });
</script>

<template>
  <section aria-label="Archived tasks">
    <div class="mb-3 flex gap-3">
      <UInput v-model="query" type="search" placeholder="Search archived titles" aria-label="Search archived titles" icon="i-lucide-search" class="min-w-0 flex-1" />
      <USelect v-model="window" :items="TASK_WINDOWS" aria-label="Archived task time filter" />
    </div>
    <p v-if="loading" role="status" class="text-sm text-dimmed">Loading archived tasks…</p>
    <div v-else-if="error" role="alert">
      <p class="text-error">{{ error }}</p>
      <UButton label="Retry" @click="retry++" />
    </div>
    <p v-else-if="!rows.length" class="text-sm text-dimmed">No archived tasks match.</p>
    <TaskRow
      v-for="task in visible" :key="task.id"
      :title="task.title" :prompt="task.prompt" :status="task.status"
      :sub="`${task.model} · ${ago(task.last_active_at)}`" :outcome="task.summary"
      :job="task.schedule_id" :unread="!!S.unreadTasks[task.id]"
      archived archive deletable
      @select="pickTask(task.id)" @open-job="openSchedule(task.schedule_id)"
      @toggle-unread="toggleTaskUnread(task.id)"
      @toggle-archive="setTaskArchived(task, false)" @rename="renameTask(task, $event)"
      @delete="$emit('delete', task)"
    />
    <div class="mt-3 flex items-center justify-between gap-3">
      <UButton label="Previous" color="neutral" variant="ghost" :disabled="loading || page === 1" @click="page--" />
      <span class="text-xs text-dimmed">Page {{ page }}</span>
      <UButton label="Next" color="neutral" variant="ghost" :disabled="loading || !more" @click="page++" />
    </div>
  </section>
</template>
