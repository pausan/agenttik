<script setup>
import { computed, ref, watch } from "vue";
import { api } from "../api";
import { S, currentProjectID, refreshChanged, refreshLog } from "../store";
import FileList from "./FileList.vue";

const emit = defineEmits(["show-in-tree"]);
const staged = computed(() => S.changed.filter((f) => f.staged));
const unstaged = computed(() => S.changed.filter((f) => f.unstaged));
const expanded = ref(true);
const message = ref("");
const busy = ref(false);
const error = ref("");
const generating = ref(false);
let contextVersion = 0;
watch(() => [currentProjectID(), S.repository], () => {
  contextVersion++;
  message.value = "";
  error.value = "";
});
async function generateMessage() {
  if (busy.value || !staged.value.length) return;
  const version = contextVersion;
  busy.value = generating.value = true;
  error.value = "";
  try {
    const result = await api("POST", `/api/projects/${currentProjectID()}/commit-message?repo=${encodeURIComponent(S.repository)}`, {
    });
    if (version === contextVersion) message.value = result.message;
  } catch (e) {
    if (version === contextVersion) error.value = e.message;
  } finally {
    busy.value = generating.value = false;
  }
}
async function act(action, path) {
  if (busy.value) return;
  const id = currentProjectID();
  const repo = S.repository;
  busy.value = true;
  error.value = "";
  try {
    await api("POST", `/api/projects/${id}/${action}?repo=${encodeURIComponent(repo)}`,
      action === "commit" ? { message: message.value } : { path });
    if (id === currentProjectID() && repo === S.repository) {
      if (action === "commit") message.value = "";
      await Promise.all([refreshChanged(), refreshLog()]);
    }
  } catch (e) {
    if (id === currentProjectID() && repo === S.repository) error.value = e.message;
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="space-y-3">
    <div class="space-y-2">
      <div class="flex flex-col items-end gap-2">
        <textarea v-model="message" :disabled="generating" aria-label="Commit message" placeholder="Commit message" rows="2" class="w-full min-h-14 resize-y rounded-md border border-default bg-default p-2 font-mono text-xs" />
        <div class="flex w-full items-center gap-1">
          <span class="mr-auto min-w-0 truncate text-xs text-muted" aria-label="Current branch" :title="S.log.branch">{{ S.log.branch }}</span>
          <UButton class="shrink-0" icon="i-lucide-sparkle" aria-label="Generate commit message" title="Generate or rewrite the message from staged changes using the model in Settings → Models → Automatic actions. Does not commit." size="sm" variant="ghost" color="neutral" :loading="generating" :disabled="busy || !staged.length" @click="generateMessage" />
          <UButton icon="i-lucide-git-commit-horizontal" label="Commit" title="Commit staged changes with this message" size="sm" class="shrink-0" :loading="busy && !generating" :disabled="busy || !staged.length || !message.trim()" @click="act('commit')" />
        </div>
      </div>
      <p v-if="error" role="alert" class="text-xs text-error">{{ error }}</p>
    </div>
    <section>
      <div class="flex items-center justify-between">
        <button type="button" class="flex items-center gap-1 text-xs font-semibold" :aria-expanded="expanded" @click="expanded = !expanded">
          <UIcon :name="expanded ? 'i-lucide-chevron-down' : 'i-lucide-chevron-right'" />
          Staged <span class="text-dimmed">{{ staged.length }}</span>
        </button>
        <UButton icon="i-lucide-minus" aria-label="Unstage all files" title="Unstage all files" size="xs" variant="ghost" color="neutral" :disabled="busy || !staged.length" @click="act('unstage')" />
      </div>
      <FileList allow-revert :disabled="busy" v-if="expanded" :files="staged" compact-empty empty="No staged files." @show-in-tree="emit('show-in-tree', $event)">
        <template #actions="{ file }">
          <UButton icon="i-lucide-minus" :aria-label="`Unstage ${file.path}`" :title="`Unstage ${file.path}`" size="xs" variant="ghost" color="neutral" :disabled="busy" @click="act('unstage', file.path)" />
        </template>
      </FileList>
    </section>
    <section>
      <div class="flex items-center justify-between">
        <span class="text-xs font-semibold">Changes <span class="text-dimmed">{{ unstaged.length }}</span></span>
        <UButton icon="i-lucide-plus" aria-label="Stage all files" title="Stage all files" size="xs" variant="ghost" color="neutral" :disabled="busy || !unstaged.length" @click="act('stage')" />
      </div>
      <FileList allow-revert :disabled="busy" :files="unstaged" empty="No edited files." @show-in-tree="emit('show-in-tree', $event)">
        <template #actions="{ file }">
          <UButton icon="i-lucide-plus" :aria-label="`Stage ${file.path}`" :title="`Stage ${file.path}`" size="xs" variant="ghost" color="neutral" :disabled="busy" @click="act('stage', file.path)" />
        </template>
      </FileList>
    </section>
  </div>
</template>
