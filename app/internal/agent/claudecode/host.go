package claudecode

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/pausan/agenttik/app/internal/agent"
)

// askUserQuestion is the CLI's tool for putting multiple-choice questions to
// the user. It arrives as a can_use_tool request like any other; allowing it
// with the answers added to its input is how they reach the model.
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

// handleControl turns a can_use_tool request into a request the user
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
	h, id, input := p.host, env.RequestID, req.Input
	if req.ToolName == askUserQuestion {
		questions, err := askedQuestions(input)
		if err != nil || len(questions) == 0 {
			h.answerTool(id, nil, false, "agenttik could not read these questions. Ask in your reply instead.")
			return
		}
		a := agent.NewApproval(id, "Question", "", "", func(r agent.Reply) error {
			if !r.Allow {
				return h.answerTool(id, nil, false, "The user declined to answer.")
			}
			answered, err := withAnswers(input, r.Answers)
			if err != nil {
				return err
			}
			return h.answerTool(id, answered, true, "")
		})
		a.Questions = questions
		out <- agent.Event{Type: agent.EventApproval, Approval: a}
		return
	}
	name := req.DisplayName
	if name == "" {
		name = req.ToolName
	}
	out <- agent.Event{Type: agent.EventApproval, Approval: agent.NewApproval(
		id, name, req.Description, string(input), func(r agent.Reply) error {
			return h.answerTool(id, input, r.Allow, "The user denied this tool call.")
		})}
}

// askInput is AskUserQuestion's input. Questions have no id; the CLI keys
// answers by the question text.
type askInput struct {
	Questions []struct {
		Question string `json:"question"`
		Header   string `json:"header"`
		Options  []struct {
			Label       string `json:"label"`
			Description string `json:"description"`
		} `json:"options"`
		MultiSelect bool `json:"multiSelect"`
	} `json:"questions"`
}

func askedQuestions(input json.RawMessage) ([]agent.Question, error) {
	var in askInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}
	var out []agent.Question
	for _, q := range in.Questions {
		// Typed answers are always allowed, as the CLI's own "Other" is.
		question := agent.Question{ID: q.Question, Header: q.Header, Question: q.Question,
			Multi: q.MultiSelect, Other: true}
		for _, o := range q.Options {
			question.Options = append(question.Options, agent.QuestionOption{Label: o.Label, Description: o.Description})
		}
		out = append(out, question)
	}
	return out, nil
}

// withAnswers adds the answers to the tool's input, which is the shape the CLI
// reads them back in: question text to answer, several joined by commas.
func withAnswers(input json.RawMessage, answers map[string][]string) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil {
		return nil, err
	}
	joined := make(map[string]string, len(answers))
	for question, picked := range answers {
		joined[question] = strings.Join(picked, ", ")
	}
	encoded, err := json.Marshal(joined)
	if err != nil {
		return nil, err
	}
	fields["answers"] = encoded
	return json.Marshal(fields)
}
