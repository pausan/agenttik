package quicktunnel

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
)

// StreamTransport is the bridge's other end, for a machine that forwards the
// UI's requests to another agenttik behind a quick tunnel (a remote profile,
// or `--remote`). The edge would hold an SSE response back until it ends, so
// an event stream asked of a trycloudflare.com host is opened as a websocket
// instead, and handed back as the SSE response the caller asked for. Every
// other request goes to Base unchanged.
type StreamTransport struct {
	Base http.RoundTripper
}

func (t StreamTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet || !IsTunnelHost(req.URL.Hostname()) ||
		!strings.Contains(req.Header.Get("Accept"), "text/event-stream") {
		return t.Base.RoundTrip(req)
	}
	var nonce [16]byte
	rand.Read(nonce[:])
	key := base64.StdEncoding.EncodeToString(nonce[:])
	req = req.Clone(req.Context())
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-Websocket-Version", "13")
	req.Header.Set("Sec-Websocket-Key", key)
	res, err := t.Base.RoundTrip(req)
	if err != nil || res.StatusCode != http.StatusSwitchingProtocols {
		return res, err // a refusal, such as the lock's 401, is the answer
	}
	conn, ok := res.Body.(io.ReadWriteCloser)
	sum := sha1.Sum([]byte(key + websocketGUID))
	if !ok || res.Header.Get("Sec-Websocket-Accept") != base64.StdEncoding.EncodeToString(sum[:]) {
		res.Body.Close()
		return nil, errors.New("quick tunnel refused the websocket")
	}
	for _, name := range []string{"Connection", "Upgrade", "Sec-Websocket-Accept"} {
		res.Header.Del(name)
	}
	res.Header.Set("Content-Type", "text/event-stream")
	res.Header.Set("Cache-Control", "no-cache")
	res.StatusCode, res.Status = http.StatusOK, "200 OK"
	res.ContentLength = -1
	res.Body = &socketEvents{conn: conn, r: bufio.NewReader(conn)}
	return res, nil
}

// IsTunnelHost reports whether host is a quick tunnel's address.
func IsTunnelHost(host string) bool {
	return strings.HasSuffix(strings.ToLower(host), ".trycloudflare.com")
}

// socketEvents reads the bridge's websocket back as SSE: each text message
// one event, each ping a comment, the close frame the end of the stream.
type socketEvents struct {
	conn    io.ReadWriteCloser
	r       *bufio.Reader
	pending []byte // encoded and not yet read
	message []byte // a fragmented message's parts so far
}

func (s *socketEvents) Read(p []byte) (int, error) {
	for len(s.pending) == 0 {
		op, fin, payload, err := readFrame(s.r)
		if err != nil {
			return 0, err
		}
		switch op {
		case opClose:
			return 0, io.EOF
		case opPing:
			s.pending = []byte(": ping\n\n")
		case opText, 0x0: // 0x0 continues a fragmented message
			s.message = append(s.message, payload...)
			if fin {
				s.pending = encodeEvent(s.message)
				s.message = nil
			}
		}
	}
	n := copy(p, s.pending)
	s.pending = s.pending[n:]
	return n, nil
}

func (s *socketEvents) Close() error { return s.conn.Close() }

// encodeEvent is one SSE event carrying data, a line per data line.
func encodeEvent(data []byte) []byte {
	var b bytes.Buffer
	for line := range bytes.SplitSeq(data, []byte("\n")) {
		b.WriteString("data: ")
		b.Write(line)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	return b.Bytes()
}
