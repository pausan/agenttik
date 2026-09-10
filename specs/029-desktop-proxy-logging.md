# Desktop proxy logging

The desktop window no longer prints a line like

```
suppressing panic for copyResponse error in test; copy error: write |1: broken pipe
```

every time the UI drops a response part-way through.

The window reaches the server through `httputil.ReverseProxy`, and Wails calls
that handler itself instead of running it under an `http.Server`. The proxy
looks for `http.ServerContextKey` to decide what to do when a response dies
mid-body: with a server in the context it raises `http.ErrAbortHandler`, which
the server recovers silently, and without one it takes a legacy branch that
logs the copy error and mentions tests. Nothing here is a test, and nothing was
wrong — the client really had gone away.

The client is WebKit, and it goes away often. Wails hands the response body to
the write half of a raw pipe it names `|1`, so a cancelled request closes the
read end and the next write fails with `EPIPE`. The UI reopens the event stream
whenever a tab opens or closes, which abandons the previous stream; the write
that discovers this is the next event or the 25 second heartbeat, so the line
arrives well after the click that caused it.

`quietAborts` in `desktop.go` wraps the proxy in what an `http.Server` would
have provided: it puts a server in the request context so the proxy raises the
abort, and recovers that one sentinel value. Any other panic keeps unwinding,
because Wails' asset server has no recovery of its own and a real bug there
should still be loud. The window is now as quiet as `--web`, which serves the
same handler under a real server and never logged this at all.

Filtering the proxy's `ErrorLog` was the other option and was not taken: it
matches on stdlib log wording, and it would keep the misleading "in test" text
for any copy error it did not recognise.
