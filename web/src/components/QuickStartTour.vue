<script setup>
import { computed, ref, watch } from "vue";
import { S, copyText } from "../store";
import { isMobile } from "../ui";
import { connectedAccounts, insertTourPrompt, PRACTICE_REPO, TOUR_STEPS } from "../quick-start";

const stepID = defineModel("step", { type: String, default: "welcome" });
const props = defineProps({ setupOpen: Boolean, mobile: Boolean });
const emit = defineEmits(["close"]);
const minimized = ref(false);
const promptHeight = ref(0);
// Keep Send/Enqueue reachable, including when the prompt is resized. One
// observer exists only while the tour is open and a prompt bar is mounted.
watch(() => S.activeTab, (_, __, cleanup) => {
  promptHeight.value = 0;
  const bar = document.querySelector(".prompt-bar");
  if (!bar) return;
  const observer = new ResizeObserver(() => { promptHeight.value = bar.getBoundingClientRect().height; });
  observer.observe(bar);
  cleanup(() => observer.disconnect());
}, { immediate: true, flush: "post" });
const bottom = computed(() => 12 + (!props.mobile && !minimized.value ? promptHeight.value : 0));
watch(() => props.setupOpen, (open) => {
  if (open && isMobile()) minimized.value = true;
});
const copiedRepo = ref(false);
const index = computed(() => Math.max(0, TOUR_STEPS.findIndex((step) => step.id === stepID.value)));
const step = computed(() => TOUR_STEPS[index.value]);
const connected = computed(() => connectedAccounts(S.providers));
const canInsert = computed(() => !!S.detail?.session && !!S.owner && !S.owner.draft?.trim() && !S.owner.imageUploads);
const project = computed(() => S.projects.find((p) => p.id === S.detail?.session.project_id));

function move(offset) {
  stepID.value = TOUR_STEPS[Math.max(0, Math.min(TOUR_STEPS.length - 1, index.value + offset))].id;
}
function insert() {
  if (S.detail?.session && insertTourPrompt(S.owner, step.value.prompt)) S.promptFocus++;
}
async function copyRepo() {
  copiedRepo.value = await copyText(PRACTICE_REPO);
}
</script>

<template>
  <Teleport to="body">
    <section class="quick-start-tour fixed right-3 z-[100] w-[340px] max-w-[calc(100vw-1.5rem)] rounded-xl border border-accented bg-default text-sm text-default shadow-xl" :style="{ bottom: bottom + 'px' }" aria-label="Quick Start Tour" :data-step="minimized ? '' : step.id">
      <header class="flex items-center gap-2 p-3">
        <UIcon name="i-lucide-graduation-cap" class="size-5 shrink-0 text-primary" />
        <span class="min-w-0 flex-1 font-semibold">Quick Start Tour</span>
        <UButton color="neutral" variant="ghost" size="xs" :icon="minimized ? 'i-lucide-chevron-up' : 'i-lucide-minus'" :aria-label="minimized ? 'Expand tour' : 'Minimize tour'" :aria-expanded="!minimized" @click="minimized = !minimized" />
        <UButton color="neutral" variant="ghost" size="xs" icon="i-lucide-x" aria-label="Close tour" @click="emit('close')" />
      </header>
      <template v-if="!minimized">
        <div class="overflow-y-auto px-4 pb-3" :style="{ maxHeight: `min(45dvh, calc(100dvh - ${bottom + 125}px))` }" :key="step.id">
          <p class="mb-1 text-xs text-dimmed" aria-live="polite">Step {{ index + 1 }} of {{ TOUR_STEPS.length }}</p>
          <h2 class="mb-3 text-base font-semibold text-highlighted">{{ step.title }}</h2>
          <div class="space-y-2 text-xs leading-relaxed text-muted">
            <p v-for="paragraph in step.paragraphs" :key="paragraph">{{ paragraph }}</p>
          </div>
          <p v-if="step.id === 'connect'" role="status" class="mt-3 rounded bg-elevated p-2 text-xs">
            {{ !S.providers.length ? 'Checking provider connections…' : connected.length ? `Already connected: ${connected.join(', ')}. You can start with one of these.` : 'No connected agent detected yet. Complete sign-in in Subscriptions to get started.' }}
          </p>
          <div v-if="step.repo" class="mt-3">
            <code class="block break-all rounded bg-elevated p-2 text-xs">{{ PRACTICE_REPO }}</code>
            <UButton class="mt-2" size="xs" variant="soft" :label="copiedRepo ? 'Repository URL copied' : 'Copy repository URL'" @click="copyRepo" />
          </div>
          <template v-if="step.prompt">
            <details class="mt-3 text-xs">
              <summary class="cursor-pointer font-medium text-primary">Read the prompt</summary>
              <p class="mt-2 whitespace-pre-wrap rounded bg-elevated p-2 leading-relaxed">{{ step.prompt }}</p>
            </details>
            <p class="mt-3 text-xs text-dimmed">{{ project ? `Prompt destination: ${project.name} · current task` : 'Open a task in your practice project first.' }}</p>
            <p v-if="S.owner?.draft?.trim() || S.owner?.imageUploads" class="mt-1 text-xs text-muted">Your draft is kept. Send, enqueue, or clear it before copying another prompt.</p>
            <UButton class="mt-2" size="sm" icon="i-lucide-copy" label="Copy to prompt area" :disabled="!canInsert" @click="insert" />
          </template>
        </div>
        <footer class="flex items-center justify-between border-t border-default p-3">
          <UButton color="neutral" variant="ghost" size="sm" label="Previous" :disabled="index === 0" @click="move(-1)" />
          <UButton v-if="index < TOUR_STEPS.length - 1" size="sm" label="Next" trailing-icon="i-lucide-arrow-right" @click="move(1)" />
          <UButton v-else size="sm" label="Finish tour" @click="emit('close')" />
        </footer>
      </template>
    </section>
  </Teleport>
</template>
