import { writeFileSync } from "node:fs";
import { fileURLToPath, URL } from "node:url";
import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import ui from "@nuxt/ui/vite";

// The UI is embedded in the Go binary, so it builds to web/dist and everything
// it needs — icons, fonts, styles — is bundled. Nothing is fetched at runtime.
/* web/dist is generated, but `//go:embed all:dist` needs the directory to hold
   at least one file even before the UI is built — so a placeholder is tracked.
   Vite empties the directory on every build, so put it back afterwards. */
function keepPlaceholder() {
  return {
    name: "agenttik:keep-placeholder",
    closeBundle() {
      writeFileSync(fileURLToPath(new URL("../dist/.gitkeep", import.meta.url)), "");
    },
  };
}

export default defineConfig({
  plugins: [
    vue(),
    ui({ ui: { colors: { primary: "green", neutral: "zinc" } } }),
    keepPlaceholder(),
  ],
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
  },
  build: {
    outDir: fileURLToPath(new URL("../dist", import.meta.url)),
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
