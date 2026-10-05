<script setup>
import { computed, ref, watch } from "vue";
import { editFile } from "../store";
import { HEX_PAGE_BYTES, hexRows, parseHex, replaceHex } from "../hex-editor";

const props = defineProps({ tab: { type: Object, required: true } });
const hex = computed(() => props.tab.edited ?? props.tab.content ?? "");
const offset = computed(() => props.tab.hexOffset || 0);
const from = computed(() => Math.floor(offset.value / HEX_PAGE_BYTES) * HEX_PAGE_BYTES);
const size = computed(() => hex.value.length / 2);
const rows = computed(() => hexRows(hex.value, from.value));
const address = ref("0");
const replacement = ref("");
const error = ref("");
const undo = ref([]);
const redo = ref([]);
const formatOffset = (value) => value.toString(16).padStart(8, "0");

function select(value) {
  props.tab.hexOffset = Math.max(0, Math.min(value, Math.max(0, size.value - 1)));
  address.value = offset.value.toString(16);
  replacement.value = hex.value.slice(offset.value * 2, offset.value * 2 + 2);
  error.value = "";
}
watch(() => props.tab.id, () => { undo.value = []; redo.value = []; select(props.tab.hexOffset || 0); }, { immediate: true });
watch(() => props.tab.content, () => { undo.value = []; redo.value = []; select(offset.value); });

function jump() {
  if (!/^(?:0x)?[0-9a-f]+$/i.test(address.value.trim())) {
    error.value = "Enter a hexadecimal offset.";
    return;
  }
  const value = parseInt(address.value.trim(), 16);
  if (!Number.isSafeInteger(value) || value >= size.value) {
    error.value = "Offset is outside the loaded bytes.";
    return;
  }
  select(value);
}
function apply() {
  if (props.tab.readOnly) return;
  const bytes = parseHex(replacement.value);
  const next = replaceHex(hex.value, offset.value, bytes);
  if (next === null) {
    error.value = "Enter complete hex bytes (00–FF) that fit in the file.";
    return;
  }
  if (next !== hex.value) {
    undo.value.push({ offset: offset.value, before: hex.value.slice(offset.value * 2, offset.value * 2 + bytes.length), after: bytes });
    redo.value = [];
    editFile(props.tab, next);
  }
  error.value = "";
}
function history(backwards) {
  if (props.tab.readOnly) return;
  const source = backwards ? undo.value : redo.value;
  const target = backwards ? redo.value : undo.value;
  const patch = source.pop();
  if (!patch) return;
  editFile(props.tab, replaceHex(hex.value, patch.offset, backwards ? patch.before : patch.after));
  target.push(patch);
  select(patch.offset);
}
function onKey(event) {
  if (event.target.tagName === "INPUT" || (!event.ctrlKey && !event.metaKey) || event.altKey) return;
  if (event.code !== "KeyZ" && event.code !== "KeyY") return;
  event.preventDefault();
  history(event.code === "KeyZ" && !event.shiftKey);
}
</script>

<template>
  <div class="px-5 py-4 font-mono text-xs" tabindex="0" aria-label="Hex editor" @keydown="onKey">
    <form class="flex flex-wrap items-center gap-2 pb-3" @submit.prevent="jump">
      <label for="hex-offset">Offset (hex)</label>
      <input id="hex-offset" v-model="address" class="w-28 rounded border border-default bg-default px-2 py-1" autocomplete="off" spellcheck="false" />
      <UButton type="submit" size="xs" color="neutral" variant="soft">Go</UButton>
      <UButton size="xs" color="neutral" variant="ghost" :disabled="!from" @click="select(from - HEX_PAGE_BYTES)">Previous page</UButton>
      <UButton size="xs" color="neutral" variant="ghost" :disabled="from + HEX_PAGE_BYTES >= size" @click="select(from + HEX_PAGE_BYTES)">Next page</UButton>
      <span>{{ formatOffset(from) }}–{{ formatOffset(Math.max(from, Math.min(from + HEX_PAGE_BYTES, size) - 1)) }}</span>
    </form>
    <form v-if="!tab.readOnly && size" class="flex flex-wrap items-center gap-2 pb-3" @submit.prevent="apply">
      <label for="hex-bytes">Replace bytes</label>
      <input id="hex-bytes" v-model="replacement" class="w-48 rounded border border-default bg-default px-2 py-1" autocomplete="off" spellcheck="false" placeholder="00 FF 80" />
      <UButton type="submit" size="xs" color="neutral" variant="soft">Apply bytes</UButton>
      <UButton size="xs" color="neutral" variant="ghost" :disabled="!undo.length" @click="history(true)">Undo</UButton>
      <UButton size="xs" color="neutral" variant="ghost" :disabled="tab.readOnly || !redo.length" @click="history(false)">Redo</UButton>
      <span>Replaces bytes at {{ formatOffset(offset) }}; keeps the file size.</span>
    </form>
    <p v-if="error" role="alert" class="pb-3 text-warning">{{ error }}</p>
    <p v-if="tab.partial" class="pb-3 text-warning">Showing the first {{ size.toLocaleString() }} bytes. This file is too large to edit.</p>
    <p v-if="!size" class="text-dimmed">Empty file.</p>
    <div v-else class="overflow-x-auto">
      <div class="hex-grid min-w-max" aria-label="Hex bytes">
        <div class="flex gap-4 pb-2 text-dimmed"><span class="w-[8ch]">Offset</span><span class="w-[48ch]">Hex bytes</span><span>ASCII</span></div>
        <div v-for="row in rows" :key="row.offset" class="flex items-center gap-4 leading-6">
          <span class="w-[8ch] text-dimmed">{{ formatOffset(row.offset) }}</span>
          <div class="flex w-[48ch]">
            <button v-for="(byte, i) in row.bytes" :key="i" type="button" class="w-[3ch] shrink-0 rounded text-left hover:bg-elevated" :class="{ 'bg-primary/20 text-primary': offset === row.offset + i }" :aria-label="`Byte ${formatOffset(row.offset + i)}: ${byte}`" :aria-pressed="offset === row.offset + i" @click="select(row.offset + i)">{{ byte }}</button>
          </div>
          <span class="whitespace-pre text-muted" aria-label="ASCII bytes">{{ row.ascii }}</span>
        </div>
      </div>
    </div>
  </div>
</template>
