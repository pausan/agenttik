package server

// The flat view is the virtual profile "all": every profile on this machine
// and on each saved remote, shown as one. Lists and the event stream are read
// from every source and merged; anything naming a project, task, job or
// terminal goes to the source that owns it. See specs/100-flat-view.md.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"hash/fnv"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

const (
	flatProfile = "all"
	// Project and job IDs carry their source in the low bits, so the same
	// number on two machines stays two numbers here.
	flatBits    = 16
	flatMask    = 1<<flatBits - 1
	flatTimeout = 4 * time.Second
	// A remote that did not answer is skipped for this long, so one machine
	// that is off does not slow every list.
	flatDown = 20 * time.Second
)

// flatSource is one profile the flat view reads: this machine's when Remote
// is empty.
type flatSource struct {
	Remote  string
	Profile string
	Machine string
	Name    string
	code    int64
}

func (f flatSource) key() string { return f.Remote + "/" + f.Profile }
func (f flatSource) home() bool  { return f.Remote == "" && f.Profile == "default" }

func (f flatSource) encode(id int64) int64 {
	if id <= 0 {
		return id
	}
	return id<<flatBits | f.code
}

// flatState is what the flat view has learned about where things live.
type flatState struct {
	mu        sync.Mutex
	sessions  map[string]string    // task or thread ID -> source key
	terminals map[string]string    // terminal ID -> source key
	copied    map[string]bool      // source key + attachment name, already uploaded
	down      map[string]time.Time // remote ID -> when to try it again
}

func (f *flatState) remember(m *map[string]string, id, key string) {
	if id == "" {
		return
	}
	f.mu.Lock()
	if *m == nil {
		*m = map[string]string{}
	}
	(*m)[id] = key
	f.mu.Unlock()
}

func (f *flatState) lookup(m map[string]string, id string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return m[id]
}

func (f *flatState) isDown(remote string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return time.Now().Before(f.down[remote])
}

func (f *flatState) markDown(remote string, down bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down == nil {
		f.down = map[string]time.Time{}
	}
	if down {
		f.down[remote] = time.Now().Add(flatDown)
	} else {
		delete(f.down, remote)
	}
}

// flatSources lists this machine's Default first, then its other profiles in
// their saved order, then each saved remote's profiles as last listed.
func (s *Server) flatSources() []flatSource {
	machine, _ := s.store.ServerName()
	var list []flatSource
	if m := s.profiles; m != nil {
		m.mu.RLock()
		for _, p := range m.profiles {
			list = append(list, flatSource{Profile: p.ID, Name: p.Name, Machine: machine})
		}
		m.mu.RUnlock()
	} else {
		list = append(list, flatSource{Profile: "default", Name: "Default", Machine: machine})
	}
	if m := s.remotes; m != nil {
		m.mu.Lock()
		var remotes []savedRemote
		if m.load() == nil {
			remotes = append(remotes, m.remotes...)
		}
		m.mu.Unlock()
		for _, r := range remotes {
			name := r.Name
			if name == "" {
				name = r.Address
			}
			for _, p := range r.Profiles {
				list = append(list, flatSource{Remote: r.ID, Profile: p.ID, Name: p.Name, Machine: name})
			}
		}
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].home() && !list[j].home() })
	assignFlatCodes(list)
	return list
}

// assignFlatCodes derives each source's code from its identity, so a code
// survives restarts and other sources coming and going. A rare collision
// takes the next free code, in key order.
func assignFlatCodes(list []flatSource) {
	order := make([]int, len(list))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return list[order[a]].key() < list[order[b]].key() })
	taken := map[int64]bool{}
	for _, i := range order {
		h := fnv.New32a()
		h.Write([]byte(list[i].key()))
		code := int64(h.Sum32()) & flatMask
		for taken[code] {
			code = (code + 1) & flatMask
		}
		taken[code] = true
		list[i].code = code
	}
}

func flatByCode(sources []flatSource, code int64) (flatSource, bool) {
	for _, src := range sources {
		if src.code == code {
			return src, true
		}
	}
	return flatSource{}, false
}

func flatByKey(sources []flatSource, key string) (flatSource, bool) {
	for _, src := range sources {
		if src.key() == key {
			return src, true
		}
	}
	return flatSource{}, false
}

func flatHome(sources []flatSource) flatSource {
	for _, src := range sources {
		if src.home() {
			return src
		}
	}
	return sources[0]
}

var errFlatGone = fiber.NewError(404, "That project's profile or machine is not available")

// flatDecode splits a flat ID into its source and the source's own ID.
func flatDecode(sources []flatSource, raw string) (flatSource, int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return flatSource{}, 0, badRequest("invalid id %q", raw)
	}
	src, ok := flatByCode(sources, id&flatMask)
	if !ok {
		return flatSource{}, 0, errFlatGone
	}
	return src, id >> flatBits, nil
}

// FlatProjectID is the orchestrator's own project as the flat view numbers
// it, and false when there is nothing to flatten: one profile, no remotes.
func (s *Server) FlatProjectID(id int64) (int64, bool) {
	sources := s.flatSources()
	if len(sources) < 2 {
		return 0, false
	}
	return flatHome(sources).encode(id), true
}

// flatRequest is an API request on its way to one source.
type flatRequest struct {
	method string
	path   string
	query  url.Values
	body   []byte
	header http.Header
}

func newFlatRequest(c *fiber.Ctx) flatRequest {
	query := url.Values{}
	c.Request().URI().QueryArgs().VisitAll(func(k, v []byte) {
		if key := string(k); key != "profile" && key != "remote" {
			query.Add(key, string(v))
		}
	})
	header := http.Header{}
	for _, name := range forwardedHeaders {
		if v := c.Get(name); v != "" {
			header.Set(name, v)
		}
	}
	return flatRequest{method: c.Method(), path: c.Path(), query: query, body: append([]byte(nil), c.Body()...), header: header}
}

// routeFlat serves `?profile=all`. Only this instance's own server has one:
// added profiles have no remotes and never see it.
func (s *Server) routeFlat(c *fiber.Ctx) error {
	if s.remotes == nil || c.Query("profile") != flatProfile || c.Query("remote") != "" || !strings.HasPrefix(c.Path(), "/api/") {
		return c.Next()
	}
	if s.profiles != nil {
		c.Set("X-Agenttik-Instance", s.profiles.instance)
	}
	sources := s.flatSources()
	req := newFlatRequest(c)
	parts := strings.Split(strings.TrimPrefix(req.path, "/api/"), "/")
	switch {
	case req.path == "/api/stream":
		return s.flatStream(c, sources, req)
	case req.method == "GET" && req.path == "/api/projects":
		return s.flatProjects(c, sources, req)
	case req.method == "POST" && req.path == "/api/projects/order":
		return s.flatProjectOrder(c, sources, req)
	case req.method == "GET" && (req.path == "/api/sessions" || req.path == "/api/schedules") && req.query.Get("project_id") == "":
		return s.flatList(c, sources, req, parts[0])
	case req.method == "GET" && req.path == "/api/approvals":
		return s.flatApprovals(c, sources, req)
	case req.method == "GET" && len(parts) == 2 && parts[0] == "attachments":
		return s.flatAttachment(c, sources, req)
	}
	src, err := s.flatLocate(sources, &req, parts)
	if err != nil {
		return err
	}
	if !src.home() {
		if err := s.flatCopyAttachments(src, req.body); err != nil {
			return err
		}
	}
	res, err := s.flatDo(c.Context(), src, req)
	if err != nil {
		return fiber.NewError(502, "Could not reach "+src.Machine)
	}
	return s.flatReply(c, res, func(data []byte) []byte {
		return s.flatRewrite(data, src, flatShapeOf(req.method, parts))
	})
}

// flatLocate finds the source a request belongs to and rewrites its IDs into
// that source's own. Requests naming nothing go to this machine's Default.
func (s *Server) flatLocate(sources []flatSource, req *flatRequest, parts []string) (flatSource, error) {
	home := flatHome(sources)
	if len(parts) >= 2 {
		switch parts[0] {
		case "projects", "schedules":
			if _, err := strconv.ParseInt(parts[1], 10, 64); err != nil {
				break
			}
			src, id, err := flatDecode(sources, parts[1])
			if err != nil {
				return src, err
			}
			parts[1] = strconv.FormatInt(id, 10)
			req.path = "/api/" + strings.Join(parts, "/")
			// A job order names the project's jobs, all on the same source.
			if len(parts) == 4 && parts[2] == "schedules" && parts[3] == "order" {
				body, err := flatDecodeIDs(sources, src, req.body)
				if err != nil {
					return src, err
				}
				req.body = body
			}
			return src, nil
		case "sessions":
			return s.flatSession(sources, parts[1])
		case "terminals":
			return s.flatTerminal(sources, parts[1])
		case "stream":
			if parts[1] == "terminals" {
				ids := splitList(req.query.Get("ids"))
				if len(ids) == 0 {
					return home, nil
				}
				return s.flatTerminal(sources, ids[0])
			}
		}
	}
	if raw := req.query.Get("project_id"); raw != "" && raw != "0" {
		src, id, err := flatDecode(sources, raw)
		if err != nil {
			return src, err
		}
		req.query.Set("project_id", strconv.FormatInt(id, 10))
		return src, nil
	}
	if len(req.body) > 0 && bytes.Contains(req.body, []byte(`"project_id"`)) {
		var body map[string]json.RawMessage
		if json.Unmarshal(req.body, &body) == nil {
			var raw json.Number
			if json.Unmarshal(body["project_id"], &raw) == nil && raw != "" && raw != "0" {
				src, id, err := flatDecode(sources, raw.String())
				if err != nil {
					return src, err
				}
				body["project_id"], _ = json.Marshal(id)
				req.body, _ = json.Marshal(body)
				return src, nil
			}
		}
	}
	return home, nil
}

// flatDecodeIDs rewrites a reorder body's IDs, which must all belong to src.
func flatDecodeIDs(sources []flatSource, src flatSource, data []byte) ([]byte, error) {
	var body struct {
		IDs []json.Number `json:"ids"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return data, nil // the source reports it
	}
	ids := make([]int64, 0, len(body.IDs))
	for _, raw := range body.IDs {
		owner, id, err := flatDecode(sources, raw.String())
		if err != nil {
			return nil, err
		}
		if owner.key() != src.key() {
			return nil, badRequest("ids belong to different profiles")
		}
		ids = append(ids, id)
	}
	return json.Marshal(map[string]any{"ids": ids})
}

// flatSession finds a task's source: learned from any list or event that
// named it, or else asked of every source.
func (s *Server) flatSession(sources []flatSource, id string) (flatSource, error) {
	if src, ok := flatByKey(sources, s.flat.lookup(s.flat.sessions, id)); ok {
		return src, nil
	}
	src, ok := s.flatFind(sources, "/api/sessions/"+url.PathEscape(id))
	if !ok {
		return src, fiber.NewError(404, "Task not found")
	}
	s.flat.remember(&s.flat.sessions, id, src.key())
	return src, nil
}

func (s *Server) flatTerminal(sources []flatSource, id string) (flatSource, error) {
	if src, ok := flatByKey(sources, s.flat.lookup(s.flat.terminals, id)); ok {
		return src, nil
	}
	return flatSource{}, fiber.NewError(404, "Terminal not found")
}

// flatFind asks every source at once for path and answers the first that has
// it, preferring this machine's order on ties.
func (s *Server) flatFind(sources []flatSource, path string) (flatSource, bool) {
	found := make([]bool, len(sources))
	var wg sync.WaitGroup
	for i, src := range sources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), flatTimeout)
			defer cancel()
			res, err := s.flatDo(ctx, src, flatRequest{method: "GET", path: path, header: http.Header{"Accept": {"application/json"}}})
			if err != nil {
				return
			}
			io.Copy(io.Discard, res.Body) //nolint:errcheck // only the status matters
			res.Body.Close()
			found[i] = res.StatusCode == http.StatusOK
		}()
	}
	wg.Wait()
	for i, ok := range found {
		if ok {
			return sources[i], true
		}
	}
	return flatSource{}, false
}

// flatDo sends one request to a source: in process for this machine's
// profiles, and with the saved session for a remote.
func (s *Server) flatDo(ctx context.Context, src flatSource, req flatRequest) (*http.Response, error) {
	query := url.Values{}
	for k, v := range req.query {
		query[k] = v
	}
	if src.Profile != "default" {
		query.Set("profile", src.Profile)
	}
	target := req.path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	if src.Remote == "" {
		return s.flatLocal(req.method, target, req.body, req.header), nil
	}
	if s.flat.isDown(src.Remote) {
		return nil, errors.New(src.Machine + " is not answering")
	}
	r, err := s.remotes.get(src.Remote)
	if err != nil {
		return nil, err
	}
	var body io.Reader
	if len(req.body) > 0 {
		body = bytes.NewReader(req.body)
	}
	res, err := s.remotes.request(ctx, r, req.method, target, body, req.header)
	if err != nil {
		if ctx.Err() == nil {
			s.flat.markDown(src.Remote, true)
		}
		return nil, err
	}
	s.flat.markDown(src.Remote, false)
	// The flat view cannot open one machine's sign-in dialog for another.
	if res.StatusCode == http.StatusUnauthorized {
		res.Body.Close()
		return &http.Response{StatusCode: http.StatusServiceUnavailable, ContentLength: -1,
			Header: http.Header{"Content-Type": {"application/json"}},
			Body:   io.NopCloser(strings.NewReader(`{"error":` + strconv.Quote("Sign in to "+src.Machine+" in Settings → Profiles") + `}`))}, nil
	}
	return res, nil
}

// flatLocal runs a request through this app in process, as routeProfile hands
// a request to an added profile. Streams stay streams.
func (s *Server) flatLocal(method, target string, body []byte, header http.Header) *http.Response {
	var req fasthttp.Request
	req.Header.SetMethod(method)
	req.SetRequestURI(target)
	req.Header.SetHost("localhost")
	for name, values := range header {
		for _, v := range values {
			req.Header.Add(name, v)
		}
	}
	if len(body) > 0 {
		req.SetBody(body)
	}
	ctx := new(fasthttp.RequestCtx)
	ctx.Init(&req, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}, nil)
	s.app.Handler()(ctx)
	res := &http.Response{StatusCode: ctx.Response.StatusCode(), Header: http.Header{}}
	ctx.Response.Header.VisitAll(func(k, v []byte) { res.Header.Add(string(k), string(v)) })
	res.Header.Set("Content-Type", string(ctx.Response.Header.ContentType()))
	if ctx.Response.IsBodyStream() {
		res.ContentLength = -1
		stream := ctx.Response.BodyStream()
		if rc, ok := stream.(io.ReadCloser); ok {
			res.Body = rc
		} else {
			res.Body = io.NopCloser(stream)
		}
		return res
	}
	data := append([]byte(nil), ctx.Response.Body()...)
	res.ContentLength = int64(len(data))
	res.Body = io.NopCloser(bytes.NewReader(data))
	return res
}

// flatReply hands a source's answer to the window, its JSON rewritten.
func (s *Server) flatReply(c *fiber.Ctx, res *http.Response, rewrite func([]byte) []byte) error {
	for name, values := range res.Header {
		switch http.CanonicalHeaderKey(name) {
		case "Set-Cookie", "Connection", "Transfer-Encoding", "Content-Length", "Content-Encoding", "X-Agenttik-Instance", "X-Agenttik-Remote", "Location":
			continue
		}
		for _, v := range values {
			c.Response().Header.Add(name, v)
		}
	}
	c.Status(res.StatusCode)
	kind := res.Header.Get("Content-Type")
	if strings.HasPrefix(kind, "text/event-stream") {
		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			defer res.Body.Close()
			buf := make([]byte, 32<<10)
			for {
				n, err := res.Body.Read(buf)
				if n > 0 {
					if _, werr := w.Write(buf[:n]); werr != nil || w.Flush() != nil {
						return
					}
				}
				if err != nil {
					return
				}
			}
		})
		return nil
	}
	if rewrite != nil && strings.Contains(kind, "json") {
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		if err != nil {
			return fiber.NewError(502, "The answer was cut short")
		}
		return c.Send(rewrite(data))
	}
	c.Response().SetBodyStream(res.Body, int(res.ContentLength))
	return nil
}

type flatShape int

const (
	flatPlain flatShape = iota
	flatProjectsShape
	flatSchedulesShape
	flatTerminalsShape
)

// flatShapeOf says what the top-level `id` of an answer is, the only field
// whose meaning depends on the route.
func flatShapeOf(method string, parts []string) flatShape {
	switch {
	case parts[0] == "projects" && len(parts) <= 2:
		return flatProjectsShape
	case parts[0] == "projects" && len(parts) == 3 && parts[2] == "terminals":
		return flatTerminalsShape
	case parts[0] == "schedules" && (len(parts) <= 2 || (len(parts) == 3 && parts[2] == "run" && method == "POST")):
		return flatSchedulesShape
	}
	return flatPlain
}

// flatRewrite numbers a source's answer the flat way and learns which tasks
// and terminals live there.
func (s *Server) flatRewrite(data []byte, src flatSource, shape flatShape) []byte {
	v, ok := flatParse(data)
	if !ok {
		return data
	}
	out, err := json.Marshal(s.flatWalk(v, src, shape, true))
	if err != nil {
		return data
	}
	return out
}

func flatParse(data []byte) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return nil, false
	}
	return v, true
}

func (s *Server) flatWalk(v any, src flatSource, shape flatShape, top bool) any {
	switch x := v.(type) {
	case []any:
		for i := range x {
			x[i] = s.flatWalk(x[i], src, shape, top)
		}
	case map[string]any:
		for k, val := range x {
			switch k {
			case "project_id", "schedule_id":
				x[k] = flatEncodeValue(src, val)
				continue
			case "session_id", "parent_id", "origin_id", "thread_id":
				s.flatLearn(src, val)
				continue
			case "id":
				switch {
				case top && shape == flatTerminalsShape:
					if id, ok := val.(string); ok {
						s.flat.remember(&s.flat.terminals, id, src.key())
					}
				case top && shape != flatPlain:
					x[k] = flatEncodeValue(src, val)
				default:
					s.flatLearn(src, val)
				}
				continue
			case "schedules":
				if top && shape == flatProjectsShape {
					x[k] = s.flatWalk(val, src, flatSchedulesShape, true)
					continue
				}
			}
			x[k] = s.flatWalk(val, src, flatPlain, false)
		}
		if top && shape == flatProjectsShape {
			x["source"] = map[string]any{"machine": src.Machine, "profile": src.Name, "remote": src.Remote != ""}
		}
	}
	return v
}

func flatEncodeValue(src flatSource, v any) any {
	n, ok := v.(json.Number)
	if !ok {
		return v
	}
	id, err := n.Int64()
	if err != nil {
		return v
	}
	return src.encode(id)
}

func (s *Server) flatLearn(src flatSource, v any) {
	if id, ok := v.(string); ok && len(id) == 36 {
		if _, err := uuid.Parse(id); err == nil {
			s.flat.remember(&s.flat.sessions, id, src.key())
		}
	}
}

// flatResult is one source's answer to a request asked of all of them.
type flatResult struct {
	src  flatSource
	data []byte
}

// flatAll asks every source and keeps the JSON answers that came back 200,
// in source order. Only this machine's Default failing fails the request.
func (s *Server) flatAll(sources []flatSource, req flatRequest) ([]flatResult, error) {
	results := make([]flatResult, len(sources))
	errs := make([]error, len(sources))
	var wg sync.WaitGroup
	for i, src := range sources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), flatTimeout)
			defer cancel()
			res, err := s.flatDo(ctx, src, req)
			if err != nil {
				errs[i] = err
				return
			}
			defer res.Body.Close()
			data, err := io.ReadAll(res.Body)
			if err != nil || res.StatusCode != http.StatusOK {
				errs[i] = fiber.NewError(res.StatusCode, flatError(data))
				return
			}
			results[i] = flatResult{src: src, data: data}
		}()
	}
	wg.Wait()
	out := results[:0]
	for i, r := range results {
		if sources[i].home() && errs[i] != nil {
			return nil, errs[i]
		}
		if errs[i] == nil {
			out = append(out, r)
		}
	}
	return out, nil
}

func flatError(data []byte) string {
	var body struct{ Error string }
	if json.Unmarshal(data, &body) == nil && body.Error != "" {
		return body.Error
	}
	return "request failed"
}

// flatItems parses every answer as a list and rewrites each item. keep, when
// given, drops the items it answers false for.
func (s *Server) flatItems(results []flatResult, shape flatShape, keep func(flatSource, map[string]any) bool) []map[string]any {
	var items []map[string]any
	for _, r := range results {
		v, ok := flatParse(r.data)
		if !ok {
			continue
		}
		list, _ := s.flatWalk(v, r.src, shape, true).([]any)
		for _, item := range list {
			if m, ok := item.(map[string]any); ok && (keep == nil || keep(r.src, m)) {
				items = append(items, m)
			}
		}
	}
	if items == nil {
		items = []map[string]any{}
	}
	return items
}

// flatProjects lists every source's projects, this machine's Default first.
// Each profile has its own orchestrator; the flat view keeps Default's,
// which is the one that works across all of them.
func (s *Server) flatProjects(c *fiber.Ctx, sources []flatSource, req flatRequest) error {
	results, err := s.flatAll(sources, req)
	if err != nil {
		return err
	}
	return c.JSON(s.flatItems(results, flatProjectsShape, func(src flatSource, p map[string]any) bool {
		return src.home() || p["kind"] != "orchestrator"
	}))
}

// flatProjectOrder saves each source's part of a dragged order. Projects
// only move among their own source's.
func (s *Server) flatProjectOrder(c *fiber.Ctx, sources []flatSource, req flatRequest) error {
	var body struct {
		IDs []json.Number `json:"ids"`
	}
	if err := json.Unmarshal(req.body, &body); err != nil || len(body.IDs) == 0 {
		return badRequest("ids are required")
	}
	groups := map[string][]int64{}
	var order []flatSource
	for _, raw := range body.IDs {
		src, id, err := flatDecode(sources, raw.String())
		if err != nil {
			return err
		}
		if groups[src.key()] == nil {
			order = append(order, src)
		}
		groups[src.key()] = append(groups[src.key()], id)
	}
	for _, src := range order {
		data, _ := json.Marshal(map[string]any{"ids": groups[src.key()]})
		ctx, cancel := context.WithTimeout(context.Background(), flatTimeout)
		res, err := s.flatDo(ctx, src, flatRequest{method: "POST", path: req.path, body: data, header: http.Header{"Content-Type": {"application/json"}}})
		cancel()
		if err != nil {
			return fiber.NewError(502, "Could not reach "+src.Machine)
		}
		res.Body.Close()
		if res.StatusCode >= 300 {
			return fiber.NewError(res.StatusCode, "Could not save the order on "+src.Machine)
		}
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// flatList merges the task or job lists of every source, ordered as one
// source would order them, and pages the merged list.
func (s *Server) flatList(c *fiber.Ctx, sources []flatSource, req flatRequest, kind string) error {
	limit, offset := 200, 0
	if v := req.query.Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	if v := req.query.Get("offset"); v != "" {
		offset, _ = strconv.Atoi(v)
	}
	if offset < 0 || (offset > 0 && limit <= 0) {
		return badRequest("offset requires a positive limit and must not be negative")
	}
	if offset > 0 {
		req.query.Set("limit", strconv.Itoa(offset+limit))
		req.query.Del("offset")
	}
	results, err := s.flatAll(sources, req)
	if err != nil {
		return err
	}
	shape := flatPlain
	if kind == "schedules" {
		shape = flatSchedulesShape
	}
	items := s.flatItems(results, shape, nil)
	if kind == "sessions" {
		sortFlatSessions(items, req.query)
	} else {
		sortFlatSchedules(items)
	}
	if offset > len(items) {
		offset = len(items)
	}
	items = items[offset:]
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return c.JSON(items)
}

func flatNum(m map[string]any, key string) int64 {
	n, _ := m[key].(json.Number)
	v, _ := n.Int64()
	return v
}

// sortFlatSessions follows store.ListSessions for lists not scoped to a project.
func sortFlatSessions(items []map[string]any, query url.Values) {
	newest := query.Get("newest") == "true" || query.Get("newest") == "1"
	onlyDone := query.Get("only_done") == "true" || query.Get("only_done") == "1"
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		switch {
		case newest:
			return flatNum(a, "created_at") > flatNum(b, "created_at")
		case onlyDone:
			af, bf := a["flag"] != "" && a["flag"] != nil, b["flag"] != "" && b["flag"] != nil
			if af != bf {
				return af
			}
		default:
			ar, br := a["status"] == "running", b["status"] == "running"
			if ar != br {
				return ar
			}
		}
		return flatNum(a, "last_active_at") > flatNum(b, "last_active_at")
	})
}

// sortFlatSchedules follows store.ListSchedules for lists not scoped to a
// project: pinned prompts, then the next to run, then those with none.
func sortFlatSchedules(items []map[string]any) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		ap, bp := a["every"] == "pinned", b["every"] == "pinned"
		if ap != bp {
			return ap
		}
		an, bn := flatNum(a, "next_run_at"), flatNum(b, "next_run_at")
		if (an == 0) != (bn == 0) {
			return bn == 0
		}
		if an != bn {
			return an < bn
		}
		return flatNum(a, "created_at") > flatNum(b, "created_at")
	})
}

func (s *Server) flatApprovals(c *fiber.Ctx, sources []flatSource, req flatRequest) error {
	results, err := s.flatAll(sources, req)
	if err != nil {
		return err
	}
	return c.JSON(s.flatItems(results, flatPlain, nil))
}

// flatAttachment serves an image from whichever source holds it. Names are
// content hashes, so any copy is the same image.
func (s *Server) flatAttachment(c *fiber.Ctx, sources []flatSource, req flatRequest) error {
	for _, src := range sources {
		ctx, cancel := context.WithTimeout(context.Background(), flatTimeout)
		res, err := s.flatDo(ctx, src, req)
		if err != nil {
			cancel()
			continue
		}
		if res.StatusCode == http.StatusOK {
			// The body is read after this returns; the deadline goes with it.
			res.Body = cancelOnClose{res.Body, cancel}
			return s.flatReply(c, res, nil)
		}
		res.Body.Close()
		cancel()
	}
	return fiber.ErrNotFound
}

var flatAttachmentRef = regexp.MustCompile(`/api/attachments/([0-9a-f]{64}\.(?:png|jpg|gif|webp))`)

// flatCopyAttachments uploads the images a request mentions to the source it
// goes to. The window uploads to this machine, but an agent elsewhere reads
// images from its own data directory.
func (s *Server) flatCopyAttachments(src flatSource, body []byte) error {
	for _, m := range flatAttachmentRef.FindAllSubmatch(body, -1) {
		name := string(m[1])
		key := src.key() + "/" + name
		s.flat.mu.Lock()
		done := s.flat.copied[key]
		s.flat.mu.Unlock()
		if done {
			continue
		}
		res := s.flatLocal("GET", "/api/attachments/"+name, nil, nil)
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			continue // not this machine's; the source may already have it
		}
		ctx, cancel := context.WithTimeout(context.Background(), flatTimeout)
		up, err := s.flatDo(ctx, src, flatRequest{method: "POST", path: "/api/attachments", body: data, header: http.Header{"Content-Type": {"application/octet-stream"}}})
		cancel()
		if err != nil {
			return fiber.NewError(502, "Could not send the image to "+src.Machine)
		}
		up.Body.Close()
		if up.StatusCode != http.StatusOK {
			return fiber.NewError(up.StatusCode, "Could not send the image to "+src.Machine)
		}
		s.flat.mu.Lock()
		if s.flat.copied == nil {
			s.flat.copied = map[string]bool{}
		}
		s.flat.copied[key] = true
		s.flat.mu.Unlock()
	}
	return nil
}
