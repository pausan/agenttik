/* Draw historical output without sending its terminal-query replies back to
   the shell. Parser handlers fall through to xterm's normal handling, keeping
   color changes and screen state intact. Its built-in replies are synchronous;
   the microtask restores input forwarding before any keyboard or paste event. */
export function terminalOutput(term, onInput, onDraw) {
  let replaying = false;
  let answeringReplay = false;
  let writes = Promise.resolve();
  let disposed = false;
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
  handlers.push(term.parser.registerDcsHandler({ intermediates: "$", final: "q" }, query));
  for (const id of [4, 10, 11, 12]) {
    handlers.push(term.parser.registerOscHandler(id, query));
  }
  handlers.push(term.onData(data => {
    if (!answeringReplay) onInput(data);
  }));

  return {
    write(bytes, reset, replay) {
      // xterm parses asynchronously. Finish each frame before changing its
      // replay flag or resetting the screen for the next one.
      writes = writes.then(() => new Promise(resolve => {
        if (disposed) return resolve();
        if (reset) term.reset();
        replaying = replay;
        term.write(bytes, () => {
          replaying = false;
          answeringReplay = false;
          if (!disposed) onDraw?.();
          resolve();
        });
      }));
      return writes;
    },
    dispose() {
      disposed = true;
      for (const handler of handlers) handler.dispose();
    },
  };
}
