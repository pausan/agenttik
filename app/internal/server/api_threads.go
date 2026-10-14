package server

import (
	"strings"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/pausan/agenttik/app/internal/store"
)

// Threads answer about a passage of a conversation without adding to it.
// Each is a session of its own, hidden from task lists, that starts from a
// copy of its conversation; a thread can then be spawned as a task that
// starts from a copy of the thread. See specs/092-threads.md.

const maxQuote = 4000

// threadOrigin is where a task spawned from a thread came from, for the
// link at the top of its transcript.
type threadOrigin struct {
	ThreadID    string `json:"thread_id"`
	ParentID    string `json:"parent_id"`
	ParentTitle string `json:"parent_title"`
	Quote       string `json:"quote"`
}

// createThread opens a thread on a message of a conversation and sends its
// first question.
func (s *Server) createThread(c *fiber.Ctx) error {
	parent, err := s.store.GetSession(c.Params("id"))
	if err != nil {
		return err
	}
	if parent.ParentID != "" {
		return badRequest("a thread cannot have threads of its own; open it as a task first")
	}
	var body struct {
		MessageID int64  `json:"message_id"`
		Quote     string `json:"quote"`
		Prompt    string `json:"prompt"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	body.Quote, body.Prompt = strings.TrimSpace(body.Quote), strings.TrimSpace(body.Prompt)
	if body.Quote == "" || body.Prompt == "" {
		return badRequest("a thread needs a passage and a question")
	}
	if message, err := s.store.GetMessage(body.MessageID); err != nil || message.SessionID != parent.ID {
		return badRequest("that message is not in this conversation")
	}
	thread := &store.Session{ID: uuid.NewString(), ProjectID: parent.ProjectID,
		Title: clipText(firstLine(body.Prompt), 120), Provider: parent.Provider,
		AccountID: parent.AccountID, Model: parent.Model, Effort: parent.Effort,
		Permission: parent.Permission, ParentID: parent.ID,
		AnchorMessageID: body.MessageID, AnchorQuote: clipText(body.Quote, maxQuote)}
	if err := s.store.CreateSession(thread); err != nil {
		return err
	}
	turn, err := s.runner.Send(thread.ID, body.Prompt)
	if err != nil {
		_ = s.store.DeleteSession(thread.ID)
		return err
	}
	s.sessionQueueChanged(parent.ID)
	full, err := s.store.GetSession(thread.ID)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"thread": full, "turn": turn})
}

// spawnThread opens a thread as a task of its own. The task shows the
// thread's messages and its first turn starts from a copy of the thread, so
// it knows everything the thread did; the thread stays where it was.
func (s *Server) spawnThread(c *fiber.Ctx) error {
	thread, err := s.store.GetSession(c.Params("id"))
	if err != nil {
		return err
	}
	if thread.ParentID == "" {
		return badRequest("only a thread can be opened as a task")
	}
	if s.runner.Running(thread.ID) {
		return badRequest("wait for the thread's answer before opening it as a task")
	}
	task := &store.Session{ID: uuid.NewString(), ProjectID: thread.ProjectID, Title: thread.Title,
		Provider: thread.Provider, AccountID: thread.AccountID, Model: thread.Model,
		Effort: thread.Effort, Permission: thread.Permission, OriginID: thread.ID}
	if err := s.store.CreateSession(task); err != nil {
		return err
	}
	if err := s.store.CopyMessages(thread.ID, task.ID); err != nil {
		_ = s.store.DeleteSession(task.ID)
		return err
	}
	full, err := s.store.GetSession(task.ID)
	if err != nil {
		return err
	}
	s.sessionChanged(task.ID, full)
	return c.Status(fiber.StatusCreated).JSON(full)
}

// origin describes the thread a spawned task came from, nil when it is gone.
func (s *Server) origin(sess *store.Session) *threadOrigin {
	if sess.OriginID == "" {
		return nil
	}
	thread, err := s.store.GetSession(sess.OriginID)
	if err != nil {
		return nil
	}
	o := &threadOrigin{ThreadID: thread.ID, ParentID: thread.ParentID, Quote: thread.AnchorQuote}
	if parent, err := s.store.GetSession(thread.ParentID); err == nil {
		o.ParentTitle = parent.Title
	}
	return o
}

// stopThreads ends the turns of threads about to be deleted with their
// messages or their conversation.
func (s *Server) stopThreads(ids []string) {
	for _, id := range ids {
		s.runner.Stop(id)
	}
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}

func clipText(text string, max int) string {
	if len(text) <= max {
		return text
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}
