/* openEventStream opens one of the server's SSE streams. Behind a Cloudflare
   quick tunnel (Settings › Server › Public address) the edge holds every
   response back until it ends, so a stream would never arrive; there the same
   URL is opened as a websocket instead — see specs/084-quick-tunnel.md. A
   browser that can inflate gzip asks for the stream gzipped, which the tunnel
   sends as binary messages and this reads as SSE: terminal output and event
   JSON are several times smaller, and the tunnel can be a slow link. One that
   cannot is sent each event's data as a text message.

   What comes back has the part of EventSource the UI uses: onopen, onmessage
   with { data }, onerror, close(), and reconnecting by itself after an
   error until closed. */
export function openEventStream(url, where = globalThis.location) {
  if (!where?.hostname?.endsWith(".trycloudflare.com")) return new EventSource(url);
  return new SocketEventSource(new URL(url, where.href));
}

/* Browsers wait about three seconds before an EventSource reconnects. */
const RETRY_MS = 3000;

class SocketEventSource {
  constructor(url) {
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    if (typeof DecompressionStream === "function") url.searchParams.set("encoding", "gzip");
    this.url = url.href;
    this.onopen = this.onmessage = this.onerror = null;
    this.closed = false;
    this.connect();
  }

  connect() {
    const ws = new WebSocket(this.url);
    ws.binaryType = "arraybuffer";
    this.ws = ws;
    let gzip = null;
    ws.onopen = () => this.onopen?.();
    ws.onmessage = (e) => {
      if (typeof e.data === "string") return this.onmessage?.({ data: e.data });
      gzip ??= this.inflate(ws);
      gzip.writer.write(new Uint8Array(e.data)).catch(() => {});
    };
    ws.onclose = async () => {
      // Like EventSource, every event that arrived is handed on before the
      // error: a terminal resumes from what it has drawn, so one drawn after
      // it had reconnected would be drawn twice.
      if (gzip) {
        gzip.writer.close().catch(() => {});
        await gzip.drained;
      }
      if (this.closed || this.ws !== ws) return;
      this.onerror?.();
      if (!this.closed) this.timer = setTimeout(() => this.connect(), RETRY_MS);
    };
  }

  /* inflate reads one socket's binary messages as a single gzipped SSE
     stream, handing each event's data on in order. `drained` settles once
     the stream has ended or failed to decode. */
  inflate(ws) {
    const inflater = new DecompressionStream("gzip");
    const reader = inflater.readable.pipeThrough(new TextDecoderStream()).getReader();
    const live = () => this.ws === ws && !this.closed;
    const drained = (async () => {
      let text = "";
      for (;;) {
        const { value, done } = await reader.read();
        if (done || !live()) return;
        text += value;
        let end;
        while ((end = text.indexOf("\n\n")) >= 0) {
          const data = text
            .slice(0, end)
            .split("\n")
            .filter((line) => line.startsWith("data:"))
            .map((line) => line.slice(line.startsWith("data: ") ? 6 : 5));
          text = text.slice(end + 2);
          if (data.length && live()) this.onmessage?.({ data: data.join("\n") });
        }
      }
    })().catch(() => {});
    return { writer: inflater.writable.getWriter(), drained };
  }

  close() {
    this.closed = true;
    clearTimeout(this.timer);
    this.ws?.close();
  }
}
