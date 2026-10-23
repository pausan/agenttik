package screen

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// The agent API: a screenshot, and a list of actions in the shape of the
// computer-use tool, so an agent can pass its tool calls straight through.

// Shot is one screenshot, scaled to Width x Height from a ScreenW x ScreenH
// screen.
type Shot struct {
	Data             []byte
	Type             string
	Width, Height    int
	ScreenW, ScreenH int
}

// Screenshot captures the screen, scaled down to width (0 keeps it), as PNG
// or JPEG.
func (m *Manager) Screenshot(width int, format string) (Shot, error) {
	d, err := m.Display()
	if err != nil {
		return Shot{}, err
	}
	pix, w, h, err := d.Grab(nil)
	if err != nil {
		return Shot{}, err
	}
	shot := Shot{ScreenW: w, ScreenH: h, Width: w, Height: h}
	if width > 0 && width < w {
		shot.Width = width
		shot.Height = max(1, h*width/w)
		pix = shrink(pix, w, h, shot.Width, shot.Height)
	}
	if format == "jpeg" || format == "jpg" {
		shot.Type = "image/jpeg"
		shot.Data, err = encodeJPEG(pix, shot.Width, 0, shot.Height, 85)
	} else {
		shot.Type = "image/png"
		img := rgba(pix, shot.Width, 0, shot.Height)
		shot.Data, err = encodePNG(img.Pix, shot.Width, shot.Height)
	}
	return shot, err
}

// shrink scales a BGRA image down by averaging each destination pixel's
// source area.
func shrink(pix []byte, w, h, dw, dh int) []byte {
	out := make([]byte, dw*dh*4)
	for y := 0; y < dh; y++ {
		y0, y1 := y*h/dh, max((y+1)*h/dh, y*h/dh+1)
		for x := 0; x < dw; x++ {
			x0, x1 := x*w/dw, max((x+1)*w/dw, x*w/dw+1)
			var b, g, r, n int
			for sy := y0; sy < y1; sy++ {
				row := pix[(sy*w+x0)*4 : (sy*w+x1)*4]
				for i := 0; i < len(row); i += 4 {
					b, g, r = b+int(row[i]), g+int(row[i+1]), r+int(row[i+2])
					n++
				}
			}
			o := (y*dw + x) * 4
			out[o], out[o+1], out[o+2], out[o+3] = byte(b/n), byte(g/n), byte(r/n), 255
		}
	}
	return out
}

// Action is one computer-use step. Coordinates are [x, y] in the space of
// the screenshot the agent saw (Actions' width and height).
type Action struct {
	Action          string  `json:"action"`
	Coordinate      []int   `json:"coordinate,omitempty"`
	StartCoordinate []int   `json:"start_coordinate,omitempty"`
	Text            string  `json:"text,omitempty"`
	ScrollDirection string  `json:"scroll_direction,omitempty"`
	ScrollAmount    int     `json:"scroll_amount,omitempty"`
	Duration        float64 `json:"duration,omitempty"` // seconds
}

// Actions is a request: the steps, and the size of the screenshot their
// coordinates refer to (zero means screen pixels).
type Actions struct {
	Width   int      `json:"width,omitempty"`
	Height  int      `json:"height,omitempty"`
	Actions []Action `json:"actions"`
}

// Cursor is where the pointer ended up, in the request's coordinate space.
type Cursor struct {
	X int `json:"x"`
	Y int `json:"y"`
}

const (
	maxActionWait = 30 * time.Second
	clickGap      = 60 * time.Millisecond
)

// Do runs the actions in order and stops at the first that fails, saying
// which.
func (m *Manager) Do(req Actions) (Cursor, error) {
	d, err := m.Display()
	if err != nil {
		return Cursor{}, err
	}
	sw, sh := d.Size()
	toScreen := func(c []int) (int, int, error) {
		if len(c) != 2 {
			return 0, 0, errors.New("coordinate must be [x, y]")
		}
		x, y := c[0], c[1]
		if req.Width > 0 {
			x = x * sw / req.Width
		}
		if req.Height > 0 {
			y = y * sh / req.Height
		} else if req.Width > 0 {
			y = y * sw / req.Width
		}
		if x < 0 || y < 0 || x >= sw || y >= sh {
			return 0, 0, fmt.Errorf("coordinate %v is off the screen", c)
		}
		return x, y, nil
	}
	for i, a := range req.Actions {
		if err := do(d, a, toScreen); err != nil {
			return Cursor{}, fmt.Errorf("action %d (%s): %w", i+1, a.Action, err)
		}
	}
	x, y, _, err := d.Cursor()
	if err != nil {
		return Cursor{}, err
	}
	if req.Width > 0 {
		x = x * req.Width / sw
		if req.Height > 0 {
			y = y * req.Height / sh
		} else {
			y = y * req.Width / sw
		}
	}
	return Cursor{X: x, Y: y}, nil
}

func do(d Display, a Action, toScreen func([]int) (int, int, error)) error {
	moveTo := func(c []int) error {
		if c == nil {
			return nil // where the pointer already is
		}
		x, y, err := toScreen(c)
		if err != nil {
			return err
		}
		return d.Move(x, y)
	}
	// Clicks and scrolls take modifier names in text, held around them.
	withKeys := func(fn func() error) error {
		if strings.TrimSpace(a.Text) == "" {
			return fn()
		}
		keys, ok := ParseKeyCombo(a.Text)
		if !ok {
			return fmt.Errorf("unknown keys %q", a.Text)
		}
		for _, k := range keys {
			d.Key(k, true) //nolint:errcheck
		}
		defer func() {
			for i := len(keys) - 1; i >= 0; i-- {
				d.Key(keys[i], false) //nolint:errcheck
			}
		}()
		return fn()
	}
	click := func(b, times int) error {
		if err := moveTo(a.Coordinate); err != nil {
			return err
		}
		return withKeys(func() error {
			for i := 0; i < times; i++ {
				if i > 0 {
					time.Sleep(clickGap)
				}
				if err := d.Button(b, true); err != nil {
					return err
				}
				if err := d.Button(b, false); err != nil {
					return err
				}
			}
			return nil
		})
	}

	switch a.Action {
	case "mouse_move":
		if a.Coordinate == nil {
			return errors.New("needs coordinate")
		}
		return moveTo(a.Coordinate)
	case "left_click":
		return click(1, 1)
	case "right_click":
		return click(3, 1)
	case "middle_click":
		return click(2, 1)
	case "double_click":
		return click(1, 2)
	case "triple_click":
		return click(1, 3)
	case "left_mouse_down", "left_mouse_up":
		if err := moveTo(a.Coordinate); err != nil {
			return err
		}
		return d.Button(1, a.Action == "left_mouse_down")
	case "left_click_drag":
		if a.Coordinate == nil {
			return errors.New("needs coordinate")
		}
		if err := moveTo(a.StartCoordinate); err != nil {
			return err
		}
		if err := d.Button(1, true); err != nil {
			return err
		}
		time.Sleep(clickGap)
		err := moveTo(a.Coordinate)
		time.Sleep(clickGap)
		return errors.Join(err, d.Button(1, false))
	case "scroll":
		if err := moveTo(a.Coordinate); err != nil {
			return err
		}
		n := max(1, a.ScrollAmount)
		dx, dy := 0, 0
		switch a.ScrollDirection {
		case "up":
			dy = -n
		case "down", "":
			dy = n
		case "left":
			dx = -n
		case "right":
			dx = n
		default:
			return fmt.Errorf("unknown scroll direction %q", a.ScrollDirection)
		}
		return withKeys(func() error { return wheel(d, dx, dy) })
	case "type":
		for _, r := range a.Text {
			if err := d.Type(r); err != nil {
				return err
			}
		}
		return nil
	case "key", "hold_key":
		keys, ok := ParseKeyCombo(a.Text)
		if !ok {
			return fmt.Errorf("unknown keys %q", a.Text)
		}
		for _, k := range keys {
			if err := d.Key(k, true); err != nil {
				return err
			}
		}
		if a.Action == "hold_key" {
			time.Sleep(wait(a.Duration))
		}
		var errs []error
		for i := len(keys) - 1; i >= 0; i-- {
			errs = append(errs, d.Key(keys[i], false))
		}
		return errors.Join(errs...)
	case "wait":
		time.Sleep(wait(a.Duration))
		return nil
	case "cursor_position":
		return nil // every answer carries it
	case "screenshot":
		return errors.New("take screenshots with GET /api/screen/shot")
	}
	return fmt.Errorf("unknown action %q", a.Action)
}

func wait(seconds float64) time.Duration {
	d := time.Duration(seconds * float64(time.Second))
	if d <= 0 {
		d = time.Second
	}
	return min(d, maxActionWait)
}
