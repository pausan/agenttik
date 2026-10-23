/* The Screen tab's end of the wire — see specs/096-screen.md.

   Frames come down one SSE stream per viewer and are drawn on a canvas:
   VP9 or VP8 through WebCodecs, or JPEG bands where WebCodecs is missing.
   Input, the acknowledgement of the last frame drawn and keyframe requests go
   back up in one POST at a time, which is what paces the stream.

   Nothing here holds a component. ScreenView wires DOM events into the
   functions below and draws what ScreenStream hands it. */

import { apiURL } from "./api.js";
import { openEventStream } from "./eventStream.js";

/* WebCodecs names for what the server sends. Profile 0, level 1.0, 8 bits:
   a decoder that accepts this accepts any 8-bit 4:2:0 stream. */
const WEBCODECS = { vp9: "vp09.00.10.08", vp8: "vp8" };

/* chooseCodec picks the best codec both ends have: the server's list is
   best first, and a browser without WebCodecs (or without that decoder)
   gets JPEG. */
export async function chooseCodec(serverCodecs, decoder = globalThis.VideoDecoder) {
  for (const codec of serverCodecs || []) {
    if (codec === "jpeg") return "jpeg";
    if (!decoder?.isConfigSupported || !WEBCODECS[codec]) continue;
    try {
      const { supported } = await decoder.isConfigSupported({ codec: WEBCODECS[codec], optimizeForLatency: true });
      if (supported) return codec;
    } catch {
      /* try the next one */
    }
  }
  return "jpeg";
}

export function decodeBase64(text) {
  if (typeof Uint8Array.fromBase64 === "function") return Uint8Array.fromBase64(text);
  const bin = atob(text);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

/* ------------------------------------------------------------------ input */

/* InputQueue keeps one request in flight. What happens meanwhile joins the
   next one: pointer moves collapse into the latest, since only where the
   pointer ends up matters, and the acknowledgement is always the newest.
   `send(body)` performs the request; it is injectable for tests. */
export class InputQueue {
  constructor(send = postInput) {
    this.send = send;
    this.viewer = "";
    this.events = [];
    this.ack = 0;
    this.sentAck = 0;
    this.key = false;
    this.received = null;
    this.busy = false;
    this.closed = false;
  }

  setViewer(id) {
    this.viewer = id;
    this.events = [];
    this.ack = this.sentAck = 0;
    this.key = false;
    this.received = null;
  }

  push(...events) {
    for (const ev of events) {
      const last = this.events.at(-1);
      if (ev.t === "m" && last?.t === "m") this.events[this.events.length - 1] = ev;
      else this.events.push(ev);
    }
    this.flush();
  }

  acknowledge(seq) {
    if (seq > this.ack) this.ack = seq;
    this.flush();
  }

  requestKey() {
    this.key = true;
    this.flush();
  }

  async flush() {
    if (this.busy || this.closed || !this.viewer) return;
    if (!this.events.length && this.ack === this.sentAck && !this.key) return;
    const body = { viewer: this.viewer, ack: this.ack, key: this.key, events: this.events };
    if (this.received) body.received = this.received;
    this.received = null;
    this.events = [];
    this.sentAck = this.ack;
    this.key = false;
    this.busy = true;
    try {
      await this.send(body);
    } catch {
      /* the viewer has gone or the network blinked; the stream says which */
    } finally {
      this.busy = false;
    }
    this.flush();
  }

  close() {
    this.closed = true;
  }
}

async function postInput(body) {
  const res = await fetch(apiURL("/api/screen/input"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok && res.status !== 404) throw new Error(res.statusText);
}

/* ------------------------------------------------------------------- keys */

/* keyEvents translates a browser key event into what the server takes. A
   character typed without a shortcut modifier travels as text, so it comes
   out the same whatever the two keyboard layouts are; everything else
   travels as a key, held and released, so modifiers combine on the other end.

   `cmdIsCtrl` is a Mac viewing a machine that is not one: Cmd does what Ctrl
   does there, so Cmd+C copies. `typed` remembers which keys went as text, so
   their release sends nothing. */
export function keyEvents(e, { down, cmdIsCtrl = false, mac = false, typed = new Set() }) {
  let { key, code } = e;
  if (cmdIsCtrl && key === "Meta") {
    key = "Control";
    code = code === "MetaRight" || code === "OSRight" ? "ControlRight" : "ControlLeft";
  }
  if (!down) {
    if (typed.delete(code)) return [];
    return [{ t: "k", key, code, d: false }];
  }
  if (key === "Dead" || key === "Unidentified" || key === "Process" || key === "AltGraph") return [];
  const altGraph = e.getModifierState?.("AltGraph") || (mac && e.altKey);
  const shortcut = e.ctrlKey || e.metaKey || (e.altKey && !mac);
  if ([...key].length === 1 && (!shortcut || altGraph)) {
    typed.add(code);
    return [{ t: "c", text: key }];
  }
  return [{ t: "k", key, code, d: true }];
}

/* ------------------------------------------------------- pointer and wheel */

/* toScreen maps a point on the drawn canvas to screen pixels. */
export function toScreen(clientX, clientY, rect, sw, sh) {
  const x = Math.floor(((clientX - rect.left) / rect.width) * sw);
  const y = Math.floor(((clientY - rect.top) / rect.height) * sh);
  return [Math.max(0, Math.min(sw - 1, x)), Math.max(0, Math.min(sh - 1, y))];
}

/* fitSize is the largest size with the screen's shape that fits the pane. */
export function fitSize(paneW, paneH, sw, sh) {
  if (!sw || !sh || !paneW || !paneH) return [0, 0];
  const scale = Math.min(paneW / sw, paneH / sh);
  return [Math.floor(sw * scale), Math.floor(sh * scale)];
}

/* mouseButton is X's numbering for a browser button: left, middle, right. */
export function mouseButton(button) {
  return [1, 2, 3][button] || 0;
}

/* WheelSteps turns wheel deltas into whole wheel clicks. A mouse notch is
   about 100 pixels or 3 lines; a trackpad sends many small deltas, which add
   up to a click at a time. */
export class WheelSteps {
  constructor() {
    this.x = 0;
    this.y = 0;
  }

  add(e) {
    const unit = e.deltaMode === 1 ? 3 : e.deltaMode === 2 ? 0.1 : 60;
    this.x += e.deltaX / unit;
    this.y += e.deltaY / unit;
    const dx = Math.trunc(this.x);
    const dy = Math.trunc(this.y);
    this.x -= dx;
    this.y -= dy;
    return [dx, dy];
  }
}

/* --------------------------------------------------------------- touch */

const TAP_MS = 250;
const TAP_SLOP = 10; // pixels a tap may wander
const DRAG_GAP_MS = 300; // tap, then press again this soon to drag
const SCROLL_STEP = 30; // pixels of two-finger travel per wheel click

/* Trackpad makes a touch screen a laptop's trackpad: one finger moves the
   pointer by how far it slides, a tap clicks where the pointer is, a
   two-finger tap right-clicks, two fingers sliding scroll, and tap-then-hold
   drags. Its methods take touch points in CSS pixels and answer the events
   to send; `scale` is screen pixels per CSS pixel, and `cursor` the pointer's
   last known screen position. */
export class Trackpad {
  constructor() {
    this.touches = new Map();
    this.cursor = [0, 0];
    this.screen = [1, 1];
    this.scale = 1;
    this.reset();
  }

  reset() {
    this.started = 0;
    this.moved = false;
    this.most = 0;
    this.dragging = false;
    this.lastTap = -Infinity;
    this.scrolled = 0;
  }

  down(id, x, y, now) {
    if (!this.touches.size) {
      this.started = now;
      this.moved = false;
      this.most = 0;
      this.scrolled = 0;
    }
    this.touches.set(id, { x, y, x0: x, y0: y });
    this.most = Math.max(this.most, this.touches.size);
    if (this.touches.size === 1 && now - this.lastTap < DRAG_GAP_MS) {
      this.dragging = true;
      return [{ t: "b", x: this.cursor[0], y: this.cursor[1], b: 1, d: true }];
    }
    return [];
  }

  move(id, x, y) {
    const t = this.touches.get(id);
    if (!t) return [];
    const dx = x - t.x;
    const dy = y - t.y;
    t.x = x;
    t.y = y;
    if (Math.hypot(x - t.x0, y - t.y0) > TAP_SLOP) this.moved = true;
    if (this.touches.size >= 2) {
      // Both fingers move together; one of them is enough to measure by.
      if (id !== this.touches.keys().next().value) return [];
      this.scrolled += dy;
      const steps = Math.trunc(this.scrolled / SCROLL_STEP);
      if (!steps) return [];
      this.scrolled -= steps * SCROLL_STEP;
      // Content follows the fingers, as it does on a phone.
      return [{ t: "w", x: this.cursor[0], y: this.cursor[1], dx: 0, dy: -steps }];
    }
    const [sw, sh] = this.screen;
    this.cursor = [
      Math.max(0, Math.min(sw - 1, Math.round(this.cursor[0] + dx * this.scale))),
      Math.max(0, Math.min(sh - 1, Math.round(this.cursor[1] + dy * this.scale))),
    ];
    return [{ t: "m", x: this.cursor[0], y: this.cursor[1] }];
  }

  up(id, now) {
    if (!this.touches.delete(id) || this.touches.size) return [];
    const [x, y] = this.cursor;
    if (this.dragging) {
      this.dragging = false;
      this.lastTap = -Infinity;
      return [{ t: "b", x, y, b: 1, d: false }];
    }
    if (this.moved || now - this.started > TAP_MS) return [];
    const b = this.most >= 2 ? 3 : 1;
    if (b === 1) this.lastTap = now;
    return [{ t: "b", x, y, b, d: true }, { t: "b", x, y, b, d: false }];
  }

  cancel() {
    this.touches.clear();
    const out = this.dragging ? [{ t: "b", x: this.cursor[0], y: this.cursor[1], b: 1, d: false }] : [];
    this.reset();
    return out;
  }
}

/* ----------------------------------------------------------------- stream */

/* ArrivalMeter measures payload throughput over the last second. The first
   frame only starts the clock; including its bytes would overstate the rate.
   An idle screen starts a fresh window when its next frame arrives. */
export class ArrivalMeter {
  constructor() {
    this.samples = [];
  }

  record(seq, bytes, at = performance.now()) {
    while (this.samples.length && at - this.samples[0].at > 1000) this.samples.shift();
    this.samples.push({ at, bytes });
    const elapsed = at - this.samples[0].at;
    let received = 0;
    for (let i = 1; i < this.samples.length; i++) received += this.samples[i].bytes;
    return { seq, at, kbps: elapsed > 0 ? received * 8 / elapsed : 0 };
  }
}

/* ScreenStream opens the viewer's stream and draws it on `canvas`. It calls
   `onState({ codec, sw, sh, fps, kbps })` as things change, `onCursor(x, y)`,
   `onShape(url, hx, hy, w, h)` and `onClip(text)`. */
export class ScreenStream {
  constructor(canvas, codec, { onState, onCursor, onShape, onClip, onError } = {}) {
    this.canvas = canvas;
    this.ctx = canvas.getContext("2d", { alpha: false });
    this.codec = codec;
    this.handlers = { onState, onCursor, onShape, onClip, onError };
    this.input = new InputQueue();
    this.decoder = null;
    this.pending = [];
    this.needKey = true;
    this.state = { codec, sw: 0, sh: 0, fps: 0, kbps: 0 };
    this.drawn = 0;
    this.bytes = 0;
    this.arrivals = new ArrivalMeter();
    this.chain = Promise.resolve();
    this.meter = setInterval(() => this.measure(), 1000);
    this.es = openEventStream(apiURL("/api/stream/screen?codec=" + codec));
    this.es.onmessage = (e) => this.receive(e.data);
    this.es.onerror = () => this.handlers.onError?.();
  }

  receive(data) {
    let msg;
    try {
      msg = JSON.parse(data);
    } catch {
      return;
    }
    switch (msg.t) {
      case "hello":
        // A new viewer: the first one, or one opened by a reconnect.
        this.input.setViewer(msg.id);
        this.arrivals = new ArrivalMeter();
        this.resetDecoder();
        this.codec = msg.codec;
        this.update({ codec: msg.codec, sw: msg.sw, sh: msg.sh });
        break;
      case "frame": {
        if (msg.sw !== this.state.sw || msg.sh !== this.state.sh) this.update({ sw: msg.sw, sh: msg.sh });
        let bytes = msg.d ? (msg.d.length * 3) / 4 : 0;
        for (const band of msg.bands || []) bytes += (band.d.length * 3) / 4;
        this.bytes += bytes;
        this.input.received = this.arrivals.record(msg.seq, bytes);
        if (msg.bands) this.chain = this.chain.then(() => this.drawBands(msg));
        else this.decode(msg);
        break;
      }
      case "cursor":
        this.handlers.onCursor?.(msg.x, msg.y);
        break;
      case "shape":
        this.handlers.onShape?.(`data:image/png;base64,${msg.d}`, msg.hx || 0, msg.hy || 0, msg.w, msg.h);
        break;
      case "clip":
        this.handlers.onClip?.(msg.text || "");
        break;
    }
  }

  update(change) {
    this.state = { ...this.state, ...change };
    this.handlers.onState?.(this.state);
  }

  size(w, h) {
    if (this.canvas.width !== w || this.canvas.height !== h) {
      this.canvas.width = w;
      this.canvas.height = h;
    }
  }

  async drawBands(msg) {
    this.size(msg.w, msg.h);
    for (const band of msg.bands) {
      const blob = new Blob([decodeBase64(band.d)], { type: "image/jpeg" });
      try {
        const bitmap = await createImageBitmap(blob);
        this.ctx.drawImage(bitmap, 0, band.y);
        bitmap.close();
      } catch {
        this.input.requestKey();
        return;
      }
    }
    this.drawn++;
    this.input.acknowledge(msg.seq);
  }

  decode(msg) {
    if (this.needKey && !msg.key) {
      // Nothing can be drawn until a frame that stands on its own; still
      // acknowledge it, so the stream keeps coming.
      this.input.acknowledge(msg.seq);
      return;
    }
    if (!this.decoder) this.openDecoder();
    this.size(msg.w, msg.h);
    this.needKey = false;
    this.pending.push(msg.seq);
    try {
      this.decoder.decode(new EncodedVideoChunk({
        type: msg.key ? "key" : "delta",
        timestamp: msg.seq * 33333,
        data: decodeBase64(msg.d),
      }));
    } catch {
      this.failDecoder();
    }
  }

  openDecoder() {
    this.decoder = new VideoDecoder({
      output: (frame) => {
        this.ctx.drawImage(frame, 0, 0, this.canvas.width, this.canvas.height);
        frame.close();
        this.drawn++;
        const seq = this.pending.shift();
        if (seq) this.input.acknowledge(seq);
      },
      error: () => this.failDecoder(),
    });
    this.decoder.configure({ codec: WEBCODECS[this.codec], optimizeForLatency: true });
  }

  failDecoder() {
    const last = this.pending.at(-1);
    this.resetDecoder();
    if (last) this.input.acknowledge(last);
    this.input.requestKey();
  }

  resetDecoder() {
    if (this.decoder && this.decoder.state !== "closed") this.decoder.close();
    this.decoder = null;
    this.pending = [];
    this.needKey = true;
  }

  measure() {
    this.update({ fps: this.drawn, kbps: Math.round((this.bytes * 8) / 1000) });
    this.drawn = 0;
    this.bytes = 0;
  }

  close() {
    clearInterval(this.meter);
    this.es.close();
    this.input.close();
    this.resetDecoder();
  }
}

/* setRemoteClipboard puts the viewer's text on the other machine's clipboard,
   ahead of the paste keystroke that will use it. */
export async function setRemoteClipboard(text) {
  const res = await fetch(apiURL("/api/screen/clipboard"), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text }),
  });
  if (!res.ok) throw new Error((await res.json().catch(() => null))?.error || res.statusText);
}
