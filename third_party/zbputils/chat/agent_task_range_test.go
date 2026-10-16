package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"
)

func TestTaskStartRangeBeforePagingAndScope(t *testing.T) {
	s := taskFixture(t).tasks
	ctx := context.Background()
	for i := 0; i < 240; i++ {
		v := AgentTaskInfo{ID: fmt.Sprintf("%032x", i+1), Version: "1", Status: "failed", Error: "execution_error", Phase: "finished", StartedAt: "2026-10-15T00:00:00.000000001Z", Persisted: true}
		if i%2 == 0 {
			v.StartedAt = "2026-10-16T00:00:00Z"
		}
		if i >= 180 {
			v.StartedAt = "2026-10-17T00:00:00Z"
		}
		raw, _ := json.Marshal(v)
		if _, err := s.db.Exec(`INSERT INTO agent_tasks(seq,id,conversation,instance,status,payload) VALUES(?,?,10,'fixture',?,?)`, int64(9007199254740993)+int64(i), v.ID, v.Status, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	from, until := "2026-10-15T08:00:00.000000001+08:00", "2026-10-15T00:00:00.000000002Z"
	seen := map[string]bool{}
	before := int64(0)
	for {
		p, err := s.PageInRange(ctx, 10, before, "failed", "execution_error", "finished", from, until)
		if err != nil || len(p.Items) > 50 {
			t.Fatal(p, err)
		}
		for _, v := range p.Items {
			if seen[v.ID] || v.StartedAt != "2026-10-15T00:00:00.000000001Z" {
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
	if len(seen) != 90 {
		t.Fatal("range filtered after pagination", len(seen))
	}
	for _, tc := range []struct {
		grp                               int64
		state, reason, phase, from, until string
		count                             int
	}{
		{-10, "all", "all", "all", from, until, 0},
		{11, "all", "all", "all", from, until, 0},
		{10, "running", "all", "all", from, until, 0},
		{10, "all", "none", "all", from, until, 0},
		{10, "all", "all", "model_wait", from, until, 0},
		{10, "all", "all", "all", "", until, 50},
		{10, "all", "all", "all", "2026-10-17T00:00:00Z", "", 50},
		{10, "all", "all", "all", "", "2026-10-15T00:00:00.000000001Z", 0},
	} {
		p, err := s.PageInRange(ctx, tc.grp, 0, tc.state, tc.reason, tc.phase, tc.from, tc.until)
		if err != nil || len(p.Items) != tc.count {
			t.Fatal(tc, p, err)
		}
	}
	for _, bounds := range [][2]string{{"bad", ""}, {"2026-10-16T00:00:00", ""}, {until, from}, {from, from}, {"0001-01-01T00:00:00Z", ""}} {
		if _, err := s.PageInRange(ctx, 10, 0, "all", "all", "all", bounds[0], bounds[1]); !errors.Is(err, ErrAgentTaskConflict) {
			t.Fatal(bounds, err)
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.PageInRange(ctx, 10, 0, "all", "all", "all", from, until); !errors.Is(err, ErrAgentTaskStore) {
		t.Fatal(err)
	}
}

func TestTaskStartRangeUnknownAndLiveFault(t *testing.T) {
	s := taskFixture(t).tasks
	ctx := context.Background()
	for i, stamp := range []string{"", "bad", "2026-10-16T00:00:00.000000001Z", "2026-10-16T08:00:00.000000001+08:00", "2026-10-16T00:00:00.000000002Z"} {
		v := AgentTaskInfo{ID: fmt.Sprintf("%032x", i+1), Status: "completed", StartedAt: stamp, Persisted: true}
		raw, _ := json.Marshal(v)
		if _, err := s.db.Exec(`INSERT INTO agent_tasks(id,conversation,instance,status,payload) VALUES(?,20,'fixture','completed',?)`, v.ID, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	p, err := s.PageInRange(ctx, 20, 0, "all", "all", "all", "2026-10-16T00:00:00.000000001Z", "2026-10-16T00:00:00.000000002Z")
	if err != nil || len(p.Items) != 2 {
		t.Fatal(p, err)
	}
	p, err = s.Page(ctx, 20, 0)
	if err != nil || len(p.Items) != 5 {
		t.Fatal("unfiltered old records changed", p, err)
	}
	r, err := s.Begin(10)
	if err != nil {
		t.Fatal(err)
	}
	start, err := time.Parse(time.RFC3339Nano, r.info.StartedAt)
	if err != nil {
		t.Fatal(err)
	}
	from, until := start.Format(time.RFC3339Nano), start.Add(time.Nanosecond).Format(time.RFC3339Nano)
	if _, err = s.db.Exec(`CREATE TRIGGER reject_task_update BEFORE UPDATE ON agent_tasks BEGIN SELECT RAISE(ABORT,'synthetic'); END`); err != nil {
		t.Fatal(err)
	}
	if err = r.Model(); !errors.Is(err, ErrAgentTaskStore) {
		t.Fatal(err)
	}
	r.Finish(ErrAgentTaskStore)
	for _, missing := range []bool{false, true} {
		if missing {
			if _, err = s.db.Exec(`DELETE FROM agent_tasks WHERE conversation=10`); err != nil {
				t.Fatal(err)
			}
		}
		p, err = s.PageInRange(ctx, 10, 0, "failed", "storage_error", "finished", from, until)
		if err != nil || len(p.Items) != 1 || p.Items[0].Persisted {
			t.Fatal("live fault hidden", p, err)
		}
		p, err = s.PageInRange(ctx, 10, 0, "all", "all", "all", until, "")
		if err != nil || len(p.Items) != 0 {
			t.Fatal("live task escaped time filter", p, err)
		}
		p, err = s.PageInRange(ctx, 10, 1, "failed", "storage_error", "finished", from, until)
		if err != nil || len(p.Items) != 0 {
			t.Fatal("fault leaked into older page", p, err)
		}
	}
}
