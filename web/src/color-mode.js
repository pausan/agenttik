import { ref, watch } from "vue";
import { useColorMode } from "@vueuse/core";
import { storage } from "./api";

// Keep the existing key so Default retains its saved appearance.
const KEY = "vueuse-color-scheme";
export const COLOR_MODES = [
  { label: "System", value: "auto", icon: "i-lucide-monitor" },
  { label: "Light", value: "light", icon: "i-lucide-sun" },
  { label: "Dark", value: "dark", icon: "i-lucide-moon" },
];
export const colorPreference = ref("auto");

// Called once, after the profile is known and before the first paint.
export function loadColorMode() {
  try {
    const saved = storage.getItem(KEY);
    if (COLOR_MODES.some(mode => mode.value === saved)) colorPreference.value = saved;
  } catch { /* Keep the system preference when storage is unavailable. */ }
  useColorMode({ storageRef: colorPreference });
  watch(colorPreference, value => {
    try { storage.setItem(KEY, value); }
    catch { /* The visible preference still changes without persistence. */ }
  });
}
