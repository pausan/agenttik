package copilot

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
)

// Copilot starts background work with ordinary tool calls: a shell in async
// mode, or a task subagent in background mode. The call's result names the
// shell or agent, and a system notification says when it ends. See
// specs/083-background-tasks.md.

// backgroundArgs are what a call that may start background work is read for.
type backgroundArgs struct {
	Mode        string `json:"mode"`
	Command     string `json:"command"`
	Description string `json:"description"`
	ShellID     string `json:"shellId"`
}

type systemNotification struct {
	Kind struct {
		Type     string `json:"type"`
		ShellID  string `json:"shellId"`
		AgentID  string `json:"agentId"`
		ExitCode *int   `json:"exitCode"`
		Status   string `json:"status"`
	} `json:"kind"`
}

var (
	shellIDPattern = regexp.MustCompile(`shellId: ([^>\s]+)`)
	agentIDPattern = regexp.MustCompile(`agent_id: ([^\s.]+)`)
)

// startCall remembers a call that will start background work if it succeeds.
func (p *streamParser) startCall(call toolExecutionStart) {
	var args backgroundArgs
	if json.Unmarshal(call.Arguments, &args) != nil {
		return
	}
	var t *agent.BackgroundTask
	switch {
	case (call.ToolName == "bash" || call.ToolName == "powershell") && args.Mode == "async":
		t = &agent.BackgroundTask{ID: args.ShellID, Kind: "shell", Description: args.Command}
	case call.ToolName == "task" && args.Mode == "background":
		t = &agent.BackgroundTask{Kind: "agent", Description: args.Description}
	case call.ToolName == "stop_bash" || call.ToolName == "stop_powershell":
		t = &agent.BackgroundTask{ID: args.ShellID, Status: "stopped"}
	default:
		return
	}
	t.StartedAt = time.Now().UnixMilli()
	if p.calls == nil {
		p.calls = make(map[string]*agent.BackgroundTask)
	}
	p.calls[call.ToolCallID] = t
}

// finishCall reports the background work a successful call started, or the
// shell it stopped.
func (p *streamParser) finishCall(id string, ok bool, output string, out chan<- agent.Event) {
	t := p.calls[id]
	if t == nil {
		return
	}
	delete(p.calls, id)
	if !ok {
		return
	}
	if t.Status == "stopped" {
		p.endTask(t.ID, "stopped", "", out)
		return
	}
	pattern := shellIDPattern
	if t.Kind == "agent" {
		pattern = agentIDPattern
	}
	if m := pattern.FindStringSubmatch(output); m != nil {
		t.ID = m[1]
	}
	if t.ID == "" || p.tasks[t.ID] != nil {
		return
	}
	t.Status = "running"
	if p.tasks == nil {
		p.tasks = make(map[string]*agent.BackgroundTask)
	}
	p.tasks[t.ID] = t
	out <- agent.BackgroundEvent(*t)
}

func (p *streamParser) notified(data systemNotification, out chan<- agent.Event) {
	k := data.Kind
	switch k.Type {
	case "shell_completed", "shell_detached_completed":
		status, summary := "completed", ""
		if k.ExitCode != nil {
			summary = fmt.Sprintf("exit code %d", *k.ExitCode)
			if *k.ExitCode != 0 {
				status = "failed"
			}
		}
		p.endTask(k.ShellID, status, summary, out)
	case "agent_completed":
		status := "completed"
		if k.Status == "failed" {
			status = "failed"
		}
		p.endTask(k.AgentID, status, "", out)
	case "agent_idle":
		p.endTask(k.AgentID, "completed", "", out)
	}
}

func (p *streamParser) endTask(id, status, summary string, out chan<- agent.Event) {
	t := p.tasks[id]
	if t == nil || t.Status != "running" {
		return
	}
	t.Status, t.Summary, t.EndedAt = status, summary, time.Now().UnixMilli()
	out <- agent.BackgroundEvent(*t)
}
