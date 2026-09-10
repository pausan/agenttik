<script setup>
/* The Logs pane: the history of the branch the project is on.

   A row is a commit — its subject, then its identity underneath: short hash,
   author and date. Clicking one expands the files it touched; clicking a file
   opens it as that commit changed it. Right-clicking one offers its hash to
   the clipboard.

   Typing filters what has already been fetched, the same subsequence match
   the Tree pane uses, against the subject, the author and the hash together —
   so a hash prefix, a name, or the words of a message all reach the same
   commit. */
import { computed, ref } from "vue";

import { S, copyText, openCommitFile, toggleCommit } from "../store";
import { fuzzy, segments } from "../fuzzy";

/* Each commit is matched once against one string. Building it per keystroke
   is cheaper than three matches per row, and it lets "pau context" find a
   commit by author and subject at the same time. */
const rows = computed(() => {
  const filter = S.logFilter.trim();
  const out = [];
  for (const commit of S.log.commits) {
    if (!filter) {
      out.push({ commit });
      continue;
    }
    const m = fuzzy(`${commit.subject} ${commit.author} ${commit.hash}`, filter);
    if (m) out.push({ commit, score: m.score });
  }
  if (filter) out.sort((a, b) => a.score - b.score);
  return out;
});

/* The subject is highlighted on its own, so the letters that matched the
   author or the hash do not light up arbitrary characters in the title. */
function subjectParts(commit) {
  const filter = S.logFilter.trim();
  if (!filter) return [{ text: commit.subject, hit: false }];
  const m = fuzzy(commit.subject, filter);
  return segments(commit.subject, m?.hits);
}

const filesOf = (hash) => S.logFiles[hash] || null;

/* One menu for the whole list, aimed by the event on its way to the trigger,
   the way Message.vue answers its file links with a single listener: a menu
   component per row would be 500 of them for a full log, and only the row
   under the pointer has anything to offer. A right click that lands beside
   the rows aims at nothing, and the item greys out rather than copying
   whatever was aimed at last. */
const aimed = ref("");
const aim = (e) => (aimed.value = e.target.closest("[data-hash]")?.dataset.hash || "");

const menu = computed(() => [
  {
    label: "Copy hash",
    icon: "i-lucide-copy",
    disabled: !aimed.value,
    onSelect: () => copyText(aimed.value),
  },
]);
</script>

<template>
  <div class="flex min-h-0 flex-col gap-2">
    <div class="flex shrink-0 items-baseline gap-2 px-0.5">
      <UIcon name="i-lucide-git-branch" class="shrink-0 text-dimmed" />
      <span class="truncate font-mono text-xs text-highlighted">
        {{ S.log.branch || (S.log.head ? "detached" : "no branch") }}
      </span>
      <span v-if="S.log.head" class="shrink-0 font-mono text-[11px] text-dimmed">
        {{ S.log.head }}
      </span>
    </div>

    <UInput
      v-model="S.logFilter"
      icon="i-lucide-search"
      placeholder="Filter commits"
      class="w-full shrink-0"
      :ui="{ base: 'text-xs' }"
    />

    <p v-if="!S.log.commits.length" class="px-3 py-5 text-center text-dimmed">
      No commits to show.
    </p>
    <p v-else-if="!rows.length" class="px-3 py-5 text-center text-dimmed">
      Nothing matches “{{ S.logFilter }}”.
    </p>

    <!-- The trigger is the scroller itself: as-child, so it adds no element
         and the pane keeps its one flex column. -->
    <UContextMenu v-else :items="menu">
      <div class="min-h-0 flex-1 overflow-auto" @contextmenu="aim">
        <div v-for="{ commit } in rows" :key="commit.hash" :data-hash="commit.hash">
          <button
            type="button"
            class="w-full rounded-[var(--ui-radius)] px-1.5 py-1 text-left hover:bg-elevated"
            :aria-expanded="S.logOpen === commit.hash"
            @click="toggleCommit(commit.hash)"
          >
            <span class="block truncate text-xs text-highlighted">
              <span v-for="(part, i) in subjectParts(commit)" :key="i" :class="part.hit ? 'text-primary' : ''">{{ part.text }}</span>
            </span>
            <!-- Wraps rather than truncates: the hash, the author and the date
                 need not fit one narrow line, and a panel that hides half of
                 them is worse than one that uses two. -->
            <span class="block font-mono text-[11px] leading-snug text-dimmed tabular-nums">
              {{ commit.hash }} · {{ commit.author }} · {{ commit.date }}
            </span>
          </button>

          <div v-if="S.logOpen === commit.hash" class="mb-1 ml-2 border-l border-default pl-2">
            <p v-if="!filesOf(commit.hash)" class="px-1.5 py-1 text-xs text-dimmed">Loading…</p>
            <p
              v-else-if="!filesOf(commit.hash).length"
              class="px-1.5 py-1 text-xs text-dimmed"
            >
              No files — a merge commit.
            </p>
            <button
              v-for="file in filesOf(commit.hash) || []"
              :key="file.path"
              type="button"
              class="flex w-full select-none items-center gap-1.5 rounded-[var(--ui-radius)] px-1.5 py-0.5 text-left font-mono text-xs hover:bg-elevated hover:text-highlighted"
              :title="file.path"
              @click="openCommitFile(commit.hash, file.path)"
              @dblclick="openCommitFile(commit.hash, file.path, true)"
            >
              <span class="w-8 shrink-0 text-primary">{{ file.status }}</span>
              <span class="path-clip min-w-0 flex-1 truncate"><span>{{ file.path }}</span></span>
              <span v-if="file.binary" class="shrink-0 text-dimmed">bin</span>
              <template v-else>
                <span class="shrink-0 text-success tabular-nums">+{{ file.additions }}</span>
                <span class="shrink-0 text-error tabular-nums">−{{ file.deletions }}</span>
              </template>
            </button>
          </div>
        </div>
      </div>
    </UContextMenu>
  </div>
</template>
