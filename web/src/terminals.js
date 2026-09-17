/* The terminal tabs' end of the wire — see specs/073-terminals.md.

   Output arrives on one stream shared by every open terminal, because a
   browser holds only a handful of connections to one origin and a stream per
   tab would spend them all. Keystrokes go back the other way over POST: the
   window is served through a handler that can stream a response but cannot be
   upgraded to a socket, so there is no two-way channel to put them on.

   Nothing here touches the DOM or holds a component. TerminalView registers a
   pair of callbacks and this calls them; what draws the bytes is its problem.
*/

import { apiURL } from "./api.js";

/* Every view currently on screen, by terminal id. `at` is how many bytes of
   that shell this view has drawn, which is what the server resumes from. */
const views = new Map();

let stream = null;
/* Which terminals the open stream carries, as its own membership rather than
   its URL: the URL also holds the byte counts, which move constantly. */
let carried = "";
let retry = 0;

/* How long a dropped stream waits before it is opened again. Short, because
   the usual reason for a drop is this window falling behind a burst of output
   and the screen is stale until it comes back. */
const RETRY_MS = 1000;

/* watchTerminal draws one terminal's output through `onData(bytes, reset)`
   and its shell ending through `onExit()`. It hands back the function that
   stops watching, which is what the view calls when it goes. */
export function watchTerminal(id, { onData, onExit }) {
  views.set(id, { onData, onExit, at: 0 });
  resubscribe();
  return () => {
    views.delete(id);
    resubscribe();
  };
}

/* resubscribe reopens the stream when the set of watched terminals changes,
   and does nothing when it has not. The reopen is what makes the byte counts
   worth carrying: a second terminal opening must not replay the first one's
   output over a view already showing it. */
function resubscribe() {
  const ids = [...views.keys()].sort().join(",");
  if (ids === carried && (stream || !ids)) return;
  carried = ids;
  open();
}

function open() {
  clearTimeout(retry);
  if (stream) {
    stream.close();
    stream = null;
  }
  if (!views.size) return;
  const ids = [...views.entries()].map(([id, view]) => `${id}:${view.at}`).join(",");
  const es = new EventSource(apiURL("/api/stream/terminals?ids=" + encodeURIComponent(ids)));
  stream = es;
  es.onmessage = (e) => {
    let frame;
    try {
      frame = JSON.parse(e.data);
    } catch {
      return;
    }
    const view = views.get(frame.id);
    if (!view) return;
    if (frame.data) {
      // The count comes from the frame rather than from the bytes in it: a
      // redrawn screen is the tail of a longer history, so its length is not
      // how far the shell has got.
      view.at = frame.at;
      view.onData(bytes(frame.data), !!frame.reset);
    }
    if (frame.exit) {
      view.at = frame.at;
      view.onExit?.();
    }
  };
  /* EventSource reconnects on its own, to the URL it was opened with — which
     would ask for output this view has already drawn. So the retry is taken
     over here, where the counts are current. */
  es.onerror = () => {
    if (stream !== es) return;
    es.close();
    stream = null;
    retry = setTimeout(open, RETRY_MS);
  };
}

/* Terminal output is bytes, not text: it is base64 on the wire and a byte
   array here, because only the emulator knows where one character ends. */
function bytes(encoded) {
  const binary = atob(encoded);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i);
  return out;
}

/* Keystrokes waiting for a POST, and the ids that have one in flight. Only
   one request per terminal is out at a time: two would be free to arrive in
   either order, and a shell given its input out of order is a shell given
   different input. Anything typed while one is in flight joins the next,
   which also means holding a key down costs one request rather than thirty.

   The queue holds byte arrays rather than text because the two things that
   feed it are encoded differently, and a shell cannot be told which was
   which. */
const pending = new Map();
const sending = new Set();
const utf8 = new TextEncoder();

/* What was typed or pasted, as text. */
export function sendTerminalInput(id, data) {
  if (data) queue(id, utf8.encode(data));
}

/* The emulator's escape hatch for the report modes that answer in raw bytes.
   Its string carries one byte per character, so encoding it as text would
   turn every byte above 0x7f into two. */
export function sendTerminalBinary(id, data) {
  if (!data) return;
  const out = new Uint8Array(data.length);
  for (let i = 0; i < data.length; i++) out[i] = data.charCodeAt(i) & 0xff;
  queue(id, out);
}

function queue(id, chunk) {
  const waiting = pending.get(id);
  if (waiting) waiting.push(chunk);
  else pending.set(id, [chunk]);
  flush(id);
}

async function flush(id) {
  if (sending.has(id)) return;
  const chunks = pending.get(id);
  if (!chunks?.length) return;
  pending.delete(id);
  sending.add(id);
  try {
    await fetch(apiURL(`/api/terminals/${encodeURIComponent(id)}/input`), {
      method: "POST",
      body: new Blob(chunks, { type: "application/octet-stream" }),
    });
  } catch {
    /* A shell that has gone announces itself on the stream, which is where
       the view learns of it. A lost keystroke needs no second message. */
  } finally {
    sending.delete(id);
    if (pending.has(id)) flush(id);
  }
}

/* A resize is only worth sending once the dragging stops: a full-screen
   program redraws itself on every one, and a drag is a hundred of them. The
   first is sent at once all the same, because until it arrives the shell is
   drawing its prompt for a window of some default width rather than the one
   it is actually in. */
const resizes = new Map();
const sized = new Set();
const RESIZE_MS = 120;

export function resizeTerminal(id, cols, rows) {
  if (!sized.has(id)) {
    sized.add(id);
    postSize(id, cols, rows);
    return;
  }
  clearTimeout(resizes.get(id));
  resizes.set(
    id,
    setTimeout(() => {
      resizes.delete(id);
      postSize(id, cols, rows);
    }, RESIZE_MS),
  );
}

function postSize(id, cols, rows) {
  fetch(apiURL(`/api/terminals/${encodeURIComponent(id)}/resize`), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ cols, rows }),
  }).catch(() => {});
}
