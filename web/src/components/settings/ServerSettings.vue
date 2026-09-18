<script setup>
/* Whether this desktop window's server also answers a browser, and where.
   Off by default and remembered for the next launch — see store.js and
   specs/043-exposed-server.md. Not offered on a web launch, which already is
   the server on the address it was started with. */
import { computed, ref, watch, watchEffect } from "vue";

import {
  S,
  loadInstanceInfo,
  copyText,
  fail,
  openExternal,
  resetServerTOTP,
  setServerAuth,
  setServerConfig,
} from "../../store";
import { api } from "../../api";
import { fuzzyAny } from "../../fuzzy";

const props = defineProps({
  filter: { type: String, default: "" },
  active: { type: Boolean, default: false },
});
const emit = defineEmits(["count"]);

const rows = computed(() =>
  fuzzyAny(
    ["Server", "name", "instance", "network", "expose", "browser", "host", "port", "0.0.0.0", "127.0.0.1", "localhost", "lan",
     "login", "password", "totp", "2fa", "authenticator", "qr", "seed", "secure"],
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

const serverName = ref("");
const nameError = ref("");
const savingName = ref(false);
watch(() => S.instanceInfo.name, (name) => { serverName.value = name || ""; }, { immediate: true });

watch(serverName, () => { nameError.value = ""; });

async function saveName() {
  nameError.value = "";
  const name = serverName.value.trim();
  if ([...name].length < 3) {
    nameError.value = "Server name must contain at least 3 characters.";
    return;
  }
  savingName.value = true;
  try {
    await api("PUT", "/api/server/name", { name });
    await loadInstanceInfo();
  } catch (e) {
    nameError.value = e.message;
  } finally {
    savingName.value = false;
  }
}

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

/* WILDCARD is what a listener bound to every interface reports itself as.
   Everybody asks for 0.0.0.0, but a dual-stack machine answers [::], which
   is where the address a browser is handed parts ways with the one that was
   bound. */
const WILDCARD = ["0.0.0.0", "::", "[::]"];

/* The address to actually open. A wildcard is every interface, not a
   destination — no browser resolves it — so the link points at loopback,
   which reaches that same listener from the machine it runs on. The warning
   above is what says it answers the network as well. */
const url = computed(() => {
  const addr = S.serverConfig.addr || "";
  const colon = addr.lastIndexOf(":");
  const host = colon < 0 ? addr : addr.slice(0, colon);
  return WILDCARD.includes(host) ? `http://localhost${addr.slice(colon)}` : `http://${addr}`;
});

/* The anchor is the whole story in a browser, which opens the tab itself.
   Only the desktop window needs the click taken off it. */
function follow(e) {
  if (openExternal(url.value)) e.preventDefault();
}

const remoteCommand = computed(() => `agenttik --remote ${url.value}`);
const commandCopied = ref(false);
async function copyCommand() {
  if (!(await copyText(remoteCommand.value))) return;
  commandCopied.value = true;
  window.setTimeout(() => (commandCopied.value = false), 1200);
}

const copied = ref(false);
async function copyLink() {
  if (!(await copyText(url.value))) return;
  copied.value = true;
  window.setTimeout(() => (copied.value = false), 1200);
}

/* Enabling the lock takes effect even before a password is saved. */
const password = ref("");
const busy = ref(false);

function toggleAuth(on) {
  apply({ enabled: on });
}

async function apply(patch) {
  busy.value = true;
  try {
    await setServerAuth(patch);
  } catch (e) {
    fail(e);
  } finally {
    busy.value = false;
  }
}

// Clear the password field after submitting it.
async function savePassword() {
  if (!password.value || busy.value) return;
  const pw = password.value;
  password.value = "";
  await apply({ enabled: true, password: pw });
}

/* The seed field is the whole of "reset it or set it": it shows the current
   one as selectable text, takes a typed one on blur, and the button beside it
   rolls a random one. */
const seed = ref("");
watch(
  () => S.serverConfig.totp_secret,
  (v) => (seed.value = v || ""),
  { immediate: true },
);

function applySeed() {
  const v = seed.value.trim();
  if (!v || v === S.serverConfig.totp_secret) {
    seed.value = S.serverConfig.totp_secret;
    return;
  }
  apply({ totp_secret: v }).then(() => (seed.value = S.serverConfig.totp_secret));
}

async function rollSeed() {
  busy.value = true;
  try {
    await resetServerTOTP();
  } catch (e) {
    fail(e);
  } finally {
    busy.value = false;
  }
}

const seedCopied = ref(false);
async function copySeed() {
  if (!(await copyText(S.serverConfig.totp_secret))) return;
  seedCopied.value = true;
  window.setTimeout(() => (seedCopied.value = false), 1200);
}

// Only poll while this pane is visible. Cleanup also discards old seed responses.
const currentCode = ref("");
const codeCopied = ref(false);
async function copyCode() {
  if (!currentCode.value || !(await copyText(currentCode.value))) return;
  codeCopied.value = true;
  window.setTimeout(() => (codeCopied.value = false), 1200);
}
watch(
  () => [props.active, rows.value.length, S.serverConfig.auth_enabled, S.serverConfig.totp_secret, S.serverConfig.totp_enabled],
  ([active, visible, enabled, secret, totpEnabled], _, onCleanup) => {
    let stopped = false;
    let timer;
    currentCode.value = "";
    onCleanup(() => {
      stopped = true;
      clearTimeout(timer);
    });
    if (!active || !visible || !enabled || !secret || !totpEnabled) return;
    async function refresh() {
      let delay = 30000;
      try {
        const result = await api("GET", "/api/server/auth/code");
        if (stopped) return;
        currentCode.value = result.code;
        delay = Math.max(100, result.refresh_after_ms);
      } catch {
        if (stopped) return;
        currentCode.value = "";
      }
      timer = setTimeout(refresh, delay);
    }
    refresh();
  },
  { immediate: true },
);

/* The QR is an endpoint rather than a data URI, so the seed itself is the
   cache key: it changes exactly when the picture has to. */
const qr = computed(() =>
  S.serverConfig.totp_secret
    ? `/api/server/auth/totp.png?seed=${encodeURIComponent(S.serverConfig.totp_secret)}`
    : "",
);
</script>

<template>
  <section v-if="rows.length">
    <div class="mb-0.5 font-semibold text-highlighted">Server</div>
    <p class="mb-3.5 text-xs text-dimmed">
      Reach agenttik from a browser, on this machine or another, in addition to this window.
      Whoever opens the address you pick gets the same access this window has, so put a login on it
      unless the address is one only you can reach.
    </p>

    <form class="mb-4" @submit.prevent="saveName">
      <UFormField label="Server name" help="At least 3 characters. Shown to clients before they sign in." :error="nameError || false">
        <div class="mt-1 flex items-center gap-2">
          <UInput v-model="serverName" aria-label="Server name" class="w-64" :disabled="savingName" />
          <UButton type="submit" label="Save" size="xs" color="neutral" variant="subtle" :loading="savingName" />
        </div>
      </UFormField>
    </form>

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
        {{
          S.serverConfig.auth_enabled
            ? (S.serverConfig.totp_enabled ? "It asks for a password and a code." : "It asks for a password.")
            : "There is no login — treat that as a trusted address."
        }}
      </p>

      <div class="mt-4 border-t border-default pt-3.5">
        <USwitch
          :model-value="S.serverConfig.auth_enabled"
          :disabled="busy"
          label="Enable password"
          @update:model-value="toggleAuth"
        />
        <p class="mt-1 text-xs text-dimmed">
          Only the browser is asked. This window talks to its own private connection and never signs
          in, so a forgotten password locks nobody out of here.
        </p>

        <div v-if="S.serverConfig.auth_enabled" class="mt-3.5">
          <p v-if="!S.serverConfig.has_password" class="mb-2 text-xs text-warning">
            Browser access is blocked until you save a password.
          </p>
          <div class="text-xs font-medium text-muted">
            {{ S.serverConfig.has_password ? "Change the password" : "Password" }}
          </div>
          <div class="mt-1 flex items-center gap-1.5">
            <UInput
              v-model="password"
              type="password"
              autocomplete="new-password"
              :placeholder="`At least 8 characters`"
              class="w-44 font-normal"
              @keydown.enter="savePassword"
            />
            <UButton
              label="Save"
              size="xs"
              color="neutral"
              variant="subtle"
              :disabled="!password || busy"
              @click="savePassword"
            />
          </div>

          <USwitch
            :model-value="S.serverConfig.totp_enabled"
            :disabled="busy"
            label="Enable 2FA"
            class="mt-4"
            @update:model-value="apply({ totp_enabled: $event })"
          />
          <div v-if="S.serverConfig.totp_enabled">
            <div class="mt-4 text-xs font-medium text-muted">Authenticator</div>
            <p class="mt-0.5 text-xs text-dimmed">
              Scan this with an authenticator app, or type the seed into one by hand.
            </p>
            <div class="mt-2 flex items-start gap-3">
              <img
                v-if="qr"
                :src="qr"
                alt="QR code pairing an authenticator app with this server"
                width="128"
                height="128"
                class="shrink-0 rounded bg-white p-1.5"
              />
              <div>
                <div class="flex items-center gap-1">
                  <UInput
                    v-model="seed"
                    spellcheck="false"
                    autocapitalize="off"
                    class="w-80 font-mono text-xs font-normal"
                    aria-label="Authenticator seed"
                    @change="applySeed"
                    @keydown.enter="$event.target.blur()"
                  />
                  <UButton
                    :icon="seedCopied ? 'i-lucide-check' : 'i-lucide-copy'"
                    size="xs"
                    color="neutral"
                    variant="ghost"
                    aria-label="Copy the seed"
                    title="Copy the seed"
                    @click="copySeed"
                  />
                </div>
                <div class="mt-2">
                  <div class="text-xs text-muted">Current code</div>
                  <div class="flex items-center gap-1">
                    <output aria-label="Current code" class="font-mono text-lg tracking-widest">{{ currentCode || "------" }}</output>
                    <UButton
                      :icon="codeCopied ? 'i-lucide-check' : 'i-lucide-copy'"
                      size="xs"
                      color="neutral"
                      variant="ghost"
                      aria-label="Copy the code"
                      title="Copy the code"
                      :disabled="!currentCode"
                      @click="copyCode"
                    />
                  </div>
                  <p class="text-xs text-dimmed">Refreshes every 30 seconds.</p>
                </div>
                <UButton
                  label="New seed"
                  icon="i-lucide-refresh-cw"
                  size="xs"
                  color="neutral"
                  variant="subtle"
                  class="mt-2"
                  :disabled="busy"
                  @click="rollSeed"
                />
                <p class="mt-2 max-w-80 text-xs text-dimmed">
                  Changing sign-in settings signs every browser out. A new seed needs pairing again.
                </p>
              </div>
            </div>
          </div>
        </div>
      </div>

      <p
        v-if="enabled"
        class="mt-3 flex items-center gap-1.5 text-xs"
        :class="S.serverConfig.listening ? 'text-dimmed' : 'text-error'"
      >
        <template v-if="S.serverConfig.listening">
          <span>Listening on</span>
          <a
            :href="url"
            target="_blank"
            rel="noreferrer noopener"
            class="font-mono text-primary hover:underline"
            @click="follow"
          >{{ url }}</a>
          <UButton
            :icon="copied ? 'i-lucide-check' : 'i-lucide-copy'"
            size="xs"
            color="neutral"
            variant="ghost"
            aria-label="Copy the server link"
            title="Copy link"
            @click="copyLink"
          />
        </template>
        <template v-else>Not listening{{ S.serverConfig.error ? ": " + S.serverConfig.error : "" }}.</template>
      </p>
      <div v-if="enabled && S.serverConfig.listening" class="mt-2 flex items-center gap-1.5 text-xs text-dimmed">
        <code class="select-text break-all">{{ remoteCommand }}</code>
        <UButton
          :icon="commandCopied ? 'i-lucide-check' : 'i-lucide-copy'"
          size="xs"
          color="neutral"
          variant="ghost"
          aria-label="Copy remote command"
          title="Copy remote command"
          @click="copyCommand"
        />
      </div>
    </template>
  </section>
</template>
