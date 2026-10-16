package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func TestMemorySourceSearchCurrentRevisionAndContinuation(t *testing.T) {
	a, b := memoryFixture(t)
	ctx := context.Background()
	for _, text := range []string{"needle old", "needle manual", "needle recent", "needle imported", "needle disabled"} {
		if err := a.Save(10, text); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.Update(ctx, 10, 2, 1, "edit", "needle manual edited"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.history.db.Exec(`UPDATE agent_memories SET source='legacy' WHERE id=4`); err != nil {
		t.Fatal(err)
	}
	if err := a.Update(ctx, 10, 5, 1, "disable", ""); err != nil {
		t.Fatal(err)
	}
	if err := a.Save(-10, "needle private"); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(10, "needle other bot"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		source, ids string
		scanned     int
	}{{"", "4,3,2,1", 4}, {"agent", "3,1", 2}, {"admin", "2", 1}, {"legacy", "4", 1}} {
		p, err := a.SearchEnabledFromSource(ctx, 10, "needle", 20, 0, tc.source)
		ids := []string{}
		for _, item := range p.Memories {
			ids = append(ids, item.ID)
			if tc.source != "" && item.Source != tc.source {
				t.Fatal(item)
			}
		}
		if err != nil || strings.Join(ids, ",") != tc.ids || p.Scanned != tc.scanned || p.Source != tc.source {
			t.Fatal(tc, p, err)
		}
		if tc.source != "" {
			for _, keyword := range []string{"", " \t "} {
				all, err := a.SearchEnabledFromSource(ctx, 10, keyword, 20, 0, tc.source)
				if err != nil || len(all.Memories) != len(p.Memories) || all.Scanned != p.Scanned {
					t.Fatal("source-only scope lost", all, err)
				}
				for i, item := range all.Memories {
					if item.ID != p.Memories[i].ID || !item.Active || item.Source != tc.source {
						t.Fatal(item, p.Memories[i])
					}
				}
			}
		}
	}
	page, err := a.SearchEnabledFromSource(ctx, 10, "", 1, 0, "agent")
	if err != nil || len(page.Memories) != 1 || page.Memories[0].ID != "3" || !page.SearchMore {
		t.Fatal(page, err)
	}
	before, _ := strconv.ParseInt(page.NextBefore, 10, 64)
	page, err = a.SearchEnabledFromSource(ctx, 10, "", 1, before, "agent")
	if err != nil || len(page.Memories) != 1 || page.Memories[0].ID != "1" || page.SearchMore {
		t.Fatal(page, err)
	}
	if err := a.Update(ctx, 10, 3, 1, "edit", "needle edited by admin"); err != nil {
		t.Fatal(err)
	}
	page, err = a.SearchEnabledFromSource(ctx, 10, "", 20, 0, "agent")
	if err != nil || len(page.Memories) != 1 || page.Memories[0].ID != "1" {
		t.Fatal("old source revision searched", page, err)
	}
	page, err = a.SearchEnabledFromSource(ctx, 10, "", 20, 0, "admin")
	if err != nil || len(page.Memories) != 2 || page.Memories[0].Revision != "2" {
		t.Fatal(page, err)
	}
	if _, err = a.SearchEnabledFromSource(ctx, 10, "needle", 20, 0, "unknown"); !errors.Is(err, ErrMemoryInput) {
		t.Fatal(err)
	}
}

func TestAgentMemorySourceOnlyParameters(t *testing.T) {
	a, _ := memoryFixture(t)
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	for _, tc := range []struct {
		params zero.Params
		valid  bool
	}{
		{zero.Params{"source": "admin"}, true},
		{zero.Params{"source": "admin", "keyword": ""}, true},
		{zero.Params{"source": "admin", "keyword": " \t "}, true},
		{zero.Params{}, false},
		{zero.Params{"source": ""}, false},
		{zero.Params{"source": "unknown"}, false},
		{zero.Params{"before": "9007199254740993"}, false},
		{zero.Params{"source": "admin", "keyword": nil}, false},
		{zero.Params{"source": "admin", "keyword": 123}, false},
		{zero.Params{"source": "admin", "sender": "1"}, false},
	} {
		s := NewHistorySearch(456, 10)
		called := false
		s.memory = func(_ context.Context, bot, grp int64, keyword string, limit int, before int64, source, from, until string) (MemorySearchPage, error) {
			called = true
			if bot != 456 || grp != 10 || strings.TrimSpace(keyword) != "" || limit != 20 || before != 0 || source != "admin" {
				t.Fatal("source-only parameters lost")
			}
			return MemorySearchPage{Available: true, Mode: "managed", Source: source, Memories: []MemoryMatch{}}, nil
		}
		if err := s.Execute(&ag, zero.APIRequest{Action: goba.SearchMemory, Params: tc.params}); err != nil || called != tc.valid || s.Usage().Used != 1 || (s.Usage().Successful == 1) != tc.valid {
			t.Fatal(tc, err, s.Usage())
		}
	}
}

func TestAgentMemorySourceParametersReceiptAndQuota(t *testing.T) {
	a, _ := memoryFixture(t)
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	s := NewHistorySearch(456, 10)
	calls := 0
	s.memory = func(_ context.Context, bot, grp int64, keyword string, limit int, before int64, source, from, until string) (MemorySearchPage, error) {
		calls++
		if bot != 456 || grp != 10 || keyword != "needle" || limit != 1 || before != 9007199254740993 || source != "admin" {
			t.Fatal("source range lost")
		}
		return MemorySearchPage{Available: true, Mode: "managed", Source: source, Memories: []MemoryMatch{}}, nil
	}
	params := zero.Params{"keyword": "needle", "limit": json.Number("1"), "before": "9007199254740993", "source": "admin"}
	for i := 0; i < 4; i++ {
		err := s.Execute(&ag, zero.APIRequest{Action: goba.SearchMemory, Params: params})
		if (i < 3 && err != nil) || (i == 3 && err == nil) {
			t.Fatal(i, err)
		}
	}
	if calls != 3 || s.Usage().Used != 3 || s.Usage().Denied != 1 {
		t.Fatal(s.Usage())
	}
	var raw string
	if err := a.history.db.QueryRow(`SELECT payload FROM history WHERE kind='response' ORDER BY seq DESC LIMIT 1`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var response struct{ Data MemorySearchPage }
	if err := json.Unmarshal([]byte(raw), &response); err != nil || response.Data.Source != "admin" {
		t.Fatal(raw, err)
	}
	for _, req := range []zero.APIRequest{
		{Action: goba.SearchMemory, Params: zero.Params{"keyword": "needle", "source": "unknown"}},
		{Action: goba.SearchMemory, Params: zero.Params{"keyword": "needle", "source": 123}},
		{Action: goba.SearchMemory, Params: zero.Params{"keyword": "needle", "state": "all"}},
		{Action: goba.SearchHistory, Params: zero.Params{"keyword": "needle", "source": "admin"}},
		{Action: goba.ReadHistory, Params: zero.Params{"seq": "1", "source": "admin"}},
		{Action: goba.ReadMemory, Params: zero.Params{"id": "1", "revision": "1", "source": "admin"}},
	} {
		s := NewHistorySearch(456, 10)
		if err := s.Execute(&ag, req); err != nil || s.Usage().Errors != 1 || s.Usage().Used != 1 {
			t.Fatal(req, err, s.Usage())
		}
	}
}
