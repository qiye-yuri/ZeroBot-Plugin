package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"
)

func TestTaskPhaseFiltersBeforePaging(t *testing.T) {
	s := taskFixture(t).tasks
	ctx := context.Background()
	for i := 0; i < 180; i++ {
		v := AgentTaskInfo{ID: fmt.Sprintf("%032x", i+1), Version: "1", Status: "cancel_requested", Phase: "model_wait", Persisted: true}
		if i%3 == 0 {
			v.Phase = "action_wait"
		}
		raw, _ := json.Marshal(v)
		if _, e := s.db.Exec(`INSERT INTO agent_tasks(seq,id,conversation,instance,status,payload) VALUES(?,?,10,'fixture',?,?)`, int64(9007199254740993)+int64(i), v.ID, v.Status, string(raw)); e != nil {
			t.Fatal(e)
		}
	}
	seen := map[string]bool{}
	before := int64(0)
	for {
		p, e := s.PageWithFilters(ctx, 10, before, "cancel_requested", "none", "model_wait")
		if e != nil || len(p.Items) > 50 {
			t.Fatal(p, e)
		}
		for _, v := range p.Items {
			if seen[v.ID] || v.Phase != "model_wait" || v.Status != "cancel_requested" {
				t.Fatal(v)
			}
			seen[v.ID] = true
		}
		if !p.More {
			break
		}
		next, e := strconv.ParseInt(p.Next, 10, 64)
		if e != nil || next <= 9007199254740992 || (before > 0 && next >= before) {
			t.Fatal("inexact pagination", p.Next, e)
		}
		before = next
	}
	if len(seen) != 120 {
		t.Fatal("missing filtered tasks", len(seen))
	}
	for _, tc := range []struct {
		state, reason, phase string
		grp                  int64
		count                int
	}{
		{"all", "all", "action_wait", 10, 50},
		{"running", "all", "model_wait", 10, 0},
		{"all", "storage_error", "model_wait", 10, 0},
		{"all", "all", "model_wait", -10, 0},
		{"all", "all", "model_wait", 11, 0},
	} {
		p, e := s.PageWithFilters(ctx, tc.grp, 0, tc.state, tc.reason, tc.phase)
		if e != nil || len(p.Items) != tc.count {
			t.Fatal(tc, p, e)
		}
	}
	for _, phase := range []string{"", "bad", "MODEL_WAIT", "model_wait' OR 1=1--"} {
		if _, e := s.PageWithFilters(ctx, 10, 0, "all", "all", phase); !errors.Is(e, ErrAgentTaskConflict) {
			t.Fatal(phase, e)
		}
	}
	// Old summaries with no phase remain visible in an unfiltered list.
	if _, e := s.db.Exec(`INSERT INTO agent_tasks(id,conversation,instance,status,payload) VALUES('legacy',20,'fixture','completed','{"taskId":"legacy","status":"completed"}')`); e != nil {
		t.Fatal(e)
	}
	for _, phase := range []string{"all", "finished"} {
		p, e := s.PageWithFilters(ctx, 20, 0, "all", "all", phase)
		want := 0
		if phase == "all" {
			want = 1
		}
		if e != nil || len(p.Items) != want {
			t.Fatal(phase, p, e)
		}
	}
}

func TestTaskPhaseUsesLiveSnapshotAfterFailedWrite(t *testing.T) {
	s := taskFixture(t).tasks
	r, e := s.Begin(10)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Exec(`CREATE TRIGGER reject_task_update BEFORE UPDATE ON agent_tasks BEGIN SELECT RAISE(ABORT,'synthetic'); END`); e != nil {
		t.Fatal(e)
	}
	if e = r.Model(); !errors.Is(e, ErrAgentTaskStore) {
		t.Fatal(e)
	}
	live, e := s.PageWithFilters(context.Background(), 10, 0, "running", "storage_error", "model_wait")
	if e != nil || len(live.Items) != 1 || live.Items[0].Persisted {
		t.Fatal("live phase lost after failed save", live, e)
	}
	r.Finish(ErrAgentTaskStore)
	for _, tc := range []struct {
		state, reason, phase string
		count                int
	}{
		{"failed", "storage_error", "finished", 1},
		{"all", "all", "starting", 0},
		{"running", "none", "finished", 0},
	} {
		p, e := s.PageWithFilters(context.Background(), 10, 0, tc.state, tc.reason, tc.phase)
		if e != nil || len(p.Items) != tc.count {
			t.Fatal(tc, p, e)
		}
		if tc.count > 0 && (p.Items[0].Persisted || p.Items[0].ID != r.info.ID) {
			t.Fatal(p)
		}
	}
	if _, e = s.db.Exec(`DELETE FROM agent_tasks`); e != nil {
		t.Fatal(e)
	}
	p, e := s.PageWithFilters(context.Background(), 10, 0, "failed", "storage_error", "finished")
	if e != nil || len(p.Items) != 1 {
		t.Fatal("unsaved snapshot hidden", p, e)
	}
	p, e = s.PageWithFilters(context.Background(), 10, 1, "failed", "storage_error", "finished")
	if e != nil || len(p.Items) != 0 {
		t.Fatal("snapshot leaked to older page", p, e)
	}
}
