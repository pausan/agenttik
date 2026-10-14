<script>
import { reactive } from "vue";

// A card closed stays closed while the transcript redraws around it.
const collapsed = reactive(new Set());
</script>

<script setup>
/* The result files a turn left: reports, charts and tables its reply
   presented. They are drawn under that reply, live, so a result is read and
   iterated on — and commented on — where the conversation is. Pages run their
   scripts in the same sandbox as the HTML preview; images and tables are read
   from the disk. See specs/090-turn-outputs.md. */
import { computed, reactive as state, ref, watch, watchEffect } from "vue";

import { api, profileID, remoteID } from "../api";
import { htmlPreviewDocument, htmlPreviewRoot, htmlPreviewURL } from "../html-preview";
import { htmlPreviewToken, openFile, openInSystem, S } from "../store";
import CsvPreview from "./CsvPreview.vue";
import ConversationImage from "./ConversationImage.vue";
import HtmlFrame from "./HtmlFrame.vue";

const props = defineProps({
  outputs: { type: Array, required: true },
  projectId: { type: Number, required: true },
  messageId: { type: Number, default: 0 },
});

const pages = computed(() => props.outputs.filter((o) => o.kind !== "image"));
const images = computed(() => props.outputs.filter((o) => o.kind === "image"));
const key = (o) => `${o.turn_id}:${o.path}`;
const name = (o) => o.path.split("/").pop();
const folder = (o) => o.path.split("/").slice(0, -1).join("/");

// One token per root: the project, or the folder of a file outside it.
const tokens = state({});
watch(() => props.outputs, async (outputs) => {
  for (const o of outputs) {
    const { root } = htmlPreviewRoot(o.path);
    if (root in tokens) continue;
    tokens[root] = "";
    try {
      tokens[root] = await htmlPreviewToken(props.projectId, o.path);
    } catch {
      delete tokens[root];
    }
  }
}, { immediate: true });

// The modification time changes when the file is rewritten, which reloads it.
function src(o) {
  const token = tokens[htmlPreviewRoot(o.path).root];
  if (!token) return "";
  return `${htmlPreviewURL({ projectID: props.projectId, path: o.path }, { remote: remoteID, profile: profileID, token })}?v=${o.modified}`;
}

// Pages and tables are read as text: a page goes in through srcdoc, which
// is how the annotator gets into it. Read again when the file is rewritten.
const texts = state({});
const textOf = (o) => texts[`${o.path}@${o.modified}`];
watchEffect(() => {
  for (const o of pages.value) {
    const at = `${o.path}@${o.modified}`;
    if (collapsed.has(key(o)) || at in texts) continue;
    texts[at] = null;
    api("GET", `/api/projects/${props.projectId}/file?path=${encodeURIComponent(o.path)}`)
      .then((f) => { texts[at] = f.binary || f.partial ? "" : f.content; })
      .catch(() => { texts[at] = ""; });
  }
});

function page(o) {
  const url = src(o);
  return url && textOf(o) ? htmlPreviewDocument(textOf(o), new URL(url, window.location.href).href) : "";
}

const annotating = ref("");
const preview = ref(null);
const canComment = computed(() => S.owner?.kind === "session");

function toggle(o) {
  if (collapsed.has(key(o))) collapsed.delete(key(o));
  else collapsed.add(key(o));
}
</script>

<template>
  <div class="mb-4 flex flex-col gap-2" aria-label="Turn outputs">
    <section
      v-for="o in pages"
      :key="o.path"
      class="overflow-hidden rounded-[var(--ui-radius)] border border-default"
      :aria-label="`Output ${name(o)}`"
    >
      <header class="flex min-w-0 items-center gap-1 bg-elevated/50 py-0.5 pr-1 pl-0.5 text-xs">
        <UButton
          size="xs"
          color="neutral"
          variant="ghost"
          :icon="collapsed.has(key(o)) ? 'i-lucide-chevron-right' : 'i-lucide-chevron-down'"
          :aria-expanded="!collapsed.has(key(o))"
          :title="collapsed.has(key(o)) ? 'Show' : 'Hide'"
          @click="toggle(o)"
        />
        <UIcon :name="o.kind === 'html' ? 'i-lucide-chart-column' : 'i-lucide-table'" class="shrink-0 text-dimmed" />
        <span class="shrink-0 font-mono">{{ name(o) }}</span>
        <span class="min-w-0 truncate text-dimmed" :title="o.path">{{ folder(o) }}</span>
        <span class="ml-auto" />
        <UButton
          v-if="o.kind === 'html' && canComment && !collapsed.has(key(o))"
          size="xs"
          :color="annotating === key(o) ? 'primary' : 'neutral'"
          :variant="annotating === key(o) ? 'solid' : 'ghost'"
          icon="i-lucide-message-square-plus"
          :label="annotating === key(o) ? 'Click an element' : 'Comment'"
          :aria-pressed="annotating === key(o)"
          title="Click an element of the page to comment on it in the prompt"
          @click="annotating = annotating === key(o) ? '' : key(o)"
        />
        <UButton
          size="xs"
          color="neutral"
          variant="ghost"
          icon="i-lucide-file-plus"
          title="Open in a tab"
          :aria-label="`Open ${name(o)} in a tab`"
          @click="openFile(o.path, true, 'preview')"
        />
        <UButton
          size="xs"
          color="neutral"
          variant="ghost"
          icon="i-lucide-external-link"
          title="Open in system browser"
          :aria-label="`Open ${name(o)} in system browser`"
          @click="openInSystem(o.path, projectId)"
        />
      </header>
      <template v-if="!collapsed.has(key(o))">
        <!-- Resizable: a report is as tall as it is, and the frame cannot
             measure a page on another origin. -->
        <div v-if="o.kind === 'html'" class="h-[480px] min-h-24 resize-y overflow-hidden border-t border-default">
          <HtmlFrame
            v-if="page(o)"
            :annotating="annotating === key(o)"
            :html="page(o)"
            :title="o.path"
            :path="o.path"
            @update:annotating="annotating = $event ? key(o) : ''"
          />
        </div>
        <div v-else-if="textOf(o)" class="max-h-80 overflow-auto border-t border-default">
          <CsvPreview :text="textOf(o)" />
        </div>
        <p v-if="!textOf(o)" class="border-t border-default px-5 py-4 text-sm text-dimmed">
          {{ textOf(o) === "" ? "This file could not be read." : "Loading…" }}
        </p>
      </template>
    </section>

    <div v-if="images.length" class="flex flex-wrap gap-2" aria-label="Output images">
      <div
        v-for="o in images"
        :key="o.path"
        class="flex max-w-full flex-col items-start gap-1"
      >
        <button type="button" class="max-w-full cursor-zoom-in rounded focus-visible:outline-2 focus-visible:outline-primary"
          :title="o.path" :aria-label="`View ${name(o)}`" @click="preview = o">
          <img v-if="src(o)" :src="src(o)" :alt="name(o)" loading="lazy"
            class="max-h-80 max-w-full rounded border border-default object-contain" />
        </button>
        <UButton size="xs" color="neutral" variant="ghost" icon="i-lucide-file-plus" label="Open in a tab"
          :aria-label="`Open ${name(o)} in a tab`" @click="openFile(o.path, true, 'preview')" />
      </div>
    </div>
    <ConversationImage v-if="preview" :src="src(preview)" :title="name(preview)" :source="preview.path"
      :message-id="messageId" @close="preview = null" />
  </div>
</template>
