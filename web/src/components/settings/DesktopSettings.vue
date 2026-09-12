<script setup>
import { computed, onMounted, ref, watchEffect } from "vue";
import { api } from "../../api";
import { fail } from "../../store";
import { fuzzyAny } from "../../fuzzy";

const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count"]);
const config = ref(null);
const enabled = ref(false);
const shortcut = ref("Ctrl+Shift+A");
const saving = ref(false);
const saved = ref(false);
const visible = computed(() => config.value?.available && fuzzyAny(
  ["general", "desktop", "tray", "close", "hide", "show", "global", "shortcut", "keyboard", shortcut.value], props.filter,
) !== null);
watchEffect(() => emit("count", visible.value ? 2 : 0));
onMounted(async () => {
  try {
    config.value = await api("GET", "/api/desktop");
    enabled.value = config.value.close_to_tray;
    shortcut.value = config.value.toggle_shortcut;
  } catch (e) { fail(e); }
});
async function save() {
  saving.value = true;
  saved.value = false;
  try {
    config.value = await api("PUT", "/api/desktop", {
      close_to_tray: enabled.value,
      toggle_shortcut: shortcut.value.trim(),
    });
    shortcut.value = config.value.toggle_shortcut;
    saved.value = true;
  } catch (e) { fail(e); }
  finally { saving.value = false; }
}
</script>

<template>
  <section v-if="visible" class="mb-5">
    <div class="mb-0.5 font-semibold text-highlighted">Desktop tray</div>
    <p class="mb-2 text-xs text-dimmed">
      Close the window to the system tray and keep tasks running. Use the global shortcut
      or the tray's Show / Hide action to toggle the window. Choose Quit in the tray to exit.
    </p>
    <form class="flex flex-col items-start gap-2" @submit.prevent="save">
      <UCheckbox v-model="enabled" label="Close to tray" :disabled="saving" @update:model-value="saved = false" />
      <label for="tray-shortcut" class="text-sm text-muted">Show / hide shortcut</label>
      <UInput id="tray-shortcut" v-model="shortcut" :disabled="saving" placeholder="Ctrl+Shift+A" @update:model-value="saved = false" />
      <p class="text-xs text-dimmed">Use Ctrl, Alt or Shift with A–Z, 0–9 or F1–F12. Default: Ctrl+Shift+A.</p>
      <UButton type="submit" size="sm" :loading="saving">Save tray settings</UButton>
      <p class="text-xs" :class="saved ? 'text-success' : 'text-dimmed'">
        {{ saved ? "Saved. Restart agenttik to apply these settings." : "Changes take effect after restarting agenttik." }}
      </p>
      <p v-if="config.error" role="alert" class="text-xs text-warning">{{ config.error }}</p>
    </form>
  </section>
</template>
