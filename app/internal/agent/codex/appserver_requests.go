package codex

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/pausan/agenttik/app/internal/agent"
)

// Requests app-server makes of its client mid-turn: approvals and questions.
// Each becomes an approval event the user answers from any window; the reply
// goes back as the JSON-RPC result whenever it comes, while the read loop
// carries on. See specs/082-tool-approvals.md.

// write sends one JSON-RPC message. The read loop and answers from the UI
// both write, so writes are serialized, and once the turn is over a late
// answer is an error rather than a write to a closed pipe.
func (r *appServerRun) write(v any) error {
	r.wmu.Lock()
	defer r.wmu.Unlock()
	if r.closed {
		return errors.New("turn has ended")
	}
	return r.enc.Encode(v)
}

func (r *appServerRun) closeWrites() {
	r.wmu.Lock()
	r.closed = true
	r.wmu.Unlock()
}

// rpcKey is a request id as a map key. Ids are numbers or strings.
func rpcKey(raw json.RawMessage) string { return string(bytes.TrimSpace(raw)) }

// ask publishes a request and answers it with reply's result once the user
// decides. ids are agenttik's own: app-server's restart with every process.
func (r *appServerRun) ask(rpcID json.RawMessage, a *agent.Approval, result func(agent.Reply) any) {
	r.pending[rpcKey(rpcID)] = a.ID
	asked := agent.NewApproval(a.ID, a.Tool, a.Description, a.Input, func(reply agent.Reply) error {
		return r.write(map[string]any{"id": rpcID, "result": result(reply)})
	})
	asked.Questions = a.Questions
	r.out <- agent.Event{Type: agent.EventApproval, Approval: asked}
}

// resolved reports a request app-server stopped waiting on: answered here,
// resolved some other way, or dropped with its turn.
func (r *appServerRun) resolved(rpcID json.RawMessage) {
	key := rpcKey(rpcID)
	id, ok := r.pending[key]
	if !ok {
		return
	}
	delete(r.pending, key)
	r.out <- agent.Event{Type: agent.EventApprovalResolved, Approval: &agent.Approval{ID: id}}
}

func newID() string { return "codex-" + uuid.NewString() }

func decision(allow bool, yes, no string) map[string]any {
	if allow {
		return map[string]any{"decision": yes}
	}
	return map[string]any{"decision": no}
}

func inputJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// replyToServerRequest handles a request app-server makes of this client.
// What the user can decide is asked; anything else is refused at once so the
// turn never waits on it.
func (r *appServerRun) replyToServerRequest(message appRPCMessage) error {
	switch message.Method {
	case "item/commandExecution/requestApproval":
		var p struct {
			Command string `json:"command"`
			Cwd     string `json:"cwd"`
			Reason  string `json:"reason"`
		}
		json.Unmarshal(message.Params, &p)
		r.ask(message.ID, agent.NewApproval(newID(), "Shell", p.Reason,
			inputJSON(map[string]string{"command": p.Command, "cwd": p.Cwd}), nil),
			func(reply agent.Reply) any { return decision(reply.Allow, "accept", "decline") })
		return nil
	case "item/fileChange/requestApproval":
		var p struct {
			ItemID    string `json:"itemId"`
			Reason    string `json:"reason"`
			GrantRoot string `json:"grantRoot"`
		}
		json.Unmarshal(message.Params, &p)
		input := map[string]any{"files": r.fileChanges[p.ItemID]}
		if p.GrantRoot != "" {
			input["grantRoot"] = p.GrantRoot
		}
		r.ask(message.ID, agent.NewApproval(newID(), "Edit files", p.Reason, inputJSON(input), nil),
			func(reply agent.Reply) any { return decision(reply.Allow, "accept", "decline") })
		return nil
	case "execCommandApproval":
		var p struct {
			Command []string `json:"command"`
			Cwd     string   `json:"cwd"`
			Reason  string   `json:"reason"`
		}
		json.Unmarshal(message.Params, &p)
		r.ask(message.ID, agent.NewApproval(newID(), "Shell", p.Reason,
			inputJSON(map[string]string{"command": strings.Join(p.Command, " "), "cwd": p.Cwd}), nil),
			func(reply agent.Reply) any { return decision(reply.Allow, "approved", "denied") })
		return nil
	case "applyPatchApproval":
		var p struct {
			FileChanges map[string]json.RawMessage `json:"fileChanges"`
			Reason      string                     `json:"reason"`
		}
		json.Unmarshal(message.Params, &p)
		var files []string
		for path := range p.FileChanges {
			files = append(files, path)
		}
		r.ask(message.ID, agent.NewApproval(newID(), "Edit files", p.Reason,
			inputJSON(map[string]any{"files": files}), nil),
			func(reply agent.Reply) any { return decision(reply.Allow, "approved", "denied") })
		return nil
	case "item/tool/requestUserInput":
		return r.askUserInput(message)
	case "mcpServer/elicitation/request":
		return r.askElicitation(message)
	}
	return r.write(map[string]any{
		"id":    message.ID,
		"error": map[string]any{"code": -32601, "message": "client method not supported"},
	})
}

func (r *appServerRun) askUserInput(message appRPCMessage) error {
	var p struct {
		Questions []struct {
			ID       string `json:"id"`
			Header   string `json:"header"`
			Question string `json:"question"`
			IsOther  bool   `json:"isOther"`
			IsSecret bool   `json:"isSecret"`
			Options  []struct {
				Label       string `json:"label"`
				Description string `json:"description"`
			} `json:"options"`
		} `json:"questions"`
	}
	json.Unmarshal(message.Params, &p)
	var questions []agent.Question
	for _, q := range p.Questions {
		question := agent.Question{ID: q.ID, Header: q.Header, Question: q.Question,
			Other: q.IsOther || len(q.Options) == 0, Secret: q.IsSecret}
		for _, o := range q.Options {
			question.Options = append(question.Options, agent.QuestionOption{Label: o.Label, Description: o.Description})
		}
		questions = append(questions, question)
	}
	a := agent.NewApproval(newID(), "Question", "", "", nil)
	a.Questions = questions
	r.ask(message.ID, a, func(reply agent.Reply) any {
		// Declining sends no answers, which lets the model carry on and say
		// what it still needs.
		answers := map[string]any{}
		if reply.Allow {
			for id, picked := range reply.Answers {
				answers[id] = map[string]any{"answers": picked}
			}
		}
		return map[string]any{"answers": answers}
	})
	return nil
}

// elicitation is an MCP server asking the user something through Codex. A
// form with no fields is a yes or no — Codex asks this way before an MCP tool
// call — and a form with fields is a set of questions. A URL to visit cannot
// be shown here and is declined.
type elicitation struct {
	ServerName      string           `json:"serverName"`
	Mode            string           `json:"mode"`
	Message         string           `json:"message"`
	RequestedSchema *elicitSchema    `json:"requestedSchema"`
	Meta            *elicitationMeta `json:"_meta"`
}

type elicitationMeta struct {
	Kind            string          `json:"codex_approval_kind"`
	ToolDescription string          `json:"tool_description"`
	ToolParams      json.RawMessage `json:"tool_params"`
}

type elicitSchema struct {
	Properties map[string]elicitField `json:"properties"`
	Required   []string               `json:"required"`
}

type elicitConst struct {
	Const string `json:"const"`
	Title string `json:"title"`
}

type elicitField struct {
	Type        string        `json:"type"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Enum        []string      `json:"enum"`
	EnumNames   []string      `json:"enumNames"`
	OneOf       []elicitConst `json:"oneOf"`
	AnyOf       []elicitConst `json:"anyOf"`
	Items       *elicitField  `json:"items"`
}

// choices lists a field's options as label and value. Titled enums show the
// title and send the value.
func (f elicitField) choices() (labels, values []string) {
	switch {
	case len(f.OneOf) > 0 || len(f.AnyOf) > 0:
		for _, c := range append(f.OneOf, f.AnyOf...) {
			label := c.Title
			if label == "" {
				label = c.Const
			}
			labels, values = append(labels, label), append(values, c.Const)
		}
	case len(f.Enum) > 0:
		for i, v := range f.Enum {
			label := v
			if i < len(f.EnumNames) && f.EnumNames[i] != "" {
				label = f.EnumNames[i]
			}
			labels, values = append(labels, label), append(values, v)
		}
	case f.Type == "boolean":
		labels, values = []string{"Yes", "No"}, []string{"true", "false"}
	}
	return labels, values
}

func (r *appServerRun) askElicitation(message appRPCMessage) error {
	var p elicitation
	json.Unmarshal(message.Params, &p)
	decline := map[string]any{"action": "decline", "content": nil, "_meta": nil}
	if p.Mode == "url" || p.RequestedSchema == nil {
		return r.write(map[string]any{"id": message.ID, "result": decline})
	}
	if len(p.RequestedSchema.Properties) == 0 {
		a := agent.NewApproval(newID(), p.ServerName+" MCP tool", p.Message, "", nil)
		if p.Meta != nil {
			if len(p.Meta.ToolParams) > 0 {
				a.Input = string(p.Meta.ToolParams)
			}
			if p.Meta.ToolDescription != "" {
				a.Description = p.Message + " " + p.Meta.ToolDescription
			}
		}
		r.ask(message.ID, a, func(reply agent.Reply) any {
			if !reply.Allow {
				return decline
			}
			return map[string]any{"action": "accept", "content": map[string]any{}, "_meta": nil}
		})
		return nil
	}

	fields := p.RequestedSchema.Properties
	var questions []agent.Question
	for _, name := range fieldOrder(message.Params, fields) {
		f := fields[name]
		item := f
		if f.Type == "array" && f.Items != nil {
			item = *f.Items
		}
		labels, _ := item.choices()
		text := f.Title
		if text == "" {
			text = name
		}
		if f.Description != "" {
			text += " — " + f.Description
		}
		q := agent.Question{ID: name, Header: p.ServerName, Question: text,
			Multi: f.Type == "array", Other: len(labels) == 0}
		for _, label := range labels {
			q.Options = append(q.Options, agent.QuestionOption{Label: label})
		}
		questions = append(questions, q)
	}
	a := agent.NewApproval(newID(), p.ServerName, p.Message, "", nil)
	a.Questions = questions
	r.ask(message.ID, a, func(reply agent.Reply) any {
		if !reply.Allow {
			return decline
		}
		return map[string]any{"action": "accept", "content": elicitContent(fields, reply.Answers), "_meta": nil}
	})
	return nil
}

// elicitContent turns answers back into the form's typed values.
func elicitContent(fields map[string]elicitField, answers map[string][]string) map[string]any {
	content := map[string]any{}
	for name, f := range fields {
		picked := answers[name]
		if len(picked) == 0 {
			continue
		}
		item := f
		if f.Type == "array" && f.Items != nil {
			item = *f.Items
		}
		labels, values := item.choices()
		convert := func(answer string) any {
			for i, label := range labels {
				if label == answer {
					answer = values[i]
				}
			}
			switch item.Type {
			case "boolean":
				return answer == "true"
			case "number":
				if n, err := strconv.ParseFloat(answer, 64); err == nil {
					return n
				}
			case "integer":
				if n, err := strconv.ParseInt(answer, 10, 64); err == nil {
					return n
				}
			}
			return answer
		}
		if f.Type == "array" {
			var list []any
			for _, answer := range picked {
				list = append(list, convert(answer))
			}
			content[name] = list
		} else {
			content[name] = convert(picked[0])
		}
	}
	return content
}

// fieldOrder lists the form's fields in the order the server wrote them,
// which a map forgets. Fields the order misses are added sorted.
func fieldOrder(params json.RawMessage, fields map[string]elicitField) []string {
	var raw struct {
		RequestedSchema struct {
			Properties json.RawMessage `json:"properties"`
		} `json:"requestedSchema"`
	}
	json.Unmarshal(params, &raw)
	var order []string
	dec := json.NewDecoder(bytes.NewReader(raw.RequestedSchema.Properties))
	if tok, err := dec.Token(); err == nil && tok == json.Delim('{') {
		for dec.More() {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			if key, ok := tok.(string); ok {
				order = append(order, key)
			}
			var skip json.RawMessage
			if dec.Decode(&skip) != nil {
				break
			}
		}
	}
	var rest []string
	for k := range fields {
		if !slices.Contains(order, k) {
			rest = append(rest, k)
		}
	}
	slices.Sort(rest)
	return append(order, rest...)
}
