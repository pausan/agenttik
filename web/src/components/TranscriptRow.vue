<script setup>
import { computed, inject } from "vue";

import { S } from "../store";
import BackgroundTask from "./BackgroundTask.vue";
import Message from "./Message.vue";
import MessageThreads from "./MessageThreads.vue";
import ToolGroup from "./ToolGroup.vue";
import TurnOutputs from "./TurnOutputs.vue";

const props = defineProps({ row: Object, threads: Array });
const now = inject("transcriptClock");
const hasThreads = computed(() => props.row.message?.id &&
  (props.threads.length || S.threadDraft?.messageID === props.row.message.id));
</script>

<template>
  <ToolGroup v-if="row.tools" :tools="row.tools" />
  <BackgroundTask v-else-if="row.message.role === 'background'" :message="row.message" :now="now" />
  <Message v-else :message="row.message" />
  <MessageThreads v-if="hasThreads" :message-id="row.message.id" :threads="threads" />
  <TurnOutputs v-if="row.outputs" :outputs="row.outputs" :project-id="S.detail.session.project_id" :message-id="row.message?.id || 0" />
  <p v-if="row.summary" class="-mt-2 mb-4 text-[11px] text-dimmed tabular-nums" aria-label="Turn usage">
    {{ row.summary }}
  </p>
</template>
