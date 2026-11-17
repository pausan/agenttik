package runner

import (
	"testing"

	"github.com/pausan/agenttik/app/internal/store"
)

func TestModeGoesAheadOfTheProjectPrompt(t *testing.T) {
	fp := &fakeProvider{script: script("main")}
	r, st, sess := setup(t, fp)
	review, err := st.CreateMode("Code review", "Review only.")
	if err != nil {
		t.Fatal(err)
	}
	tdd, err := st.CreateMode("TDD", "Test first.")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetProjectPrompt(sess.ProjectID, "project rules"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetProjectMode(sess.ProjectID, review.ID); err != nil {
		t.Fatal(err)
	}

	// The task defers to the project's mode.
	turn(t, r, sess.ID, "hello")
	if fp.lastReq.Prompt != "Review only.\n\nproject rules\n\nhello" {
		t.Fatalf("project mode prompt = %q", fp.lastReq.Prompt)
	}
	got, _ := st.GetSession(sess.ID)
	if got.ModeTitle != "Code review" {
		t.Fatalf("mode title = %q", got.ModeTitle)
	}
	// Later edits reach new conversations, not this one.
	if err := st.UpdateMode(review.ID, "Code review", "Changed."); err != nil {
		t.Fatal(err)
	}
	turn(t, r, sess.ID, "again")
	if fp.lastReq.Prompt != "again" {
		t.Fatalf("second turn = %q", fp.lastReq.Prompt)
	}

	for _, c := range []struct {
		mode        int64
		prompt, tag string
	}{
		{tdd.ID, "Test first.\n\nproject rules\n\nhi", "TDD"},
		{store.ModeNone, "project rules\n\nhi", ""},
	} {
		s := &store.Session{ID: "t" + c.tag, ProjectID: sess.ProjectID, Provider: "fake", Model: "m1", ModeID: c.mode}
		if err := st.CreateSession(s); err != nil {
			t.Fatal(err)
		}
		turn(t, r, s.ID, "hi")
		got, _ := st.GetSession(s.ID)
		if fp.lastReq.Prompt != c.prompt || got.ModeTitle != c.tag {
			t.Fatalf("mode %d: prompt %q, title %q", c.mode, fp.lastReq.Prompt, got.ModeTitle)
		}
	}
}

func TestModesAreOfferedAsSubagents(t *testing.T) {
	fp := &fakeProvider{script: script("main")}
	r, st, sess := setup(t, fp)
	for _, title := range []string{"Code Review!", "code-review", "¿?"} {
		if _, err := st.CreateMode(title, title+" body"); err != nil {
			t.Fatal(err)
		}
	}
	turn(t, r, sess.ID, "hello")
	names := map[string]string{}
	for _, a := range fp.lastReq.Subagents {
		names[a.Name] = a.Prompt
	}
	if len(names) != 3 || names["code-review"] != "Code Review! body" ||
		names["code-review-2"] != "code-review body" || names["mode"] != "¿? body" {
		t.Fatalf("subagents = %+v", fp.lastReq.Subagents)
	}
}

func TestSubagentNamesStayDistinctWhenTheSuffixCollides(t *testing.T) {
	fp := &fakeProvider{script: script("main")}
	r, st, sess := setup(t, fp)
	// "Review!" slugs onto "review", and its ID then builds the name
	// "Review 3" already has.
	for _, title := range []string{"Review", "Review 3", "Review!"} {
		if _, err := st.CreateMode(title, title+" body"); err != nil {
			t.Fatal(err)
		}
	}
	turn(t, r, sess.ID, "hello")
	names := map[string]string{}
	for _, a := range fp.lastReq.Subagents {
		names[a.Name] = a.Prompt
	}
	if len(names) != 3 || names["review"] != "Review body" ||
		names["review-3"] != "Review 3 body" || names["review-3-3"] != "Review! body" {
		t.Fatalf("subagents = %+v", fp.lastReq.Subagents)
	}
}
