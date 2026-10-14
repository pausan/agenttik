<script setup>
import { ref, shallowRef } from "vue";
import { areaQuote, markedImage } from "../image-area";
import { uploadImage } from "../prompt-images";
import { startThread } from "../store";
import ImageArea from "./ImageArea.vue";
import ImageFrame from "./ImageFrame.vue";

const props = defineProps({
  src: { type: String, required: true },
  title: { type: String, default: "Attached image" },
  source: { type: String, required: true },
  messageId: { type: Number, default: 0 },
});
const emit = defineEmits(["close"]);
const circling = ref(false);
const selection = shallowRef(null);
const saving = ref(false);
const error = ref("");

// A browser can suppress the synthetic click after a touch drag.
function touchAsk(e) {
  if (e.touches.length || e.changedTouches.length !== 1) return;
  const point = e.changedTouches[0];
  const box = e.currentTarget.getBoundingClientRect();
  if (point.clientX < box.left || point.clientX > box.right || point.clientY < box.top || point.clientY > box.bottom) return;
  e.preventDefault();
  ask();
}

async function ask() {
  if (!selection.value || saving.value) return;
  saving.value = true;
  error.value = "";
  try {
    const { area, image } = selection.value;
    const attachment = await uploadImage(await markedImage(image, area));
    startThread(props.messageId, areaQuote(props.source, area, image), attachment);
    emit("close");
  } catch (e) {
    error.value = e.message || "Could not save the selected area.";
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <UModal :open="true" :title="title" :dismissible="!saving"
    :ui="{ content: 'max-w-6xl', body: 'flex h-[78vh] flex-none min-h-0 flex-col gap-3' }" @update:open="!saving && emit('close')">
    <template #body>
      <ImageArea v-if="circling" :src="src" @select="selection = $event" />
      <ImageFrame v-else :src="src" zoomable />
      <div v-if="messageId" class="flex shrink-0 flex-wrap items-center gap-3">
        <UButton size="sm" :color="circling ? 'primary' : 'neutral'" variant="soft" icon="i-lucide-circle"
          label="Circle an area" :aria-pressed="circling" :disabled="saving"
          @click="circling = !circling; selection = null; error = ''" />
        <span v-if="circling" class="text-sm text-muted">Drag around the area you want to ask about.</span>
        <UButton v-if="circling" size="sm" icon="i-lucide-messages-square" label="Ask in thread"
          :loading="saving" :disabled="!selection || saving" @click="ask" @touchend="touchAsk" />
      </div>
      <p v-if="error" role="alert" class="shrink-0 text-sm text-error">{{ error }}</p>
    </template>
  </UModal>
</template>
