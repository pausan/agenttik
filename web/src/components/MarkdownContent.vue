<script setup>
import { computed, ref } from "vue";

import { markdown } from "../markdown";
import { copyText, openExternal, openFileRef, openInSystem, resolveFileRef } from "../store";

defineOptions({ inheritAttrs: false });

const props = defineProps({
  text: { type: String, default: "" },
  basePath: { type: String, default: "" },
});
const html = computed(() => markdown(props.text));
const aimedLink = ref("");
const aimedFile = ref(null);

function fileAt(e) {
  const hit = e.target.closest("button.file");
  return hit ? { path: hit.dataset.file, line: Number(hit.dataset.line) || 0 } : null;
}

function openFile(file) {
  if (file) openFileRef(file.path, file.line, props.basePath);
}

function onClick(e) {
  const link = e.target.closest("a[href]");
  if (link && openExternal(link.href)) {
    e.preventDefault();
    return;
  }
  openFile(fileAt(e));
}

// One delegated handler and menu per rendered block, regardless of link count.
function aimLink(e) {
  aimedLink.value = e.target.closest("a[href]")?.href || "";
  aimedFile.value = fileAt(e);
}

function openLink() {
  if (!aimedLink.value) return;
  if (!openExternal(aimedLink.value)) window.open(aimedLink.value, "_blank", "noopener,noreferrer");
}

const menu = computed(() => aimedFile.value ? [
  { label: "Open in new tab", icon: "i-lucide-file-plus", onSelect: () => openFile(aimedFile.value) },
  { label: "Open in system browser", icon: "i-lucide-external-link", onSelect: () => openInSystem(resolveFileRef(aimedFile.value.path, props.basePath)) },
] : [
  { label: "Open in browser", icon: "i-lucide-external-link", disabled: !aimedLink.value, onSelect: openLink },
  { label: "Copy link", icon: "i-lucide-copy", disabled: !aimedLink.value, onSelect: () => copyText(aimedLink.value) },
]);
</script>

<template>
  <UContextMenu :items="menu">
    <div v-bind="$attrs" v-html="html" @click="onClick" @contextmenu="aimLink" />
  </UContextMenu>
</template>
