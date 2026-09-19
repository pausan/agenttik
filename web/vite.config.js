import { writeFileSync } from "node:fs";
import { fileURLToPath, URL } from "node:url";
import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import ui from "@nuxt/ui/vite";

// The UI is embedded in the Go binary, so it builds to web/dist and everything
// it needs — icons, fonts, styles — is bundled. Opt-in Smart Search downloads
// its model separately; its inference runtime is bundled in a lazy worker.
/* dist/ is generated, but `//go:embed all:dist` in embed.go needs the directory to hold
   at least one file even before the UI is built — so a placeholder is tracked.
   Vite empties the directory on every build, so put it back afterwards. */
function keepPlaceholder() {
  return {
    name: "agenttik:keep-placeholder",
    closeBundle() {
      writeFileSync(fileURLToPath(new URL("./dist/.gitkeep", import.meta.url)), "");
    },
  };
}

export default defineConfig({
  worker: { format: "es" },
  plugins: [
    vue(),
    // The defaults; Settings changes both at runtime through the same config.
    // `icon.clientBundle.scan` is off by default for this plugin (unlike
    // `@nuxt/icon`'s own, which scans by default) — without it, only Nuxt UI's
    // built-in icons are bundled and every icon agenttik actually uses falls
    // back to a live fetch against the Iconify API the first time it renders.
    ui({
      colorMode: false, // The app persists color mode within the selected profile.
      ui: {
        colors: { primary: "blue", neutral: "zinc" },
        // Portals must sit above pane scrollbars and resize handles. Keep
        // the same level so later portals (including nested dialogs) win.
        modal: { slots: { overlay: "z-50", content: "z-50" } },
        slideover: { slots: { overlay: "z-50", content: "z-50" } },
        popover: { slots: { content: "z-50" } },
        tooltip: { slots: { content: "z-50" } },
        dropdownMenu: { slots: { content: "z-50" } },
        contextMenu: { slots: { content: "z-50" } },
        select: { slots: { content: "z-50" } },
        selectMenu: { slots: { content: "z-50" } },
        inputMenu: { slots: { content: "z-50" } },
      },
      icon: { clientBundle: { scan: { globInclude: ["src/**/*.{vue,js}"] } } },
    }),
    keepPlaceholder(),
  ],
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
  },
  build: {
    outDir: fileURLToPath(new URL("./dist", import.meta.url)),
    emptyOutDir: true,
  },
  // `npm run dev` serves the UI with hot reload and sends the API — SSE
  // included — to an `agenttik --web` running on its default port.
  server: {
    proxy: {
      "/api": { target: "http://127.0.0.1:7717", changeOrigin: true },
    },
  },
});
