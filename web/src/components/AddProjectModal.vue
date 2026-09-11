<script setup>
import { computed, nextTick, reactive, ref, watch } from "vue";

import { api } from "../api";
import { addProject } from "../store";
import { SEGMENTED } from "../ui";
import FolderPicker from "./FolderPicker.vue";

/* A project is one folder either way. **Folder** takes one that is already
   there. **Git repos** takes a folder to hold several checkouts and clones
   them into it, so the agent sees every repository under the one working
   directory. Nothing about the project itself differs afterwards, which is
   why neither mode is stored: the clones are just what the folder contains. */

const open = defineModel("open", { type: Boolean, default: false });

const MODES = [
  { label: "Folder", value: "folder" },
  { label: "Git repos", value: "repos" },
];

const mode = ref("folder");
const path = ref("");
const name = ref("");
const error = ref("");
const picker = ref(null);

const url = ref("");
const urlField = ref(null);
const repos = ref([]); // one row per clone asked for, in the order asked

const cloning = computed(() => repos.value.some((r) => r.state === "cloning"));
const cloned = computed(() => repos.value.filter((r) => r.state === "cloned").length);

watch(open, async (on) => {
  if (!on) return;
  mode.value = "folder";
  path.value = "";
  name.value = "";
  url.value = "";
  error.value = "";
  repos.value = [];
  await nextTick();
  picker.value?.open();
});

/* A rejected folder — one already added, or one that is not a directory —
   leaves the dialog open on the field that has to change, so the reason
   belongs beside that field rather than in a toast behind the dialog. Moving
   anywhere in the picker clears it: the message was about the old folder. */
watch(path, () => (error.value = ""));

/* Cloning happens as each repository is named, not when the project is added:
   a clone takes as long as it takes, and its failures — a typo, a private
   repository, no network — are worth seeing one at a time while the dialog is
   still open. Clones run alongside each other; each row carries its own
   outcome, so a slow one never holds up the next. */
async function clone() {
  const at = path.value.trim();
  const remote = url.value.trim();
  if (!remote) return;
  if (!at) {
    error.value = "Pick the folder to clone into first.";
    return;
  }
  const row = reactive({ url: remote, at, name: "", state: "cloning", error: "" });
  repos.value.push(row);
  url.value = "";
  try {
    const dir = await api("POST", "/api/fs/clone", { path: at, url: remote });
    row.name = dir.name;
    row.state = "cloned";
    // The browser above is listing the folder this landed in, and was drawn
    // before it existed. Re-list it so the clone shows up as the folder it is.
    if (path.value.trim() === at) picker.value?.open();
  } catch (e) {
    row.state = "failed";
    row.error = e?.message || String(e);
  }
}

// Only a failed row is dropped. A clone that worked is on disk and inside the
// folder the project is about to point at, so forgetting it here would not
// take it away — it would only make the list lie.
function forget(row) {
  repos.value = repos.value.filter((r) => r !== row);
}

async function add() {
  if (!path.value.trim() || cloning.value) return;
  if (mode.value === "repos" && !cloned.value) {
    error.value = "Clone at least one repository, or add the folder on its own.";
    return;
  }
  try {
    await addProject(path.value.trim(), name.value.trim());
    open.value = false;
  } catch (e) {
    error.value = e?.message || String(e);
  }
}

// In Git repos mode the folder picker's Enter belongs to the repository field
// below it, which is where the next thing to say is; adding the project is
// the button's job once the clones are in.
function submit() {
  if (mode.value !== "repos") return add();
  nextTick(() => urlField.value?.inputRef?.focus());
}
</script>

<template>
  <UModal v-model:open="open" title="Add project" :ui="{ content: 'max-w-lg' }">
    <template #body>
      <UTabs
        v-model="mode"
        :items="MODES"
        :content="false"
        size="sm"
        class="mb-3"
        :ui="SEGMENTED"
      />

      <FolderPicker
        ref="picker"
        v-model="path"
        :label="mode === 'repos' ? 'Base folder' : 'Folder'"
        @submit="submit"
      />

      <template v-if="mode === 'repos'">
        <label class="block text-xs font-medium text-muted">
          Repository
          <div class="mt-1 flex gap-2">
            <UInput
              ref="urlField"
              v-model="url"
              class="w-full font-normal"
              placeholder="https://github.com/org/repo.git"
              autocomplete="off"
              @keydown.enter.prevent="clone"
            />
            <UButton
              color="neutral"
              variant="subtle"
              label="Clone"
              :disabled="!url.trim()"
              @click="clone"
            />
          </div>
        </label>

        <ul v-if="repos.length" class="mt-2 max-h-28 space-y-1 overflow-auto text-xs">
          <li v-for="(r, i) in repos" :key="i">
            <div class="flex items-center gap-1.5">
              <UIcon
                v-if="r.state === 'cloning'"
                name="i-lucide-loader-circle"
                class="shrink-0 animate-spin text-dimmed"
              />
              <UIcon
                v-else-if="r.state === 'cloned'"
                name="i-lucide-check"
                class="shrink-0 text-success"
              />
              <UIcon v-else name="i-lucide-x" class="shrink-0 text-error" />

              <span
                class="truncate"
                :class="r.state === 'cloned' ? 'text-highlighted' : 'text-muted'"
              >{{ r.state === "cloned" ? r.name : r.url }}</span>
              <UButton
                v-if="r.state === 'failed'"
                color="neutral"
                variant="ghost"
                size="xs"
                icon="i-lucide-x"
                class="ml-auto shrink-0"
                :aria-label="`Forget ${r.url}`"
                @click="forget(r)"
              />
            </div>
            <p v-if="r.state === 'failed'" class="pl-5 text-error">{{ r.error }}</p>
          </li>
        </ul>
        <p v-else class="mt-2 text-xs text-dimmed">
          Each repository is cloned into the base folder as you add it.
        </p>
      </template>

      <label class="mt-3 block text-xs font-medium text-muted">
        Name
        <UInput
          v-model="name"
          class="mt-1 w-full font-normal"
          placeholder="defaults to the folder name"
        />
      </label>
      <p v-if="error" class="mt-3 text-xs text-error">{{ error }}</p>
    </template>
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton color="neutral" variant="ghost" label="Cancel" @click="open = false" />
        <UButton
          label="Add"
          :disabled="cloning"
          :title="cloning ? 'Wait for the clones to finish.' : ''"
          @click="add"
        />
      </div>
    </template>
  </UModal>
</template>
