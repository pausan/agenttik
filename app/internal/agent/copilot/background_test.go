package copilot

import (
	"testing"

	"github.com/pausan/agenttik/app/internal/agent"
)

func backgroundEvents(lines ...string) []agent.BackgroundTask {
	p := &streamParser{textDeltas: map[string]bool{}, reasoningDeltas: map[string]bool{}}
	out := make(chan agent.Event, 64)
	for _, l := range lines {
		p.handleLine([]byte(l), out)
	}
	close(out)
	var got []agent.BackgroundTask
	for ev := range out {
		if ev.Type == agent.EventBackground {
			got = append(got, *ev.Background)
		}
	}
	return got
}

// Lines as copilot 1.0.28 writes them, trimmed to the fields that matter.
func TestAsyncShellIsBackgroundUntilNotified(t *testing.T) {
	got := backgroundEvents(
		`{"type":"tool.execution_start","data":{"toolCallId":"c1","toolName":"bash","arguments":{"command":"for i in 1 2 3; do sleep 3; done","description":"Run ticking loop","mode":"async","detach":true,"shellId":"tick-loop"}}}`,
		`{"type":"tool.execution_complete","data":{"toolCallId":"c1","success":true,"result":{"content":"<command started in detached background with shellId: tick-loop>"}}}`,
		`{"type":"tool.execution_start","data":{"toolCallId":"c2","toolName":"bash","arguments":{"command":"ls","mode":"sync"}}}`,
		`{"type":"tool.execution_complete","data":{"toolCallId":"c2","success":true,"result":{"content":"a\nb"}}}`,
		`{"type":"system.notification","data":{"content":"<system_notification>done</system_notification>","kind":{"type":"shell_detached_completed","shellId":"tick-loop","description":"Run ticking loop"}}}`,
	)
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	if s := got[0]; s.ID != "tick-loop" || s.Kind != "shell" || s.Status != "running" ||
		s.Description != "for i in 1 2 3; do sleep 3; done" || s.StartedAt == 0 {
		t.Errorf("started = %+v", s)
	}
	if e := got[1]; e.Status != "completed" || e.EndedAt == 0 || e.StartedAt != got[0].StartedAt {
		t.Errorf("ended = %+v", e)
	}
}

func TestBackgroundAgentAndStoppedShell(t *testing.T) {
	got := backgroundEvents(
		`{"type":"tool.execution_start","data":{"toolCallId":"c1","toolName":"task","arguments":{"description":"Explore the parser","prompt":"…","agent_type":"explore","mode":"background"}}}`,
		`{"type":"tool.execution_complete","data":{"toolCallId":"c1","success":true,"result":{"content":"Agent started in background with agent_id: explore-3. You can use read_agent tool with this agent_id to check status and retrieve results."}}}`,
		`{"type":"tool.execution_start","data":{"toolCallId":"c2","toolName":"bash","arguments":{"command":"npm run dev","mode":"async"}}}`,
		`{"type":"tool.execution_complete","data":{"toolCallId":"c2","success":true,"result":{"content":"<command started in background with shellId: 7>"}}}`,
		`{"type":"system.notification","data":{"kind":{"type":"agent_completed","agentId":"explore-3","agentType":"explore","status":"failed"}}}`,
		`{"type":"tool.execution_start","data":{"toolCallId":"c3","toolName":"stop_bash","arguments":{"shellId":"7"}}}`,
		`{"type":"tool.execution_complete","data":{"toolCallId":"c3","success":true,"result":{"content":"stopped"}}}`,
	)
	if len(got) != 4 {
		t.Fatalf("got %+v", got)
	}
	if a := got[0]; a.ID != "explore-3" || a.Kind != "agent" || a.Description != "Explore the parser" {
		t.Errorf("agent = %+v", a)
	}
	if s := got[1]; s.ID != "7" || s.Kind != "shell" || s.Description != "npm run dev" {
		t.Errorf("shell = %+v", s)
	}
	if got[2].ID != "explore-3" || got[2].Status != "failed" {
		t.Errorf("agent end = %+v", got[2])
	}
	if got[3].ID != "7" || got[3].Status != "stopped" {
		t.Errorf("shell end = %+v", got[3])
	}
}
