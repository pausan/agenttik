package server

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/store"
)

// windows are the choices offered by the Sessions sidebar.
var windows = map[string]time.Duration{
	"1h":  time.Hour,
	"1d":  24 * time.Hour,
	"3d":  72 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"1mo": 30 * 24 * time.Hour,
	"all": 0,
}

const defaultWindow = "1h"

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
		// for them to be left out, then ask for those alone for their
		// archive filter.
		ExcludeDone: !c.QueryBool("include_done", true),
		OnlyDone:    c.QueryBool("only_done", false),
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
		ProjectID int64  `json:"project_id"`
		Provider  string `json:"provider"`
		// AccountID is which subscription of that provider to run on. Absent
		// is the provider's default, which is the CLI's own login until
		// Settings says otherwise.
		AccountID  *int64 `json:"account_id"`
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
	accountID, err := s.chooseAccount(provider.Name(), body.AccountID, store.SystemAccount, true)
	if err != nil {
		return err
	}

	sess := &store.Session{
		ID:         uuid.NewString(),
		ProjectID:  project.ID,
		Title:      strings.TrimSpace(body.Title),
		Provider:   provider.Name(),
		AccountID:  accountID,
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

	// Queued are the prompts waiting their turn, oldest first. The transcript
	// draws them under the messages so a queued prompt is visible.
	Queued []store.QueuedMessage `json:"queued"`
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
	queued, err := s.store.ListQueuedMessages(id)
	if err != nil {
		return err
	}
	return c.JSON(sessionDetail{Session: sess, Messages: messages,
		Turns: turns, Stats: stats, Running: s.runner.Running(id), Queued: queued})
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
		// AccountID is which subscription of the provider runs the next turn.
		// Absent keeps the one the task has, unless the provider changed, in
		// which case that provider's default answers.
		AccountID *int64  `json:"account_id"`
		Model     *string `json:"model"`
		Effort    *string `json:"effort"`
		Title     *string `json:"title"`
		Done      *bool   `json:"done"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if body.Provider != nil || body.AccountID != nil || body.Model != nil || body.Effort != nil {
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
		accountID, err := s.chooseAccount(providerName, body.AccountID, sess.AccountID,
			providerName != sess.Provider)
		if err != nil {
			return err
		}
		// A different subscription opens a new conversation for the same
		// reason a different provider does: the thread id belongs to the
		// account that made it, and the other account cannot resume it.
		switched := providerName != sess.Provider || accountID != sess.AccountID
		if err := s.store.SetSessionModel(id, providerName, accountID, model, effort, switched); err != nil {
			return err
		}
	}
	if body.Title != nil {
		if err := s.store.SetSessionTitle(id, *body.Title); err != nil {
			return err
		}
	}
	if body.Done != nil {
		if *body.Done && (s.runner.Running(id) || sess.QueueCount > 0) {
			return badRequest("stop the active or scheduled session before archiving it")
		}
		if err := s.store.SetSessionDone(id, *body.Done); err != nil {
			return err
		}
		// What the task came to, written behind the response: the row is
		// archived on the click and grows its summary a few seconds later.
		// See 051-task-outcomes.md.
		if *body.Done {
			s.runner.SummarizeTask(id)
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

// editMessage rewrites one of your own prompts and carries on from there. The
// transcript is truncated at that message and the new text is sent as an
// ordinary turn.
//
// What this cannot do is rewind the agent. Continuity is the provider's own
// session id and neither CLI offers a way to truncate its history, so the
// agent still remembers the exchange that left our transcript — it reads the
// edit as "I meant this instead", which is the useful half of the intent.
// Nothing on disk is reverted either: the files are whatever the earlier turns
// left behind. The UI says both before you commit to it.
func (s *Server) editMessage(c *fiber.Ctx) error {
	sessionID := c.Params("id")
	if _, err := s.store.GetSession(sessionID); err != nil {
		return err
	}
	messageID, err := strconv.ParseInt(c.Params("message"), 10, 64)
	if err != nil {
		return badRequest("invalid message id")
	}
	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if strings.TrimSpace(body.Prompt) == "" {
		return badRequest("prompt is required")
	}

	message, err := s.store.GetMessage(messageID)
	if err != nil {
		return err
	}
	if message.SessionID != sessionID {
		return badRequest("message %d belongs to another session", messageID)
	}
	if message.Role != store.RoleUser {
		return badRequest("only your own prompts can be edited")
	}
	// Refuse before deleting anything: a busy session would otherwise lose the
	// tail of its transcript and not get a turn for it.
	if s.runner.Running(sessionID) {
		return runner.ErrBusy
	}
	if err := s.store.DeleteMessagesFrom(sessionID, messageID); err != nil {
		return err
	}
	turn, err := s.runner.Send(sessionID, body.Prompt)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusAccepted).JSON(turn)
}

func (s *Server) enqueueMessage(c *fiber.Ctx) error {
	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if strings.TrimSpace(body.Prompt) == "" {
		return badRequest("prompt is required")
	}
	queued, err := s.runner.Enqueue(c.Params("id"), body.Prompt)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"queued": queued, "queue_count": len(queued)})
}

// updateQueuedMessage changes the text or model choice held by one prompt
// that has not started yet. It deliberately does not change the session's
// picker: the queued choice becomes active only when the runner claims that
// prompt.
func (s *Server) updateQueuedMessage(c *fiber.Ctx) error {
	queuedID, err := strconv.ParseInt(c.Params("queuedID"), 10, 64)
	if err != nil || queuedID <= 0 {
		return badRequest("queued id is required")
	}
	queued, err := s.store.GetQueuedMessage(queuedID)
	if err != nil {
		return err
	}
	if queued.SessionID != c.Params("id") {
		return store.ErrNotFound
	}
	var body struct {
		Prompt    string `json:"prompt"`
		Provider  string `json:"provider"`
		AccountID *int64 `json:"account_id"`
		Model     string `json:"model"`
		Effort    string `json:"effort"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" {
		return badRequest("prompt is required")
	}
	provider, ok := s.registry.Get(body.Provider)
	if !ok {
		return badRequest("unknown provider %q", body.Provider)
	}
	if err := provider.Available(); err != nil {
		return badRequest("provider %q is unavailable: %v", body.Provider, err)
	}
	knownModel := false
	for _, candidate := range provider.Models() {
		if candidate.ID == body.Model {
			knownModel = true
			break
		}
	}
	if !knownModel {
		return badRequest("unknown model %q for provider %q", body.Model, body.Provider)
	}
	accountID, err := s.chooseAccount(body.Provider, body.AccountID, queued.AccountID,
		body.Provider != queued.Provider)
	if err != nil {
		return err
	}
	if err := s.store.SetQueuedMessage(queuedID, prompt, body.Provider, accountID, body.Model, body.Effort); err != nil {
		return err
	}
	return c.JSON(&store.QueuedMessage{ID: queuedID, SessionID: queued.SessionID, Prompt: prompt,
		Provider: body.Provider, AccountID: accountID, Model: body.Model, Effort: body.Effort,
		CreatedAt: queued.CreatedAt})
}

// forceQueuedMessage runs the selected queued prompt now rather than when the
// project's queue reaches it. It starts beside the project's other turns, and
// interrupts only this session's own turn, which cannot host a second prompt.
func (s *Server) forceQueuedMessage(c *fiber.Ctx) error {
	var body struct {
		QueuedID int64 `json:"queued_id"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if body.QueuedID <= 0 {
		return badRequest("queued_id is required")
	}
	if err := s.runner.ForceQueued(c.Params("id"), body.QueuedID); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusAccepted)
}

func (s *Server) stopSession(c *fiber.Ctx) error {
	if err := s.runner.Stop(c.Params("id")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
