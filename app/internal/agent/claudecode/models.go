package claudecode

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
)

type modelCache struct {
	mu     sync.Mutex
	asked  time.Time
	labels map[string]string
	// file keeps labels across launches; changed hears of a new answer.
	file    string
	changed func()
	// pending is the refresh in flight, which tests wait on.
	pending sync.WaitGroup
}

// RememberModels keeps the labels in path, so a launch shows the previous
// session's versions while the CLI is asked again, and calls changed when an
// answer differs from them. Call it before the first Models.
func (p *Provider) RememberModels(path string, changed func()) {
	c := &p.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	c.file, c.changed = path, changed
	agent.LoadRemembered(path, &c.labels)
}

// Ask only for initialization metadata, never a model turn. Asking costs a
// CLI start, so the labels are cached and refreshed in the background: the
// caller gets what is known now, and no labels until the first answer.
func (p *Provider) modelLabels() map[string]string {
	c := &p.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.asked) >= 10*time.Minute {
		c.asked = time.Now()
		c.pending.Add(1)
		go c.refresh()
	}
	return c.labels
}

func (c *modelCache) refresh() {
	defer c.pending.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, Binary, "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--no-session-persistence", "--safe-mode", "--setting-sources", "user")
	cmd.Dir = os.TempDir()
	cmd.Stdin = strings.NewReader("{\"type\":\"control_request\",\"request_id\":\"models\",\"request\":{\"subtype\":\"initialize\"}}\n")
	data, err := cmd.Output()
	if err != nil {
		return
	}
	labels := parseModelLabels(string(data))
	c.mu.Lock()
	if len(labels) == 0 || maps.Equal(labels, c.labels) {
		c.mu.Unlock()
		return
	}
	c.labels = labels
	file, changed := c.file, c.changed
	c.mu.Unlock()
	if file != "" {
		agent.SaveRemembered(file, labels)
	}
	if changed != nil {
		changed()
	}
}

var versionedModel = regexp.MustCompile(`^claude-(fable|opus|sonnet|haiku)-(\d+)(?:[.-](\d{1,2}))?(?:-\d{8})?(?:\[1m\])?$`)

func parseModelLabels(data string) map[string]string {
	labels := make(map[string]string)
	for _, line := range strings.Split(data, "\n") {
		var reply struct {
			Type     string `json:"type"`
			Response struct {
				RequestID string `json:"request_id"`
				Response  struct {
					Models []struct {
						Value         string `json:"value"`
						ResolvedModel string `json:"resolvedModel"`
					} `json:"models"`
				} `json:"response"`
			} `json:"response"`
		}
		if json.Unmarshal([]byte(line), &reply) != nil || reply.Type != "control_response" || reply.Response.RequestID != "models" {
			continue
		}
		for _, model := range reply.Response.Response.Models {
			match := versionedModel.FindStringSubmatch(model.ResolvedModel)
			if match == nil {
				continue
			}
			family := match[1]
			value := strings.TrimSuffix(model.Value, "[1m]")
			// The default may be a different family; explicit older versions must
			// not replace the current alias's label.
			if value != family && value != strings.TrimSuffix(model.ResolvedModel, "[1m]") {
				continue
			}
			label := strings.ToUpper(family[:1]) + family[1:] + " " + match[2]
			if match[3] != "" {
				label += "." + match[3]
			}
			if _, exists := labels[family]; !exists || value == family {
				labels[family] = label
			}
		}
	}
	return labels
}

func (p *Provider) Models() []agent.Model {
	models := []agent.Model{
		{ID: "fable", Label: "Fable (version unknown)", ContextWindow: 1_000_000},
		{ID: "opus", Label: "Opus (version unknown)", ContextWindow: 1_000_000},
		{ID: "sonnet", Label: "Sonnet (version unknown)", ContextWindow: 1_000_000},
		{ID: "haiku", Label: "Haiku (version unknown)", ContextWindow: 200_000},
	}
	labels := p.modelLabels()
	for i := range models {
		if label := labels[models[i].ID]; label != "" {
			models[i].Label = label
		}
	}
	return models
}
