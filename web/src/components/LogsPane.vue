<script setup>
/* The Logs pane: the history of the branch the project is on.

   A row is a commit — its subject, then its identity underneath: short hash,
   author, date, and how much it moved. Clicking one expands the files it
   touched; clicking a file opens it as that commit changed it.

   Typing filters what has already been fetched, the same subsequence match
   the Tree pane uses, against the subject, the author and the hash together —
   so a hash prefix, a name, or the words of a message all reach the same
   commit. */
import { computed } from "vue";

import { S, openCommitFile, toggleCommit } from "../store";
import { fuzzy, segments } from "../fuzzy";
import { nf } from "../api";

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

function meta(commit) {
  return [
    commit.author,
    commit.date,
    `${nf.format(commit.files)} ${commit.files === 1 ? "file" : "files"}`,
    `${nf.format(commit.changes)} ${commit.changes === 1 ? "change" : "changes"}`,
  ].join(" · ");
}
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

    <div v-else class="min-h-0 flex-1 overflow-auto">
      <div v-for="{ commit } in rows" :key="commit.hash">
        <button
          type="button"
          class="w-full rounded-[var(--ui-radius)] px-1.5 py-1 text-left hover:bg-elevated"
          :aria-expanded="S.logOpen === commit.hash"
          @click="toggleCommit(commit.hash)"
        >
          <span class="block truncate text-xs text-highlighted">
            <span v-for="(part, i) in subjectParts(commit)" :key="i" :class="part.hit ? 'text-primary' : ''">{{ part.text }}</span>
          </span>
          <!-- Wraps rather than truncates: the hash, the author, the date and
               the size of the change do not fit one narrow line, and a panel
               that hides half of them is worse than one that uses two. -->
          <span class="block font-mono text-[11px] leading-snug text-dimmed tabular-nums">
            {{ commit.hash }} · {{ meta(commit) }}
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
  </div>
</template>
