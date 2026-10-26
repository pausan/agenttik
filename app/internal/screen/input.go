package screen

import (
	"errors"
	"fmt"
)

// Event is one thing a viewer did, in screen pixels.
//
//	m  move to x,y
//	b  button b down (d) or up at x,y
//	w  wheel by dx,dy steps at x,y (positive is down and right)
//	k  key down (d) or up; key and code as the browser named them
//	c  type text, whatever modifiers the viewer holds
type Event struct {
	T    string `json:"t"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
	B    int    `json:"b,omitempty"`
	D    bool   `json:"d,omitempty"`
	DX   int    `json:"dx,omitempty"`
	DY   int    `json:"dy,omitempty"`
	Key  string `json:"key,omitempty"`
	Code string `json:"code,omitempty"`
	Text string `json:"text,omitempty"`
}

// maxWheelSteps bounds one wheel event, so a bad value cannot spin a list for
// minutes.
const maxWheelSteps = 50

// Input applies a viewer's events in order. Keys and buttons it holds are
// remembered, so closing the viewer lets go of them.
func (v *Viewer) Input(events []Event) error {
	var errs []error
	for _, ev := range events {
		if err := v.apply(ev); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (v *Viewer) apply(ev Event) error {
	d := v.d
	switch ev.T {
	case "m":
		return d.Move(clampTo(d, ev.X, ev.Y))
	case "b":
		if ev.B < 1 || ev.B > 3 {
			return fmt.Errorf("no mouse button %d", ev.B)
		}
		if err := d.Move(clampTo(d, ev.X, ev.Y)); err != nil {
			return err
		}
		v.mu.Lock()
		if ev.D {
			v.buttons[ev.B] = true
		} else {
			delete(v.buttons, ev.B)
		}
		v.mu.Unlock()
		return d.Button(ev.B, ev.D)
	case "w":
		if err := d.Move(clampTo(d, ev.X, ev.Y)); err != nil {
			return err
		}
		return wheel(d, ev.DX, ev.DY)
	case "k":
		v.mu.Lock()
		ks, held := v.keys[ev.Code]
		if ev.D || !held {
			var ok bool
			if ks, ok = KeysymForBrowserKey(ev.Key, ev.Code); !ok {
				v.mu.Unlock()
				return nil // a dead key or one X has no name for
			}
		}
		if ev.D {
			v.keys[ev.Code] = ks
		} else {
			delete(v.keys, ev.Code)
		}
		v.mu.Unlock()
		return d.Key(ks, ev.D)
	case "c":
		for _, r := range ev.Text {
			if err := d.Type(r); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("unknown input %q", ev.T)
}

func wheel(d Display, dx, dy int) error {
	steps := func(n, neg, pos int) error {
		b := pos
		if n < 0 {
			b, n = neg, -n
		}
		for i := 0; i < min(n, maxWheelSteps); i++ {
			if err := d.Button(b, true); err != nil {
				return err
			}
			if err := d.Button(b, false); err != nil {
				return err
			}
		}
		return nil
	}
	if err := steps(dy, 4, 5); err != nil {
		return err
	}
	return steps(dx, 6, 7)
}

func clampTo(d Display, x, y int) (int, int) {
	w, h := d.Size()
	return max(0, min(x, w-1)), max(0, min(y, h-1))
}
