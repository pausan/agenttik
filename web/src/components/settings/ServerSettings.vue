<script setup>
/* Whether this desktop window's server also answers a browser, and where.
   Off by default and remembered for the next launch — see store.js and
   specs/043-exposed-server.md. Not offered on a web launch, which already is
   the server on the address it was started with. */
import { computed, ref, watch, watchEffect } from "vue";

import {
  S,
  copyText,
  fail,
  openExternal,
  resetServerTOTP,
  setServerAuth,
  setServerConfig,
} from "../../store";
import { fuzzyAny } from "../../fuzzy";

const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count"]);

const rows = computed(() =>
  fuzzyAny(
    ["Server", "network", "expose", "browser", "host", "port", "0.0.0.0", "127.0.0.1", "localhost", "lan",
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

const copied = ref(false);
async function copyLink() {
  if (!(await copyText(url.value))) return;
  copied.value = true;
  window.setTimeout(() => (copied.value = false), 1200);
}

/* ------------------------------------------------------------------ the lock

   Turning it on needs a password, which cannot be read back out of the
   server, so the switch alone is not enough to do it with: asking for one
   opens the fields instead, and the first saved password is what actually
   turns it on. Turning it off is immediate — there is nothing to collect. */

const setUp = ref(false); // the password fields are open
const password = ref("");
const confirm = ref("");
const busy = ref(false);

const showAuth = computed(() => S.serverConfig.auth_enabled || setUp.value);

function toggleAuth(on) {
  if (!on) {
    setUp.value = false;
    apply({ enabled: false });
    return;
  }
  // A password already set is all it takes; otherwise collect one first.
  if (S.serverConfig.has_password) apply({ enabled: true });
  else setUp.value = true;
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

/* Saving a password is the one thing here with two fields to agree, and the
   one thing the server cannot tell you it got wrong after the fact — so the
   match is checked before it is sent, and the boxes are emptied either way
   rather than left holding a password on screen. */
async function savePassword() {
  if (password.value !== confirm.value) {
    fail(new Error("The two passwords are different"));
    return;
  }
  const pw = password.value;
  password.value = confirm.value = "";
  await apply({ enabled: true, password: pw });
  setUp.value = false;
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
            ? "It asks for a password and a code."
            : "There is no login — treat that as a trusted address."
        }}
      </p>

      <div class="mt-4 border-t border-default pt-3.5">
        <USwitch
          :model-value="S.serverConfig.auth_enabled"
          :disabled="busy"
          label="Ask for a password and a code"
          @update:model-value="toggleAuth"
        />
        <p class="mt-1 text-xs text-dimmed">
          Only the browser is asked. This window talks to its own private connection and never signs
          in, so a forgotten password locks nobody out of here.
        </p>

        <div v-if="showAuth" class="mt-3.5">
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
            />
            <UInput
              v-model="confirm"
              type="password"
              autocomplete="new-password"
              placeholder="Again"
              class="w-32 font-normal"
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
                A new seed, or a new password, signs every browser out and has to be paired again.
              </p>
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
    </template>
  </section>
</template>
