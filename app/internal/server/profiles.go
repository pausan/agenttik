package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/agent/apiprovider"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/single"
	"github.com/pausan/agenttik/app/internal/store"
)

type Profile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type profileRuntime struct {
	server   *Server
	cancel   context.CancelFunc
	lock     *single.Lock
	clocks   sync.WaitGroup
	listener net.Listener
}

type profileManager struct {
	mu       sync.RWMutex
	root     *Server
	profiles []Profile
	running  map[string]*profileRuntime
	instance string
	private  bool
	closed   bool
}

// EnableProfiles preserves the original database as Default. Each additional
// profile owns a complete store and runner, including its schedule clocks.
func (s *Server) EnableProfiles(private bool) error {
	m := &profileManager{root: s, private: private, profiles: []Profile{{ID: "default", Name: "Default"}}, running: make(map[string]*profileRuntime)}
	m.instance = fmt.Sprintf("%x", sha256.Sum256([]byte(s.store.Dir())))
	data, err := os.ReadFile(m.path())
	if err == nil {
		if err = json.Unmarshal(data, &m.profiles); err != nil {
			return fmt.Errorf("read profiles: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	seen := map[string]bool{}
	for _, p := range m.profiles {
		if p.ID != "default" {
			if _, err := uuid.Parse(p.ID); err != nil {
				m.close()
				return fmt.Errorf("invalid profile ID %q", p.ID)
			}
		}
		if seen[p.ID] || strings.TrimSpace(p.Name) == "" {
			m.close()
			return errors.New("invalid profile catalog")
		}
		seen[p.ID] = true
		if p.ID == "default" {
			continue
		}
		rt, err := m.open(p.ID)
		if err != nil {
			m.close()
			return err
		}
		m.running[p.ID] = rt
	}
	if !seen["default"] {
		m.close()
		return errors.New("profile catalog has no Default profile")
	}
	s.profiles = m
	return nil
}

func (m *profileManager) path() string { return filepath.Join(m.root.store.Dir(), "profiles.json") }
func (m *profileManager) dir(id string) string {
	return filepath.Join(m.root.store.Dir(), "profiles", id)
}
func (m *profileManager) save(profiles []Profile) error {
	data, err := json.Marshal(profiles)
	if err != nil {
		return err
	}
	tmp := m.path() + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, m.path())
}
func (m *profileManager) open(id string) (*profileRuntime, error) {
	dir := m.dir(id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lock, err := single.Acquire(filepath.Join(dir, "agenttik.lock"))
	if err != nil {
		return nil, fmt.Errorf("open profile %s: %w", id, err)
	}
	ok := false
	defer func() {
		if !ok {
			lock.Release()
		}
	}()
	db, err := store.Open(filepath.Join(dir, "agenttik.db"))
	if err != nil {
		return nil, err
	}
	if err := db.ResetRunningSessions(); err != nil {
		db.Close()
		return nil, err
	}
	if err := db.ResetRunningScheduleRuns(); err != nil {
		db.Close()
		return nil, err
	}
	providers := make([]agent.Provider, 0, len(m.root.registry.All()))
	for _, provider := range m.root.registry.All() {
		if api, ok := provider.(*apiprovider.Provider); ok {
			providers = append(providers, api.ForProfile(dir))
		} else {
			providers = append(providers, provider)
		}
	}
	registry := agent.NewRegistry(providers...)
	r := runner.New(db, registry, runner.NewHub())
	s := New(db, registry, r)
	s.SetVersion(m.root.version)
	// CLI calls from this profile's orchestrator discover its own endpoint.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		s.Shutdown()
		db.Close()
		return nil, err
	}
	if err := lock.Publish(ln.Addr().String()); err != nil {
		ln.Close()
		s.Shutdown()
		db.Close()
		return nil, err
	}
	go func() {
		if err := s.Listener(ln); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("profile %s: %v", id, err)
		}
	}()
	ok = true
	ctx, cancel := context.WithCancel(context.Background())
	rt := &profileRuntime{server: s, cancel: cancel, lock: lock, listener: ln}
	rt.clocks.Add(2)
	go func() { defer rt.clocks.Done(); r.RunSchedules(ctx) }()
	go func() { defer rt.clocks.Done(); r.RunQueueRetries(ctx) }()
	return rt, nil
}
func (r *profileRuntime) close() {
	r.cancel()
	r.clocks.Wait()
	r.listener.Close()
	r.server.runner.Shutdown()
	r.server.Shutdown()
	r.server.store.Close()
	r.lock.Release()
}
func (m *profileManager) close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	for _, rt := range m.running {
		rt.close()
	}
	m.running = nil
}
func (s *Server) CloseProfiles() {
	if s.profiles != nil {
		s.profiles.close()
	}
}

// The profile travels on every API URL, including streams and image requests.
// No process-global selection or cookie can redirect another window's writes.
func (s *Server) routeProfile(c *fiber.Ctx) error {
	m := s.profiles
	if m == nil || !strings.HasPrefix(c.Path(), "/api/") {
		return c.Next()
	}
	id := c.Query("profile", "default")
	if m.private || id != "default" {
		c.Set("X-Agenttik-Instance", m.instance)
	} else {
		c.Set("X-Agenttik-Instance", "")
	}
	// Mutations take their own exclusive lock.
	if strings.HasPrefix(c.Path(), "/api/profiles") && c.Method() != "GET" {
		return c.Next()
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return fiber.NewError(503, "Instance is shutting down")
	}
	// Management and native shell settings belong to the instance.
	path := c.Path()
	if strings.HasPrefix(path, "/api/profiles") {
		return c.Next()
	}
	if id != "default" && m.running[id] == nil {
		return fiber.NewError(404, "Profile no longer exists")
	}
	if id == "default" || strings.HasPrefix(path, "/api/updates") || path == "/api/foreground" || path == "/api/desktop" || strings.HasPrefix(path, "/api/server") || strings.HasPrefix(path, "/api/remote/") || path == "/api/version" {
		return c.Next()
	}
	m.running[id].server.app.Handler()(c.Context())
	return nil
}

func (s *Server) listProfiles(c *fiber.Ctx) error {
	if s.profiles == nil {
		return c.JSON(fiber.Map{"profiles": []Profile{{ID: "default", Name: "Default"}}, "private": false})
	}
	// routeProfile already holds the read lock for this request.
	return c.JSON(fiber.Map{"profiles": s.profiles.profiles, "private": s.profiles.private})
}

func (s *Server) createProfile(c *fiber.Ctx) error {
	var body struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(400, "Invalid profile")
	}
	name, err := validateProfileName(body.Name)
	if err != nil {
		return err
	}
	m := s.profiles
	if m == nil {
		return fiber.NewError(503, "Profiles are unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fiber.NewError(503, "Instance is shutting down")
	}
	for _, p := range m.profiles {
		if strings.EqualFold(p.Name, name) {
			return fiber.NewError(409, "A profile with that name already exists")
		}
	}
	p := Profile{ID: uuid.NewString(), Name: name}
	rt, err := m.open(p.ID)
	if err != nil {
		return err
	}
	next := append(append([]Profile{}, m.profiles...), p)
	if err := m.save(next); err != nil {
		rt.close()
		os.RemoveAll(m.dir(p.ID))
		return err
	}
	m.profiles = next
	m.running[p.ID] = rt
	return c.Status(201).JSON(p)
}

func validateProfileName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || len(name) > 80 {
		return "", fiber.NewError(400, "Profile name must be 1–80 bytes")
	}
	return name, nil
}

func (s *Server) reorderProfiles(c *fiber.Ctx) error {
	m := s.profiles
	if m == nil {
		return fiber.NewError(503, "Profiles are unavailable")
	}
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(400, "Invalid profile order")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fiber.NewError(503, "Instance is shutting down")
	}
	if len(body.IDs) != len(m.profiles) {
		return fiber.NewError(400, "Order must include every profile exactly once")
	}
	byID := make(map[string]Profile, len(m.profiles))
	for _, p := range m.profiles {
		byID[p.ID] = p
	}
	next := make([]Profile, 0, len(m.profiles))
	for _, id := range body.IDs {
		p, ok := byID[id]
		if !ok {
			return fiber.NewError(400, "Order must include every profile exactly once")
		}
		next = append(next, p)
		delete(byID, id)
	}
	if err := m.save(next); err != nil {
		return err
	}
	m.profiles = next
	return c.SendStatus(204)
}

func (s *Server) renameProfile(c *fiber.Ctx) error {
	m := s.profiles
	if m == nil {
		return fiber.NewError(503, "Profiles are unavailable")
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(400, "Invalid profile")
	}
	name, err := validateProfileName(body.Name)
	if err != nil {
		return err
	}
	id := c.Params("id")
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fiber.NewError(503, "Instance is shutting down")
	}
	index := -1
	for i, p := range m.profiles {
		if p.ID == id {
			index = i
			continue
		}
		if strings.EqualFold(p.Name, name) {
			return fiber.NewError(409, "A profile with that name already exists")
		}
	}
	if index < 0 {
		return fiber.NewError(404, "Profile not found")
	}
	next := append([]Profile{}, m.profiles...)
	next[index].Name = name
	if err := m.save(next); err != nil {
		return err
	}
	m.profiles = next
	return c.JSON(next[index])
}

func (s *Server) deleteProfile(c *fiber.Ctx) error {
	m := s.profiles
	if m == nil {
		return fiber.NewError(503, "Profiles are unavailable")
	}
	id := c.Params("id")
	if id == "default" {
		return fiber.NewError(400, "The built-in Default profile cannot be removed")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rt := m.running[id]
	if rt == nil {
		return fiber.NewError(404, "Profile not found")
	}
	next := make([]Profile, 0, len(m.profiles)-1)
	for _, p := range m.profiles {
		if p.ID != id {
			next = append(next, p)
		}
	}
	if err := m.save(next); err != nil {
		return err
	}
	rt.close()
	delete(m.running, id)
	m.profiles = next
	if err := os.RemoveAll(m.dir(id)); err != nil {
		return err
	}
	return c.SendStatus(204)
}

// The destination is in the route and the source in the usual profile query.
// Management's exclusive lock excludes profile deletion and routed requests.
func (s *Server) moveProjectProfile(c *fiber.Ctx) error {
	m := s.profiles
	if m == nil {
		return fiber.NewError(503, "Profiles are unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fiber.NewError(503, "Instance is shutting down")
	}
	sourceID, destinationID := c.Query("profile", "default"), c.Params("id")
	if sourceID == destinationID {
		return badRequest("choose another profile")
	}
	resolve := func(id string) *Server {
		if id == "default" {
			return m.root
		}
		if rt := m.running[id]; rt != nil {
			return rt.server
		}
		return nil
	}
	source, destination := resolve(sourceID), resolve(destinationID)
	if source == nil || destination == nil {
		return fiber.NewError(404, "Profile no longer exists")
	}
	id, err := strconv.ParseInt(c.Params("projectID"), 10, 64)
	if err != nil || id <= 0 {
		return badRequest("invalid project id")
	}
	var movedID int64
	err = source.runner.WithIdleProject(id, func() error {
		var err error
		movedID, err = source.store.MoveProjectTo(destination.store, id)
		return err
	})
	if errors.Is(err, store.ErrProjectBusy) || errors.Is(err, store.ErrMoveOrchestrator) || errors.Is(err, runner.ErrBusy) {
		return fiber.NewError(409, err.Error())
	}
	if err != nil {
		return err
	}
	source.terminals.CloseProject(id)
	source.projectsChanged()
	destination.projectsChanged()
	return c.JSON(fiber.Map{"id": movedID, "profile_id": destinationID})
}
