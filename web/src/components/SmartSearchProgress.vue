<script setup>
import { smartSearch, setSmartSearch } from "../smart-search.js";
</script>

<template>
  <div v-if="smartSearch.enabled" class="my-2 text-xs text-muted" role="status" aria-live="polite">
    <template v-if="smartSearch.phase === 'runtime'">Preparing search runtime on the Agenttik host…</template>
    <template v-else-if="smartSearch.phase === 'download'">
      Downloading Bekko a8m… {{ Math.floor(smartSearch.percent) }}%
      <progress class="mt-1 block w-full accent-current" aria-label="Model download" :value="smartSearch.percent" max="100" />
    </template>
    <template v-else-if="smartSearch.phase === 'index'">
      Indexing tasks: {{ smartSearch.completed }}/{{ smartSearch.total }} ·
      {{ smartSearch.seconds === null ? 'Estimating time left…' : `About ${smartSearch.seconds} seconds left` }}
      <progress class="mt-1 block w-full accent-current" aria-label="Task indexing" :value="smartSearch.percent" max="100" />
    </template>
    <template v-else-if="smartSearch.phase === 'error'">
      Smart Search unavailable: {{ smartSearch.error }}
      <UButton label="Retry Smart Search" size="xs" variant="ghost" @click="setSmartSearch(true)" />
    </template>
    <template v-else-if="smartSearch.phase === 'ready'">Smart Search ready · {{ smartSearch.total }} tasks indexed</template>
    <template v-else>Preparing Smart Search…</template>
  </div>
</template>
