/* openEventStream opens one of the server's SSE streams. Behind a Cloudflare
   quick tunnel (Settings › Server › Public address) the edge holds every
   response back until it ends, so a stream would never arrive; there the same
   URL is opened as a websocket instead, which the tunnel answers with each
   event's data as one message — see specs/084-quick-tunnel.md.

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
    this.url = url.href;
    this.onopen = this.onmessage = this.onerror = null;
    this.closed = false;
    this.connect();
  }

  connect() {
    const ws = new WebSocket(this.url);
    this.ws = ws;
    ws.onopen = () => this.onopen?.();
    ws.onmessage = (e) => this.onmessage?.({ data: e.data });
    ws.onclose = () => {
      if (this.closed || this.ws !== ws) return;
      this.onerror?.();
      if (!this.closed) this.timer = setTimeout(() => this.connect(), RETRY_MS);
    };
  }

  close() {
    this.closed = true;
    clearTimeout(this.timer);
    this.ws?.close();
  }
}
