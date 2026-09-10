<script setup>
/* The projects that have been put away. Archiving takes a project out of the
   sidebar without deleting anything, so this is where it waits and the only
   way back. Deleting is not offered here: it needs the project in front of
   you and a confirmation, which is what its Options pane is for. */
import { computed, watchEffect } from "vue";

import { S, setProjectArchived } from "../../store";
import { ago, isoDate } from "../../api";
import { fuzzyAny } from "../../fuzzy";

const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count"]);

/* A filter that names the section keeps every row; otherwise the projects
   themselves are matched, by name or by folder. */
const rows = computed(() =>
  fuzzyAny(["Archived projects", "restore"], props.filter) !== null
    ? S.archivedProjects
    : S.archivedProjects.filter((p) => fuzzyAny([p.name, p.path], props.filter) !== null),
);

watchEffect(() => emit("count", rows.value.length));
</script>

<template>
  <!-- With nothing archived there are no rows to match, so the section is
       still drawn unfiltered: that is where the empty state belongs. -->
  <section v-if="rows.length || !filter">
    <div class="mb-0.5 font-semibold text-highlighted">Archived projects</div>
    <p class="mb-3.5 text-xs text-dimmed">
      An archived project leaves the sidebar and stops running its schedules. Nothing is deleted —
      its tasks, its history and its place in the order all come back with it.
    </p>

    <p v-if="!rows.length" class="text-xs text-dimmed">
      Nothing archived. A project is archived from its Options pane.
    </p>

    <div>
      <div
        v-for="p in rows"
        :key="p.id"
        class="flex items-center gap-2.5 py-2 not-first:border-t not-first:border-default"
      >
        <div class="min-w-0 flex-1">
          <div class="truncate text-[13px] font-medium text-highlighted">{{ p.name }}</div>
          <div class="truncate font-mono text-xs text-dimmed">{{ p.path }}</div>
        </div>
        <span class="shrink-0 text-xs text-dimmed" :title="isoDate(p.archived_at)">
          {{ ago(p.archived_at) }}
        </span>
        <UButton
          size="xs"
          color="neutral"
          variant="subtle"
          icon="i-lucide-archive-restore"
          label="Restore"
          :title="`Bring ${p.name} back to the sidebar`"
          @click="setProjectArchived(p, false)"
        />
      </div>
    </div>
  </section>
</template>
