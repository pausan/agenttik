package runner

import (
	"strings"
	"unicode/utf8"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/store"
)

// maxInheritedText bounds the transcript a provider that cannot fork is
// given instead. The end of a conversation is what a thread asks about, so
// the start is what is cut.
const maxInheritedText = 60_000

// inherit is what a thread, or a task spawned from one, starts from: the
// conversation it came from. A provider that can fork copies that
// conversation, which keeps the tool results and files the agent saw and
// leaves the original untouched. Any other provider, or a source on another
// provider or account, is given the transcript as text. See
// specs/092-threads.md.
func (r *Runner) inherit(sess *store.Session, provider agent.Provider) (forkFrom, text string) {
	sourceID := sess.ParentID
	if sourceID == "" {
		sourceID = sess.OriginID
	}
	if sourceID == "" {
		return "", ""
	}
	source, err := r.store.GetSession(sourceID)
	if err != nil {
		return "", ""
	}
	if forker, ok := provider.(agent.Forker); ok && forker.CanFork() && source.ProviderSessionID != "" &&
		source.Provider == sess.Provider && source.AccountID == sess.AccountID {
		return source.ProviderSessionID, ""
	}
	var b strings.Builder
	if source.ParentID != "" {
		// A task spawned from a thread: the conversation the thread was
		// opened from, the passage, then the thread itself.
		b.WriteString(r.transcriptText(source.ParentID))
		b.WriteString("\n\nThe user then opened a side thread about this passage:\n<quote>\n" +
			source.AnchorQuote + "\n</quote>\n\n")
	}
	b.WriteString(r.transcriptText(source.ID))
	return "", conversationText(b.String(), "The conversation so far, for context:")
}

// handOver is what a conversation that has run is given when its provider
// thread was reset — another provider, subscription or tool access — and the
// new thread cannot resume the old one: its own transcript, so the agent
// carries on rather than starting over. A model change the provider can
// resume never comes here. See specs/009-model-picker.md.
func (r *Runner) handOver(sessionID string) string {
	return conversationText(r.transcriptText(sessionID),
		"This conversation continues from an earlier session, which this one cannot resume. What was said so far, for context:")
}

// conversationText wraps a transcript for a provider's prompt, keeping its
// end when it is too long.
func conversationText(body, intro string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if len(body) > maxInheritedText {
		cut := len(body) - maxInheritedText
		for cut < len(body) && !utf8.RuneStart(body[cut]) {
			cut++
		}
		body = "…" + body[cut:]
	}
	return "<conversation>\n" + intro + "\n\n" + body + "\n</conversation>\n\n"
}

// transcriptText is what was said in a session, without tool calls. A prompt
// no turn has taken yet is left out: no agent has seen it, and it may be the
// one about to be sent.
func (r *Runner) transcriptText(sessionID string) string {
	messages, err := r.store.ListMessages(sessionID)
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, m := range messages {
		who := map[string]string{store.RoleUser: "User", store.RoleAssistant: "Assistant"}[m.Role]
		if who == "" || m.TurnID == 0 || strings.TrimSpace(m.Content) == "" {
			continue
		}
		b.WriteString(who + ": " + strings.TrimSpace(m.Content) + "\n\n")
	}
	return b.String()
}

// threadPreamble tells a thread's first turn what it is about. The prompt
// shown in the thread is only the question; the passage is drawn above it.
func threadPreamble(sess *store.Session, hasRun bool) string {
	if hasRun || sess.ParentID == "" {
		return ""
	}
	return "<thread>\nThe user selected this passage of the conversation and opened a side thread " +
		"about it. Answer in the thread; the main conversation continues separately and will not see it.\n" +
		"<quote>\n" + sess.AnchorQuote + "\n</quote>\n</thread>\n\n"
}
