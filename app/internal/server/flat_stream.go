package server

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// flatRefresh tells the window a source came back, so it reads again what
// that source changed while it was away.
const flatRefresh = `{"session_id":"","event":{"type":"projects_changed"}}`

// flatStream merges every source's event stream into one, as the window
// holds a single stream however many profiles it shows. The topics are split
// by owner; a task not yet placed is asked of every source, where unknown IDs
// never fire.
func (s *Server) flatStream(c *fiber.Ctx, sources []flatSource, req flatRequest) error {
	queries := map[string]url.Values{}
	for _, src := range sources {
		queries[src.key()] = url.Values{}
	}
	add := func(key, param, value string) {
		if q := queries[key]; q != nil {
			if old := q.Get(param); old != "" {
				value = old + "," + value
			}
			q.Set(param, value)
		}
	}
	for _, id := range splitList(req.query.Get("sessions")) {
		if key := s.flat.lookup(s.flat.sessions, id); key != "" {
			add(key, "sessions", id)
			continue
		}
		for key := range queries {
			add(key, "sessions", id)
		}
	}
	for _, raw := range splitList(req.query.Get("projects")) {
		if src, id, err := flatDecode(sources, raw); err == nil {
			add(src.key(), "projects", strconv.FormatInt(id, 10))
		}
	}
	if raw := strings.TrimSpace(req.query.Get("watch")); raw != "" {
		if src, id, err := flatDecode(sources, raw); err == nil {
			queries[src.key()].Set("watch", strconv.FormatInt(id, 10))
		}
	}

	open := startStream(c)
	c.Context().SetBodyStreamWriter(func(bw *bufio.Writer) {
		w := open(bw)
		defer w.Close()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		events := make(chan string, maxBatch)
		for _, src := range sources {
			go s.flatFollow(ctx, src, queries[src.key()], events)
		}
		if _, err := w.WriteString(": open\n\n"); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}
		ticker := time.NewTicker(heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-s.closing:
				return
			case data := <-events:
				for n := 0; ; n++ {
					if _, err := w.WriteString("data: " + data + "\n\n"); err != nil {
						return
					}
					if n == maxBatch {
						break
					}
					select {
					case data = <-events:
						continue
					default:
					}
					break
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
	})
	return nil
}

// flatFollow keeps one source's stream open until ctx ends, reconnecting with
// a growing pause when it drops or does not answer.
func (s *Server) flatFollow(ctx context.Context, src flatSource, query url.Values, events chan<- string) {
	wait := time.Second
	missed := false
	for {
		res, err := s.flatDo(ctx, src, flatRequest{method: "GET", path: "/api/stream", query: query, header: http.Header{"Accept": {"text/event-stream"}}})
		if err == nil && res.StatusCode == http.StatusOK {
			if missed {
				select {
				case events <- flatRefresh:
				case <-ctx.Done():
					res.Body.Close()
					return
				}
			}
			wait = time.Second
			s.flatRead(ctx, src, res.Body, events)
		} else if err == nil {
			res.Body.Close()
		}
		missed = true
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, 30*time.Second)
	}
}

// flatRead hands on each event of a source's stream, numbered the flat way.
func (s *Server) flatRead(ctx context.Context, src flatSource, body io.ReadCloser, events chan<- string) {
	stop := context.AfterFunc(ctx, func() { body.Close() })
	defer stop()
	defer body.Close()
	r := bufio.NewReaderSize(body, 64<<10)
	for {
		line, err := r.ReadString('\n')
		if data, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), "data: "); ok {
			out := string(s.flatRewrite([]byte(data), src, flatPlain))
			select {
			case events <- out:
			case <-ctx.Done():
				return
			}
		}
		if err != nil {
			return
		}
	}
}
