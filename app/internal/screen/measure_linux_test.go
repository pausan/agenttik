package screen

import (
	"os"
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// TestScreenMeasure reports what a viewer gets from a real 1080p X screen:
// frames per second, bitrate and time per frame, for a screen scrolling
// text-like rows and for one where only a few words change. The viewer
// acknowledges every frame at once, like a fast client on a LAN.
//
//	AGENTTIK_SCREEN_MEASURE=1 go test ./app/internal/screen -run TestScreenMeasure -v
func TestScreenMeasure(t *testing.T) {
	if os.Getenv("AGENTTIK_SCREEN_MEASURE") == "" {
		t.Skip("set AGENTTIK_SCREEN_MEASURE=1")
	}
	startXvfbSized(t, "1920x1080x24")
	for _, scene := range []string{"scrolling", "typing"} {
		for _, codec := range Codecs() {
			t.Run(scene+"/"+codec, func(t *testing.T) { measure(t, scene, codec) })
		}
	}
}

func startXvfbSized(t *testing.T, geometry string) {
	t.Helper()
	old := xvfbScreen
	xvfbScreen = geometry
	defer func() { xvfbScreen = old }()
	startXvfb(t)
}

func measure(t *testing.T, scene, codec string) {
	conn, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	scr := xproto.Setup(conn).DefaultScreen(conn)
	win := window(t, conn, 0, 0, 1920, 1080, 0xf0f0f0, 0)
	gc, _ := xproto.NewGcontextId(conn)
	xproto.CreateGC(conn, gc, xproto.Drawable(win), xproto.GcForeground, []uint32{0x202020})
	white, _ := xproto.NewGcontextId(conn)
	xproto.CreateGC(conn, white, xproto.Drawable(win), xproto.GcForeground, []uint32{scr.WhitePixel})

	// Rows of word-sized blocks, like a page of text. Scrolling redraws the
	// whole page 6 pixels further each frame; typing adds a word at a time.
	draw := func(frame int) {
		var rects []xproto.Rectangle
		if scene == "scrolling" {
			xproto.PolyFillRectangle(conn, xproto.Drawable(win), white, []xproto.Rectangle{{Width: 1920, Height: 1080}})
			for row := 0; row < 60; row++ {
				y := (row*22 - frame*6) % 1320
				if y < 0 {
					y += 1320
				}
				for x := 40; x < 1800; {
					w := 20 + (row*37+x*13)%90
					rects = append(rects, xproto.Rectangle{X: int16(x), Y: int16(y), Width: uint16(w), Height: 12})
					x += w + 10
				}
			}
		} else {
			x := 40 + (frame*47)%1700
			y := 100 + (frame/36)%40*22
			rects = append(rects, xproto.Rectangle{X: int16(x), Y: int16(y), Width: 40, Height: 12})
		}
		xproto.PolyFillRectangle(conn, xproto.Drawable(win), gc, rects)
		xproto.GetInputFocus(conn).Reply() //nolint:errcheck // flush
	}

	m := NewManager(Open)
	defer m.Close()
	v, err := m.Watch(codec)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	stop := make(chan struct{})
	go func() {
		tick := time.NewTicker(time.Second / 30)
		defer tick.Stop()
		for frame := 0; ; frame++ {
			select {
			case <-stop:
				return
			case <-tick.C:
				draw(frame)
			}
		}
	}()
	defer close(stop)

	const settle, span = time.Second, 5 * time.Second
	var frames, bytes int
	var started time.Time
	deadline := time.After(settle + span)
	warm := time.After(settle)
	for {
		select {
		case msg := <-v.Messages():
			if msg.Type != "frame" {
				continue
			}
			v.Ack(msg.Seq)
			if started.IsZero() {
				continue
			}
			frames++
			bytes += len(msg.Data) * 3 / 4
			for _, b := range msg.Bands {
				bytes += len(b.Data) * 3 / 4
			}
		case <-warm:
			started = time.Now()
		case <-deadline:
			secs := time.Since(started).Seconds()
			t.Logf("%-9s %-4s %5.1f fps  %6.2f Mbps  %6.1f KB/frame",
				scene, codec, float64(frames)/secs, float64(bytes)*8/secs/1e6, float64(bytes)/1024/max(1, float64(frames)))
			return
		}
	}
}
