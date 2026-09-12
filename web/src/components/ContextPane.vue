<script setup>
/* What the prompt bar's context ring reveals: how full the model's context
   is, and the session's totals underneath.

   Context used is the size of the main agent's latest prompt, cached blocks
   included — not a sum over the task. Root and child agents can send many
   independent prompts, so their total can exceed one model window. */
import { computed } from "vue";

import { S, contextWindow } from "../store";
import { cost, duration, nf, tokens, usageBreakdownRows } from "../api";

const used = computed(() => S.detail?.stats.context_tokens || 0);
const total = computed(() => contextWindow(S.detail?.session, S.detail?.stats));
const pct = computed(() =>
  total.value ? Math.min(100, Math.round((used.value / total.value) * 100)) : 0,
);

const rows = computed(() => {
  const stats = S.detail?.stats;
  if (!stats) return [];
  return [
    ["Turns", nf.format(stats.turns)],
    ["Task input", tokens(stats.input_tokens)],
    ["Task output", tokens(stats.output_tokens)],
    ...usageBreakdownRows(stats),
    ["Cache read", tokens(stats.cache_read_tokens)],
    ["Cache write", tokens(stats.cache_write_tokens)],
    ["Cost", cost(stats.cost_usd)],
    ["Agent time", duration(stats.duration_ms)],
  ];
});
</script>

<template>
  <div v-if="S.detail" class="w-64 p-3">
    <div class="flex items-baseline justify-between gap-2">
      <span class="text-xs font-medium text-muted">Main context</span>
      <span class="text-xs text-dimmed tabular-nums">
        {{ !used ? "not reported" : total ? pct + "%" : "window unknown" }}
      </span>
    </div>
    <p class="mt-0.5 text-xs text-dimmed tabular-nums">
      <template v-if="used">
        {{ tokens(used) }}<template v-if="total"> / {{ tokens(total) }}</template> tokens
      </template>
      <template v-else>No verified main prompt</template>
    </p>
    <p class="mt-1 text-[11px] leading-snug text-dimmed">
      Latest main-agent prompt, cached blocks included. Subagents use separate contexts.
    </p>

    <dl class="mt-3 mb-0 border-t border-default pt-1">
      <div v-for="[k, v] in rows" :key="k" class="flex justify-between gap-2.5 py-0.5 text-xs">
        <dt class="shrink-0 text-muted">{{ k }}</dt>
        <dd class="m-0 truncate text-highlighted tabular-nums">{{ v }}</dd>
      </div>
    </dl>
  </div>
</template>
