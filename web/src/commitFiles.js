import { computed, ref, watch } from "vue";

// Only the visible prefix becomes file rows; the full response stays cached.
export function useCommitFiles(source) {
  const limit = ref(250);
  watch(source, () => { limit.value = 250; }, { flush: "sync" });
  const files = computed(() => source() || []);
  const visible = computed(() => files.value.length > 1000
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
