package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"sync"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
)

const modelsTTL = 10 * time.Minute

type modelCache struct {
	mu     sync.Mutex
	asked  time.Time
	models []agent.Model
	// file keeps the catalog across launches; changed hears of a new answer.
	file    string
	changed func()
	// pending is the refresh in flight, which tests wait on.
	pending sync.WaitGroup
}

// RememberModels keeps the catalog in path, so a launch shows the previous
// session's models while the CLI is asked again, and calls changed when an
// answer differs from them. Call it before the first Models.
func (p *Provider) RememberModels(path string, changed func()) {
	c := &p.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	c.file, c.changed = path, changed
	agent.LoadRemembered(path, &c.models)
}

// Models uses the CLI's catalog, including model-specific reasoning levels.
// Asking costs a CLI start, so the answer is cached and refreshed in the
// background: the caller gets what is known now, the fallback list until the
// first answer arrives. The last good catalog outlives an outage, and a failed
// ask waits modelsTTL before the next one.
func (p *Provider) Models() []agent.Model {
	c := &p.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.asked) >= modelsTTL {
		c.asked = time.Now()
		c.pending.Add(1)
		go c.refresh()
	}
	if len(c.models) == 0 {
		return fallbackModels()
	}
	return c.models
}

func (c *modelCache) refresh() {
	defer c.pending.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	models, err := fetchModels(ctx)
	if err != nil || len(models) == 0 {
		return
	}
	c.mu.Lock()
	if reflect.DeepEqual(models, c.models) {
		c.mu.Unlock()
		return
	}
	c.models = models
	file, changed := c.file, c.changed
	c.mu.Unlock()
	if file != "" {
		agent.SaveRemembered(file, models)
	}
	if changed != nil {
		changed()
	}
}

type modelPage struct {
	Data []struct {
		Model                     string `json:"model"`
		DisplayName               string `json:"displayName"`
		Hidden                    bool   `json:"hidden"`
		SupportedReasoningEfforts []struct {
			Effort string `json:"reasoningEffort"`
		} `json:"supportedReasoningEfforts"`
	} `json:"data"`
	NextCursor string `json:"nextCursor"`
}

// Discovery initializes app-server but never starts a thread or model turn.
func fetchModels(ctx context.Context) ([]agent.Model, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, Binary, "app-server")
	cmd.Dir = os.TempDir()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	enc := json.NewEncoder(stdin)
	send := func(method string, id int, params any) error {
		return enc.Encode(map[string]any{"method": method, "id": id, "params": params})
	}
	if err := send("initialize", 0, map[string]any{
		"clientInfo": map[string]string{"name": "agenttik", "version": "0.1"},
	}); err != nil {
		return nil, err
	}
	requestPage := func(id int, cursor string) error {
		params := map[string]any{"limit": 100, "includeHidden": false}
		if cursor != "" {
			params["cursor"] = cursor
		}
		return send("model/list", id, params)
	}
	var models []agent.Model
	seenModels, seenCursors := map[string]bool{}, map[string]bool{}
	id := 0
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var reply appServerResponse
		if json.Unmarshal(scanner.Bytes(), &reply) != nil || reply.ID == nil || *reply.ID != id {
			continue
		}
		if reply.Error != nil {
			return nil, fmt.Errorf("list Codex models: %s", reply.Error.Message)
		}
		if id == 0 {
			if err := enc.Encode(map[string]any{"method": "initialized"}); err != nil {
				return nil, err
			}
			id++
			if err := requestPage(id, ""); err != nil {
				return nil, err
			}
			continue
		}
		var page modelPage
		if err := json.Unmarshal(reply.Result, &page); err != nil {
			return nil, err
		}
		for _, m := range page.Data {
			if m.Model == "" || m.Hidden || seenModels[m.Model] {
				continue
			}
			seenModels[m.Model] = true
			label := m.DisplayName
			if label == "" {
				label = m.Model
			}
			model := agent.Model{ID: m.Model, Label: label}
			for _, effort := range m.SupportedReasoningEfforts {
				if effort.Effort != "" {
					model.Efforts = append(model.Efforts, effort.Effort)
				}
			}
			models = append(models, model)
		}
		if page.NextCursor == "" {
			return models, nil
		}
		if seenCursors[page.NextCursor] {
			return nil, fmt.Errorf("repeated Codex model cursor")
		}
		seenCursors[page.NextCursor] = true
		id++
		if err := requestPage(id, page.NextCursor); err != nil {
			return nil, err
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if scanner.Err() != nil {
		return nil, scanner.Err()
	}
	return nil, fmt.Errorf("Codex app-server closed before returning models")
}
