package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"
)

func TestTaskOutcomeFiltersBeforePaging(t *testing.T) {
	s := taskFixture(t).tasks
	ctx := context.Background()
	for i := 0; i < 240; i++ {
		v := AgentTaskInfo{ID: fmt.Sprintf("%032x", i+1), Version: "1", Status: "failed", Error: "execution_error", Phase: "finished", Outcome: "ok", StartedAt: "2026-10-16T00:00:00.000000001Z", Persisted: true}
		if i%3 == 0 {
			v.Outcome = "unknown"
		}
		if i >= 180 {
			v.StartedAt = "2026-10-17T00:00:00Z"
		}
		raw, _ := json.Marshal(v)
		if _, err := s.db.Exec(`INSERT INTO agent_tasks(seq,id,conversation,instance,status,payload) VALUES(?,?,10,'fixture',?,?)`, int64(9007199254740993)+int64(i), v.ID, v.Status, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	filter := TaskPageFilter{State: "failed", Reason: "execution_error", Phase: "finished", Outcome: "unknown", From: "2026-10-16T08:00:00.000000001+08:00", Until: "2026-10-16T00:00:00.000000002Z"}
	seen := map[string]bool{}
	before := int64(0)
	for {
		p, err := s.PageMatching(ctx, 10, before, filter)
		if err != nil || len(p.Items) > 50 {
			t.Fatal(p, err)
		}
		for _, v := range p.Items {
			if seen[v.ID] || v.Outcome != "unknown" || v.StartedAt != "2026-10-16T00:00:00.000000001Z" {
				t.Fatal(v)
			}
			seen[v.ID] = true
		}
		if !p.More {
			break
		}
		next, err := strconv.ParseInt(p.Next, 10, 64)
		if err != nil || next <= 9007199254740992 || (before > 0 && next >= before) {
			t.Fatal(p, err)
		}
		before = next
	}
	if len(seen) != 60 {
		t.Fatal("outcome/time filtering missed tasks", len(seen))
	}
	filter.From, filter.Until = "", ""
	first, err := s.PageMatching(ctx, 10, 0, filter)
	if err != nil || len(first.Items) != 50 || !first.More {
		t.Fatal(first, err)
	}
	next, err := strconv.ParseInt(first.Next, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	last, err := s.PageMatching(ctx, 10, next, filter)
	if err != nil || len(last.Items) != 30 || last.More {
		t.Fatal(last, err)
	}
	all := map[string]bool{}
	for _, v := range append(first.Items, last.Items...) {
		if all[v.ID] || v.Outcome != "unknown" {
			t.Fatal(v)
		}
		all[v.ID] = true
	}
	for _, grp := range []int64{-10, 11} {
		p, err := s.PageMatching(ctx, grp, 0, filter)
		if err != nil || len(p.Items) != 0 {
			t.Fatal(grp, p, err)
		}
	}
	for _, outcome := range []string{"", "bad", "UNKNOWN", "unknown' OR 1=1--"} {
		filter.Outcome = outcome
		if _, err := s.PageMatching(ctx, 10, 0, filter); !errors.Is(err, ErrAgentTaskConflict) {
			t.Fatal(outcome, err)
		}
	}
}

func TestTaskOutcomeMissingOldAndCumulativeResults(t *testing.T) {
	s := taskFixture(t).tasks
	ctx := context.Background()
	for i, outcome := range []string{"", "in_flight", "ok", "rejected", "unknown", "local_error", "legacy", "all", "none", "other"} {
		v := AgentTaskInfo{ID: fmt.Sprintf("%032x", i+1), Status: "completed", Outcome: outcome, Results: TaskResults{Available: true, Unknown: 3}}
		raw, _ := json.Marshal(v)
		if _, err := s.db.Exec(`INSERT INTO agent_tasks(id,conversation,instance,status,payload) VALUES(?,10,'fixture','completed',?)`, v.ID, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec(`INSERT INTO agent_tasks(id,conversation,instance,status,payload) VALUES('legacy-missing',10,'fixture','completed','{"taskId":"legacy-missing","status":"completed"}')`); err != nil {
		t.Fatal(err)
	}
	filter := TaskPageFilter{State: "completed", Reason: "all", Phase: "all", Outcome: "all"}
	for _, tc := range []struct {
		outcome string
		count   int
	}{{"all", 11}, {"none", 2}, {"other", 4}, {"in_flight", 1}, {"ok", 1}, {"rejected", 1}, {"unknown", 1}, {"local_error", 1}} {
		filter.Outcome = tc.outcome
		p, err := s.PageMatching(ctx, 10, 0, filter)
		if err != nil || len(p.Items) != tc.count {
			t.Fatal(tc, p, err)
		}
		if tc.outcome == "unknown" && p.Items[0].Outcome != "unknown" {
			t.Fatal("cumulative results used as last outcome", p)
		}
	}
	p, err := s.Page(ctx, 10, 0)
	if err != nil || len(p.Items) != 11 {
		t.Fatal("old unfiltered behavior changed", p, err)
	}
}

func TestTaskOutcomeUsesLiveSnapshotAfterFailedWrite(t *testing.T) {
	s := taskFixture(t).tasks
	ctx := context.Background()
	r, err := s.Begin(10)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Action("send_group_msg"); err != nil {
		t.Fatal(err)
	}
	filter := TaskPageFilter{State: "running", Reason: "all", Phase: "action_wait", Outcome: "in_flight"}
	p, err := s.PageMatching(ctx, 10, 0, filter)
	if err != nil || len(p.Items) != 1 {
		t.Fatal(p, err)
	}
	if _, err = s.db.Exec(`CREATE TRIGGER reject_task_update BEFORE UPDATE ON agent_tasks BEGIN SELECT RAISE(ABORT,'synthetic'); END`); err != nil {
		t.Fatal(err)
	}
	if err = r.Result("rejected"); !errors.Is(err, ErrAgentTaskStore) {
		t.Fatal(err)
	}
	r.Finish(ErrAgentTaskStore)
	filter = TaskPageFilter{State: "failed", Reason: "storage_error", Phase: "finished", Outcome: "rejected", From: r.info.StartedAt}
	for _, missing := range []bool{false, true} {
		if missing {
			if _, err = s.db.Exec(`DELETE FROM agent_tasks`); err != nil {
				t.Fatal(err)
			}
		}
		p, err = s.PageMatching(ctx, 10, 0, filter)
		if err != nil || len(p.Items) != 1 || p.Items[0].Persisted || p.Items[0].Outcome != "rejected" {
			t.Fatal("live rejected outcome hidden", p, err)
		}
		version := p.Items[0].Version
		filter.Outcome = "in_flight"
		p, err = s.PageMatching(ctx, 10, 0, filter)
		if err != nil || len(p.Items) != 0 {
			t.Fatal("stale saved outcome leaked", p, err)
		}
		filter.Outcome = "rejected"
		p, err = s.PageMatching(ctx, 10, 1, filter)
		if err != nil || len(p.Items) != 0 {
			t.Fatal("fault leaked to older page", p, err)
		}
		if r.info.Version != version {
			t.Fatal("read changed version")
		}
	}
}
