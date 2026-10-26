package runner

import (
	"os"
	"strings"
	"testing"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/store"
)

func TestTranscriptionCorrectionIsolated(t *testing.T) {
	p := newSmallProvider(" Revisa el código. ")
	r, _, sess := setup(t, p)
	choice := store.ActionModel{Provider: sess.Provider, AccountID: sess.AccountID, Model: "m1", Effort: "low"}
	got := r.CorrectTranscription("revisa el codigo", choice)
	if got != "Revisa el código." {
		t.Fatal(got)
	}
	req := <-p.asked
	if req.WorkDir != os.TempDir() || !req.Isolated || req.Permission != agent.PermissionPlan || req.SessionID != "" || req.Model != "m1" {
		t.Fatalf("unsafe correction %+v", req)
	}
	if !strings.Contains(req.Prompt, `"revisa el codigo"`) || !strings.Contains(req.Prompt, "Do not translate") {
		t.Fatal(req.Prompt)
	}
}
