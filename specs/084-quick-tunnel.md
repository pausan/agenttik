# Public internet address (quick tunnel)

Settings › Server has a third way in, **Public internet address**: a random
`https://<four-words>.trycloudflare.com` address that reaches this machine
from anywhere. It uses Cloudflare's free Quick Tunnels, the same service
`cloudflared tunnel --url` uses, with no account, no open port and nothing
else to install. The tunnel client is built into agenttik
(`app/internal/quicktunnel`). Desktop only, like the rest of the pane.

Visitors pass through the same lock as the exposed listener
([043](043-exposed-server.md)): password and, if enabled, 2FA. The pane shows
a red warning while the tunnel is on without a password, and a yellow one
while it is on without 2FA. Turning the tunnel on does not turn the lock on.

## Behaviour

- The switch is saved (`server_config.tunnel_enabled`) and applied again at
  startup, like the listener.
- The last tunnel's credentials are saved (`server_config.tunnel_credentials`,
  JSON, never sent to the UI). Turning the tunnel on, or restarting the app,
  reconnects with them, so **the address stays the same** while Cloudflare
  still knows the tunnel. In testing, a saved tunnel reconnected after
  5 minutes offline. If the edge refuses the tunnel outright, a new one
  (with a new address) is requested and saved.
- **New address** forgets the saved tunnel and requests a new one. The old
  address stops working.
- Turning it off unregisters from the edge and keeps the credentials.
- `PUT /api/server/tunnel {enabled, renew}` answers at once. `GET /api/server`
  carries `tunnel: {enabled, url, connected, ready, location, error}`. The
  pane re-reads it every second until `ready`, then every five seconds, and
  only while visible.
- The link is shown only once the hostname resolves. A new hostname takes a
  few seconds to appear in DNS, and a resolver that is asked too early
  caches the miss for at least a minute (the zone's negative TTL is 60 s).
  So `ready` is checked against trycloudflare.com's own nameservers, which
  no browser shares.

## Protocol

This is the subset of cloudflared's `--protocol http2` that a quick tunnel needs:

1. `POST https://api.trycloudflare.com/tunnel` returns id, account tag,
   secret and hostname.
2. TLS to `region{1,2}.v2.argotunnel.com:7844`, SNI `h2.cftunnel.com`,
   trusting the Cloudflare Origin CA roots that cloudflared embeds
   (`cloudflare_ca.pem`). There is one connection, as cloudflared uses for
   quick tunnels, and the two regions take turns on reconnect. Reconnects
   back off from 1 s to 1 min.
3. The edge is the HTTP/2 *client*. Its first stream carries
   `Cf-Cloudflared-Proxy-Connection-Upgrade: control-stream`, and over it one
   Cap'n Proto RPC, `RegistrationServer.registerConnection`, registers the
   connection. The call is hand-encoded on `zombiezen.com/go/capnproto2`;
   `rpc.go` documents the struct layouts and tests pin them. On stop,
   `unregisterConnection` is called before the socket closes.
4. Every other stream is a visitor request. Response headers go back
   serialized (feature `serialized_headers`) in
   `Cf-Cloudflared-Response-Headers`. `Cf-Connecting-Ip` becomes the request's
   remote address, so the lock's lock-out counts real visitors.

Requests go through `netauth.Gate.Wrap` to a reverse proxy to the window's
loopback backend, the same target the exposed listener uses.

## Event streams over websockets

Quick tunnels buffer a whole response before sending it. Cloudflare documents
this as "no Server-Sent Events", and the official cloudflared behaves the
same way over HTTP/2 and QUIC. agenttik's UI and terminals live on SSE
(`/api/stream`, `/api/stream/terminals`), so:

- `web/src/eventStream.js`'s `openEventStream` returns a plain `EventSource`,
  except on a `*.trycloudflare.com` page, where it opens the same URL as a
  websocket. That object offers `onopen`, `onmessage({data})`, `onerror`,
  `close()` and reconnects 3 s after an error, like EventSource.
- The tunnel answers a websocket request (behind the lock) by `GET`ting the
  same path from the backend and sending each SSE event's data as one text
  frame, and each comment (the heartbeat) as a ping (`bridge.go`). Websocket
  frames are not buffered by the edge.

The backend never sees a websocket.

## Limits

- No uptime guarantee; subject to Cloudflare's terms. At most 200 requests
  in flight per tunnel; more get `429`.
- `agenttik --remote <tunnel URL>` is not supported: the remote client
  proxies SSE as SSE, so its live updates would be held back.
- Not available on `--web` launches.

## Tests

- `go test ./app/internal/quicktunnel` covers RPC layouts, header
  serialization, the response writer and the SSE→websocket bridge, offline.
- `QUICKTUNNEL_LIVE=1 go test ./app/internal/quicktunnel -run Live` opens a
  real tunnel. It checks plain requests, the visitor address, the bridged
  stream arriving event by event, and reconnecting on the same address.
- `QUICKTUNNEL_LIVE=1 go test ./app/internal/server -run TunnelLive` runs the
  whole stack: API on, locked, signed in from outside, and a project event
  read through the tunnel.
