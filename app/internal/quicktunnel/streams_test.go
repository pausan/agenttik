package quicktunnel

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// tunnelClient reaches server under any host name, so a test can ask for a
// trycloudflare.com address and get the fake bridge.
func tunnelClient(server *httptest.Server) *http.Client {
	base := &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	return &http.Client{Transport: StreamTransport{Base: base}}
}

func TestStreamTransportReadsWebsocketAsSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "websocket" || r.Header.Get("Accept") != "text/event-stream" || r.URL.RequestURI() != "/api/stream?x=1" {
			http.Error(w, "not a websocket", http.StatusBadRequest)
			return
		}
		sum := sha1.Sum([]byte(r.Header.Get("Sec-Websocket-Key") + websocketGUID))
		conn, rw, _ := w.(http.Hijacker).Hijack()
		defer conn.Close()
		fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(sum[:]))
		rw.Write(frame(opPing, nil))
		rw.Write(frame(opText, []byte(`{"a":1}`)))
		rw.Write([]byte{opText, 4, 't', 'w', 'o', '\n'}) // first fragment, no FIN
		rw.Write([]byte{0x80, 5, 'l', 'i', 'n', 'e', 's'})
		rw.Write(frame(opClose, nil))
		rw.Flush()
	}))
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, "https://four-random-words-here.trycloudflare.com/api/stream?x=1", nil)
	req.URL.Scheme = "http"
	req.Header.Set("Accept", "text/event-stream")
	res, err := tunnelClient(server).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/event-stream" || res.Header.Get("Upgrade") != "" {
		t.Fatalf("status %d, headers %v", res.StatusCode, res.Header)
	}
	body, err := io.ReadAll(bufio.NewReaderSize(res.Body, 16))
	if err != nil {
		t.Fatal(err)
	}
	want := ": ping\n\ndata: {\"a\":1}\n\ndata: two\ndata: lines\n\n"
	if string(body) != want {
		t.Fatalf("body %q\nwant %q", body, want)
	}
}

func TestStreamTransportLeavesOtherRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "" {
			t.Errorf("%s %s asked for a websocket", r.Host, r.URL)
		}
		io.WriteString(w, "plain")
	}))
	defer server.Close()
	client := tunnelClient(server)
	for _, c := range []struct{ host, accept string }{
		{"words.trycloudflare.com", "application/json"},
		{"example.com", "text/event-stream"},
	} {
		req, _ := http.NewRequest(http.MethodGet, "http://"+c.host+"/api/stream", nil)
		req.Header.Set("Accept", c.accept)
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if string(body) != "plain" {
			t.Errorf("%s: %q", c.host, body)
		}
	}
}

func TestStreamTransportPassesRefusals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "sign in", http.StatusUnauthorized)
	}))
	defer server.Close()
	req, _ := http.NewRequest(http.MethodGet, "http://locked.trycloudflare.com/api/stream", nil)
	req.Header.Set("Accept", "text/event-stream")
	res, err := tunnelClient(server).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", res.StatusCode)
	}
}
