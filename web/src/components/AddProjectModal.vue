<script setup>
import { computed, nextTick, reactive, ref, watch } from "vue";
import { api } from "../api";
import { addProject } from "../store";
import { SEGMENTED } from "../ui";
import FolderPicker from "./FolderPicker.vue";

const open = defineModel("open", { type: Boolean, default: false });
const MODES = [{ label: "Folder", value: "folder" }, { label: "Git repos", value: "repos" }];
const mode = ref("folder");
const path = ref("");
const name = ref("");
const error = ref("");
const picker = ref(null);
const minimized = ref(false);
const repos = ref([]);
const cloning = computed(() => repos.value.some((r) => r.state === "cloning"));
const checking = computed(() => repos.value.some((r) => r.state === "checking"));
const broken = computed(() => repos.value.some((r) => r.state === "invalid" || r.state === "failed"));
const ready = computed(() => repos.value.some((r) => r.state === "valid" || r.state === "cloned"));
const current = computed(() => repos.value.findIndex((r) => r.state === "cloning"));
const total = computed(() => repos.value.filter((r) => r.state !== "empty").length);
const canAdd = computed(() => !path.value.trim() || cloning.value ? false :
  mode.value === "folder" || ready.value && !checking.value && !broken.value);

function addRepo() {
  repos.value.push(reactive({ url: "", state: "empty", error: "", timer: null }));
}

function reset() {
  for (const row of repos.value) clearTimeout(row.timer);
  mode.value = "folder";
  path.value = "";
  name.value = "";
  error.value = "";
  repos.value = [];
  addRepo();
}

watch(open, async (on) => {
  if (!on) return;
  if (minimized.value) {
    minimized.value = false;
    return;
  }
  reset();
  await nextTick();
  picker.value?.open();
});
watch(path, () => (error.value = ""));

// A check only asks git for remote refs, never repository history. Debouncing
// stops a request from starting for every character typed into a row.
function scheduleCheck(row) {
  clearTimeout(row.timer);
  const remote = row.url.trim();
  row.error = "";
  if (!remote) {
    row.state = "empty";
    return;
  }
  row.state = "checking";
  row.timer = setTimeout(() => check(row, remote), 350);
}

async function check(row, remote) {
  try {
    await api("POST", "/api/fs/remote", { url: remote });
    if (row.url.trim() !== remote) return;
    row.state = "valid";
    if (repos.value.at(-1) === row) addRepo();
  } catch (e) {
    if (row.url.trim() !== remote) return;
    row.state = "invalid";
    row.error = e?.message || String(e);
  }
}

function paste(row, event) {
  const remotes = event.clipboardData?.getData("text").trim().split(/\s+/).filter(Boolean) || [];
  if (remotes.length < 2) return;
  event.preventDefault();
  const index = repos.value.indexOf(row);
  const rows = remotes.map((url) => reactive({ url, state: "empty", error: "", timer: null }));
  repos.value.splice(index, 1, ...rows);
  nextTick(() => rows.forEach(scheduleCheck));
}

function forget(row) {
  clearTimeout(row.timer);
  repos.value = repos.value.filter((r) => r !== row);
  if (!repos.value.some((r) => r.state === "empty")) addRepo();
}

async function add() {
  if (!canAdd.value) return;
  if (mode.value === "repos") {
    for (const row of repos.value) {
      if (row.state !== "valid") continue;
      row.state = "cloning";
      try {
        const dir = await api("POST", "/api/fs/clone", { path: path.value.trim(), url: row.url.trim() });
        row.name = dir.name;
        row.state = "cloned";
        picker.value?.open();
      } catch (e) {
        row.state = "failed";
        row.error = e?.message || String(e);
      }
    }
    if (broken.value) {
      error.value = "Fix or remove every failed repository before adding the project.";
      return;
    }
  }
  try {
    await addProject(path.value.trim(), name.value.trim());
    open.value = false;
  } catch (e) {
    error.value = e?.message || String(e);
  }
}

function minimize() {
  minimized.value = true;
  open.value = false;
}

function updateOpen(next) {
  if (!next && cloning.value) return minimize();
  open.value = next;
}

function submit() {
  if (mode.value !== "repos") return add();
}
</script>

<template>
  <UModal
    :open="open"
    title="Add project"
    :close="cloning ? { icon: 'i-lucide-minus', title: 'Minimize cloning', 'aria-label': 'Minimize cloning' } : true"
    :ui="{ content: 'max-w-lg' }"
    @update:open="updateOpen"
  >
    <template #body>
      <UTabs v-model="mode" :items="MODES" :content="false" size="sm" class="mb-3" :ui="SEGMENTED" />
      <FolderPicker ref="picker" v-model="path" :label="mode === 'repos' ? 'Base folder' : 'Folder'" @submit="submit" />

      <template v-if="mode === 'repos'">
        <div class="space-y-2">
          <label v-for="(row, i) in repos" :key="row" class="block text-xs font-medium text-muted">
            <span v-if="i === 0">Git repositories</span>
            <UInput
              v-model="row.url"
              class="mt-1 w-full font-normal"
              placeholder="https://github.com/org/repo or git@github.com:org/repo.git"
              autocomplete="off"
              :disabled="cloning || row.state === 'cloned'"
              @input="scheduleCheck(row)"
              @paste="paste(row, $event)"
            >
              <template #trailing>
                <UIcon v-if="row.state === 'checking'" name="i-lucide-loader-circle" class="animate-spin text-dimmed" />
                <UIcon v-else-if="row.state === 'valid' || row.state === 'cloned'" name="i-lucide-check" class="text-success" />
                <UIcon v-else-if="row.state === 'invalid' || row.state === 'failed'" name="i-lucide-x" class="text-error" />
              </template>
            </UInput>
            <p v-if="row.error" class="mt-1 text-error">{{ row.error }}</p>
            <UButton
              v-if="row.state !== 'cloned'"
              color="neutral"
              variant="ghost"
              size="xs"
              icon="i-lucide-trash-2"
              class="mt-1"
              :disabled="cloning"
              :aria-label="'Delete ' + (row.url || 'repository')"
              @click="forget(row)"
            />
          </label>
        </div>
        <p class="mt-2 text-xs text-dimmed">
          GitHub and GitLab web links use SSH automatically. Paste several URLs to add rows.
        </p>
        <p v-if="cloning" class="mt-2 text-xs text-muted">
          Cloning {{ current + 1 }} of {{ total }} repositories
        </p>
      </template>

      <label class="mt-3 block text-xs font-medium text-muted">
        Name
        <UInput v-model="name" class="mt-1 w-full font-normal" placeholder="defaults to the folder name" />
      </label>
      <p v-if="error" class="mt-3 text-xs text-error">{{ error }}</p>
    </template>
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton color="neutral" variant="ghost" :label="cloning ? 'Minimize' : 'Cancel'" @click="cloning ? minimize() : (open = false)" />
        <UButton label="Add project" :disabled="!canAdd" :title="checking ? 'Wait for repository checks to finish.' : broken ? 'Fix or remove every failed repository.' : ''" @click="add" />
      </div>
    </template>
  </UModal>

  <UButton
    v-if="minimized && cloning"
    color="neutral"
    class="fixed bottom-4 right-4 z-50 shadow-lg"
    icon="i-lucide-loader-circle"
    :label="'Cloning ' + (current + 1) + ' of ' + total"
    @click="open = true"
  />
</template>
