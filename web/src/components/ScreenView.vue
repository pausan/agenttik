<script setup>
/* The Screen tab: the connected machine's screen, its mouse, keyboard and
   clipboard. The wire and the arithmetic are in ../screen.js; this wires the
   DOM to them. The stream runs only while the tab is in front — the view is
   built when it comes forward and torn down when it goes. See
   specs/096-screen.md. */
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";

import { S, copyText, fail, loadScreenInfo, setScreenEnabled } from "../store";
import { MACOS } from "../platform";
import {
  ScreenStream,
  Trackpad,
  WheelSteps,
  chooseCodec,
  fitSize,
  keyEvents,
  mouseButton,
  setRemoteClipboard,
  toScreen,
} from "../screen";

defineProps({ tab: { type: Object, required: true } });

const root = ref(null);
const bar = ref(null);
const pane = ref(null);
const canvas = ref(null);
const keys = ref(null);

const state = reactive({ codec: "", sw: 0, sh: 0, fps: 0, kbps: 0 });
const size = reactive({ w: 0, h: 0 });
const pointer = reactive({ x: 0, y: 0, url: "", hx: 0, hy: 0, w: 0, h: 0, over: false });
const status = ref("starting"); // starting | live | reconnecting | off | unavailable
const remoteClip = ref("");
const fullscreen = ref(false);
const pseudo = ref(false); // a CSS stand-in where the Fullscreen API is missing
// Where the full-screen bar was dragged to, its corner in pixels; null centres
// it at the top.
const barAt = ref(JSON.parse(localStorage.getItem("screen.bar") || "null"));
const touchDevice = typeof navigator !== "undefined" && navigator.maxTouchPoints > 0;
const touchMode = ref(localStorage.getItem("screen.touch") || "trackpad");

let stream = null;
let observer = null;
const pad = new Trackpad();
const wheel = new WheelSteps();
const typed = new Set();
const held = new Map(); // key events sent down and not yet up, by code
let pasteCode = "";
let pasteTimer = 0;
let barDrag = null;

const floating = computed(() => fullscreen.value || pseudo.value);
const machine = computed(() => S.instanceInfo?.name || "this machine");
const cmdIsCtrl = computed(() => MACOS && S.screen?.os !== "darwin");
const ctrlIsCmd = computed(() => !MACOS && S.screen?.os === "darwin");
const summary = computed(() => {
  if (status.value !== "live") return "";
  const parts = [state.codec.toUpperCase().replace("-444", " 4:4:4"), `${state.fps} fps`];
  parts.push(state.kbps >= 1000 ? `${(state.kbps / 1000).toFixed(1)} Mbps` : `${state.kbps} kbps`);
  if (state.sw) parts.push(`${state.sw}×${state.sh}`);
  return parts.join(" · ");
});

/* --------------------------------------------------------------- stream */

async function start() {
  await loadScreenInfo();
  if (!S.screen?.available) return (status.value = "unavailable");
  if (!S.screen.enabled) return (status.value = "off");
  status.value = "starting";
  const codec = await chooseCodec(S.screen.codecs);
  if (!canvas.value) return; // the tab went while the codec was chosen
  stream = new ScreenStream(canvas.value, codec, {
    onState(next) {
      const resized = next.sw !== state.sw || next.sh !== state.sh;
      Object.assign(state, next);
      pad.screen = [next.sw, next.sh];
      if (status.value !== "live") {
        status.value = "live";
        pad.cursor = [Math.floor(next.sw / 2), Math.floor(next.sh / 2)];
      }
      if (resized) nextTick(fit);
    },
    onCursor(x, y) {
      pointer.x = x;
      pointer.y = y;
      pad.cursor = [x, y];
    },
    onShape(url, hx, hy, w, h) {
      Object.assign(pointer, { url, hx, hy, w, h });
    },
    onClip: receiveClipboard,
    async onError() {
      status.value = "reconnecting";
      await loadScreenInfo();
      if (S.screen && !S.screen.enabled) stop("off");
    },
  });
}

function stop(next) {
  releaseKeys();
  stream?.close();
  stream = null;
  if (next) status.value = next;
}

async function turnOn() {
  await setScreenEnabled(true);
  if (S.screen?.enabled) start();
}

/* The canvas keeps the screen's shape at the largest size the pane allows. */
function fit() {
  const el = pane.value;
  if (!el) return;
  [size.w, size.h] = fitSize(el.clientWidth, el.clientHeight, state.sw, state.sh);
  if (size.w) pad.scale = state.sw / size.w;
  if (floating.value && barAt.value) placeBar(barAt.value.x, barAt.value.y);
}

/* ---------------------------------------------------------------- input */

function send(...events) {
  if (events.length) stream?.input.push(...events);
}

function screenPoint(e) {
  return toScreen(e.clientX, e.clientY, canvas.value.getBoundingClientRect(), state.sw, state.sh);
}

function onPointerDown(e) {
  keys.value?.focus({ preventScroll: true });
  if (!state.sw) return;
  e.preventDefault();
  if (e.pointerType === "touch" && touchMode.value === "trackpad") {
    return send(...pad.down(e.pointerId, e.clientX, e.clientY, e.timeStamp));
  }
  canvas.value.setPointerCapture?.(e.pointerId);
  const [x, y] = screenPoint(e);
  const b = e.pointerType === "touch" ? 1 : mouseButton(e.button);
  if (b) send({ t: "b", x, y, b, d: true });
}

function onPointerMove(e) {
  if (!state.sw) return;
  if (e.pointerType === "touch" && touchMode.value === "trackpad") {
    return send(...pad.move(e.pointerId, e.clientX, e.clientY));
  }
  const [x, y] = screenPoint(e);
  send({ t: "m", x, y });
}

function onPointerUp(e) {
  if (!state.sw) return;
  if (e.pointerType === "touch" && touchMode.value === "trackpad") {
    return send(...pad.up(e.pointerId, e.timeStamp));
  }
  const [x, y] = screenPoint(e);
  const b = e.pointerType === "touch" ? 1 : mouseButton(e.button);
  if (b) send({ t: "b", x, y, b, d: false });
}

function onPointerCancel(e) {
  if (e.pointerType === "touch" && touchMode.value === "trackpad") send(...pad.cancel());
}

function onWheel(e) {
  if (!state.sw) return;
  e.preventDefault();
  const [dx, dy] = wheel.add(e);
  if (!dx && !dy) return;
  const [x, y] = screenPoint(e);
  send({ t: "w", x, y, dx, dy });
}

function isPaste(e) {
  return (e.ctrlKey || e.metaKey) && !e.altKey && e.key.toLowerCase() === "v";
}

function onKeyDown(e) {
  // Every key belongs to the other machine while the screen has focus, as in
  // a terminal: Ctrl+W there closes a window over there, not this tab.
  e.stopPropagation();
  if (e.key === "Unidentified" || e.key === "Process" || e.isComposing) return; // `input` brings it
  if (isPaste(e) && !e.repeat) {
    // Let the browser fire `paste`, which carries this machine's clipboard
    // without asking; the keystroke follows once the text is over there.
    pasteCode = e.code;
    clearTimeout(pasteTimer);
    pasteTimer = setTimeout(() => sendPasteKey(e.key), 400);
    return;
  }
  e.preventDefault();
  const events = keyEvents(e, { down: true, cmdIsCtrl: cmdIsCtrl.value, ctrlIsCmd: ctrlIsCmd.value, mac: MACOS, typed });
  for (const ev of events) if (ev.t === "k") held.set(ev.code, ev.key);
  send(...events);
}

function onKeyUp(e) {
  e.stopPropagation();
  e.preventDefault();
  if (e.code && e.code === pasteCode) {
    pasteCode = "";
    return;
  }
  const events = keyEvents(e, { down: false, cmdIsCtrl: cmdIsCtrl.value, ctrlIsCmd: ctrlIsCmd.value, mac: MACOS, typed });
  for (const ev of events) held.delete(ev.code);
  send(...events);
}

async function onPaste(e) {
  e.preventDefault();
  const text = e.clipboardData?.getData("text/plain") || "";
  const key = pasteCode ? "v" : "";
  clearTimeout(pasteTimer);
  if (text) {
    try {
      await setRemoteClipboard(text);
    } catch (error) {
      fail(error);
    }
  }
  // A paste from a touch keyboard's menu has no keystroke behind it.
  if (key) sendPasteKey(key);
  else if (text) send({ t: "c", text });
}

function sendPasteKey(key) {
  clearTimeout(pasteTimer);
  const code = pasteCode || "KeyV";
  send({ t: "k", key, code, d: true }, { t: "k", key, code, d: false });
}

/* Soft keyboards say little in keydown; what they type arrives here. */
function onInput(e) {
  const el = e.target;
  if (e.inputType === "deleteContentBackward") send({ t: "k", key: "Backspace", code: "Backspace", d: true }, { t: "k", key: "Backspace", code: "Backspace", d: false });
  else if (e.inputType === "insertLineBreak") send({ t: "k", key: "Enter", code: "Enter", d: true }, { t: "k", key: "Enter", code: "Enter", d: false });
  else if (e.data) send({ t: "c", text: e.data });
  el.value = "";
}

/* Keys still down when focus leaves would stay down over there. */
function releaseKeys() {
  const events = [...held].map(([code, key]) => ({ t: "k", key, code, d: false }));
  held.clear();
  typed.clear();
  send(...events);
}

function showKeyboard() {
  keys.value?.focus();
}

function setTouchMode(mode) {
  touchMode.value = mode;
  localStorage.setItem("screen.touch", mode);
}

/* ------------------------------------------------------------ clipboard */

async function receiveClipboard(text) {
  if (!text) return;
  try {
    await navigator.clipboard.writeText(text);
    remoteClip.value = "";
  } catch {
    // Safari, and others without a recent click, need one to write.
    remoteClip.value = text;
  }
}

async function copyRemoteClipboard() {
  await copyText(remoteClip.value);
  remoteClip.value = "";
  keys.value?.focus({ preventScroll: true });
}

/* ----------------------------------------------------------- fullscreen */

function toggleFullscreen() {
  if (fullscreen.value || pseudo.value) return exitFullscreen();
  const el = root.value;
  const request = el.requestFullscreen || el.webkitRequestFullscreen;
  if (request) {
    Promise.resolve(request.call(el)).catch(() => (pseudo.value = true));
  } else {
    pseudo.value = true;
  }
  keys.value?.focus({ preventScroll: true });
}

function exitFullscreen() {
  pseudo.value = false;
  if (document.fullscreenElement || document.webkitFullscreenElement) {
    (document.exitFullscreen || document.webkitExitFullscreen).call(document);
  }
}

function onFullscreenChange() {
  fullscreen.value = (document.fullscreenElement || document.webkitFullscreenElement) === root.value;
  nextTick(fit);
}

/* In full screen the bar floats and can be dragged by anything but its
   buttons. It is kept wholly inside the screen, here and when that shrinks. */
function onBarPointerDown(e) {
  if (!floating.value || e.button !== 0 || e.target.closest("button")) return;
  e.preventDefault();
  const r = bar.value.getBoundingClientRect();
  barDrag = { id: e.pointerId, dx: e.clientX - r.left, dy: e.clientY - r.top };
  bar.value.setPointerCapture(e.pointerId);
}

function onBarPointerMove(e) {
  if (barDrag?.id !== e.pointerId) return;
  const area = root.value.getBoundingClientRect();
  placeBar(e.clientX - barDrag.dx - area.left, e.clientY - barDrag.dy - area.top);
}

function onBarPointerUp(e) {
  if (barDrag?.id !== e.pointerId) return;
  barDrag = null;
  localStorage.setItem("screen.bar", JSON.stringify(barAt.value));
}

function placeBar(x, y) {
  const el = bar.value;
  if (!el) return;
  const clamp = (v, hi) => Math.round(Math.max(0, Math.min(v, hi)));
  barAt.value = {
    x: clamp(x, root.value.clientWidth - el.offsetWidth),
    y: clamp(y, root.value.clientHeight - el.offsetHeight),
  };
}

/* --------------------------------------------------------------- cursor */

/* With a mouse over the canvas the pointer is this machine's, wearing the
   other machine's shape: it moves with no delay. Otherwise — a touch, or an
   agent moving it — the other machine's pointer is drawn where it is. */
const cssCursor = computed(() => (pointer.url ? `url(${pointer.url}) ${pointer.hx} ${pointer.hy}, default` : "default"));
const drawnCursor = computed(() => {
  if (!pointer.url || pointer.over || !state.sw || !size.w) return null;
  const k = size.w / state.sw;
  return {
    left: `${(pointer.x - pointer.hx) * k}px`,
    top: `${(pointer.y - pointer.hy) * k}px`,
    width: `${pointer.w * k}px`,
    height: `${pointer.h * k}px`,
  };
});

watch(pseudo, () => nextTick(fit));

onMounted(() => {
  observer = new ResizeObserver(fit);
  observer.observe(pane.value);
  document.addEventListener("fullscreenchange", onFullscreenChange);
  document.addEventListener("webkitfullscreenchange", onFullscreenChange);
  start();
});

onBeforeUnmount(() => {
  clearTimeout(pasteTimer);
  observer?.disconnect();
  document.removeEventListener("fullscreenchange", onFullscreenChange);
  document.removeEventListener("webkitfullscreenchange", onFullscreenChange);
  exitFullscreen();
  stop();
});
</script>

<template>
  <div
    ref="root"
    class="screen-view flex min-h-0 flex-1 flex-col bg-black"
    :class="{ 'fixed inset-0 z-[100]': pseudo }"
  >
    <div
      ref="bar"
      class="flex h-8 shrink-0 items-center gap-2 px-2 text-xs"
      :class="floating
        ? ['absolute z-10 w-[34rem] max-w-[calc(100%-1rem)] cursor-move touch-none rounded-md bg-black text-white opacity-70 transition-opacity select-none hover:opacity-100 focus-within:opacity-100',
           barAt ? '' : 'top-1 left-1/2 -translate-x-1/2']
        : 'border-b border-default bg-default text-muted'"
      :style="floating && barAt ? { left: barAt.x + 'px', top: barAt.y + 'px' } : null"
      data-testid="screen-bar"
      @pointerdown="onBarPointerDown"
      @pointermove="onBarPointerMove"
      @pointerup="onBarPointerUp"
      @pointercancel="onBarPointerUp"
    >
      <UIcon name="i-lucide-monitor" class="size-4 shrink-0" />
      <span class="truncate font-medium">{{ machine }}</span>
      <span v-if="summary" class="truncate text-dimmed tabular-nums max-sm:hidden" data-testid="screen-summary">{{ summary }}</span>
      <span v-else-if="status === 'reconnecting'" class="text-warning">Reconnecting…</span>
      <div class="ml-auto flex shrink-0 items-center gap-1">
        <UButton
          v-if="remoteClip"
          icon="i-lucide-clipboard-copy"
          label="Copy remote clipboard"
          size="xs"
          color="primary"
          variant="soft"
          @click="copyRemoteClipboard"
        />
        <template v-if="touchDevice">
          <UButton
            icon="i-lucide-keyboard"
            size="xs"
            color="neutral"
            variant="ghost"
            title="Show the keyboard"
            aria-label="Show the keyboard"
            @click="showKeyboard"
          />
          <UButton
            :icon="touchMode === 'trackpad' ? 'i-lucide-square-mouse-pointer' : 'i-lucide-pointer'"
            size="xs"
            color="neutral"
            variant="ghost"
            :title="touchMode === 'trackpad' ? 'Touch works as a trackpad; switch to touching where you click' : 'Touch clicks where you touch; switch to trackpad'"
            :aria-label="touchMode === 'trackpad' ? 'Touch as trackpad' : 'Touch to click'"
            @click="setTouchMode(touchMode === 'trackpad' ? 'direct' : 'trackpad')"
          />
        </template>
        <UButton
          :icon="floating ? 'i-lucide-minimize' : 'i-lucide-maximize'"
          size="xs"
          color="neutral"
          variant="ghost"
          :title="floating ? 'Exit full screen' : 'Full screen'"
          :aria-label="floating ? 'Exit full screen' : 'Full screen'"
          @click="toggleFullscreen"
        />
      </div>
    </div>

    <div
      v-if="S.screen?.control && status === 'live'"
      class="flex items-center gap-2 border-b border-default bg-warning/10 px-2 py-1 text-xs text-warning"
      data-testid="screen-control"
    >
      <UIcon name="i-lucide-mouse-pointer-ban" class="size-4 shrink-0" />
      <span class="min-w-0 flex-1">{{ S.screen.control }}</span>
      <UButton label="Check again" size="xs" color="warning" variant="soft" @click="loadScreenInfo" />
    </div>

    <div ref="pane" class="relative flex min-h-0 flex-1 items-center justify-center overflow-hidden">
      <div
        v-show="status === 'live' || status === 'reconnecting'"
        class="relative shrink-0"
        :style="{ width: size.w + 'px', height: size.h + 'px' }"
      >
        <canvas
          ref="canvas"
          data-testid="screen-canvas"
          class="absolute inset-0 block size-full touch-none select-none"
          :style="{ cursor: cssCursor }"
          @pointerdown="onPointerDown"
          @pointermove="onPointerMove"
          @pointerup="onPointerUp"
          @pointercancel="onPointerCancel"
          @pointerenter="pointer.over = $event.pointerType === 'mouse'"
          @pointerleave="pointer.over = false"
          @wheel="onWheel"
          @contextmenu.prevent
        />
        <img v-if="drawnCursor" :src="pointer.url" alt="" class="pointer-events-none absolute" :style="drawnCursor" />
      </div>
      <!-- Where the keyboard goes while the screen has focus. A textarea, so
           a touch keyboard opens for it and paste events reach it. -->
      <textarea
        ref="keys"
        aria-label="Keyboard input for the remote screen"
        class="pointer-events-none absolute top-0 left-0 size-px resize-none opacity-0"
        autocapitalize="off"
        autocomplete="off"
        autocorrect="off"
        spellcheck="false"
        @keydown="onKeyDown"
        @keyup="onKeyUp"
        @paste="onPaste"
        @input="onInput"
        @blur="releaseKeys"
      />

      <div v-if="status === 'starting'" class="text-sm text-dimmed">Connecting to {{ machine }}…</div>
      <div v-else-if="status === 'unavailable'" class="max-w-md p-6 text-center text-sm text-muted">
        <p class="mb-1 font-medium text-highlighted">No screen to show</p>
        <p>{{ S.screen?.reason || "This machine did not answer." }}</p>
      </div>
      <div v-else-if="status === 'off'" class="max-w-md p-6 text-center text-sm text-muted">
        <p class="mb-1 font-medium text-highlighted">Screen sharing is off on {{ machine }}</p>
        <p class="mb-3">
          Turning it on lets this window, and any agent with access to {{ machine }}, see its screen and use its
          mouse and keyboard. It can be turned off in Settings › Server.
        </p>
        <UButton label="Turn on screen sharing" icon="i-lucide-monitor-up" @click="turnOn" />
      </div>
    </div>
  </div>
</template>
