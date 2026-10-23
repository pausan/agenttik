package screen

import (
	"bytes"
	"encoding/base64"
	"image/jpeg"
	"image/png"
	"math"
	"slices"
	"testing"
	"time"
)

func fakeManager(t *testing.T, w, h int) (*Manager, *Fake) {
	t.Helper()
	f := NewFake(w, h)
	m := NewManager(func() (Display, error) { return f, nil })
	t.Cleanup(m.Close)
	return m, f
}

// next waits for the viewer's next message of one of the given types.
func next(t *testing.T, v *Viewer, types ...string) Message {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case msg, ok := <-v.Messages():
			if !ok {
				t.Fatal("stream closed")
			}
			if slices.Contains(types, msg.Type) {
				return msg
			}
		case <-deadline:
			t.Fatalf("no %v message", types)
		}
	}
}

// quiet says no frame arrives for a while.
func quiet(t *testing.T, v *Viewer, d time.Duration) {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case msg := <-v.Messages():
			if msg.Type == "frame" {
				t.Fatalf("unexpected frame %d", msg.Seq)
			}
		case <-deadline:
			return
		}
	}
}

func TestViewerSendsFramesOnlyWhenTheScreenChanges(t *testing.T) {
	for _, codec := range Codecs() {
		t.Run(codec, func(t *testing.T) {
			m, f := fakeManager(t, 320, 200)
			v, err := m.Watch(codec)
			if err != nil {
				t.Fatal(err)
			}
			defer v.Close()
			hello := next(t, v, "hello")
			if hello.Codec != codec || hello.SW != 320 || hello.SH != 200 || hello.ID != v.ID {
				t.Fatalf("hello %+v", hello)
			}
			first := next(t, v, "frame")
			if !first.Key || first.Seq != 1 || first.SW != 320 {
				t.Fatalf("first frame %+v", first)
			}
			v.Ack(first.Seq)
			quiet(t, v, 150*time.Millisecond)

			f.Fill(10, 70, 20, 20, 255, 0, 0)
			second := next(t, v, "frame")
			if second.Key || second.Seq != 2 {
				t.Fatalf("second frame key %v seq %d", second.Key, second.Seq)
			}
			if codec == "jpeg" && (len(second.Bands) != 1 || second.Bands[0].Y != 64) {
				t.Fatalf("changed bands %+v", second.Bands)
			}
			v.Ack(second.Seq)

			v.RequestKey()
			if again := next(t, v, "frame"); !again.Key {
				t.Fatal("requested keyframe was not one")
			}
		})
	}
}

func TestViewerWaitsForAcknowledgements(t *testing.T) {
	m, f := fakeManager(t, 160, 120)
	v, err := m.Watch("jpeg")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	next(t, v, "hello")
	v.mu.Lock()
	window, _ := v.rate.window()
	v.mu.Unlock()
	var last Message
	for i := 0; i < window; i++ {
		f.Fill(0, 0, 160, 120, byte(i*20), 0, 0)
		last = next(t, v, "frame")
	}
	f.Fill(0, 0, 160, 120, 0, 255, 0)
	quiet(t, v, 300*time.Millisecond)
	v.Ack(last.Seq)
	if msg := next(t, v, "frame"); msg.Seq != last.Seq+1 {
		t.Fatalf("after the ack got frame %d", msg.Seq)
	}
}

func TestViewerInputAndRelease(t *testing.T) {
	m, f := fakeManager(t, 100, 100)
	v, err := m.Watch("jpeg")
	if err != nil {
		t.Fatal(err)
	}
	err = v.Input([]Event{
		{T: "m", X: 500, Y: -3},
		{T: "b", X: 10, Y: 20, B: 1, D: true},
		{T: "k", Key: "Control", Code: "ControlLeft", D: true},
		{T: "k", Key: "c", Code: "KeyC", D: true},
		{T: "k", Key: "C", Code: "KeyC"}, // released as the key it pressed
		{T: "w", X: 5, Y: 5, DY: -2},
		{T: "c", Text: "hé"},
	})
	if err != nil {
		t.Fatal(err)
	}
	v.Close()
	want := []string{
		"move 99 0",
		"move 10 20", "button 1 true",
		"key 0xffe3 true", "key 0x63 true", "key 0x63 false",
		"move 5 5", "button 4 true", "button 4 false", "button 4 true", "button 4 false",
		"type h", "type é",
		// Closing lets go of what was still held.
		"key 0xffe3 false", "button 1 false",
	}
	if got := f.Log(); !slices.Equal(got, want) {
		t.Fatalf("log\n%q\nwant\n%q", got, want)
	}
}

func TestViewerArrivalFeedback(t *testing.T) {
	start := time.Now().Add(-100 * time.Millisecond)
	v := &Viewer{
		rate:     newRateControl(start),
		sent:     3,
		inFlight: []sentFrame{{seq: 1, at: start}, {seq: 2, at: start.Add(33 * time.Millisecond)}, {seq: 3, at: start.Add(66 * time.Millisecond)}},
	}
	v.Received(99, 1000, 600) // unknown frame
	v.Received(1, math.Inf(1), 600)
	v.Received(1, 1000, math.NaN())
	v.Received(1, 1e20, 600)
	if len(v.rate.delays) != 0 {
		t.Fatal("invalid feedback changed the rate")
	}
	v.Received(1, 1000, 600)
	v.Received(1, 1000, 600) // already recorded
	v.Received(2, 1233, 600) // 200 ms more delay
	v.Ack(2)
	if len(v.rate.delays) != 2 || v.rate.kbps != 540 || v.rate.queue != 200*time.Millisecond || v.rate.srtt == 0 {
		t.Fatalf("feedback: %+v", v.rate)
	}
	// An acknowledgement may arrive after the timeout cleared its frames.
	v.inFlight = nil
	v.Ack(3)
}

func TestJPEGQualityFollowsTheBitrate(t *testing.T) {
	const w, h = 320, 128
	pix := make([]byte, w*h*4)
	for i := range pix {
		pix[i] = byte(i*37 + i/53)
	}
	e := jpegEncoder{w: w, h: h}
	e.SetKbps(maxKbps)
	_, high, err := e.Encode(pix, nil, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	e.SetKbps(minKbps)
	msg, low, err := e.Encode(pix, nil, 0, true)
	if err != nil || low >= high {
		t.Fatalf("JPEG at minimum cap: %d bytes, full quality: %d, %v", low, high, err)
	}
	for _, band := range msg.Bands {
		data, _ := base64.StdEncoding.DecodeString(band.Data)
		if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestViewerForwardsClipboardChanges(t *testing.T) {
	m, f := fakeManager(t, 100, 100)
	v, _ := m.Watch("jpeg")
	defer v.Close()
	next(t, v, "frame")
	f.Copy("copied there")
	if msg := next(t, v, "clip"); msg.Text != "copied there" {
		t.Fatalf("clip %q", msg.Text)
	}
}

func TestScreenshotScales(t *testing.T) {
	m, f := fakeManager(t, 400, 200)
	f.Fill(0, 0, 200, 200, 255, 0, 0)
	shot, err := m.Screenshot(100, "png")
	if err != nil {
		t.Fatal(err)
	}
	if shot.Width != 100 || shot.Height != 50 || shot.ScreenW != 400 || shot.Type != "image/png" {
		t.Fatalf("shot %+v", shot)
	}
	img, err := png.Decode(bytes.NewReader(shot.Data))
	if err != nil {
		t.Fatal(err)
	}
	if r, g, _, _ := img.At(10, 10).RGBA(); r>>8 != 255 || g>>8 != 0 {
		t.Fatalf("left half %d,%d", r>>8, g>>8)
	}
	shot, err = m.Screenshot(0, "jpeg")
	if err != nil || shot.Width != 400 {
		t.Fatal(shot.Width, err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(shot.Data)); err != nil {
		t.Fatal(err)
	}
}

func TestActionsScaleCoordinatesAndHoldKeys(t *testing.T) {
	m, f := fakeManager(t, 2000, 1000)
	cursor, err := m.Do(Actions{Width: 1000, Actions: []Action{
		{Action: "left_click", Coordinate: []int{100, 50}, Text: "shift"},
		{Action: "type", Text: "ok"},
		{Action: "key", Text: "ctrl+s"},
		{Action: "scroll", Coordinate: []int{10, 10}, ScrollDirection: "down", ScrollAmount: 1},
		{Action: "mouse_move", Coordinate: []int{500, 250}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if cursor != (Cursor{500, 250}) {
		t.Fatalf("cursor %+v", cursor)
	}
	want := []string{
		"move 200 100", "key 0xffe1 true", "button 1 true", "button 1 false", "key 0xffe1 false",
		"type o", "type k",
		"key 0xffe3 true", "key 0x73 true", "key 0x73 false", "key 0xffe3 false",
		"move 20 20", "button 5 true", "button 5 false",
		"move 1000 500",
	}
	if got := f.Log(); !slices.Equal(got, want) {
		t.Fatalf("log\n%q\nwant\n%q", got, want)
	}
	if _, err := m.Do(Actions{Actions: []Action{{Action: "left_click", Coordinate: []int{5000, 1}}}}); err == nil {
		t.Fatal("a click off the screen was accepted")
	}
	if _, err := m.Do(Actions{Actions: []Action{{Action: "fly"}}}); err == nil {
		t.Fatal("an unknown action was accepted")
	}
}

func TestShapeMessageIsPNG(t *testing.T) {
	msg, ok := shapeMessage(CursorShape{W: 2, H: 2, RGBA: make([]byte, 16)})
	data, _ := base64.StdEncoding.DecodeString(msg.Data)
	if _, err := png.Decode(bytes.NewReader(data)); !ok || err != nil {
		t.Fatal(ok, err)
	}
}

// uncontrolled is a screen that shows but takes no input, as a Mac does
// before Accessibility allows it.
type uncontrolled struct{ *Fake }

func (uncontrolled) ControlReason() string { return "not allowed" }

func TestInfoSaysWhyInputWillNotArrive(t *testing.T) {
	m := NewManager(func() (Display, error) { return NewFake(64, 32), nil })
	if info := m.Info(); !info.Available || info.Control != "" {
		t.Fatalf("a controllable screen: %+v", info)
	}
	m = NewManager(func() (Display, error) { return uncontrolled{NewFake(64, 32)}, nil })
	if info := m.Info(); !info.Available || info.Control != "not allowed" {
		t.Fatalf("an uncontrollable screen: %+v", info)
	}
}
