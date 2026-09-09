<script setup>
/* What the cog on the prompt bar opens: how full the model's context is, and
   the session's totals underneath.

   Context used is the size of the last prompt the CLI actually sent, cached
   blocks included — not a sum over the turn. A turn with twenty tool calls
   sends twenty prompts, so summing them would read many times the window;
   only the last one describes what the model is holding now. */
import { computed } from "vue";

import { S, contextWindow } from "../store";
import { cost, duration, nf, tokens } from "../api";

const used = computed(() => S.detail?.stats.context_tokens || 0);
const total = computed(() => contextWindow(S.detail?.session));
const pct = computed(() =>
  total.value ? Math.min(100, Math.round((used.value / total.value) * 100)) : 0,
);
/* Getting close to the window is worth noticing before the CLI has to
   compact, so the bar changes colour rather than only growing. */
const tone = computed(() => (pct.value >= 90 ? "error" : pct.value >= 70 ? "warning" : "primary"));

const rows = computed(() => {
  const stats = S.detail?.stats;
  if (!stats) return [];
  return [
    ["Turns", nf.format(stats.turns)],
    ["Input", tokens(stats.input_tokens)],
    ["Output", tokens(stats.output_tokens)],
    ["Cache read", tokens(stats.cache_read_tokens)],
    ["Cache write", tokens(stats.cache_write_tokens)],
    ["Cost", cost(stats.cost_usd)],
    ["Agent time", duration(stats.duration_ms)],
  ];
});
</script>

<template>
  <div v-if="S.detail" class="w-64 p-3">
    <div class="mb-1 flex items-baseline justify-between gap-2">
      <span class="text-xs font-medium text-muted">Context</span>
      <span class="text-xs text-dimmed tabular-nums">
        {{ total ? pct + "%" : "window unknown" }}
      </span>
    </div>
    <div class="h-1.5 overflow-hidden rounded-full bg-accented">
      <div
        class="h-full rounded-full transition-[width]"
        :class="{ primary: 'bg-primary', warning: 'bg-warning', error: 'bg-error' }[tone]"
        :style="{ width: (total ? pct : 0) + '%' }"
      />
    </div>
    <p class="mt-1.5 text-xs text-dimmed tabular-nums">
      {{ tokens(used) }}<template v-if="total"> / {{ tokens(total) }}</template> tokens
      <template v-if="!used"> · nothing sent yet</template>
    </p>
    <p class="mt-1 text-[11px] leading-snug text-dimmed">
      The last prompt sent, cached blocks included.
    </p>

    <dl class="mt-3 mb-0 border-t border-default pt-1">
      <div v-for="[k, v] in rows" :key="k" class="flex justify-between gap-2.5 py-0.5 text-xs">
        <dt class="shrink-0 text-muted">{{ k }}</dt>
        <dd class="m-0 truncate text-highlighted tabular-nums">{{ v }}</dd>
      </div>
    </dl>
  </div>
</template>
