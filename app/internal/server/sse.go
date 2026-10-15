package server

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/pausan/agenttik/app/internal/runner"
)

// heartbeat keeps proxies and idle connections from timing the stream out.
const heartbeat = 25 * time.Second

// streamAll serves everything the UI is watching over one connection:
// `?sessions=<id>,<id>&projects=<id>`. Sessions carry their own live events
// and a project carries the end of every turn run in it, from any number of
// sessions at once.
//
// The topics travel in the query rather than one connection per tab because a
// browser holds only a handful of connections to one origin, and a dozen open
// conversations would use them all up and stall the API. The UI reopens this
// stream when a tab opens or closes.
func (s *Server) streamAll(c *fiber.Ctx) error {
	// Subscribed unconditionally: a project added elsewhere is news to every
	// window, including one that has no project to name yet.
	topics := []string{runner.ProjectsTopic}
	for _, id := range splitList(c.Query("sessions")) {
		topics = append(topics, id)
	}
	for _, id := range splitList(c.Query("projects")) {
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return badRequest("invalid project id %q", id)
		}
		topics = append(topics, runner.ProjectTopic(n))
	}
	// `watch` is the project the window is showing. Its working tree is
	// followed for as long as this stream is open, so the tree and the changed
	// list keep up with an editor or a git command outside agenttik.
	release := func() {}
	if id := strings.TrimSpace(c.Query("watch")); id != "" {
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return badRequest("invalid project id %q", id)
		}
		topics = append(topics, runner.FilesTopic(n))
		release = s.watchProject(n)
	}
	// Unknown ids are not an error: a session deleted in another window would
	// otherwise break the whole stream. They simply never fire.
	return s.stream(c, topics, release)
}

// watchProject starts following a project folder, or does nothing at all when
// there is no folder to follow. A project whose path has gone missing still
// streams its sessions.
func (s *Server) watchProject(projectID int64) func() {
	nothing := func() {}
	p, err := s.store.GetProject(projectID)
	if err != nil {
		return nothing
	}
	root, err := filepath.EvalSymlinks(p.Path)
	if err != nil {
		return nothing
	}
	return s.watchers.acquire(projectID, root)
}

func splitList(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// maxBatch bounds how many queued events one flush carries.
const maxBatch = 256

// writeEvent writes one event, reporting false once the client has gone. An
// event that cannot be encoded is skipped.
func writeEvent(w *streamWriter, ev runner.Event) bool {
	payload, err := json.Marshal(ev)
	if err != nil {
		return true
	}
	_, err = w.WriteString("data: " + string(payload) + "\n\n")
	return err == nil
}

// startStream sets an event stream's headers, and returns what turns the
// response body into the writer its frames go to. The stream is gzipped when
// the client accepts it: event JSON and terminal output shrink several times,
// which is what a remote machine on a slow link waits on. Every Flush flushes
// the compressor as well, so no frame is held back waiting for more.
func startStream(c *fiber.Ctx) func(*bufio.Writer) *streamWriter {
	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Set("X-Accel-Buffering", "no")
	gz := strings.Contains(c.Get(fiber.HeaderAcceptEncoding), "gzip")
	if gz {
		c.Set(fiber.HeaderContentEncoding, "gzip")
	}
	return func(bw *bufio.Writer) *streamWriter {
		w := &streamWriter{bw: bw}
		if gz {
			w.gz, _ = gzip.NewWriterLevel(bw, gzip.BestSpeed)
		}
		return w
	}
}

type streamWriter struct {
	bw *bufio.Writer
	gz *gzip.Writer
}

func (w *streamWriter) WriteString(s string) (int, error) {
	if w.gz != nil {
		return io.WriteString(w.gz, s)
	}
	return w.bw.WriteString(s)
}

func (w *streamWriter) Flush() error {
	if w.gz != nil {
		if err := w.gz.Flush(); err != nil {
			return err
		}
	}
	return w.bw.Flush()
}

// Close ends the gzip stream, so a client reading the body to its end sees a
// complete one. The connection may already be gone; nothing more is owed.
func (w *streamWriter) Close() {
	if w.gz != nil {
		w.gz.Close()
		w.bw.Flush()
	}
}

// stream serves a set of hub topics as SSE until the client leaves or the app
// quits. release is run when it ends: the body is written after the handler
// has returned, so a deferred cleanup in the caller would fire while the
// stream was still open.
func (s *Server) stream(c *fiber.Ctx, topics []string, release func()) error {
	events, unsubscribe := s.runner.Hub().SubscribeMany(topics)

	open := startStream(c)
	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(bw *bufio.Writer) {
		w := open(bw)
		defer w.Close()
		defer unsubscribe()
		defer release()

		ticker := time.NewTicker(heartbeat)
		defer ticker.Stop()

		// Flush a comment straight away. Nothing is sent until the first
		// flush, so without this the browser sits without an open stream
		// until either an event or the first heartbeat arrives.
		if _, err := w.WriteString(": open\n\n"); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}

		for {
			select {
			case <-s.closing:
				// The app is quitting; let go of the connection so the
				// server can finish shutting down.
				return
			case ev, ok := <-events:
				// Events already queued behind this one go out under the
				// same flush: a turn publishes in bursts, and one gzip flush
				// per event would cost more than the event.
				for n := 0; ok; n++ {
					if !writeEvent(w, ev) {
						return
					}
					if n == maxBatch {
						break
					}
					select {
					case ev, ok = <-events:
						continue
					default:
					}
					break
				}
				if !ok {
					w.Flush() //nolint:errcheck // ending either way
					return
				}
			case <-ticker.C:
				if _, err := w.WriteString(": ping\n\n"); err != nil {
					return
				}
			}
			// A failed flush means the browser is gone.
			if err := w.Flush(); err != nil {
				return
			}
		}
	}))
	return nil
}
