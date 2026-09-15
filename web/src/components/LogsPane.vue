<script setup>
/* Commit history with an optional topology graph and branch/tag refs. */
import { computed, ref, watch } from "vue";

import { S, copyText, openCommitFile, toggleCommit, currentProjectID, refreshLog, refreshChanged, refreshTree } from "../store";
import { api } from "../api";
import { fuzzy, segments } from "../fuzzy";
import { commitGraph, graphColor, graphX, graphPath } from "../commitGraph";

watch(() => S.logGraph, () => refreshLog());

const emit = defineEmits(["show-in-tree"]);

const branchSearch = ref("");
const busy = ref("");
const error = ref("");
const notice = ref("");
const operation = ref("merge");
const target = ref("");
const targets = computed(() => (S.log.branches || []).filter((name) => name !== S.log.branch));
const blocked = computed(() => !!busy.value || !!S.log.operation);
watch(() => [S.log.branch, S.log.branches], () => {
  if (!targets.value.includes(target.value)) target.value = "";
});
const branches = computed(() => {
  const names = S.log.branches || [];
  const query = branchSearch.value.trim();
  if (!query) return names;
  return names.map((name) => ({ name, match: fuzzy(name, query) }))
    .filter((item) => item.match)
    .sort((a, b) => a.match.score - b.match.score)
    .map((item) => item.name);
});
watch(() => [currentProjectID(), S.repository], () => {
  branchSearch.value = "";
  target.value = "";
  error.value = "";
  notice.value = "";
});
const actions = [
  { action: "pull", icon: "i-lucide-arrow-down", label: "Pull current branch" },
  { action: "push", icon: "i-lucide-arrow-up", label: "Push current branch" },
  { action: "clean", icon: "i-lucide-brush-cleaning", label: "Clean local branches merged into main or master" },
];
async function act(action, branch = S.log.branch) {
  if (busy.value || (!branch && !["continue", "abort"].includes(action))) return;
  const id = currentProjectID();
  const repo = S.repository;
  const stillHere = () => id === currentProjectID() && repo === S.repository;
  busy.value = action;
  error.value = "";
  notice.value = "";
  try {
    const result = await api("POST", `/api/projects/${id}/branches/${action}?repo=${encodeURIComponent(repo)}`, { branch, ...(["merge", "rebase"].includes(action) ? { target: target.value } : {}) });
    if (stillHere()) {
      branchSearch.value = "";
      if (result?.message) notice.value = result.message;
      if (action === "clean") notice.value = result.deleted.length
        ? `Removed: ${result.deleted.join(", ")}` : "No merged local branches to remove.";
    }
  } catch (e) {
    if (stillHere()) error.value = e.message;
  } finally {
    // Refresh failures too: Git may have completed part of an operation.
    if (stillHere()) await Promise.all([refreshLog(), refreshChanged(), refreshTree()]);
    busy.value = "";
  }
}

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
  if (filter && !S.logGraph) out.sort((a, b) => a.score - b.score);
  return out;
});

const graph = computed(() => S.logGraph
  ? commitGraph(S.log.commits, new Set(rows.value.map(({ commit }) => commit.hash)))
  : null);

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
const aimed = ref({ hash: "", path: "" });
function aim(e) {
  aimed.value = {
    hash: e.target.closest("[data-hash]")?.dataset.hash || "",
    path: e.target.closest("[data-path]")?.dataset.path || "",
  };
}

const menu = computed(() => [
  {
    label: "Copy hash",
    icon: "i-lucide-copy",
    disabled: !aimed.value.hash,
    onSelect: () => copyText(aimed.value.hash),
  },
  {
    label: "Show in tree",
    icon: "i-lucide-folder-tree",
    disabled: !aimed.value.path,
    onSelect: () => emit("show-in-tree", aimed.value.path),
  },
]);
</script>

<template>
  <div class="flex min-h-0 flex-col gap-2">
    <div class="flex shrink-0 items-center gap-1">
      <USelectMenu
        :model-value="S.log.branch || undefined"
        v-model:search-term="branchSearch"
        :items="branches"
        ignore-filter
        icon="i-lucide-git-branch"
        :placeholder="S.log.head ? 'detached' : 'no branch'"
        aria-label="Current branch"
        :search-input="{ placeholder: 'Search branches…' }"
        :disabled="blocked || !S.log.branches?.length"
        class="min-w-0 flex-1"
        :ui="{ base: 'font-mono text-xs' }"
        @update:model-value="act('switch', $event)"
      />
      <UTooltip v-for="item in actions" :key="item.action" :text="item.label">
        <UButton
          :icon="item.icon"
          :aria-label="item.label"
          size="xs"
          variant="ghost"
          color="neutral"
          :loading="busy === item.action"
          :disabled="blocked || !S.log.branch"
          @click="act(item.action)"
        />
      </UTooltip>
    </div>
    <details v-show="!S.log.operation" class="group shrink-0">
      <summary class="flex cursor-pointer list-none items-center gap-1 text-xs text-dimmed [&::-webkit-details-marker]:hidden">
        <UIcon name="i-lucide-chevron-right" class="size-3 shrink-0 group-open:rotate-90" />
        Merge / rebase
      </summary>
      <div class="mt-2 space-y-2">
        <div class="flex shrink-0 items-center gap-1">
          <USelect v-model="operation" :items="[{ label: 'merge into', value: 'merge' }, { label: 'rebase into', value: 'rebase' }]"
            aria-label="Branch operation" size="sm" :disabled="!!busy" class="shrink-0" />
          <USelectMenu v-model="target" :items="targets" icon="i-lucide-git-branch"
            aria-label="Destination branch" placeholder="Select branch" :search-input="{ placeholder: 'Search destination branches…' }"
            :disabled="!!busy || !S.log.branch" class="min-w-0 flex-1" :ui="{ base: 'font-mono text-xs' }" />
        </div>
        <div class="shrink-0 space-y-1">
          <div class="flex justify-end">
            <UTooltip :ui="{ content: 'max-w-xs h-auto', text: 'whitespace-normal' }" text="Changed files will be stashed temporarily. If there are conflicts, AI will solve them using the model in Settings → Models.">
              <UButton icon="i-lucide-sparkle" :label="`${operation === 'merge' ? 'Merge' : 'Rebase'} & solve conflicts`"
                size="sm" :loading="busy === operation" :disabled="!!busy || !target || target === S.log.branch"
                @click="act(operation)" />
            </UTooltip>
          </div>
          <p v-if="target" class="text-xs text-dimmed">{{ operation === 'merge'
            ? `Merge ${S.log.branch} into ${target}. Updates and checks out ${target}.`
            : `Replay ${S.log.branch} onto ${target}. Rewrites ${S.log.branch}; ${target} stays unchanged.` }}</p>
        </div>
      </div>
    </details>
    <div v-if="S.log.operation" class="shrink-0 space-y-1">
      <p role="status" class="text-xs text-dimmed">{{ S.log.operation === 'merge' ? 'Merge' : 'Rebase' }} in progress.</p>
      <div class="flex flex-wrap gap-1">
        <UButton icon="i-lucide-sparkle" label="Retry solving conflicts" size="xs" :loading="!!busy && busy !== 'abort'"
          :disabled="!!busy" @click="act('continue')" />
        <UButton label="Abort" color="neutral" variant="outline" size="xs" :disabled="!!busy" @click="act('abort')" />
      </div>
    </div>
    <div class="min-h-4 shrink-0">
      <p v-if="error" role="alert" class="text-xs text-error">{{ error }}</p>
      <p v-if="notice" role="status" class="text-xs text-dimmed">{{ notice }}</p>
    </div>

    <div class="flex shrink-0 items-center gap-1">
      <UInput
        v-model="S.logFilter"
        icon="i-lucide-search"
        placeholder="Filter commits"
        class="min-w-0 flex-1"
        :ui="{ base: 'text-xs' }"
      />
      <UTooltip text="Show commit graph">
        <UButton
          icon="i-lucide-git-fork"
          aria-label="Show commit graph"
          :aria-pressed="S.logGraph"
          :color="S.logGraph ? 'primary' : 'neutral'"
          :variant="S.logGraph ? 'soft' : 'ghost'"
          size="xs"
          @click="S.logGraph = !S.logGraph"
        />
      </UTooltip>
    </div>

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
        <div v-for="{ commit } in rows" :key="commit.hash" :data-hash="commit.hash" class="relative"
          :style="graph ? { paddingLeft: `${graph.width}px` } : undefined">
          <svg v-if="graph" aria-hidden="true" class="pointer-events-none absolute left-0 top-0 h-full"
            :width="graph.width" fill="none" stroke-width="1.5">
            <g v-for="(edge, i) in graph.rows.get(commit.hash).edges" :key="i" :stroke="graphColor(edge.color)">
              <path :d="graphPath(edge)" />
              <line v-if="edge.kind !== 'in'" :x1="graphX(edge.to)" y1="32" :x2="graphX(edge.to)" y2="100%" />
            </g>
            <rect :x="graphX(graph.rows.get(commit.hash).column) - 3" y="13" width="6" height="6" rx="1"
              :fill="commit.parents?.length > 1 ? 'var(--ui-bg)' : graphColor(graph.rows.get(commit.hash).color)"
              :stroke="graphColor(graph.rows.get(commit.hash).color)" />
          </svg>
          <button
            type="button"
            class="w-full rounded-[var(--ui-radius)] px-1.5 py-1 text-left hover:bg-elevated"
            :class="commit.hash === S.log.head ? 'bg-primary/10 border-l-2 border-primary' : ''"
            :aria-current="commit.hash === S.log.head ? 'true' : undefined"
            :aria-expanded="S.logOpen === commit.hash"
            @click="toggleCommit(commit.hash)"
          >
            <span class="flex items-center gap-1 text-xs text-highlighted">
              <span class="min-w-0 truncate" :title="commit.subject">
                <span v-for="(part, i) in subjectParts(commit)" :key="i" :class="part.hit ? 'text-primary' : ''">{{ part.text }}</span>
              </span>
              <span class="shrink-0 rounded bg-elevated px-1 text-[10px] text-muted tabular-nums"
                :aria-label="`${commit.fileCount ?? 0} files affected`">[{{ commit.fileCount ?? 0 }}]</span>
            </span>
            <!-- Wraps rather than truncates: the hash, the author and the date
                 need not fit one narrow line, and a panel that hides half of
                 them is worse than one that uses two. -->
            <span class="block font-mono text-[11px] leading-snug text-dimmed tabular-nums">
              {{ commit.hash }} · {{ commit.author }} · {{ commit.date }}
            </span>
            <span v-if="commit.branches?.length || (S.logGraph && commit.tags?.length)" class="mt-1 flex flex-wrap gap-1">
              <UBadge v-for="branch in commit.branches" :key="`branch:${branch}`" color="primary" variant="subtle" size="xs" icon="i-lucide-git-branch">{{ branch }}</UBadge>
              <UBadge v-for="tag in (S.logGraph ? commit.tags : [])" :key="`tag:${tag}`" color="secondary" variant="subtle" size="xs" icon="i-lucide-tag">{{ tag }}</UBadge>
            </span>
          </button>

          <div v-if="S.logOpen === commit.hash" class="mb-1 ml-2 border-l border-default pl-2">
            <p v-if="!filesOf(commit.hash)" class="px-1.5 py-1 text-xs text-dimmed">Loading…</p>
            <p
              v-else-if="!filesOf(commit.hash).length"
              class="px-1.5 py-1 text-xs text-dimmed"
            >
              No files changed.
            </p>
            <button
              v-for="file in filesOf(commit.hash) || []"
              :key="file.path"
              type="button"
              class="flex w-full select-none items-center gap-1.5 rounded-[var(--ui-radius)] px-1.5 py-0.5 text-left font-mono text-xs hover:bg-elevated hover:text-highlighted"
              :title="file.path"
              :data-path="file.path"
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
