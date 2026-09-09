import { createApp } from "vue";
import { createRouter, createWebHistory } from "vue-router";
import ui from "@nuxt/ui/vue-plugin";

import App from "./App.vue";
import "./assets/main.css";

// Nuxt UI's Vue plugin wants a router for its link components. agenttik is one
// page, so this is the smallest router that satisfies it.
const router = createRouter({
  history: createWebHistory(),
  routes: [{ path: "/:rest(.*)*", component: { render: () => null } }],
});

createApp(App).use(router).use(ui).mount("#app");
