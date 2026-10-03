<script setup>
import { onMounted, onUnmounted, ref, watch } from "vue";

const props = defineProps({ message: { type: Object, required: true } });
const emit = defineEmits(["reveal"]);
const text = ref(null);
const truncated = ref(false);
let observer;

function measure() {
  const el = text.value;
  truncated.value = !!el && el.scrollHeight > el.clientHeight + 1;
}

watch(() => props.message.content, measure, { flush: "post" });
onMounted(() => {
  observer = new ResizeObserver(measure);
  observer.observe(text.value);
  measure();
});
onUnmounted(() => observer?.disconnect());
</script>

<template>
  <div class="mx-auto max-w-[860px] px-6 max-md:px-3">
    <div class="flex flex-col items-end bg-default pb-2 pt-2" aria-label="Pinned human prompt">
      <button
        type="button"
        class="max-w-[85%] rounded-[var(--ui-radius)] bg-elevated px-3 py-2 text-left text-highlighted shadow-sm"
        aria-label="Show original human prompt"
        @click="emit('reveal')"
      >
        <span class="relative block">
          <span ref="text" class="line-clamp-3 whitespace-pre-wrap wrap-anywhere">{{ message.content }}</span>
          <span v-if="truncated" aria-hidden="true" class="absolute bottom-0 right-0 whitespace-nowrap bg-elevated pl-1 before:absolute before:inset-y-0 before:right-full before:w-4 before:bg-linear-to-r before:from-transparent before:to-elevated"><span class="text-dimmed">… </span><span class="text-primary">Show more</span></span>
        </span>
      </button>
    </div>
  </div>
</template>
