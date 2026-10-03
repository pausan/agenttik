package quicktunnel

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"strings"
)

// Quick tunnels hold a response back until it ends (Cloudflare documents
// them as not supporting Server-Sent Events), and agenttik's UI lives on two
// SSE streams. Websockets do pass through as they are written, so the UI,
// when it finds itself on a trycloudflare.com address, opens its streams as
// websockets instead, and this bridge answers each one by reading the same
// path from the origin as SSE and sending every event's data as one text
// message, and every comment — the stream's heartbeat — as a ping.
//
// It sits behind the same lock as every other request, so a browser that is
// not signed in gets that 401 instead of an upgrade.

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func (o *origin) bridge(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Sec-Websocket-Key")
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "http://"+o.target+r.URL.RequestURI(), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		http.Error(w, "origin unreachable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body) //nolint:errcheck // the visitor gets what there was
		return
	}

	sum := sha1.Sum([]byte(key + websocketGUID))
	w.Header().Set("Upgrade", "websocket")
	w.Header().Set("Connection", "Upgrade")
	w.Header().Set("Sec-Websocket-Accept", base64.StdEncoding.EncodeToString(sum[:]))
	w.WriteHeader(http.StatusSwitchingProtocols)
	w.(http.Flusher).Flush()

	// The browser only ever closes, pongs or goes away; reading its frames is
	// how either is noticed, and ends the origin request with it.
	go func() {
		readFrames(r.Body) //nolint:errcheck // any end is the end
		resp.Body.Close()
	}()
	err = pumpEvents(resp.Body, func(op byte, payload []byte) error {
		if _, err := w.Write(frame(op, payload)); err != nil {
			return err
		}
		w.(http.Flusher).Flush()
		return nil
	})
	if err == nil {
		w.Write(frame(opClose, nil)) //nolint:errcheck // the origin ended the stream
	}
	r.Body.Close()
}

const (
	opText  = 0x1
	opClose = 0x8
	opPing  = 0x9
)

// pumpEvents reads an SSE stream and sends each event's data as a text
// message, and each comment as a ping. It returns nil when the stream ends.
func pumpEvents(body io.Reader, send func(op byte, payload []byte) error) error {
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 64<<10), 64<<20)
	var data []string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if data != nil {
				if err := send(opText, []byte(strings.Join(data, "\n"))); err != nil {
					return err
				}
				data = nil
			}
		case strings.HasPrefix(line, ":"):
			if err := send(opPing, nil); err != nil {
				return err
			}
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	return sc.Err()
}

// frame is one unmasked, unfragmented server frame.
func frame(op byte, payload []byte) []byte {
	n := len(payload)
	b := make([]byte, 0, n+10)
	b = append(b, 0x80|op)
	switch {
	case n < 126:
		b = append(b, byte(n))
	case n <= 0xffff:
		b = append(b, 126)
		b = binary.BigEndian.AppendUint16(b, uint16(n))
	default:
		b = append(b, 127)
		b = binary.BigEndian.AppendUint64(b, uint64(n))
	}
	return append(b, payload...)
}

// readFrames reads, and discards, client frames until a close frame or the
// end of the stream.
func readFrames(r io.Reader) error {
	var hdr [14]byte
	for {
		if _, err := io.ReadFull(r, hdr[:2]); err != nil {
			return err
		}
		op, n := hdr[0]&0x0f, uint64(hdr[1]&0x7f)
		switch n {
		case 126:
			if _, err := io.ReadFull(r, hdr[2:4]); err != nil {
				return err
			}
			n = uint64(binary.BigEndian.Uint16(hdr[2:4]))
		case 127:
			if _, err := io.ReadFull(r, hdr[2:10]); err != nil {
				return err
			}
			n = binary.BigEndian.Uint64(hdr[2:10])
		}
		if hdr[1]&0x80 != 0 {
			n += 4 // masking key
		}
		if _, err := io.CopyN(io.Discard, r, int64(n)); err != nil {
			return err
		}
		if op == opClose {
			return errors.New("closed by the browser")
		}
	}
}
