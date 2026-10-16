import { computed, ref, watch } from "vue";

/* Only a prefix of a long list becomes rows; the full list stays in memory.
   Lists up to `whole` items show in full. Longer ones start at 250 and each
   step doubles what is shown, at most 1000 more at a time. A change of
   `reset` starts over at 250. */
export function useBatches(source, { whole = 1000, reset = source } = {}) {
  const limit = ref(250);
  watch(reset, () => { limit.value = 250; }, { flush: "sync" });
  const files = computed(() => source() || []);
  const visible = computed(() => files.value.length > whole
    ? files.value.slice(0, limit.value)
    : files.value);
  const more = computed(() => Math.min(
    Math.min(limit.value, 1000),
    files.value.length - visible.value.length,
  ));
  function showMore() {
    limit.value += more.value;
  }
  return { visible, more, showMore };
}
