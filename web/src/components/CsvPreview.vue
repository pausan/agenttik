<script setup>
import { computed, ref, watch } from "vue";
import { nextCSVSort, parseCSV, sortCSVRows } from "../csv-preview";

const props = defineProps({ text: { type: String, required: true } });
const data = computed(() => parseCSV(props.text));
const sort = ref({ column: null, direction: "none" });
const page = ref(0);
// Bound DOM work by both rows and cells while keeping every column visible.
const pageSize = computed(() => Math.max(1, Math.min(200, Math.floor(10000 / (data.value.columns || 1)))));
const ordered = computed(() => sortCSVRows(data.value.rows, sort.value));
const from = computed(() => page.value * pageSize.value);
const visible = computed(() => ordered.value.slice(from.value, from.value + pageSize.value));
watch(() => props.text, () => { sort.value = { column: null, direction: "none" }; page.value = 0; });

function sortBy(column) {
  sort.value = nextCSVSort(sort.value, column);
  page.value = 0;
}
const heading = (column) => data.value.header[column] || `Column ${column + 1}`;
</script>

<template>
  <div class="px-5 py-4 text-sm">
    <p v-if="!data.columns" class="text-dimmed">Empty CSV file.</p>
    <template v-else>
      <div v-if="ordered.length > pageSize" class="sticky left-5 flex w-fit items-center gap-2 pb-3">
        <UButton size="xs" color="neutral" variant="ghost" :disabled="!page" @click="page--">Previous page</UButton>
        <UButton size="xs" color="neutral" variant="ghost" :disabled="from + pageSize >= ordered.length" @click="page++">Next page</UButton>
        <span class="text-xs text-dimmed">Rows {{ from + 1 }}–{{ Math.min(from + pageSize, ordered.length) }} of {{ ordered.length }}</span>
      </div>
      <table class="w-full border-separate border-spacing-0" aria-label="CSV preview">
        <thead class="sticky top-0 z-10">
          <tr>
            <th v-for="column in data.columns" :key="column" scope="col"
              class="border-b border-r border-default bg-default p-0 text-left font-semibold"
              :aria-sort="sort.column === column - 1 ? sort.direction : 'none'">
              <button type="button" class="flex w-full items-center gap-2 whitespace-pre px-3 py-2 text-left hover:bg-elevated"
                :title="'Click to sort ascending, descending, then restore file order'" @click="sortBy(column - 1)">
                {{ heading(column - 1) }}
                <span class="text-dimmed" aria-hidden="true">{{ sort.column === column - 1 ? (sort.direction === 'ascending' ? '↑' : '↓') : '↕' }}</span>
              </button>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in visible" :key="row.index" class="hover:bg-elevated">
            <td v-for="column in data.columns" :key="column" class="whitespace-pre border-b border-r border-default px-3 py-2 align-top">{{ row.cells[column - 1] ?? '' }}</td>
          </tr>
        </tbody>
      </table>
    </template>
  </div>
</template>
