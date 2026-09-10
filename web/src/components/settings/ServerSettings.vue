<script setup>
/* Whether this desktop window's server also answers a browser, and where.
   Off by default and remembered for the next launch — see store.js and
   specs/043-exposed-server.md. Not offered on a web launch, which already is
   the server on the address it was started with. */
import { computed, ref, watch, watchEffect } from "vue";

import { S, fail, setServerConfig } from "../../store";
import { fuzzyAny } from "../../fuzzy";

const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count"]);

const rows = computed(() =>
  fuzzyAny(
    ["Server", "network", "expose", "browser", "host", "port", "0.0.0.0", "127.0.0.1", "localhost", "lan"],
    props.filter,
  ) !== null
    ? [1]
    : [],
);

watchEffect(() => emit("count", rows.value.length));

const HOSTS = [
  { value: "0.0.0.0", label: "Everybody (0.0.0.0)" },
  { value: "127.0.0.1", label: "localhost (127.0.0.1)" },
  { value: "custom", label: "Other" },
];

const enabled = computed({
  get: () => S.serverConfig.enabled,
  set: (v) => setServerConfig({ enabled: v }),
});

/* mode is which radio is picked, not just a read of the saved host: Other has
   to be selectable — and reveal its field — before a custom host has actually
   been typed into it, which a value derived purely from S.serverConfig.host
   could never represent (a fresh "custom" is indistinguishable from whatever
   well-known host was saved before). Picking Everybody or localhost applies
   at once, same as General's Enter/Enqueue pair; Other only applies once a
   host is actually typed into it, in applyHost. */
const mode = ref("127.0.0.1");
const customHost = ref("");
const port = ref(7717);

watch(
  () => S.serverConfig,
  (c) => {
    mode.value = ["0.0.0.0", "127.0.0.1"].includes(c.host) ? c.host : "custom";
    if (mode.value === "custom") customHost.value = c.host || "";
    port.value = c.port || 7717;
  },
  { immediate: true, deep: true },
);

function chooseMode(value) {
  mode.value = value;
  if (value !== "custom") setServerConfig({ host: value });
}

function applyHost() {
  const host = customHost.value.trim();
  if (!host || host === S.serverConfig.host) return;
  setServerConfig({ host });
}

function applyPort() {
  const n = Math.trunc(Number(port.value));
  if (!n || n < 1 || n > 65535) {
    port.value = S.serverConfig.port;
    fail(new Error("Port must be between 1 and 65535"));
    return;
  }
  if (n === S.serverConfig.port) return;
  setServerConfig({ port: n });
}
</script>

<template>
  <section v-if="rows.length">
    <div class="mb-0.5 font-semibold text-highlighted">Server</div>
    <p class="mb-3.5 text-xs text-dimmed">
      Reach agenttik from a browser, on this machine or another, in addition to this window. It
      carries no login of its own — anyone who can reach the address you pick has the same access
      this window does.
    </p>

    <p v-if="!S.serverConfig.available" class="text-xs text-dimmed">
      Not available here — a web launch is already the server, on the address it was started with.
    </p>

    <template v-else>
      <USwitch v-model="enabled" label="Expose a web server" />

      <div class="mt-3">
        <URadioGroup
          :model-value="mode"
          :items="HOSTS"
          size="sm"
          :ui="{ item: 'py-1' }"
          @update:model-value="chooseMode"
        />
        <UInput
          v-if="mode === 'custom'"
          v-model="customHost"
          placeholder="192.168.1.50"
          class="mt-1.5 ml-6 w-48 font-mono text-xs font-normal"
          @change="applyHost"
          @keydown.enter="$event.target.blur()"
        />
      </div>

      <label class="mt-3 block text-xs font-medium text-muted">
        Port
        <UInput
          v-model="port"
          type="number"
          min="1"
          max="65535"
          class="mt-1 w-28 font-normal"
          @change="applyPort"
          @keydown.enter="$event.target.blur()"
        />
      </label>

      <p v-if="enabled && S.serverConfig.host !== '127.0.0.1'" class="mt-3 text-xs text-warning">
        {{
          S.serverConfig.host === "0.0.0.0"
            ? "Reachable from any device on the network, at this machine's own address and this port."
            : `Reachable from ${S.serverConfig.host}.`
        }}
        There is no login — treat that as a trusted address.
      </p>

      <p
        v-if="enabled"
        class="mt-3 text-xs"
        :class="S.serverConfig.listening ? 'text-dimmed' : 'text-error'"
      >
        <template v-if="S.serverConfig.listening">Listening on {{ S.serverConfig.addr }}.</template>
        <template v-else>Not listening{{ S.serverConfig.error ? ": " + S.serverConfig.error : "" }}.</template>
      </p>
    </template>
  </section>
</template>
