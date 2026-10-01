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
