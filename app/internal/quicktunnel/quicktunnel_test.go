package quicktunnel

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	capnp "zombiezen.com/go/capnproto2"
)

func TestDecodeResponse(t *testing.T) {
	ok := `{"success":true,"result":{"id":"2d8f2f26-6f2b-4c39-9c0b-9b7a1f0e3c11","name":"qt-x","hostname":"a-b-c-d.trycloudflare.com","account_tag":"acc","secret":"c2VjcmV0"},"errors":[]}`
	c, err := decodeResponse(200, strings.NewReader(ok))
	if err != nil {
		t.Fatal(err)
	}
	if c.Hostname != "a-b-c-d.trycloudflare.com" || string(c.Secret) != "secret" || c.AccountTag != "acc" {
		t.Fatalf("%+v", c)
	}
	if c.URL() != "https://a-b-c-d.trycloudflare.com" {
		t.Fatal(c.URL())
	}
	for _, bad := range []string{
		`{"success":false,"errors":[{"code":1003,"message":"too many tunnels"}]}`,
		`{"success":true,"result":{}}`,
		`<html>rate limited</html>`,
	} {
		if _, err := decodeResponse(429, strings.NewReader(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

// TestRegisterParamsLayout pins registerConnection's parameters to the
// offsets tunnelrpc.capnp compiles to, since nothing generated checks them.
func TestRegisterParamsLayout(t *testing.T) {
	_, seg, _ := capnp.NewMessage(capnp.SingleSegment(nil))
	s, err := capnp.NewRootStruct(seg, capnp.ObjectSize{DataSize: 8, PointerCount: 3})
	if err != nil {
		t.Fatal(err)
	}
	p := registerParams{
		creds:    Credentials{AccountTag: "acc", Secret: []byte("sec")},
		tunnelID: [16]byte{1, 2, 3},
		clientID: [16]byte{9},
		version:  "1.2.3",
		attempts: 4,
	}
	if err := p.encode(s); err != nil {
		t.Fatal(err)
	}
	auth := ptr(t, s, 0).Struct()
	if ptr(t, auth, 0).Text() != "acc" || string(ptr(t, auth, 1).Data()) != "sec" {
		t.Error("auth")
	}
	if !bytes.Equal(ptr(t, s, 1).Data(), p.tunnelID[:]) {
		t.Error("tunnelId")
	}
	opts := ptr(t, s, 2).Struct()
	if opts.Uint8(2) != 4 {
		t.Error("numPreviousAttempts")
	}
	client := ptr(t, opts, 0).Struct()
	if !bytes.Equal(ptr(t, client, 0).Data(), p.clientID[:]) || ptr(t, client, 2).Text() != "1.2.3" || !strings.Contains(ptr(t, client, 3).Text(), "_") {
		t.Error("client info")
	}
	features := capnp.TextList{List: ptr(t, client, 1).List()}
	if f, _ := features.At(0); features.Len() != 1 || f != "serialized_headers" {
		t.Error("features")
	}
}

func TestDecodeConnectionResponse(t *testing.T) {
	results := func(which uint16, fill func(capnp.Struct)) capnp.Struct {
		_, seg, _ := capnp.NewMessage(capnp.SingleSegment(nil))
		res, _ := capnp.NewRootStruct(seg, capnp.ObjectSize{PointerCount: 1})
		resp, _ := capnp.NewStruct(seg, capnp.ObjectSize{DataSize: 8, PointerCount: 1})
		resp.SetUint16(0, which)
		inner, _ := capnp.NewStruct(seg, capnp.ObjectSize{DataSize: 16, PointerCount: 2})
		fill(inner)
		resp.SetPtr(0, inner.ToPtr())
		res.SetPtr(0, resp.ToPtr())
		return res
	}

	loc, err := decodeConnectionResponse(results(1, func(s capnp.Struct) { s.SetText(1, "mad01") }))
	if err != nil || loc != "mad01" {
		t.Fatalf("details: %q %v", loc, err)
	}
	_, err = decodeConnectionResponse(results(0, func(s capnp.Struct) { s.SetText(0, "busy"); s.SetBit(64, true) }))
	if err == nil || errors.Is(err, errRejected) {
		t.Fatalf("retryable error: %v", err)
	}
	_, err = decodeConnectionResponse(results(0, func(s capnp.Struct) { s.SetText(0, "Unauthorized") }))
	if !errors.Is(err, errRejected) {
		t.Fatalf("permanent error: %v", err)
	}
}

func ptr(t *testing.T, s capnp.Struct, i uint16) capnp.Ptr {
	t.Helper()
	p, err := s.Ptr(i)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSerializeHeaders(t *testing.T) {
	h := http.Header{
		"Content-Type":             {"text/plain"},
		"Set-Cookie":               {"a=1", "b=2"},
		"Cf-Cloudflared-Something": {"internal"},
	}
	var got []string
	for _, pair := range strings.Split(serializeHeaders(h), ";") {
		name, value, _ := strings.Cut(pair, ":")
		n, _ := base64.RawStdEncoding.DecodeString(name)
		v, _ := base64.RawStdEncoding.DecodeString(value)
		got = append(got, string(n)+"="+string(v))
	}
	sort.Strings(got)
	if strings.Join(got, "|") != "Content-Type=text/plain|Set-Cookie=a=1|Set-Cookie=b=2" {
		t.Fatal(got)
	}
}

func TestResponseWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{w: rec, header: http.Header{}}
	rw.Header().Set("Content-Length", "2")
	rw.Header().Set("X-A", "b")
	rw.WriteHeader(http.StatusSwitchingProtocols)
	rw.WriteHeader(http.StatusTeapot) // ignored: already written
	rw.Write([]byte("hi"))
	if rec.Code != http.StatusOK {
		t.Errorf("status %d, want 101 sent as 200", rec.Code)
	}
	if rec.Header().Get("Content-Length") != "2" || rec.Header().Get("X-A") != "" {
		t.Errorf("headers %v", rec.Header())
	}
	if rec.Header().Get("Cf-Cloudflared-Response-Headers") == "" || rec.Header().Get("Cf-Cloudflared-Response-Meta") != `{"src":"origin"}` {
		t.Errorf("headers %v", rec.Header())
	}
}

// TestServeBridgesAnEventStream runs a websocket request the way the edge
// hands one over, through the lock, to an origin answering SSE.
func TestServeBridgesAnEventStream(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != "/api/stream?sessions=a" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, ": open\n\ndata: {\"a\":1}\n\n: ping\n\ndata: two\ndata: lines\n\n")
	}))
	defer origin.Close()

	var visitor string
	c := New(strings.TrimPrefix(origin.URL, "http://"), func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			visitor = r.RemoteAddr
			next.ServeHTTP(w, r)
		})
	}, "test")

	body, browser := io.Pipe()
	defer browser.Close()
	req := httptest.NewRequest(http.MethodGet, "/api/stream?sessions=a", body)
	req.Header.Set(upgradeHeader, "websocket")
	req.Header.Set("Sec-Websocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Cf-Connecting-Ip", "203.0.113.7")
	rec := httptest.NewRecorder()
	c.serve(rec, req)

	if visitor != "203.0.113.7:0" {
		t.Errorf("visitor %q", visitor)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Cf-Cloudflared-Response-Headers"), base64.RawStdEncoding.EncodeToString([]byte("s3pPLMBiTxaQ9kYGzzhZRbK+xOo="))) {
		t.Error("no Sec-WebSocket-Accept")
	}
	want := string(frame(opPing, nil)) + string(frame(opText, []byte(`{"a":1}`))) + string(frame(opPing, nil)) +
		string(frame(opText, []byte("two\nlines"))) + string(frame(opClose, nil))
	if rec.Body.String() != want {
		t.Errorf("frames %q\nwant   %q", rec.Body.String(), want)
	}
}

func TestServeProxiesPlainRequests(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Origin", "yes")
		fmt.Fprintf(w, "%s %s", r.Method, r.URL.Path)
	}))
	defer origin.Close()
	c := New(strings.TrimPrefix(origin.URL, "http://"), nil, "test")
	rec := httptest.NewRecorder()
	c.serve(rec, httptest.NewRequest(http.MethodPost, "/api/x", strings.NewReader("{}")))
	if rec.Body.String() != "POST /api/x" {
		t.Fatalf("body %q", rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Cf-Cloudflared-Response-Headers"), base64.RawStdEncoding.EncodeToString([]byte("X-Origin"))) {
		t.Error("origin header not serialized")
	}
}

func TestFrameLengths(t *testing.T) {
	for _, n := range []int{0, 125, 126, 65535, 65536} {
		f := frame(opText, make([]byte, n))
		var got int
		switch f[1] {
		case 126:
			got = int(binary.BigEndian.Uint16(f[2:4]))
		case 127:
			got = int(binary.BigEndian.Uint64(f[2:10]))
		default:
			got = int(f[1])
		}
		if f[0] != 0x81 || got != n {
			t.Errorf("n=%d: header % x", n, f[:min(len(f), 10)])
		}
	}
}

func TestReadFramesStopsAtClose(t *testing.T) {
	masked := func(op byte, payload []byte) []byte {
		b := []byte{0x80 | op, 0x80 | byte(len(payload)), 1, 2, 3, 4}
		return append(b, payload...) // masking is irrelevant to a reader that discards
	}
	in := bytes.NewReader(append(append(masked(0xA, []byte("pong")), masked(opClose, nil)...), masked(opText, []byte("unread"))...))
	if err := readFrames(in); err == nil || in.Len() == 0 {
		t.Fatalf("err %v, left %d", err, in.Len())
	}
}
