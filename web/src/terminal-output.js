/* Draw historical output without sending its terminal-query replies back to
   the shell. Parser handlers fall through to xterm's normal handling, keeping
   color changes and screen state intact. Its built-in replies are synchronous;
   the microtask restores input forwarding before any keyboard or paste event. */
export function terminalOutput(term, onInput, onDraw) {
  let replaying = false;
  let answeringReplay = false;
  let queued = [];
  let drawing = null;
  let disposed = false;
  // Set by a scrollback clear until its output has settled. xterm shrinks its
  // scroll area a frame later, and the browser's clamped scroll event then
  // reads as the user scrolling up, pinning the view to the top of the
  // scrollback as more output arrives.
  let following = false;
  const query = () => {
    answeringReplay = replaying;
    queueMicrotask(() => { answeringReplay = false; });
    return false;
  };
  const handlers = [
    { final: "c" }, { prefix: ">", final: "c" },
    { final: "n" }, { prefix: "?", final: "n" },
    { intermediates: "$", final: "p" },
    { prefix: "?", intermediates: "$", final: "p" },
    { final: "t" },
  ].map(id => term.parser.registerCsiHandler(id, query));
  handlers.push(term.parser.registerCsiHandler({ final: "J" }, params => {
    // Clearing scrollback also ends reading old output. Otherwise xterm can
    // leave the new output pinned to the top of its buffer after `clear`.
    if (params[0] === 3) {
      following = true;
      queueMicrotask(() => { if (!disposed) term.scrollToBottom(); });
    }
    return false;
  }));
  handlers.push(term.parser.registerDcsHandler({ intermediates: "$", final: "q" }, query));
  for (const id of [4, 10, 11, 12]) {
    handlers.push(term.parser.registerOscHandler(id, query));
  }
  handlers.push(term.onData(data => {
    if (!answeringReplay) onInput(data);
  }));

  // xterm parses asynchronously. Finish each write before changing its replay
  // flag or resetting the screen for the next one.
  const draw = (bytes, reset, replay) => new Promise(resolve => {
    if (reset) term.reset();
    replaying = replay;
    term.write(bytes, () => {
      replaying = false;
      answeringReplay = false;
      if (following && !disposed) term.scrollToBottom();
      if (!disposed) onDraw?.();
      resolve();
    });
  });
  // Once the output after a clear has stopped, the late scroll event comes
  // within two frames; scrolling after them leaves the view at the bottom.
  const nextFrame = globalThis.requestAnimationFrame ?? (callback => setTimeout(callback, 16));
  const settle = () => nextFrame(() => nextFrame(() => {
    if (disposed || drawing) return; // more output came, and its drain settles
    term.scrollToBottom();
    following = false;
  }));
  // Frames that arrive while one is drawn go to xterm together. One write per
  // frame paid a clamped timer each, which made a long listing crawl.
  const batchBytes = 128 << 10;
  const drain = async () => {
    while (queued.length && !disposed) {
      const { bytes, reset, replay } = queued[0];
      let size = bytes.length;
      let count = 1;
      // Keep parser batches small enough for input and paint between writes.
      while (count < queued.length && !queued[count].reset && queued[count].replay === replay
        && size + queued[count].bytes.length <= batchBytes) {
        size += queued[count++].bytes.length;
      }
      const chunks = queued.splice(0, count).map(frame => frame.bytes);
      await draw(join(chunks), reset, replay);
    }
    queued = [];
    drawing = null;
    if (following && !disposed) settle();
  };

  return {
    write(bytes, reset, replay) {
      if (disposed) return Promise.resolve();
      queued.push({ bytes, reset, replay });
      drawing ??= drain();
      return drawing;
    },
    dispose() {
      disposed = true;
      queued = [];
      for (const handler of handlers) handler.dispose();
    },
  };
}

function join(chunks) {
  if (chunks.length === 1) return chunks[0];
  let total = 0;
  for (const chunk of chunks) total += chunk.length;
  const out = new Uint8Array(total);
  let at = 0;
  for (const chunk of chunks) {
    out.set(chunk, at);
    at += chunk.length;
  }
  return out;
}
