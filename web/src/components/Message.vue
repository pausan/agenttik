<script setup>
/* One bubble in the transcript.

   It is a component of its own for a reason beyond tidiness: a streaming turn
   grows the last message a few characters at a time, and if the bubbles were
   inlined in the transcript's v-for every one of them would be re-rendered —
   and re-parsed as markdown — on every delta. Here only the message whose
   content changed re-renders, and its computed markdown is what re-runs.

   Every message can be copied. Only your own prompts can be edited, and only
   once they have been stored: a message still arriving over the stream has no
   id to rewrite. */
import { computed, nextTick, ref } from "vue";

import { S, copyText, editMessage, effortsFor as effortsOf, modelPickerGroups, openExternal, openFileRef, parseModelChoice, setModel } from "../store";
import { markdown } from "../markdown";

const props = defineProps({ message: { type: Object, required: true } });

const WHO = { user: "You", assistant: "Agent", tool: "Tool", thinking: "Thinking", error: "Error" };

/* What the agent writes is markdown, so it is rendered. What you typed is
   shown as you typed it, and a tool line is a literal command. */
const RENDERED = { assistant: true, thinking: true };

const BUBBLE = {
  user: "max-w-[85%] whitespace-pre-wrap rounded-[var(--ui-radius)] bg-elevated px-3 py-2 text-highlighted",
  assistant: "markdown text-highlighted",
  thinking: "markdown text-dimmed italic",
  tool: "max-h-[9em] overflow-auto whitespace-pre-wrap rounded-[var(--ui-radius)] bg-muted px-2.5 py-1.5 font-mono text-xs text-muted inset-ring inset-ring-default",
  error:
    "whitespace-pre-wrap rounded-[var(--ui-radius)] bg-error/8 px-2.5 py-2 text-error inset-ring inset-ring-error/25",
};

const rendered = computed(() => !!RENDERED[props.message.role]);
/* markdown() emits a fixed set of tags over escaped text, which is what makes
   v-html safe here: nothing an agent writes can reach the DOM as markup. */
const html = computed(() => (rendered.value ? markdown(props.message.content) : ""));
const classes = computed(() => [
  "wrap-anywhere",
  BUBBLE[props.message.role],
  props.message.streaming ? "streaming" : "",
]);

const editable = computed(
  () => props.message.role === "user" && !!props.message.id && !props.message.streaming,
);

/* A path the agent wrote is a button inside the rendered markdown, so one
   listener on the bubble answers all of them however many there are. The
   desktop webview cannot follow target=_blank, so its web links go through
   the same system-browser bridge as Settings' server address. */
function onClick(e) {
  const link = e.target.closest("a[href]");
  if (link && openExternal(link.href)) {
    e.preventDefault();
    return;
  }
  const hit = e.target.closest("button.file");
  if (!hit) return;
  openFileRef(hit.dataset.file, Number(hit.dataset.line) || 0);
}

/* An embedded desktop view has no browser context menu, so aim one shared
   menu at the link under the pointer. In a normal browser the anchor still
   follows itself; opening from this menu uses a new tab. */
const aimedLink = ref("");
function aimLink(e) {
  aimedLink.value = e.target.closest("a[href]")?.href || "";
}

function openLink() {
  if (!aimedLink.value) return;
  if (!openExternal(aimedLink.value)) window.open(aimedLink.value, "_blank", "noopener,noreferrer");
}

const linkMenu = computed(() => [
  { label: "Open in browser", icon: "i-lucide-external-link", disabled: !aimedLink.value, onSelect: openLink },
  { label: "Copy link", icon: "i-lucide-copy", disabled: !aimedLink.value, onSelect: () => copyText(aimedLink.value) },
]);

const copied = ref(false);
async function copy() {
  if (!(await copyText(props.message.content))) return;
  copied.value = true;
  window.setTimeout(() => (copied.value = false), 1200);
}

const draft = ref(null); // non-null while editing
const box = ref(null);
const sending = ref(false);
const choosing = ref(false);
const modelOpen = ref(false);
const modelGroups = computed(modelPickerGroups);

async function pickModel(value) {
  modelOpen.value = false;
  const { provider, accountID, model } = parseModelChoice(value);
  const effort = S.detail.session.effort;
  choosing.value = true;
  try {
    await setModel(provider, model, effortsOf(provider, model).includes(effort) ? effort : "", accountID);
  } finally {
    choosing.value = false;
  }
}

async function startEdit() {
  draft.value = props.message.content;
  await nextTick();
  box.value?.textareaRef?.focus();
}

async function submit(enqueue = false) {
  if (sending.value || choosing.value) return;
  sending.value = true;
  const sent = await editMessage(props.message, draft.value, enqueue);
  sending.value = false;
  // On success this bubble is about to be replaced by the truncation, so the
  // editor is only closed when it is still here to close.
  if (sent) draft.value = null;
}

function onKey(e) {
  if (e.key === "Escape") draft.value = null;
}
</script>

<template>
  <div class="group mb-4" :class="message.role === 'user' ? 'flex flex-col items-end' : ''">
    <div
      class="mb-0.5 flex items-center gap-1.5"
      :class="message.role === 'user' ? 'flex-row-reverse' : ''"
    >
      <span class="text-[11px] font-medium tracking-wider text-dimmed uppercase">
        {{ WHO[message.role] || message.role }}
      </span>
      <!-- The actions stay in the layout and only appear on hover, so a
           bubble does not shift when the pointer reaches it. -->
      <span
        class="message-actions flex items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100 max-md:opacity-100"
      >
        <UButton
          :icon="copied ? 'i-lucide-check' : 'i-lucide-copy'"
          size="xs"
          color="neutral"
          variant="ghost"
          :aria-label="`Copy ${WHO[message.role] || message.role} message`"
          title="Copy"
          @click="copy"
        />
        <UButton
          v-if="editable"
          icon="i-lucide-pencil"
          size="xs"
          color="neutral"
          variant="ghost"
          :disabled="draft !== null || S.detail?.running"
          aria-label="Edit this prompt"
          title="Edit and continue from here"
          @click="startEdit"
        />
      </span>
    </div>

    <!-- Editing replaces the bubble in place, so the prompt is rewritten
         where it sits rather than in a dialogue over the transcript. -->
    <form v-if="draft !== null" aria-label="Edit prompt" class="w-full max-w-[85%] self-end" @submit.prevent="submit()">
      <UTextarea
        ref="box"
        v-model="draft"
        :rows="4"
        autoresize
        class="w-full"
        @keydown="onKey"
        @keydown.enter.exact.prevent="submit()"
      />
      <div class="mt-1.5 flex flex-wrap items-center gap-2">
        <UPopover v-model:open="modelOpen">
          <UButton type="button" size="xs" :label="S.detail.session.model" title="Change edited prompt model" :disabled="sending || choosing" />
          <template #content>
            <UCommandPalette
              class="w-80 max-w-[calc(100vw-2rem)]"
              :groups="modelGroups"
              value-key="value"
              placeholder="Search models…"
              :ui="{ viewport: 'max-h-[min(28rem,60vh)]' }"
              @update:model-value="pickModel"
            />
          </template>
        </UPopover>
        <UButton type="submit" size="xs" :loading="sending" :disabled="!draft.trim() || choosing" label="Send" />
        <UButton type="button" size="xs" :disabled="!draft.trim() || sending || choosing" label="Enqueue" @click="submit(true)" />
        <UButton
          type="button"
          size="xs"
          color="neutral"
          variant="ghost"
          label="Cancel"
          @click="draft = null"
        />
        <span class="text-[11px] leading-snug text-dimmed">
          Replaces the transcript from here down. The agent keeps its own memory
          of the turns after this one, and files on disk are left as they are.
        </span>
      </div>
    </form>

    <template v-else>
      <UContextMenu v-if="rendered" :items="linkMenu">
        <div :class="classes" v-html="html" @click="onClick" @contextmenu="aimLink" />
      </UContextMenu>
      <div v-else :class="classes">{{ message.content }}</div>
    </template>
  </div>
</template>
