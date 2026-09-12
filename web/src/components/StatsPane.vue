<script setup>
import { computed } from "vue";

import { S, accountLabel, contextWindow } from "../store";
import { cost, duration, isoDate, nf, tokens, usageBreakdownRows } from "../api";

/* The context row is the main agent's latest prompt against its model window,
   not a task sum — see ContextPane.vue, which shows the same number as a gauge. */
function context(session, stats) {
  if (!stats.context_tokens) return "not reported";
  const total = contextWindow(session, stats);
  return tokens(stats.context_tokens) + (total ? " / " + tokens(total) : "");
}

/* Tasks in a project run concurrently, so agent time is summed across
   them and can exceed the wall clock. */
const rows = computed(() => {
  if (S.project) {
    const { project, stats } = S.project;
    return [
      ["Folder", project.path],
      ["Tasks", nf.format(stats.sessions)],
      ["Running now", nf.format(stats.running)],
      ["Turns", nf.format(stats.turns)],
      ["Task input tokens", tokens(stats.input_tokens)],
      ["Task output tokens", tokens(stats.output_tokens)],
      ...usageBreakdownRows(stats),
      ["Cache read", tokens(stats.cache_read_tokens)],
      ["Cache write", tokens(stats.cache_write_tokens)],
      ["Cost", cost(stats.cost_usd)],
      ["Agent time", duration(stats.duration_ms)],
      ["Last used", isoDate(stats.last_active_at)],
      ["Added", isoDate(project.created_at)],
    ];
  }
  if (!S.detail) return [];
  const { session: s, stats } = S.detail;
  return [
    ["Provider", s.provider],
    ...(accountLabel(s.provider, s.account_id)
      ? [["Subscription", accountLabel(s.provider, s.account_id)]]
      : []),
    ["Model", s.model],
    ["Effort", s.effort || "default"],
    ["Permission", s.permission],
    ["Project", s.project_name],
    ["Folder", s.project_path],
    ["Turns", nf.format(stats.turns)],
    ["Main context", context(s, stats)],
    ["Task input tokens", tokens(stats.input_tokens)],
    ["Task output tokens", tokens(stats.output_tokens)],
    ...usageBreakdownRows(stats),
    ["Cache read", tokens(stats.cache_read_tokens)],
    ["Cache write", tokens(stats.cache_write_tokens)],
    ["Cost", cost(stats.cost_usd)],
    ["Agent time", duration(stats.duration_ms)],
    ["Started", isoDate(s.created_at)],
    ["Last used", isoDate(s.last_active_at)],
  ];
});
</script>

<template>
  <dl class="m-0">
    <div
      v-for="[k, v] in rows"
      :key="k"
      class="flex justify-between gap-2.5 px-0.5 py-1 not-first:border-t not-first:border-default"
    >
      <dt class="shrink-0 text-muted">{{ k }}</dt>
      <dd class="m-0 truncate text-highlighted tabular-nums">{{ v }}</dd>
    </div>
  </dl>
</template>
