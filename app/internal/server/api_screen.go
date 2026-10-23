package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/pausan/agenttik/app/internal/screen"
)

// The machine's screen, for a Screen tab and for agents. The screen belongs
// to the machine, not to a profile, so every profile answers from the
// instance's one manager and the instance's switch. Frames come back over
// SSE and input goes out over POST, for the reasons terminals do. See
// specs/096-screen.md.

// screenRoot is the server holding the machine's screen and its switch.
func (s *Server) screenRoot() *Server {
	if s.apiRoot != nil {
		return s.apiRoot
	}
	return s
}

// UseScreen replaces how the screen is opened, for tests.
func (s *Server) UseScreen(open func() (screen.Display, error)) {
	s.screen.Close()
	s.screen = screen.NewManager(open)
}

type screenInfo struct {
	screen.Info
	Enabled bool `json:"enabled"`
	// SameMachine is true when the browser asking runs on this machine,
	// where a Screen tab would only show itself.
	SameMachine bool `json:"same_machine"`
}

func (s *Server) getScreen(c *fiber.Ctx) error {
	root := s.screenRoot()
	cfg, err := root.store.GetServerConfig()
	if err != nil {
		return err
	}
	return c.JSON(screenInfo{Info: root.screen.Info(), Enabled: cfg.ScreenEnabled, SameMachine: sameMachine(c)})
}

func (s *Server) putScreen(c *fiber.Ctx) error {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	root := s.screenRoot()
	if err := root.store.SetScreenEnabled(body.Enabled); err != nil {
		return err
	}
	if !body.Enabled {
		// Whoever is watching stops now, not at their next request.
		root.screen.Close()
	}
	return s.getScreen(c)
}

// sameMachine says whether the request comes from a browser on this
// machine. A proxy of agenttik's own (the exposed listener, the tunnel)
// appends the client's address to X-Forwarded-For, so its last entry is the
// one to trust; with none, the connection's own address is the client's.
func sameMachine(c *fiber.Ctx) bool {
	addr := c.IP()
	if fwd := c.Get(fiber.HeaderXForwardedFor); fwd != "" {
		parts := strings.Split(fwd, ",")
		addr = strings.TrimSpace(parts[len(parts)-1])
	}
	ip := net.ParseIP(addr)
	return ip != nil && ip.IsLoopback()
}

// screenManager is the machine's screen once sharing is on.
func (s *Server) screenManager() (*screen.Manager, error) {
	root := s.screenRoot()
	cfg, err := root.store.GetServerConfig()
	if err != nil {
		return nil, err
	}
	if !cfg.ScreenEnabled {
		return nil, fiber.NewError(fiber.StatusForbidden, "screen sharing is off on this machine; turn it on in Settings › Server")
	}
	return root.screen, nil
}

func screenError(err error) error {
	if errors.Is(err, screen.ErrUnavailable) {
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
	}
	return err
}

// streamScreen is one viewer's frames, cursor and clipboard, until it
// disconnects.
func (s *Server) streamScreen(c *fiber.Ctx) error {
	m, err := s.screenManager()
	if err != nil {
		return err
	}
	v, err := m.Watch(c.Query("codec"))
	if err != nil {
		return screenError(err)
	}
	open := startStream(c)
	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(bw *bufio.Writer) {
		w := open(bw)
		defer w.Close()
		defer v.Close()
		ticker := time.NewTicker(heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-s.closing:
				return
			case msg, ok := <-v.Messages():
				if !ok {
					return
				}
				payload, err := json.Marshal(msg)
				if err != nil {
					continue
				}
				if _, err := w.WriteString("data: " + string(payload) + "\n\n"); err != nil {
					return
				}
			case <-ticker.C:
				if _, err := w.WriteString(": ping\n\n"); err != nil {
					return
				}
			}
			if err := w.Flush(); err != nil {
				return
			}
		}
	}))
	return nil
}

// screenInput is a viewer's input, its acknowledgement of the frames it has
// drawn, its request for a keyframe and whether it wants the sound, in one
// request so a viewer has one in flight at a time.
func (s *Server) screenInput(c *fiber.Ctx) error {
	m, err := s.screenManager()
	if err != nil {
		return err
	}
	var body struct {
		Viewer   string         `json:"viewer"`
		Ack      uint64         `json:"ack"`
		Key      bool           `json:"key"`
		Audio    *bool          `json:"audio"`
		Events   []screen.Event `json:"events"`
		Received *struct {
			Seq  uint64  `json:"seq"`
			At   float64 `json:"at"`
			Kbps float64 `json:"kbps"`
		} `json:"received"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	v := m.Viewer(body.Viewer)
	if v == nil {
		return fiber.NewError(fiber.StatusNotFound, screen.ErrNotViewing.Error())
	}
	if r := body.Received; r != nil {
		v.Received(r.Seq, r.At, r.Kbps)
	}
	if body.Ack > 0 {
		v.Ack(body.Ack)
	}
	if body.Key {
		v.RequestKey()
	}
	if body.Audio != nil {
		v.SetAudio(*body.Audio)
	}
	if err := v.Input(body.Events); err != nil {
		return badRequest("%v", err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) getScreenClipboard(c *fiber.Ctx) error {
	m, err := s.screenManager()
	if err != nil {
		return err
	}
	d, err := m.Display()
	if err != nil {
		return screenError(err)
	}
	text, err := d.Clipboard()
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"text": text})
}

func (s *Server) putScreenClipboard(c *fiber.Ctx) error {
	m, err := s.screenManager()
	if err != nil {
		return err
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	d, err := m.Display()
	if err != nil {
		return screenError(err)
	}
	if err := d.SetClipboard(body.Text); err != nil {
		return badRequest("%v", err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// screenShot is one screenshot, `?width=` to scale it down and
// `?format=jpeg` for a smaller one. The headers say the screen's own size,
// which actions without a width are in.
func (s *Server) screenShot(c *fiber.Ctx) error {
	m, err := s.screenManager()
	if err != nil {
		return err
	}
	width, _ := strconv.Atoi(c.Query("width"))
	shot, err := m.Screenshot(width, c.Query("format"))
	if err != nil {
		return screenError(err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set("X-Screen-Width", strconv.Itoa(shot.ScreenW))
	c.Set("X-Screen-Height", strconv.Itoa(shot.ScreenH))
	c.Set("X-Image-Width", strconv.Itoa(shot.Width))
	c.Set("X-Image-Height", strconv.Itoa(shot.Height))
	c.Type(strings.TrimPrefix(shot.Type, "image/"))
	return c.Send(shot.Data)
}

// screenActions runs computer-use actions and answers where the pointer
// ended up.
func (s *Server) screenActions(c *fiber.Ctx) error {
	m, err := s.screenManager()
	if err != nil {
		return err
	}
	var req screen.Actions
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if len(req.Actions) == 0 {
		return badRequest("no actions")
	}
	cursor, err := m.Do(req)
	if err != nil {
		if errors.Is(err, screen.ErrUnavailable) {
			return screenError(err)
		}
		return badRequest("%v", err)
	}
	return c.JSON(fiber.Map{"cursor": cursor})
}
