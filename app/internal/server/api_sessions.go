package server

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/store"
)

// windows are the choices offered by the Sessions sidebar.
var windows = map[string]time.Duration{
	"1d":  24 * time.Hour,
	"3d":  72 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"1mo": 30 * 24 * time.Hour,
	"all": 0,
}

const defaultWindow = "3d"

func (s *Server) listSessions(c *fiber.Ctx) error {
	name := c.Query("window", defaultWindow)
	d, ok := windows[name]
	if !ok {
		return badRequest("unknown window %q", name)
	}
	f := store.SessionFilter{
		ProjectID: int64(c.QueryInt("project_id")),
		Query:     c.Query("q"),
		Limit:     c.QueryInt("limit", 200),
		// The Sessions list keeps ticked-off sessions; the project views ask
		// for them to be left out.
		ExcludeDone: !c.QueryBool("include_done", true),
	}
	if d > 0 {
		f.Since = time.Now().Add(-d).UnixMilli()
	}
	sessions, err := s.store.ListSessions(f)
	if err != nil {
		return err
	}
	return c.JSON(sessions)
}

func (s *Server) createSession(c *fiber.Ctx) error {
	var body struct {
		ProjectID  int64  `json:"project_id"`
		Provider   string `json:"provider"`
		Model      string `json:"model"`
		Effort     string `json:"effort"`
		Permission string `json:"permission"`
		Title      string `json:"title"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	project, err := s.store.GetProject(body.ProjectID)
	if err != nil {
		return err
	}
	provider, ok := s.registry.Get(body.Provider)
	if !ok {
		return badRequest("unknown provider %q", body.Provider)
	}
	if body.Model == "" {
		body.Model = provider.Models()[0].ID
	}

	sess := &store.Session{
		ID:         uuid.NewString(),
		ProjectID:  project.ID,
		Title:      strings.TrimSpace(body.Title),
		Provider:   provider.Name(),
		Model:      body.Model,
		Effort:     body.Effort,
		Permission: string(agent.Permission(body.Permission).Valid()),
	}
	if err := s.store.CreateSession(sess); err != nil {
		return err
	}
	full, err := s.store.GetSession(sess.ID)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(full)
}

// sessionDetail is everything the centre panel needs to render a session.
type sessionDetail struct {
	Session  *store.Session  `json:"session"`
	Messages []store.Message `json:"messages"`
	Turns    []store.Turn    `json:"turns"`
	Stats    *store.Stats    `json:"stats"`
	Running  bool            `json:"running"`
}

func (s *Server) getSession(c *fiber.Ctx) error {
	id := c.Params("id")
	sess, err := s.store.GetSession(id)
	if err != nil {
		return err
	}
	messages, err := s.store.ListMessages(id)
	if err != nil {
		return err
	}
	turns, err := s.store.ListTurns(id)
	if err != nil {
		return err
	}
	stats, err := s.store.SessionStats(id)
	if err != nil {
		return err
	}
	return c.JSON(sessionDetail{Session: sess, Messages: messages,
		Turns: turns, Stats: stats, Running: s.runner.Running(id)})
}

// updateSession changes the model, effort or title mid-session.
func (s *Server) updateSession(c *fiber.Ctx) error {
	id := c.Params("id")
	sess, err := s.store.GetSession(id)
	if err != nil {
		return err
	}
	var body struct {
		Provider *string `json:"provider"`
		Model    *string `json:"model"`
		Effort   *string `json:"effort"`
		Title    *string `json:"title"`
		Done     *bool   `json:"done"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if body.Provider != nil || body.Model != nil || body.Effort != nil {
		providerName, model, effort := sess.Provider, sess.Model, sess.Effort
		if body.Provider != nil {
			providerName = *body.Provider
		}
		provider, ok := s.registry.Get(providerName)
		if !ok {
			return badRequest("unknown provider %q", providerName)
		}
		if err := provider.Available(); err != nil {
			return badRequest("provider %q is unavailable: %v", providerName, err)
		}
		if body.Model != nil {
			model = *body.Model
		}
		if body.Effort != nil {
			effort = *body.Effort
		}
		knownModel := false
		for _, candidate := range provider.Models() {
			if candidate.ID == model {
				knownModel = true
				break
			}
		}
		if !knownModel {
			return badRequest("unknown model %q for provider %q", model, providerName)
		}
		if err := s.store.SetSessionModel(id, providerName, model, effort, providerName != sess.Provider); err != nil {
			return err
		}
	}
	if body.Title != nil {
		if err := s.store.SetSessionTitle(id, *body.Title); err != nil {
			return err
		}
	}
	if body.Done != nil {
		if err := s.store.SetSessionDone(id, *body.Done); err != nil {
			return err
		}
	}
	updated, err := s.store.GetSession(id)
	if err != nil {
		return err
	}
	return c.JSON(updated)
}

// reorderSessions records the order the project view was dragged into. The
// body lists the project's sessions top to bottom; ids from another project
// are ignored.
func (s *Server) reorderSessions(c *fiber.Ctx) error {
	id, err := projectID(c)
	if err != nil {
		return err
	}
	if _, err := s.store.GetProject(id); err != nil {
		return err
	}
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if len(body.IDs) == 0 {
		return badRequest("ids are required")
	}
	if err := s.store.ReorderSessions(id, body.IDs); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) deleteSession(c *fiber.Ctx) error {
	id := c.Params("id")
	s.runner.Stop(id) // no-op when idle
	if err := s.store.DeleteSession(id); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) postMessage(c *fiber.Ctx) error {
	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if strings.TrimSpace(body.Prompt) == "" {
		return badRequest("prompt is required")
	}
	turn, err := s.runner.Send(c.Params("id"), body.Prompt)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusAccepted).JSON(turn)
}

func (s *Server) stopSession(c *fiber.Ctx) error {
	if err := s.runner.Stop(c.Params("id")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
