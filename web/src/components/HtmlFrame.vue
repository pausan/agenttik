<script setup>
/* A previewed page, and comments on it. The page runs its scripts sandboxed
   on an opaque origin (specs/086-html-preview.md); the annotator injected
   into it reports a clicked element while commenting is on, and the comment
   goes into the task's prompt with a description of that element. See
   specs/091-page-annotations.md. */
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";

import { annotationText } from "../html-preview";
import { addToPrompt, notice, S } from "../store";

const props = defineProps({
  html: { type: String, required: true },
  title: { type: String, required: true },
  // The file as the agent knows it, for the comment's text.
  path: { type: String, required: true },
});
const annotating = defineModel("annotating", { type: Boolean, default: false });

const frame = ref(null);
const box = ref(null);
const field = ref(null);
const picked = ref(null);
const comment = ref("");

const pickedLabel = computed(() => {
  const el = picked.value;
  return el ? [`<${el.tag}>`, el.label || el.text, el.heading && `under ${el.heading}`].filter(Boolean).join(" ") : "";
});

function sendMode() {
  frame.value?.contentWindow?.postMessage({ agenttik: "annotate-mode", on: annotating.value }, "*");
}
watch(annotating, (on) => {
  if (!on) picked.value = null;
  sendMode();
});

// The page could post anything, so only plain, bounded strings are kept.
const text = (value, max) => (typeof value === "string" ? value.slice(0, max) : "");
function onMessage(e) {
  if (!frame.value || e.source !== frame.value.contentWindow || e.data?.agenttik !== "annotate") return;
  if (e.data.cancel) {
    if (picked.value) picked.value = null;
    else annotating.value = false;
    return;
  }
  const el = e.data.element;
  if (!annotating.value || !el || typeof el !== "object") return;
  const rect = el.rect || {};
  picked.value = {
    tag: text(el.tag, 40),
    selector: text(el.selector, 400),
    label: text(el.label, 160),
    text: text(el.text, 160),
    heading: text(el.heading, 120),
    data: Array.isArray(el.data) ? el.data.slice(0, 6).map((d) => text(d, 80)) : [],
    rect: { x: Number(rect.x) || 0, y: Number(rect.y) || 0, height: Number(rect.height) || 0 },
  };
  comment.value = "";
  nextTick(() => field.value?.textarea?.focus());
}
onMounted(() => window.addEventListener("message", onMessage));
onUnmounted(() => window.removeEventListener("message", onMessage));

// Under the element, kept inside the frame.
const popover = computed(() => {
  const r = picked.value?.rect;
  if (!r || !box.value) return {};
  const width = Math.min(320, box.value.clientWidth - 16);
  const left = Math.max(8, Math.min(r.x, box.value.clientWidth - width - 8));
  const top = Math.max(8, Math.min(r.y + r.height + 6, box.value.clientHeight - 150));
  return { left: `${left}px`, top: `${top}px`, width: `${width}px` };
});

function add() {
  if (!picked.value || !comment.value.trim()) return;
  if (!addToPrompt(annotationText(props.path, picked.value, comment.value))) return;
  // On a file tab the prompt bar is out of sight, so say where it went.
  if (S.tabs.find((t) => t.id === S.activeTab)?.kind === "file") notice("Comment added", "It is in the task's prompt, ready to send.");
  picked.value = null;
}
</script>

<template>
  <div ref="box" class="relative h-full w-full">
    <iframe
      ref="frame"
      class="h-full w-full border-0 bg-white"
      sandbox="allow-scripts"
      referrerpolicy="no-referrer"
      :title="title"
      :srcdoc="html"
      @load="sendMode"
    ></iframe>
    <form
      v-if="picked"
      class="absolute z-10 rounded-[var(--ui-radius)] border border-default bg-default p-2 shadow-lg"
      :style="popover"
      aria-label="Comment on element"
      @submit.prevent="add"
    >
      <p class="mb-1 truncate text-xs text-dimmed" :title="picked.selector">{{ pickedLabel }}</p>
      <UTextarea
        ref="field"
        v-model="comment"
        :rows="2"
        autoresize
        class="w-full"
        placeholder="What should change here?"
        aria-label="Comment"
        @keydown.enter.exact.prevent="add"
        @keydown.esc.prevent="picked = null"
      />
      <div class="mt-1.5 flex items-center gap-2">
        <UButton type="submit" size="xs" label="Add to prompt" :disabled="!comment.trim()" />
        <UButton type="button" size="xs" color="neutral" variant="ghost" label="Cancel" @click="picked = null" />
      </div>
    </form>
  </div>
</template>
