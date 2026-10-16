package chat

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func TestAgentEventTypeParametersAndReceipt(t *testing.T) {
	a, _ := memoryFixture(t)
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	for _, eventType := range []string{"message", "notice", "request", ""} {
		s := NewHistorySearch(456, -10)
		s.read = func(_ context.Context, bot int64, q ArchiveQuery) (ArchivePage, error) {
			if bot != 456 || q.Conversation != -10 || q.SearchEventType != eventType || !q.Search || !q.EventsOnly || q.Before != 9007199254740993 {
				t.Fatal(q)
			}
			return ArchivePage{Available: true, Entries: []ArchiveEntry{}}, nil
		}
		params := zero.Params{"eventType": eventType, "before": "9007199254740993"}
		if eventType == "" {
			params["keyword"] = "needle"
		}
		for i := 0; i < 3; i++ {
			if err := s.Execute(&ag, zero.APIRequest{Action: goba.SearchHistory, Params: params}); err != nil || s.Outcome() != "ok" {
				t.Fatal(err, s.Usage())
			}
		}
		if err := s.Execute(&ag, zero.APIRequest{Action: goba.SearchHistory, Params: params}); err == nil || s.Usage().Used != 3 || s.Usage().Denied != 1 {
			t.Fatal("quota bypassed", s.Usage())
		}
		var raw string
		if err := a.history.db.QueryRow(`SELECT payload FROM history WHERE kind='response' ORDER BY seq DESC LIMIT 1`).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var response struct{ Data map[string]any }
		if err := json.Unmarshal([]byte(raw), &response); err != nil {
			t.Fatal(err)
		}
		if value, present := response.Data["eventType"]; (eventType != "" && value != eventType) || (eventType == "" && present) {
			t.Fatal(response.Data)
		}
	}
	for _, req := range []zero.APIRequest{
		{Action: goba.SearchHistory, Params: zero.Params{"eventType": ""}},
		{Action: goba.SearchHistory, Params: zero.Params{"eventType": "Message"}},
		{Action: goba.SearchHistory, Params: zero.Params{"eventType": 1}},
		{Action: goba.SearchHistory, Params: zero.Params{"eventType": nil}},
		{Action: goba.SearchMemory, Params: zero.Params{"keyword": "needle", "eventType": "message"}},
		{Action: goba.ReadHistory, Params: zero.Params{"seq": "1", "eventType": "notice"}},
		{Action: goba.ReadMemory, Params: zero.Params{"id": "1", "revision": "1", "eventType": "request"}},
	} {
		s := NewHistorySearch(456, -10)
		if err := s.Execute(&ag, req); err != nil || s.Outcome() != "local_error" || s.Usage().Errors != 1 || s.Usage().Used != 1 {
			t.Fatal(req, err, s.Usage())
		}
	}
}
