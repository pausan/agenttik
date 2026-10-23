package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/screen"
)

func fakeScreen(t *testing.T, s *Server) *screen.Fake {
	t.Helper()
	f := screen.NewFake(320, 200)
	s.UseScreen(func() (screen.Display, error) { return f, nil })
	return f
}

func TestScreenRoutesNeedTheSwitch(t *testing.T) {
	s, _ := newTestServer(t)
	f := fakeScreen(t, s)

	info := decode[screenInfo](t, do(t, s, "GET", "/api/screen", nil))
	if info.Enabled || !info.Available || info.Width != 320 || len(info.Codecs) == 0 {
		t.Fatalf("info %+v", info)
	}
	for _, path := range []string{"/api/screen/shot", "/api/screen/clipboard", "/api/stream/screen"} {
		if resp := do(t, s, "GET", path, nil); resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%s while off: %d", path, resp.StatusCode)
		}
	}
	if resp := do(t, s, "POST", "/api/screen/actions", map[string]any{"actions": []any{map[string]string{"action": "wait"}}}); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("actions while off: %d", resp.StatusCode)
	}

	if info := decode[screenInfo](t, do(t, s, "PUT", "/api/screen", map[string]bool{"enabled": true})); !info.Enabled {
		t.Fatal("switch did not stick")
	}

	resp := do(t, s, "GET", "/api/screen/shot?width=160", nil)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" || resp.Header.Get("X-Screen-Width") != "320" || resp.Header.Get("X-Image-Width") != "160" {
		t.Fatalf("shot: %d %v", resp.StatusCode, resp.Header)
	}
	if img, err := png.Decode(resp.Body); err != nil || img.Bounds().Dx() != 160 {
		t.Fatalf("shot image: %v", err)
	}

	act := decode[struct{ Cursor screen.Cursor }](t, do(t, s, "POST", "/api/screen/actions", map[string]any{
		"width":   160,
		"actions": []any{map[string]any{"action": "left_click", "coordinate": []int{10, 20}}},
	}))
	if act.Cursor != (screen.Cursor{X: 10, Y: 20}) || !slices.Contains(f.Log(), "move 20 40") {
		t.Fatalf("click: %+v %q", act.Cursor, f.Log())
	}
	if resp := do(t, s, "POST", "/api/screen/actions", map[string]any{"actions": []any{map[string]string{"action": "fly"}}}); resp.StatusCode != 400 {
		t.Fatalf("bad action: %d", resp.StatusCode)
	}

	if resp := do(t, s, "PUT", "/api/screen/clipboard", map[string]string{"text": "from the viewer"}); resp.StatusCode != 204 {
		t.Fatalf("set clipboard: %d", resp.StatusCode)
	}
	if got := decode[struct{ Text string }](t, do(t, s, "GET", "/api/screen/clipboard", nil)); got.Text != "from the viewer" {
		t.Fatalf("clipboard %q", got.Text)
	}
	if resp := do(t, s, "POST", "/api/screen/input", map[string]any{"viewer": "gone"}); resp.StatusCode != 404 {
		t.Fatalf("input to no viewer: %d", resp.StatusCode)
	}
}

func TestScreenSameMachine(t *testing.T) {
	s, _ := newTestServer(t)
	fakeScreen(t, s)
	get := func(forwarded string) bool {
		req := httptest.NewRequest("GET", "/api/screen", nil)
		if forwarded != "" {
			req.Header.Set("X-Forwarded-For", forwarded)
		}
		resp, err := s.app.Test(req, 5000)
		if err != nil {
			t.Fatal(err)
		}
		var info screenInfo
		json.NewDecoder(resp.Body).Decode(&info)
		return info.SameMachine
	}
	if !get("127.0.0.1") || !get("10.0.0.9, ::1") {
		t.Error("a proxied loopback client is this machine")
	}
	if get("192.168.1.20") || get("127.0.0.1, 192.168.1.20") {
		t.Error("a client on the network is another machine")
	}
}

func TestScreenInputAppliesArrivalBeforeAcknowledgement(t *testing.T) {
	s, store := newTestServer(t)
	f := fakeScreen(t, s)
	if err := store.SetScreenEnabled(true); err != nil {
		t.Fatal(err)
	}
	v, err := s.screen.Watch("jpeg")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	frame := func() screen.Message {
		t.Helper()
		deadline := time.After(3 * time.Second)
		for {
			select {
			case msg := <-v.Messages():
				if msg.Type == "frame" {
					return msg
				}
			case <-deadline:
				t.Fatal("no frame")
			}
		}
	}
	for i := 0; i < 2; i++ {
		msg := frame()
		resp := do(t, s, "POST", "/api/screen/input", map[string]any{
			"viewer": v.ID, "ack": msg.Seq,
			"received": map[string]any{"seq": msg.Seq, "at": 1000 + i*1000, "kbps": 600},
		})
		if resp.StatusCode != 204 {
			t.Fatalf("feedback: %d", resp.StatusCode)
		}
		f.Fill(0, 0, 20, 20, byte(i*100), 255, 0)
	}
	if msg := frame(); msg.Cap != 540 {
		t.Fatalf("cap after queueing feedback: %d, want 540", msg.Cap)
	}
}

// The whole trip a Screen tab on another machine makes: its window asks its
// own instance for the remote profile's stream, which forwards it, and its
// input and acknowledgements go the same way.
func TestScreenThroughRemoteProfile(t *testing.T) {
	remoteSrv, remoteStore := newTestServer(t)
	f := fakeScreen(t, remoteSrv)
	if err := remoteStore.SetScreenEnabled(true); err != nil {
		t.Fatal(err)
	}
	address := listen(t, remoteSrv)
	local, _ := newTestServer(t)
	state := decode[remoteState](t, do(t, local, "POST", "/api/remotes", map[string]string{"address": address}))
	if state.Status != "ok" {
		t.Fatalf("%+v", state)
	}
	id := machineID(t, remoteStore)
	localAddr := listen(t, local)

	resp, err := http.Get("http://" + localAddr + "/api/stream/screen?codec=jpeg&remote=" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("stream: %d", resp.StatusCode)
	}
	messages := make(chan screen.Message, 16)
	go func() {
		r := bufio.NewReader(resp.Body)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				close(messages)
				return
			}
			if data, ok := strings.CutPrefix(line, "data: "); ok {
				var msg screen.Message
				json.Unmarshal([]byte(data), &msg)
				messages <- msg
			}
		}
	}()
	wait := func(kind string) screen.Message {
		for {
			select {
			case msg, ok := <-messages:
				if !ok {
					t.Fatal("stream ended")
				}
				if msg.Type == kind {
					return msg
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("no %s", kind)
			}
		}
	}
	hello := wait("hello")
	frame := wait("frame")
	if !frame.Key || len(frame.Bands) == 0 {
		t.Fatalf("first frame %+v", frame)
	}

	body, _ := json.Marshal(map[string]any{
		"viewer": hello.ID, "ack": frame.Seq,
		"events": []screen.Event{{T: "m", X: 7, Y: 8}, {T: "c", Text: "x"}},
	})
	post, err := http.Post("http://"+localAddr+"/api/screen/input?remote="+id, "application/json", bytes.NewReader(body))
	if err != nil || post.StatusCode != 204 {
		t.Fatalf("input: %v %v", post.StatusCode, err)
	}
	if got := f.Log(); !slices.Equal(got, []string{"move 7 8", "type x"}) {
		t.Fatalf("remote got %q", got)
	}
	f.Fill(0, 0, 10, 10, 255, 255, 255)
	if next := wait("frame"); next.Seq != frame.Seq+1 {
		t.Fatalf("next frame %d", next.Seq)
	}
}
