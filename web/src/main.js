import { createApp } from "vue";
import { createRouter, createWebHistory } from "vue-router";
import { _api as iconApi } from "@iconify/vue";
import ui from "@nuxt/ui/vue-plugin";

import { loadColorMode } from "./color-mode";
import { loadProfiles } from "./profiles";
import App from "./App.vue";
import { loadColors } from "./store";
import "./assets/main.css";

// Every icon agenttik uses is bundled at build time (vite.config.js's
// `icon.clientBundle.scan`). If one is ever missed — a name built at runtime
// rather than written as a literal, say — Iconify's default behaviour is to
// fetch it from api.iconify.design and two mirrors, which is slow to fail
// with no network and is a network call this local tool should never make.
// Refusing the fetch turns that into an instantly blank icon plus a loud
// console error, instead of a page that hangs offline.
iconApi.setFetch(() => {
  console.error("blocked an icon fetch: an icon is missing from the offline bundle");
  return Promise.reject(new Error("icons are bundled at build time; no network fetch"));
});

// Nuxt UI's Vue plugin wants a router for its link components. agenttik is one
// page, so this is the smallest router that satisfies it.
const router = createRouter({
  history: createWebHistory(),
  routes: [{ path: "/:rest(.*)*", component: { render: () => null } }],
});

// The saved colours are applied before mounting, so the app is never painted
// in the default palette first and repainted in the chosen one.
loadProfiles().then(() => {
  loadColorMode();
  loadColors();
  createApp(App).use(router).use(ui).mount("#app");
}).catch((error) => {
  document.getElementById("app").textContent = `Could not load profiles: ${error.message}. Reload to try again.`;
});
