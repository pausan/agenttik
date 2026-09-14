<script setup>
import { computed, ref, watch } from "vue";
import { api } from "../api";
import { S, currentProjectID, refreshChanged, refreshLog } from "../store";
import FileList from "./FileList.vue";

const emit = defineEmits(["show-in-tree"]);
const staged = computed(() => S.changed.filter((f) => f.staged));
const unstaged = computed(() => S.changed.filter((f) => f.unstaged));
const expanded = ref(true);
const composing = ref(false);
const editor = ref(null);
const message = ref("");
const busy = ref(false);
const error = ref("");
watch(() => [currentProjectID(), S.repository], () => {
  message.value = "";
  error.value = "";
  composing.value = false;
});
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
      if (action === "commit") {
        message.value = "";
        composing.value = false;
      }
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
    <section>
      <div class="flex items-center justify-between">
        <button type="button" class="flex items-center gap-1 text-xs font-semibold" :aria-expanded="expanded" @click="expanded = !expanded">
          <UIcon :name="expanded ? 'i-lucide-chevron-down' : 'i-lucide-chevron-right'" />
          Staged <span class="text-dimmed">{{ staged.length }}</span>
        </button>
        <UButton icon="i-lucide-arrow-down-to-line" aria-label="Unstage all files" title="Unstage all files" size="xs" variant="ghost" color="neutral" :disabled="busy || !staged.length" @click="act('unstage')" />
      </div>
      <FileList v-if="expanded" :files="staged" empty="No staged files." @show-in-tree="emit('show-in-tree', $event)">
        <template #actions="{ file }">
          <UButton icon="i-lucide-arrow-down-to-line" :aria-label="`Unstage ${file.path}`" :title="`Unstage ${file.path}`" size="xs" variant="ghost" color="neutral" :disabled="busy" @click="act('unstage', file.path)" />
        </template>
      </FileList>
    </section>
    <div class="space-y-2">
      <textarea v-model="message" aria-label="Commit message" placeholder="Commit message" rows="2" class="w-full resize-none rounded-md border border-default bg-default p-2 font-mono text-xs" @focus="composing = true" />
      <UButton label="Commit" size="sm" :disabled="busy || !staged.length || !message.trim()" @click="act('commit')" />
      <p v-if="error && !composing" role="alert" class="text-xs text-error">{{ error }}</p>
    </div>
    <section>
      <div class="flex items-center justify-between">
        <span class="text-xs font-semibold">Changes <span class="text-dimmed">{{ unstaged.length }}</span></span>
        <UButton icon="i-lucide-arrow-up-from-line" aria-label="Stage all files" title="Stage all files" size="xs" variant="ghost" color="neutral" :disabled="busy || !unstaged.length" @click="act('stage')" />
      </div>
      <FileList :files="unstaged" empty="No edited files." @show-in-tree="emit('show-in-tree', $event)">
        <template #actions="{ file }">
          <UButton icon="i-lucide-arrow-up-from-line" :aria-label="`Stage ${file.path}`" :title="`Stage ${file.path}`" size="xs" variant="ghost" color="neutral" :disabled="busy" @click="act('stage', file.path)" />
        </template>
      </FileList>
    </section>
    <UModal v-model:open="composing" title="Commit changes" :ui="{ content: 'commit-composer' }" :content="{ onOpenAutoFocus: (e) => { e.preventDefault(); editor?.focus(); }, onCloseAutoFocus: (e) => e.preventDefault() }">
      <template #body>
        <form class="space-y-3" @submit.prevent="act('commit')">
          <textarea ref="editor" v-model="message" aria-label="Commit message" placeholder="Commit message" rows="7" cols="72" class="w-full resize-y rounded-md border border-default bg-default p-2 font-mono text-sm" />
          <p v-if="error" role="alert" class="text-xs text-error">{{ error }}</p>
          <UButton type="submit" label="Commit" :loading="busy" :disabled="busy || !staged.length || !message.trim()" />
        </form>
      </template>
    </UModal>
  </div>
</template>

<style>
.commit-composer {
  font-family: var(--font-mono);
  font-size: 0.875rem;
  width: calc(72ch + 5rem);
  max-width: calc(100vw - 2rem);
}
</style>
