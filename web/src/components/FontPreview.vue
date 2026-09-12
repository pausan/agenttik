<script setup>
import { computed, ref, useId, watch } from "vue";

const props = defineProps({ src: { type: String, required: true } });
const id = useId();
const family = `preview-font-${id.replace(/[^a-z0-9-]/gi, "")}`;
const defaultText = "The quick brown fox jumps over the lazy dog. 0123456789";
const sample = ref(defaultText);
const selectedSize = ref(48);
const size = computed(() => Math.min(160, Math.max(8, Number(selectedSize.value) || 48)));
const fontStyle = computed(() => ({ fontFamily: `"${family}"`, fontSize: `${size.value}px` }));
const glyphs = Array.from({ length: 94 }, (_, i) => String.fromCharCode(i + 33));
const status = ref("loading");

// A reused file tab can change source while the previous font is loading.
// Only its current face belongs in the document, and unmounting releases it.
watch(() => props.src, async (src, _, onCleanup) => {
  let active = true;
  const face = new FontFace(family, `url(${JSON.stringify(src)})`);
  onCleanup(() => {
    active = false;
    document.fonts.delete(face);
  });
  status.value = "loading";
  sample.value = defaultText;
  selectedSize.value = 48;
  try {
    await face.load();
    if (!active) return;
    document.fonts.add(face);
    status.value = "ready";
  } catch {
    if (active) status.value = "error";
  }
}, { immediate: true });
</script>

<template>
  <div class="min-w-0 space-y-6 px-5 py-5">
    <div class="flex flex-wrap items-end gap-3">
      <div class="min-w-0 flex-1 basis-64 space-y-1">
        <label :for="`${id}-text`" class="block text-xs text-muted">Sample text</label>
        <UInput :id="`${id}-text`" v-model="sample" class="w-full" />
      </div>
      <UButton color="neutral" variant="soft" @click="sample = defaultText">Reset text</UButton>
      <div class="space-y-1">
        <label :for="`${id}-size`" class="block text-xs text-muted">Size (px)</label>
        <UInput
          :id="`${id}-size`"
          v-model="selectedSize"
          type="number"
          :min="8"
          :max="160"
          :step="1"
          class="w-24"
          @blur="selectedSize = size"
        />
      </div>
    </div>

    <p v-if="status === 'loading'" role="status" class="text-dimmed">Loading font…</p>
    <p v-else-if="status === 'error'" role="alert" class="text-warning">
      Cannot preview this font. The file may be invalid or unsupported by this browser.
    </p>
    <template v-else>
      <div class="overflow-x-auto rounded border border-default bg-elevated/30 p-4">
        <p aria-label="Font sample" class="font-sample whitespace-pre" :style="fontStyle">{{ sample || '\u00a0' }}</p>
      </div>
      <section aria-label="Sample glyphs" class="space-y-3">
        <div class="text-xs text-muted">
          Sample glyphs · squares are {{ size }} × {{ size }} px (1 em), with ¼ em padding.
        </div>
        <div class="flex flex-wrap gap-2" :style="fontStyle">
          <div v-for="glyph in glyphs" :key="glyph" class="glyph-cell" :title="`U+${glyph.codePointAt(0).toString(16).toUpperCase().padStart(4, '0')}`">
            <span class="glyph-square">{{ glyph }}</span>
          </div>
        </div>
      </section>
    </template>
  </div>
</template>

<style scoped>
.font-sample {
  line-height: 1.5;
}
.glyph-cell {
  padding: 0.25em;
  border-radius: 4px;
  background: var(--ui-bg-elevated);
}
.glyph-square {
  display: block;
  width: 1em;
  height: 1em;
  outline: 1px solid var(--ui-border);
  text-align: center;
  line-height: 1;
}
</style>
