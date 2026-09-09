<script setup>
import { S, openSession, startSession } from "../store";
import { ago } from "../api";
import SessionRow from "./SessionRow.vue";
</script>

<template>
  <div v-if="S.project" class="min-h-0 flex-1 overflow-auto">
    <div class="mx-auto max-w-[860px] px-6 py-5">
      <div class="mb-3.5 flex items-start gap-3 border-b border-default pb-3">
        <div class="min-w-0">
          <h2 class="m-0 text-[17px] tracking-tight text-highlighted">{{ S.project.project.name }}</h2>
          <div class="truncate text-xs text-dimmed">{{ S.project.project.path }}</div>
        </div>
        <UButton
          class="ml-auto shrink-0"
          label="New session"
          @click="startSession(S.project.project)"
        />
      </div>

      <p v-if="!S.project.sessions.length" class="px-3 py-5 text-center text-dimmed">
        No sessions yet in this project.
      </p>
      <SessionRow
        v-for="s in S.project.sessions"
        :key="s.id"
        :title="s.title || 'Untitled session'"
        :status="s.status"
        :sub="`${s.model}${s.effort ? ' · ' + s.effort : ''} · ${ago(s.last_active_at)}`"
        @click="openSession(s.id)"
      />
    </div>
  </div>
</template>
