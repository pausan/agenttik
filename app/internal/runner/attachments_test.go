package runner

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pausan/agenttik/app/internal/agent"
)

func TestImagePromptReachesProviderAsLocalFile(t *testing.T) {
	fp := &fakeProvider{script: []agent.Event{{Type: agent.EventDone}}}
	r, st, sess := setup(t, fp)
	name := strings.Repeat("a", 64) + ".png"
	prompt := "![Attached image](/api/attachments/" + name + ")"
	if _, err := r.Send(sess.ID, prompt); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "image turn")
	if !strings.Contains(fp.lastReq.Prompt, filepath.Join(st.Dir(), "attachments", name)) {
		t.Fatal(fp.lastReq.Prompt)
	}
	messages, err := st.ListMessages(sess.ID)
	if err != nil || len(messages) == 0 || messages[0].Content != prompt {
		t.Fatal("stored prompt lost its portable image reference", err)
	}
}
