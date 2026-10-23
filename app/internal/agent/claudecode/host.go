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

// host is agenttik's end of the CLI's stdin during a task turn. Prompts and
// every approval answer are written through it, from different goroutines, so
// writes are serialized and a write after close is an error, not a panic.
type host struct {
	mu     sync.Mutex
	w      io.WriteCloser
	closed bool
	// waiting are the follow-ups written and not taken yet, oldest first, as
	// written. The CLI echoes a prompt back (--replay-user-messages) as it
	// starts on it. A follow-up not taken yet keeps stdin open, so its reply
	// can still leave background work running. The first prompt is not
	// among them: a reply to it is proof it was taken, echo or not.
	waiting []string
}

func (h *host) send(v any) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.write(v)
}

// write is send's body, with h.mu held.
func (h *host) write(v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
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
	h.closeLocked()
}

func (h *host) closeLocked() {
	if !h.closed {
		h.closed = true
		h.w.Close()
	}
}

// closeIfTaken closes the input unless a follow-up written to it has not been
// taken yet, and reports whether it did. Checked under the same lock as
// writes, so a prompt is either written before the close or refused.
func (h *host) closeIfTaken() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.waiting) > 0 {
		return false
	}
	h.closeLocked()
	return true
}

// took matches a prompt the CLI echoed back against the oldest follow-up
// waiting, and reports whether it was that one, which is then taken. Any
// other echo changes nothing: the first prompt's, or one the CLI adds.
func (h *host) took(echo string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.waiting) == 0 || strings.TrimSpace(h.waiting[0]) != strings.TrimSpace(echo) {
		return false
	}
	h.waiting = h.waiting[1:]
	return true
}

// Sent with each task prompt, including resumes. CLI system-prompt snapshots
// can otherwise keep new lifecycle guidance out of existing conversations.
const backgroundTaskGuidance = `Agenttik keeps this turn open while background tasks are running,
and the user can keep writing to you meanwhile. Do not wait idly for a
background task: end your reply, and a task notification or the user's next
message will wake you.
Once the user's requested work is complete, stop disposable background helpers
you started that are no longer needed, using TaskStop where available, before
your final reply. Keep waiting for tests, builds, or agents whose results are
still needed. Do not stop work just to end the turn, or stop processes the user
explicitly asked to keep running.`

// sendPrompt writes the turn's first prompt.
func (h *host) sendPrompt(prompt string) error {
	return h.send(userLine(backgroundTaskGuidance + "\n\n" + prompt))
}

// sendUser writes a follow-up, which waits until the CLI takes it.
func (h *host) sendUser(prompt string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.write(userLine(prompt)); err != nil {
		return err
	}
	h.waiting = append(h.waiting, prompt)
	return nil
}

func userLine(prompt string) userInput {
	return userInput{Type: "user", Message: userMessage{Role: "user", Content: prompt}}
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
