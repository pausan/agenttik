<script setup>
import { computed, ref } from "vue";

import { markdown } from "../markdown";
import { copyText, openExternal, openFileRef, resolveFileRef } from "../store";

defineOptions({ inheritAttrs: false });

const props = defineProps({
  text: { type: String, default: "" },
  basePath: { type: String, default: "" },
});
const html = computed(() => markdown(props.text));
const block = ref(null);
const aimedLink = ref("");
const aimedFile = ref(null);

function fileAt(e) {
  const hit = e.target.closest("button.file");
  return hit ? { path: hit.dataset.file, line: Number(hit.dataset.line) || 0 } : null;
}

function openFile(file, system = false) {
  if (file) {
    const hints = [...block.value.querySelectorAll("button.file")].map((link) => link.dataset.file);
    openFileRef(file.path, file.line, props.basePath, hints, system);
  }
}

// The tick shows on the button that was clicked until the next render
// replaces it or the timer takes it off.
async function copyCode(button) {
  if (!(await copyText(button.parentElement.querySelector("code").textContent))) return;
  button.classList.add("copied");
  window.setTimeout(() => button.classList.remove("copied"), 1200);
}

function onClick(e) {
  const copy = e.target.closest("button.copy");
  if (copy) {
    copyCode(copy);
    return;
  }
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
  { label: "Open in system browser", icon: "i-lucide-external-link", onSelect: () => openFile(aimedFile.value, true) },
  { label: "Copy path", icon: "i-lucide-copy", onSelect: () => copyText(resolveFileRef(aimedFile.value.path, props.basePath) || ".") },
] : [
  { label: "Open in browser", icon: "i-lucide-external-link", disabled: !aimedLink.value, onSelect: openLink },
  { label: "Copy link", icon: "i-lucide-copy", disabled: !aimedLink.value, onSelect: () => copyText(aimedLink.value) },
]);
</script>

<template>
  <UContextMenu :items="menu">
    <div ref="block" v-bind="$attrs" v-html="html" @click="onClick" @contextmenu="aimLink" />
  </UContextMenu>
</template>
