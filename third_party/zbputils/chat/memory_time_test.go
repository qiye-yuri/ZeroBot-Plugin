package chat

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func TestMemoryRevisionTimeRangeScopeAndPaging(t *testing.T) {
	a, b := memoryFixture(t)
	ctx := context.Background()
	for i := 0; i < 6; i++ {
		if e := a.Save(10, "needle "+strconv.Itoa(i)); e != nil {
			t.Fatal(e)
		}
	}
	if e := a.Update(ctx, 10, 2, 1, "disable", ""); e != nil {
		t.Fatal(e)
	}
	if e := a.Update(ctx, 10, 6, 1, "edit", "needle edited synthetic-agent-key"); e != nil {
		t.Fatal(e)
	}
	stamps := []string{"2026-10-16T00:00:00Z", "2026-10-16T08:00:00.000000001+08:00", "2026-10-16T00:00:00.000000002Z", "2026-10-16T00:00:00.000000003Z", "unknown", "2026-10-16T00:00:00.000000001Z"}
	for i, stamp := range stamps {
		if _, e := a.history.db.Exec(`UPDATE agent_memories SET updated_at=? WHERE id=?`, stamp, i+1); e != nil {
			t.Fatal(e)
		}
	}
	from, until := "2026-10-16T08:00:00.000000001+08:00", "2026-10-16T00:00:00.000000003Z"
	ids := func(p MemorySearchPage) string {
		values := []string{}
		for _, m := range p.Memories {
			values = append(values, m.ID)
			if strings.Contains(m.Text, "synthetic-agent-key") {
				t.Fatal("unredacted")
			}
		}
		return strings.Join(values, ",")
	}
	for _, tc := range []struct{ source, state, want string }{{"", "active", "6,3"}, {"", "all", "6,3,2"}, {"", "disabled", "2"}, {"agent", "all", "3,2"}, {"admin", "all", "6"}} {
		p, e := a.SearchCurrentInRange(ctx, 10, "", 20, 0, tc.source, tc.state, from, until)
		if e != nil || ids(p) != tc.want || p.From != "2026-10-16T00:00:00.000000001Z" || p.Until != until || p.State != tc.state {
			t.Fatal(tc, p, e)
		}
		if tc.source == "" && tc.state == "all" && (!p.Partial || p.Scanned != 6) {
			t.Fatal("unknown time was inferred", p)
		}
	}
	for _, bounds := range [][2]string{{from, until}, {from, ""}, {"", until}} {
		p, e := a.SearchEnabledInRange(ctx, 10, "", 20, 0, "", bounds[0], bounds[1])
		if e != nil || p.State != "" || len(p.Memories) == 0 {
			t.Fatal(p, e)
		}
	}
	seen := []string{}
	before := int64(0)
	for i := 0; i < 6; i++ {
		p, e := a.SearchEnabledInRange(ctx, 10, "", 1, before, "", from, until)
		if e != nil {
			t.Fatal(e)
		}
		for _, m := range p.Memories {
			seen = append(seen, m.ID)
		}
		if !p.SearchMore {
			break
		}
		next, e := strconv.ParseInt(p.NextBefore, 10, 64)
		if e != nil || next <= 0 || (before > 0 && next >= before) {
			t.Fatal(p, e)
		}
		before = next
	}
	if strings.Join(seen, ",") != "6,3" {
		t.Fatal("lost continuation", seen)
	}
	for _, bounds := range [][2]string{{"bad", ""}, {"2026-10-16T00:00:00", ""}, {until, from}, {from, from}, {"0001-01-01T00:00:00Z", ""}} {
		if _, e := a.SearchEnabledInRange(ctx, 10, "needle", 20, 0, "", bounds[0], bounds[1]); e != ErrMemoryInput {
			t.Fatal(bounds, e)
		}
	}
	for _, store := range []*MemoryStore{a, b} {
		p, e := store.SearchEnabledInRange(ctx, -10, "", 20, 0, "", from, until)
		if e != nil || len(p.Memories) != 0 {
			t.Fatal("scope", p, e)
		}
	}
	if e := a.Update(ctx, 10, 6, 2, "edit", "needle changed again"); e != nil {
		t.Fatal(e)
	}
	if _, e := a.history.db.Exec(`UPDATE agent_memories SET updated_at=? WHERE id=6`, until); e != nil {
		t.Fatal(e)
	}
	p, e := a.SearchEnabledInRange(ctx, 10, "", 20, 0, "", from, until)
	if e != nil || ids(p) != "3" {
		t.Fatal("old revision remained in range", p, e)
	}
	p, e = a.SearchEnabledBefore(ctx, 10, "needle", 20, 0)
	if e != nil || len(p.Memories) != 5 || p.From != "" || p.Until != "" {
		t.Fatal("old search changed", p, e)
	}
}

func TestMemoryTimeRangeKeepsScanBudgetAndEmptyContinuation(t *testing.T) {
	a, _ := memoryFixture(t)
	if e := a.Save(10, "needle target"); e != nil {
		t.Fatal(e)
	}
	if _, e := a.history.db.Exec(`UPDATE agent_memories SET updated_at='2026-10-16T00:00:00Z' WHERE id=1`); e != nil {
		t.Fatal(e)
	}
	tx, e := a.history.db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 5001; i++ {
		if _, e = tx.Exec(`INSERT INTO agent_memories(conversation,text,source,source_seq,active,revision,created_at,updated_at) VALUES(10,'needle old','agent',0,1,1,'old','2026-10-15T00:00:00Z')`); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	p, e := a.SearchEnabledInRange(context.Background(), 10, "", 20, 0, "", "2026-10-16T00:00:00Z", "")
	if e != nil || len(p.Memories) != 0 || p.Scanned != 5000 || !p.SearchMore || !p.Partial || p.NextBefore != "3" {
		t.Fatal(p, e)
	}
	p, e = a.SearchEnabledInRange(context.Background(), 10, "", 20, 3, "", "2026-10-16T00:00:00Z", "")
	if e != nil || len(p.Memories) != 1 || p.Memories[0].ID != "1" || p.SearchMore {
		t.Fatal(p, e)
	}
}

func TestAgentMemoryRevisionTimeValidationAndQuota(t *testing.T) {
	h, task, ag, s := queryTaskFixture(t)
	from, until := "2026-10-16T08:00:00.000000001+08:00", "2026-10-16T00:00:00.000000003Z"
	calls := 0
	s.memory = func(_ context.Context, bot, grp int64, keyword string, limit int, before int64, source, start, end string) (MemorySearchPage, error) {
		calls++
		if bot != 456 || grp != 10 || keyword != "" || limit != 1 || before != 9007199254740993 || source != "admin" || start != from || end != until {
			t.Fatal("binding changed")
		}
		return MemorySearchPage{Available: true, Mode: "managed", Partial: true, From: "2026-10-16T00:00:00.000000001Z", Until: until, SearchMore: true, NextBefore: "9007199254740992", Memories: []MemoryMatch{}}, nil
	}
	req := zero.APIRequest{Action: goba.SearchMemory, Params: zero.Params{"from": from, "until": until, "source": "admin", "limit": json.Number("1"), "before": "9007199254740993"}}
	execute := func(req zero.APIRequest) error {
		if e := task.Action(req.Action); e != nil {
			return e
		}
		e := s.Execute(ag, req)
		if done := task.Result(s.Outcome()); done != nil {
			t.Fatal(done)
		}
		return e
	}
	if e := execute(req); e != nil || s.Outcome() != "ok" {
		t.Fatal(e)
	}
	for _, params := range []zero.Params{{"from": true}, {"from": until, "until": from}} {
		if e := execute(zero.APIRequest{Action: goba.SearchMemory, Params: params}); e != nil || s.Outcome() != "local_error" {
			t.Fatal(e)
		}
	}
	if e := execute(req); e == nil || calls != 1 {
		t.Fatal("quota changed", e, calls)
	}
	v, e := h.tasks.Read(context.Background(), 10, task.info.ID)
	if e != nil || v.Queries.Used != 3 || v.Queries.SearchMemory != 3 || v.Queries.Successful != 1 || v.Queries.Empty != 1 || v.Queries.Partial != 1 || v.Queries.Errors != 2 || v.Queries.Denied != 1 {
		t.Fatal(v, e)
	}
	rows, _, _ := h.recent(10)
	found := false
	for _, r := range rows {
		if strings.Contains(r.payload, `"from":"2026-10-16T00:00:00.000000001Z"`) {
			found = true
		}
	}
	if !found {
		t.Fatal("UTC receipt missing")
	}
}
