<script setup>
/* One terminal tab: a shell in the project's folder, drawn by xterm.js.

   Everything about the wire is in ../terminals.js. This holds the emulator,
   keeps it the size of the pane, and hands what is typed one way and what
   arrives the other. It is loaded from its own chunk — the emulator is a
   couple of hundred kilobytes and no launch that never opens a terminal
   should pay for it. See specs/073-terminals.md and specs/052-launch-budget.md. */
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";

import { resizeTerminal, sendTerminalBinary, sendTerminalInput, watchTerminal } from "../terminals";
import { S, closeTab, hit } from "../store";
import { colorPreference } from "../color-mode";

const props = defineProps({ tab: { type: Object, required: true } });

const host = ref(null);
let term = null;
let fit = null;
let unwatch = null;
let observer = null;

/* The emulator needs its colours as values, so they are read off the panel it
   is drawn in rather than named again here: recolouring the app in Settings
   recolours the terminal with it. The ANSI sixteen are fixed, because those
   are the colours a program asked for by number and not the app's to restyle.

   Nuxt UI's variables resolve to whatever the current mode and palette say,
   so this is re-read whenever either changes. */
function theme() {
  const style = getComputedStyle(host.value);
  const read = (name, fallback) => style.getPropertyValue(name).trim() || fallback;
  return {
    background: read("--ui-bg", "#111215"),
    foreground: read("--ui-text-highlighted", "#e5e7eb"),
    cursor: read("--ui-primary", "#3b82f6"),
    selectionBackground: read("--ui-bg-accented", "#33343a"),
  };
}

/* A pane with no width yet — a tab mounted behind a transition, a window
   still opening — makes the fit addon divide by zero. Sizing is skipped
   until there is something to size to. */
function refit() {
  if (!term || !fit || !host.value?.clientWidth || !host.value?.clientHeight) return;
  fit.fit();
  resizeTerminal(props.tab.terminalID, term.cols, term.rows);
}

onMounted(() => {
  term = new Terminal({
    // The shell's own scrollback, on top of the screen the server replays.
    scrollback: 10000,
    fontSize: 13,
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
    cursorBlink: true,
    theme: theme(),
  });
  fit = new FitAddon();
  term.loadAddon(fit);
  term.open(host.value);
  refit();

  /* The shell gets every chord except the one that opens another terminal.
     xterm.js turns Ctrl+Alt+T into an escape sequence and then stops the
     event, so without this the window's handler never sees it — and a
     terminal is the likeliest place to want a second one. The custom handler
     runs before any of that, and returning false leaves the event to bubble. */
  term.attachCustomKeyEventHandler((e) => !(e.type === "keydown" && hit(e, "terminal.new")));

  term.onData((data) => sendTerminalInput(props.tab.terminalID, data));
  term.onBinary((data) => sendTerminalBinary(props.tab.terminalID, data));

  unwatch = watchTerminal(props.tab.terminalID, {
    /* A reset frame is the whole screen rather than the next piece of one:
       the stream was reopened, or this view fell behind and was caught up. */
    onData: (bytes, reset) => {
      if (reset) term.reset();
      term.write(bytes);
    },
    /* The shell ended — `exit`, or Ctrl+D. The tab stood for that shell, so
       it goes with it rather than sitting there as a dead screen. */
    onExit: () => closeTab(props.tab.id, false),
  });

  observer = new ResizeObserver(refit);
  observer.observe(host.value);
  term.focus();
});

/* Recolour in place rather than rebuild: the screen and its scrollback belong
   to the emulator, and switching to dark mode is not a reason to lose them. */
watch([colorPreference, () => S.colors.accent, () => S.colors.neutral], () => {
  if (term) term.options.theme = theme();
});

onBeforeUnmount(() => {
  observer?.disconnect();
  unwatch?.();
  term?.dispose();
  term = null;
  fit = null;
});
</script>

<template>
  <!-- The emulator measures itself against this element, so it owns the whole
       pane and the padding is inside it rather than around it. -->
  <div class="terminal-pane min-h-0 flex-1 overflow-hidden bg-default p-2">
    <div ref="host" class="size-full" />
  </div>
</template>

<style>
/* The emulator ships a fixed background on its viewport. The pane already
   carries the panel's, and two of them means a rectangle that does not follow
   the app into dark mode. */
.terminal-pane .xterm .xterm-viewport {
  background-color: transparent;
}
</style>
