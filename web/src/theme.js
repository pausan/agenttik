/* The colours Settings offers, and the one place that talks to Nuxt UI's
   palette config.

   Nuxt UI keeps one reactive style tag that maps `--ui-primary` and the whole
   neutral scale onto a Tailwind palette, so recolouring the app is setting two
   names — every component follows on its own. Light mode takes shade 500 and
   dark mode 400, which is why a palette is picked by name rather than by hex.

   The swatch classes are written out instead of built from the name because
   Tailwind only emits the colours it can see in the source. */
import { useAppConfig } from "@nuxt/ui/runtime/vue/composables/useAppConfig.js";

/* Round the hue wheel from agenttik's own green. */
export const ACCENTS = [
  { name: "agenttik", swatch: "bg-agenttik-500" },
  { name: "emerald", swatch: "bg-emerald-500" },
  { name: "green", swatch: "bg-green-500" },
  { name: "lime", swatch: "bg-lime-500" },
  { name: "yellow", swatch: "bg-yellow-500" },
  { name: "amber", swatch: "bg-amber-500" },
  { name: "orange", swatch: "bg-orange-500" },
  { name: "red", swatch: "bg-red-500" },
  { name: "rose", swatch: "bg-rose-500" },
  { name: "pink", swatch: "bg-pink-500" },
  { name: "fuchsia", swatch: "bg-fuchsia-500" },
  { name: "purple", swatch: "bg-purple-500" },
  { name: "violet", swatch: "bg-violet-500" },
  { name: "indigo", swatch: "bg-indigo-500" },
  { name: "blue", swatch: "bg-blue-500" },
  { name: "sky", swatch: "bg-sky-500" },
  { name: "cyan", swatch: "bg-cyan-500" },
  { name: "teal", swatch: "bg-teal-500" },
];

/* The grey carries every surface, border and label, so the choice is between
   Tailwind's five, from cool to warm. Tailwind's own `neutral` swatch is
   `old-neutral`: Nuxt UI takes the name `neutral` for the grey in use, so
   `bg-neutral-500` would draw the selected one five times over. */
export const NEUTRALS = [
  { name: "slate", swatch: "bg-slate-500" },
  { name: "gray", swatch: "bg-gray-500" },
  { name: "zinc", swatch: "bg-zinc-500" },
  { name: "neutral", swatch: "bg-old-neutral-500" },
  { name: "stone", swatch: "bg-stone-500" },
];

export const DEFAULT_COLORS = { accent: "blue", neutral: "zinc" };

export function applyColors({ accent, neutral }) {
  const { colors } = useAppConfig().ui;
  colors.primary = accent;
  colors.secondary = accent === "violet" ? "pink" : "violet";
  colors.neutral = neutral;
}
