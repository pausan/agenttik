package agent

import "testing"

func TestCheckReply(t *testing.T) {
	a := &Approval{Questions: []Question{
		{ID: "color", Question: "Color?", Options: []QuestionOption{{Label: "Red"}, {Label: "Blue"}}},
		{ID: "sizes", Question: "Sizes?", Options: []QuestionOption{{Label: "S"}, {Label: "M"}}, Multi: true},
		{ID: "name", Question: "Name?", Other: true},
	}}
	ok := map[string][]string{"color": {"Red"}, "sizes": {"S", "M"}, "name": {"Ada"}}
	for name, tc := range map[string]struct {
		reply Reply
		ok    bool
	}{
		"complete":            {Reply{Allow: true, Answers: ok}, true},
		"declined":            {Reply{}, true},
		"missing":             {Reply{Allow: true, Answers: map[string][]string{"color": {"Red"}}}, false},
		"two for single":      {Reply{Allow: true, Answers: map[string][]string{"color": {"Red", "Blue"}, "sizes": {"S"}, "name": {"x"}}}, false},
		"option not offered":  {Reply{Allow: true, Answers: map[string][]string{"color": {"Green"}, "sizes": {"S"}, "name": {"x"}}}, false},
		"blank typed answer":  {Reply{Allow: true, Answers: map[string][]string{"color": {"Red"}, "sizes": {"S"}, "name": {"  "}}}, false},
		"approval, no answer": {Reply{Allow: true}, false},
	} {
		if err := a.CheckReply(tc.reply); (err == nil) != tc.ok {
			t.Errorf("%s: err = %v, want ok=%v", name, err, tc.ok)
		}
	}
	if err := (&Approval{}).CheckReply(Reply{Allow: true}); err != nil {
		t.Errorf("plain approval: %v", err)
	}
}

func TestAutoReply(t *testing.T) {
	if r := (&Approval{Tool: "Bash"}).AutoReply(); !r.Allow || r.Answers != nil {
		t.Errorf("tool call = %+v", r)
	}
	a := &Approval{Questions: []Question{
		{ID: "db", Options: []QuestionOption{{Label: "Redis"}, {Label: "SQLite (Recommended)"}}},
		{ID: "color", Options: []QuestionOption{{Label: "Red"}, {Label: "Blue"}}, Multi: true},
	}}
	r := a.AutoReply()
	if !r.Allow || r.Answers["db"][0] != "SQLite (Recommended)" || r.Answers["color"][0] != "Red" {
		t.Errorf("questions = %+v", r)
	}
	if err := a.CheckReply(r); err != nil {
		t.Errorf("auto reply does not check: %v", err)
	}
	typed := &Approval{Questions: []Question{{ID: "db", Options: []QuestionOption{{Label: "A"}}}, {ID: "name", Other: true}}}
	if r := typed.AutoReply(); r.Allow {
		t.Errorf("a question with no options should be declined, got %+v", r)
	}
}
