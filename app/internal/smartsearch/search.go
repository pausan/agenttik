// Package smartsearch owns the shared, local semantic task index.
package smartsearch

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pausan/agenttik/app/internal/store"
)

type Status struct {
	Phase     string  `json:"phase"`
	Percent   float64 `json:"percent"`
	Completed int     `json:"completed"`
	Total     int     `json:"total"`
	Seconds   *int    `json:"seconds"`
	Error     string  `json:"error"`
	Version   uint64  `json:"version"`
}

type entry struct {
	Text   string    `json:"text"`
	Vector []float32 `json:"vector"`
}

type Service struct {
	mu           sync.Mutex
	status       Status
	busy, closed bool
	work         sync.Mutex // Serializes model initialization, indexing, queries and close.
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	dir          string
	tasks        func() ([]store.Session, error)
	load         func(context.Context, string, func(Status)) (embedder, error)
	model        embedder
	vectors      map[string]entry
	lastQuery    string
	lastVector   []float32
}

func New(dir string, tasks func() ([]store.Session, error)) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{dir: dir, tasks: tasks, load: loadModel, ctx: ctx, cancel: cancel,
		status: Status{Phase: "idle"}, vectors: make(map[string]entry)}
}

func (s *Service) Status() Status { s.mu.Lock(); defer s.mu.Unlock(); return s.status }

func (s *Service) progress(status Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status.Version = s.status.Version
	s.status = status
}

// Refresh coalesces concurrent browser requests into one indexing pass.
func (s *Service) Refresh() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.busy && !s.closed {
		s.busy = true
		s.status.Phase = "loading"
		s.status.Error = ""
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.work.Lock()
			err := s.index()
			s.work.Unlock()
			s.mu.Lock()
			defer s.mu.Unlock()
			s.busy = false
			if err != nil {
				s.status.Phase = "error"
				s.status.Error = err.Error()
			}
		}()
	}
	return s.status
}

func taskText(t store.Session) string {
	title := t.Title
	if title == "" {
		title = "Untitled task"
	}
	parts := []string{title}
	if t.Prompt != "" {
		parts = append(parts, t.Prompt)
	}
	if t.Summary != "" {
		parts = append(parts, t.Summary)
	}
	return strings.Join(parts, "\n")
}

func validVector(v []float32) bool {
	if len(v) != 384 {
		return false
	}
	for _, x := range v {
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return false
		}
	}
	return true
}

func (s *Service) embed(text string) ([]float32, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	v, err := s.model.EmbedDocuments([]string{text})
	if err != nil {
		return nil, err
	}
	if len(v) != 1 || !validVector(v[0]) {
		return nil, errors.New("Bekko returned an invalid embedding")
	}
	return v[0], nil
}

func (s *Service) cachePath(id string) string {
	return filepath.Join(s.dir, revision, "tasks", fmt.Sprintf("%x.json", sha256.Sum256([]byte(id))))
}

func (s *Service) index() error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if s.model == nil {
		m, err := s.load(s.ctx, s.dir, s.progress)
		if err != nil {
			return err
		}
		s.model = m
	}
	tasks, err := s.tasks()
	if err != nil {
		return err
	}
	next := make(map[string]entry, len(tasks))
	var pending []store.Session
	for _, t := range tasks {
		e, ok := s.vectors[t.ID]
		if !ok {
			data, _ := os.ReadFile(s.cachePath(t.ID))
			_ = json.Unmarshal(data, &e)
		}
		if e.Text == taskText(t) && validVector(e.Vector) {
			next[t.ID] = e
		} else {
			pending = append(pending, t)
		}
	}
	changed := len(pending) > 0 || len(next) != len(s.vectors)
	if !changed {
		for id := range next {
			if _, ok := s.vectors[id]; !ok {
				changed = true
				break
			}
		}
	}
	started := time.Now()
	s.progress(Status{Phase: "index", Total: len(pending)})
	for i, t := range pending {
		text := taskText(t)
		v, err := s.embed(text)
		if err != nil {
			return err
		}
		e := entry{Text: text, Vector: v}
		data, err := json.Marshal(e)
		if err != nil {
			return err
		}
		path := s.cachePath(t.ID)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(path+".tmp", data, 0600); err != nil {
			return err
		}
		if err := os.Rename(path+".tmp", path); err != nil {
			return err
		}
		next[t.ID] = e
		seconds := int(math.Ceil(time.Since(started).Seconds() / float64(i+1) * float64(len(pending)-i-1)))
		s.progress(Status{Phase: "index", Completed: i + 1, Total: len(pending), Seconds: &seconds,
			Percent: float64(i+1) / float64(len(pending)) * 100})
	}
	// Prune persisted entries too, including deletions while the app was stopped.
	paths, err := filepath.Glob(filepath.Join(s.dir, revision, "tasks", "*.json"))
	if err != nil {
		return err
	}
	keep := make(map[string]bool, len(next))
	for id := range next {
		keep[s.cachePath(id)] = true
	}
	for _, path := range paths {
		if !keep[path] {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}
	s.vectors = next
	s.mu.Lock()
	version := s.status.Version
	if changed {
		version++
	}
	zero := 0
	s.status = Status{Phase: "ready", Percent: 100, Completed: len(tasks), Total: len(tasks), Seconds: &zero, Version: version}
	s.mu.Unlock()
	return nil
}

func (s *Service) Search(query string, ids []string) (map[string]float64, error) {
	s.work.Lock()
	defer s.work.Unlock()
	if s.ctx.Err() != nil {
		return nil, errors.New("Smart Search stopped")
	}
	if s.model == nil || s.Status().Phase == "error" {
		return nil, errors.New("Smart Search is preparing")
	}
	if query != s.lastQuery || s.lastVector == nil {
		v, err := s.embed(query)
		if err != nil {
			return nil, err
		}
		s.lastQuery, s.lastVector = query, v
	}
	scores := make(map[string]float64, len(ids))
	for _, id := range ids {
		if e, ok := s.vectors[id]; ok {
			var score float64
			for i, v := range e.Vector {
				score += float64(v) * float64(s.lastVector[i])
			}
			scores[id] = score
		}
	}
	return scores, nil
}

func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	s.wg.Wait()
	s.work.Lock()
	defer s.work.Unlock()
	if s.model != nil {
		_ = s.model.Close()
		s.model = nil
	}
}
