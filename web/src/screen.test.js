import { deepStrictEqual, strictEqual } from "node:assert/strict";
import { test } from "node:test";

import { InputQueue, Trackpad, WheelSteps, chooseCodec, decodeBase64, fitSize, keyEvents, mouseButton, toScreen } from "./screen.js";

/* A send that waits until the test lets it finish, so a test can act while a
   request is in flight. */
function heldSend() {
  const sent = [];
  let release = () => {};
  const send = (body) => {
    sent.push(body);
    return new Promise((resolve) => (release = resolve));
  };
  return { sent, send, release: () => release() };
}

const tick = () => new Promise((resolve) => setTimeout(resolve, 0));

test("input keeps one request in flight and joins what comes meanwhile", async () => {
  const { sent, send, release } = heldSend();
  const q = new InputQueue(send);
  q.setViewer("v1");
  q.push({ t: "m", x: 1, y: 1 });
  strictEqual(sent.length, 1);
  q.push({ t: "m", x: 2, y: 2 }, { t: "m", x: 3, y: 3 });
  q.push({ t: "b", x: 3, y: 3, b: 1, d: true });
  q.push({ t: "m", x: 4, y: 4 });
  q.acknowledge(5);
  q.acknowledge(4);
  strictEqual(sent.length, 1);
  release();
  await tick();
  strictEqual(sent.length, 2);
  deepStrictEqual(sent[1], {
    viewer: "v1",
    ack: 5,
    key: false,
    events: [{ t: "m", x: 3, y: 3 }, { t: "b", x: 3, y: 3, b: 1, d: true }, { t: "m", x: 4, y: 4 }],
  });
  release();
  await tick();
  strictEqual(sent.length, 2, "nothing new, nothing sent");
  q.requestKey();
  strictEqual(sent[2].key, true);
});

test("input waits for a viewer and starts afresh with a new one", async () => {
  const { sent, send, release } = heldSend();
  const q = new InputQueue(send);
  q.push({ t: "c", text: "a" });
  strictEqual(sent.length, 0);
  q.setViewer("v2");
  q.acknowledge(1);
  deepStrictEqual(sent[0], { viewer: "v2", ack: 1, key: false, events: [] });
  release();
});

const key = (key, code, mods = {}) => ({ key, code, ctrlKey: false, metaKey: false, altKey: false, getModifierState: () => false, ...mods });

test("characters travel as text and shortcuts as keys", () => {
  const typed = new Set();
  deepStrictEqual(keyEvents(key("é", "Quote"), { down: true, typed }), [{ t: "c", text: "é" }]);
  deepStrictEqual(keyEvents(key("é", "Quote"), { down: false, typed }), [], "its release sends nothing");
  deepStrictEqual(keyEvents(key("Control", "ControlLeft", { ctrlKey: true }), { down: true, typed }), [{ t: "k", key: "Control", code: "ControlLeft", d: true }]);
  deepStrictEqual(keyEvents(key("c", "KeyC", { ctrlKey: true }), { down: true, typed }), [{ t: "k", key: "c", code: "KeyC", d: true }]);
  deepStrictEqual(keyEvents(key("c", "KeyC"), { down: false, typed }), [{ t: "k", key: "c", code: "KeyC", d: false }]);
  deepStrictEqual(keyEvents(key("Enter", "Enter"), { down: true, typed }), [{ t: "k", key: "Enter", code: "Enter", d: true }]);
  deepStrictEqual(keyEvents(key("Dead", "Quote"), { down: true, typed }), []);
  deepStrictEqual(keyEvents(key("@", "Digit2", { ctrlKey: true, altKey: true, getModifierState: (m) => m === "AltGraph" }), { down: true, typed }), [{ t: "c", text: "@" }]);
});

test("a Mac's Cmd is the other machine's Ctrl, and Option types", () => {
  const typed = new Set();
  const mac = { cmdIsCtrl: true, mac: true, typed };
  deepStrictEqual(keyEvents(key("Meta", "MetaLeft", { metaKey: true }), { down: true, ...mac }), [{ t: "k", key: "Control", code: "ControlLeft", d: true }]);
  deepStrictEqual(keyEvents(key("v", "KeyV", { metaKey: true }), { down: true, ...mac }), [{ t: "k", key: "v", code: "KeyV", d: true }]);
  deepStrictEqual(keyEvents(key("Meta", "MetaRight"), { down: false, ...mac }), [{ t: "k", key: "Control", code: "ControlRight", d: false }]);
  deepStrictEqual(keyEvents(key("ñ", "KeyN", { altKey: true }), { down: true, ...mac }), [{ t: "c", text: "ñ" }]);
  deepStrictEqual(keyEvents(key("f", "KeyF", { altKey: true }), { down: true, typed }), [{ t: "k", key: "f", code: "KeyF", d: true }], "Alt+F elsewhere is a shortcut");
});

test("pointer, fit and wheel arithmetic", () => {
  const rect = { left: 10, top: 20, width: 960, height: 540 };
  deepStrictEqual(toScreen(10, 20, rect, 1920, 1080), [0, 0]);
  deepStrictEqual(toScreen(490, 290, rect, 1920, 1080), [960, 540]);
  deepStrictEqual(toScreen(5000, -5, rect, 1920, 1080), [1919, 0]);
  deepStrictEqual(fitSize(1000, 1000, 1920, 1080), [1000, 562]);
  deepStrictEqual(fitSize(800, 300, 1920, 1080), [533, 300]);
  deepStrictEqual([mouseButton(0), mouseButton(1), mouseButton(2), mouseButton(3)], [1, 2, 3, 0]);
  const wheel = new WheelSteps();
  deepStrictEqual(wheel.add({ deltaMode: 0, deltaX: 0, deltaY: 100 }), [0, 1]);
  deepStrictEqual(wheel.add({ deltaMode: 0, deltaX: 0, deltaY: 30 }), [0, 1], "the 40 left over and 30 more make a click");
  deepStrictEqual(new WheelSteps().add({ deltaMode: 1, deltaX: 0, deltaY: -6 }), [0, -2]);
});

test("trackpad: slide moves, tap clicks, two-finger tap right-clicks, two fingers scroll", () => {
  const pad = new Trackpad();
  pad.screen = [1000, 800];
  pad.cursor = [500, 400];
  pad.scale = 2;
  deepStrictEqual(pad.down(1, 100, 100, 0), []);
  deepStrictEqual(pad.move(1, 110, 95), [{ t: "m", x: 520, y: 390 }]);
  deepStrictEqual(pad.up(1, 50), [], "a slide is not a tap");

  pad.down(1, 100, 100, 1000);
  deepStrictEqual(pad.up(1, 1100), [{ t: "b", x: 520, y: 390, b: 1, d: true }, { t: "b", x: 520, y: 390, b: 1, d: false }]);

  pad.down(1, 100, 100, 5000);
  pad.down(2, 200, 100, 5010);
  pad.up(2, 5080);
  deepStrictEqual(pad.up(1, 5090), [{ t: "b", x: 520, y: 390, b: 3, d: true }, { t: "b", x: 520, y: 390, b: 3, d: false }]);

  pad.down(1, 100, 100, 9000);
  pad.down(2, 200, 100, 9000);
  deepStrictEqual(pad.move(1, 100, 165), [{ t: "w", x: 520, y: 390, dx: 0, dy: -2 }]);
  deepStrictEqual(pad.move(2, 200, 165), [], "the second finger only follows");
  pad.up(1, 9500);
  deepStrictEqual(pad.up(2, 9500), [], "a scroll is not a tap");
});

test("trackpad: tap then hold drags", () => {
  const pad = new Trackpad();
  pad.screen = [1000, 800];
  pad.cursor = [100, 100];
  pad.down(1, 0, 0, 0);
  pad.up(1, 100);
  deepStrictEqual(pad.down(1, 0, 0, 250), [{ t: "b", x: 100, y: 100, b: 1, d: true }]);
  deepStrictEqual(pad.move(1, 20, 0), [{ t: "m", x: 120, y: 100 }]);
  deepStrictEqual(pad.up(1, 900), [{ t: "b", x: 120, y: 100, b: 1, d: false }]);
});

test("the codec is the best both ends have", async () => {
  const decoder = { isConfigSupported: async ({ codec }) => ({ supported: codec === "vp8" }) };
  strictEqual(await chooseCodec(["vp9", "vp8", "jpeg"], decoder), "vp8");
  strictEqual(await chooseCodec(["vp9", "vp8", "jpeg"], undefined), "jpeg");
  strictEqual(await chooseCodec(["jpeg"], decoder), "jpeg");
});

test("base64 decodes to bytes", () => {
  deepStrictEqual([...decodeBase64("AAEC/w==")], [0, 1, 2, 255]);
});
