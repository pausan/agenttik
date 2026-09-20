package server

import (
	"encoding/json"
	"testing"

	"github.com/pausan/agenttik/app/internal/store"
)

func TestAnalyticsAPI(t *testing.T) {
	s, _ := newTestServer(t)
	for _, query := range []string{"0", "-1", "3651", "1.5", "nope"} {
		res := do(t, s, "GET", "/api/analytics?days="+query, nil)
		res.Body.Close()
		if res.StatusCode != 400 {
			t.Fatalf("days=%s: status %d", query, res.StatusCode)
		}
	}
	for _, query := range []string{"", "?days=1", "?days=3650"} {
		res := do(t, s, "GET", "/api/analytics"+query, nil)
		var body struct {
			From int64                `json:"from"`
			To   int64                `json:"to"`
			Rows []store.AnalyticsRow `json:"rows"`
		}
		err := json.NewDecoder(res.Body).Decode(&body)
		res.Body.Close()
		if err != nil || res.StatusCode != 200 || body.Rows == nil || len(body.Rows) != 0 {
			t.Fatalf("query %s: %+v, status %d, error %v", query, body, res.StatusCode, err)
		}
		days := int64(7)
		if query == "?days=1" {
			days = 1
		}
		if query == "?days=3650" {
			days = 3650
		}
		if body.To-body.From != days*86400000 {
			t.Fatalf("wrong window: %+v", body)
		}
	}
}
