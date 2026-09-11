<script setup>
import { computed, nextTick, ref } from "vue";

import { api } from "../api";
import { debounce } from "../debounce";
import { filterDirs, segments } from "../fuzzy";

/* The picker walks one directory at a time through /api/fs. The text field
   stays the source of truth, so a folder can be typed as well as clicked.
   What is typed after the last "/" never reaches the server: it fuzzy-filters
   the listing, so "~/gh/att" narrows ~/gh down to agenttik.

   One level is fetched at a time rather than a whole tree: a deep home
   directory is far too big to send, and only one level is ever shown. */

const path = defineModel({ type: String, default: "" });
defineProps({ label: { type: String, default: "Folder" } });
const emit = defineEmits(["submit"]);

const hidden = ref(false);
const listing = ref(null);
const filter = ref("");
const error = ref("");
const naming = ref(false); // the New folder row is open
const newName = ref("");
const nameField = ref(null);

let at = ""; // folder currently listed
let key = null; // query that produced `listing`, so filtering does not refetch

const matches = computed(() => (listing.value ? filterDirs(listing.value.dirs, filter.value) : []));

async function browse(to, quiet, narrow) {
  // What the field says now. A listing takes a moment to arrive, and by then
  // the user may have typed on; their text wins over a stale answer.
  const asked = path.value;
  const want = (to || "") + " " + (hidden.value ? "1" : "");
  if (want === key) {
    // Same folder, new filter — the listing we have is still good.
    filter.value = narrow || "";
    if (!quiet) setPath(listing.value.path);
    return;
  }
  let next;
  try {
    const params = new URLSearchParams({ path: to || "" });
    if (hidden.value) params.set("hidden", "1");
    next = await api("GET", "/api/fs?" + params);
  } catch (e) {
    // A half-typed path is not worth an error; only a click deserves one.
    if (!quiet) error.value = e.message;
    return;
  }
  error.value = "";
  key = want;
  listing.value = next;
  // The filter belongs to this listing, so it only applies now that the
  // listing arrived. A half-typed path that resolves to nothing leaves both
  // alone rather than filtering the folder still on screen by its tail.
  filter.value = narrow || "";
  at = next.path;
  if (!quiet && path.value === asked) setPath(next.path);
}

function browseTyped(text) {
  const cut = text.lastIndexOf("/");
  if (cut < 0) return browse(at, true, text);
  return browse(text.slice(0, cut) || "/", true, text.slice(cut + 1));
}

// setPath shows a folder the user clicked, with a trailing slash so the next
// keystroke starts filtering inside it.
function setPath(p) {
  path.value = p.endsWith("/") ? p : p + "/";
}

const typed = debounce((text) => browseTyped(text), 150);

// Enter takes the best match, so a folder can be reached by typing alone.
// With nothing left to filter it adds the project instead.
async function onEnter() {
  const text = path.value.trim();
  if (!text) return;
  if (!text.endsWith("/")) {
    typed.cancel();
    await browseTyped(text);
    if (matches.value.length) return browse(matches.value[0].dir.path);
  }
  emit("submit");
}

/* New folder makes one folder in the folder on screen and walks into it, so
   the picker ends up on it the same way clicking an existing one would. A
   project can then be started somewhere that did not exist a moment ago
   without leaving for a shell. */
async function startNaming() {
  naming.value = true;
  newName.value = "";
  await nextTick();
  nameField.value?.inputRef?.focus();
}

async function create() {
  const name = newName.value.trim();
  if (!name) return (naming.value = false);
  let dir;
  try {
    dir = await api("POST", "/api/fs/dir", { path: at, name });
  } catch (e) {
    error.value = e.message; // the row stays open on the name to change
    return;
  }
  naming.value = false;
  key = null; // the listing in hand predates the folder just made
  browse(dir.path);
}

function toggleHidden() {
  key = null; // the listing changes shape, so it has to be fetched again
  browseTyped(path.value);
}

/* open re-lists a folder: the one asked for, or else the one in view, which
   may have changed since last time. */
function open(to) {
  key = null;
  naming.value = false;
  browse(to || at);
}

defineExpose({ open });
</script>

<template>
  <div>
    <label class="mb-3 block text-xs font-medium text-muted">
      {{ label }}
      <UInput
        v-model="path"
        class="mt-1 w-full font-normal"
        placeholder="~/code/myproject"
        autocomplete="off"
        autofocus
        @update:model-value="typed"
        @keydown.enter.prevent="onEnter"
      />
    </label>

    <div class="mb-3 overflow-hidden rounded-[var(--ui-radius)] inset-ring inset-ring-accented">
      <div class="flex items-center border-b border-default bg-muted">
        <div class="flex min-w-0 flex-1 items-center gap-px overflow-x-auto px-1.5 py-1 whitespace-nowrap [scrollbar-width:none]">
          <UButton
            v-if="listing?.home"
            color="neutral"
            variant="ghost"
            size="xs"
            icon="i-lucide-house"
            :title="listing.home"
            @click="browse(listing.home)"
          />
          <template v-for="(c, i) in listing?.crumbs || []" :key="c.path">
            <span v-if="i > 1" class="shrink-0 text-xs text-dimmed">/</span>
            <UButton
              color="neutral"
              variant="ghost"
              size="xs"
              class="max-w-40 shrink-0 truncate"
              :class="i === (listing.crumbs.length - 1) ? 'font-medium text-highlighted' : ''"
              :label="c.name"
              @click="browse(c.path)"
            />
          </template>
        </div>
        <UButton
          color="neutral"
          variant="ghost"
          size="xs"
          class="mr-1 shrink-0"
          icon="i-lucide-folder-plus"
          title="New folder"
          aria-label="New folder"
          :disabled="!listing"
          @click="startNaming"
        />
      </div>

      <div class="h-[190px] overflow-auto p-1">
        <div v-if="naming" class="flex items-center gap-1.5 px-1.5 py-1">
          <span class="shrink-0 text-dimmed">▸</span>
          <UInput
            ref="nameField"
            v-model="newName"
            class="w-full font-normal"
            size="xs"
            placeholder="new folder name"
            autocomplete="off"
            @keydown.enter.stop.prevent="create"
            @keydown.esc.stop.prevent="naming = false"
            @blur="naming = false"
          />
        </div>

        <button
          v-if="listing?.parent && !filter && !naming"
          type="button"
          class="flex w-full items-center gap-1.5 rounded-[var(--ui-radius)] px-1.5 py-1 text-left hover:bg-elevated hover:text-highlighted"
          @click="browse(listing.parent)"
        >
          <span class="shrink-0 text-dimmed">↑</span>
          <span class="truncate">..</span>
        </button>

        <button
          v-for="m in matches"
          :key="m.dir.path"
          type="button"
          class="flex w-full items-center gap-1.5 rounded-[var(--ui-radius)] px-1.5 py-1 text-left hover:bg-elevated hover:text-highlighted"
          @click="browse(m.dir.path)"
        >
          <span class="shrink-0 text-dimmed">▸</span>
          <span class="truncate">
            <span
              v-for="(seg, i) in segments(m.dir.name, m.hits)"
              :key="i"
              :class="seg.hit ? 'font-semibold text-primary' : ''"
            >{{ seg.text }}</span>
          </span>
        </button>

        <p v-if="error" class="p-3 text-center text-xs text-error">{{ error }}</p>
        <p v-else-if="!matches.length" class="p-3 text-center text-xs text-dimmed">
          {{ filter ? `No folder matches "${filter}".` : "No folders here." }}
        </p>
        <p v-if="listing?.truncated" class="p-3 text-center text-xs text-dimmed">
          More folders than shown.
        </p>
      </div>
    </div>

    <UCheckbox
      v-model="hidden"
      label="Show hidden folders"
      :ui="{ label: 'text-muted font-normal' }"
      @update:model-value="toggleHidden"
    />
  </div>
</template>
