package claudecode

import (
	"encoding/json"
	"errors"
	"io"
	"sync"

	"github.com/pausan/agenttik/app/internal/agent"
)

// askUserQuestion is the CLI's tool for putting a multiple-choice question to
// the user. Its answer is not a yes or no, so it is refused with a note
// telling the model to ask in its reply instead.
const askUserQuestion = "AskUserQuestion"

// host is agenttik's end of the CLI's stdin during a task turn. The prompt and
// every approval answer are written through it, from different goroutines, so
// writes are serialized and a write after close is an error, not a panic.
type host struct {
	mu     sync.Mutex
	w      io.WriteCloser
	closed bool
}

func (h *host) send(v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return errors.New("turn has ended")
	}
	_, err = h.w.Write(append(line, '\n'))
	return err
}

// close ends the input. The CLI exits once the turn's result is out and its
// stdin is closed.
func (h *host) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.closed {
		h.closed = true
		h.w.Close()
	}
}

func (h *host) sendPrompt(prompt string) error {
	return h.send(userInput{Type: "user", Message: userMessage{Role: "user", Content: prompt}})
}

func (h *host) answerTool(requestID string, input json.RawMessage, allow bool, message string) error {
	result := permissionResult{Behavior: "deny", Message: message}
	if allow {
		result = permissionResult{Behavior: "allow", UpdatedInput: input}
		if len(input) == 0 {
			result.UpdatedInput = json.RawMessage("{}")
		}
	}
	return h.send(controlResponse{Type: "control_response", Response: controlResponseRef{
		Subtype: "success", RequestID: requestID, Response: result}})
}

// refuse answers a control request this host does not handle, so the CLI
// does not wait on it forever.
func (h *host) refuse(requestID, subtype string) error {
	return h.send(controlResponse{Type: "control_response", Response: controlResponseRef{
		Subtype: "error", RequestID: requestID, Error: "unsupported control request: " + subtype}})
}

// handleControl turns a can_use_tool request into an approval event the user
// answers. Without a host (an isolated request) nothing is asked: the CLI was
// started without stdio prompts and never sends one.
func (p *streamParser) handleControl(env envelope, out chan<- agent.Event) {
	if p.host == nil || env.RequestID == "" || env.Request == nil {
		return
	}
	req := env.Request
	if req.Subtype != "can_use_tool" {
		p.host.refuse(env.RequestID, req.Subtype)
		return
	}
	if req.ToolName == askUserQuestion {
		p.host.answerTool(env.RequestID, nil, false,
			"agenttik cannot show questions to the user. Ask in your reply instead.")
		return
	}
	name := req.DisplayName
	if name == "" {
		name = req.ToolName
	}
	h, id, input := p.host, env.RequestID, req.Input
	out <- agent.Event{Type: agent.EventApproval, Approval: agent.NewApproval(
		id, name, req.Description, string(input), func(allow bool) error {
			return h.answerTool(id, input, allow, "The user denied this tool call.")
		})}
}
