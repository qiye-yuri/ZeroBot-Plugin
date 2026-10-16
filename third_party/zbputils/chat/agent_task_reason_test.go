package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"

	goba "github.com/fumiama/go-onebot-agent"
)

func TestTaskReasonPagingLegacyAndScope(t *testing.T) {
	s := taskFixture(t).tasks
	ctx := context.Background()
	want := map[string]map[string]bool{}
	for i := 0; i < 240; i++ {
		reason := []string{"model_request_failed", "model_output_invalid", "old_reason"}[i%3]
		state := "failed"
		if i%10 == 0 {
			state = "interrupted"
		}
		id := fmt.Sprintf("%032x", i+1)
		v := AgentTaskInfo{ID: id, Version: "9007199254740993", Status: state, Error: reason, Persisted: true}
		raw, _ := json.Marshal(v)
		if _, e := s.db.Exec(`INSERT INTO agent_tasks(seq,id,conversation,instance,status,payload) VALUES(?,?,10,'fixture',?,?)`, int64(9007199254740993)+int64(i), id, state, string(raw)); e != nil {
			t.Fatal(e)
		}
		key := state + ":" + reason
		if want[key] == nil {
			want[key] = map[string]bool{}
		}
		want[key][id] = true
	}
	for _, tc := range []struct{ state, reason, actual string }{
		{"failed", "model_request_failed", "model_request_failed"}, {"failed", "model_output_invalid", "model_output_invalid"},
		{"failed", "other", "old_reason"}, {"interrupted", "other", "old_reason"},
	} {
		seen := map[string]bool{}
		before := int64(0)
		for {
			p, e := s.PageWithReason(ctx, 10, before, tc.state, tc.reason)
			if e != nil {
				t.Fatal(e)
			}
			if len(p.Items) > 50 {
				t.Fatal("unbounded page")
			}
			for _, item := range p.Items {
				if seen[item.ID] || !want[tc.state+":"+tc.actual][item.ID] || item.Error != tc.actual || item.Status != tc.state {
					t.Fatal("scope/filter/duplicate", item)
				}
				seen[item.ID] = true
			}
			if !p.More {
				break
			}
			next, e := strconv.ParseInt(p.Next, 10, 64)
			if e != nil || next <= 9007199254740992 || (before > 0 && next >= before) {
				t.Fatal("inexact cursor", p.Next, e)
			}
			before = next
		}
		if len(seen) != len(want[tc.state+":"+tc.actual]) {
			t.Fatal("lost filtered rows", tc, len(seen))
		}
	}
	// Missing old fields mean unrecorded, never successful. Reserved selector
	// words in legacy error strings are unknown categories, not selectors.
	for i, reason := range []string{"", "all", "none", "other", "storage_error", "settings_changed", "repeated_action", "model_output_limit", "process_interrupted", "execution_error"} {
		v := map[string]any{"taskId": fmt.Sprintf("%032x", 1000+i), "status": "failed", "persisted": true}
		if reason != "" {
			v["error"] = reason
		}
		raw, _ := json.Marshal(v)
		if _, e := s.db.Exec(`INSERT INTO agent_tasks(id,conversation,instance,status,payload) VALUES(?,-10,'fixture','failed',?)`, v["taskId"], string(raw)); e != nil {
			t.Fatal(e)
		}
	}
	for _, tc := range []struct {
		reason string
		count  int
	}{{"none", 1}, {"other", 3}, {"all", 10}, {"storage_error", 1}, {"settings_changed", 1}, {"repeated_action", 1}, {"model_output_limit", 1}, {"process_interrupted", 1}, {"execution_error", 1}} {
		p, e := s.PageWithReason(ctx, -10, 0, "all", tc.reason)
		if e != nil || len(p.Items) != tc.count {
			t.Fatal(tc, p, e)
		}
	}
	for _, reason := range []string{"", "bad", "MODEL_REQUEST_FAILED", "model_request_failed' OR 1=1--"} {
		if _, e := s.PageWithReason(ctx, 10, 0, "all", reason); !errors.Is(e, ErrAgentTaskConflict) {
			t.Fatal("invalid reason", reason, e)
		}
	}
	p, e := s.PageWithReason(ctx, 11, 0, "all", "other")
	if e != nil || len(p.Items) != 0 {
		t.Fatal("cross-scope", p, e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = s.PageWithReason(cancelled, 10, 0, "all", "other"); e == nil {
		t.Fatal("cancelled read succeeded")
	}
}

func TestTaskReasonUsesActualMemoryAfterFailedWrites(t *testing.T) {
	s := taskFixture(t).tasks
	for i := 0; i < 70; i++ {
		r, e := s.Begin(10)
		if e != nil {
			t.Fatal(e)
		}
		if e = r.Finish(goba.ErrModelRequestFailed); e != nil {
			t.Fatal(e)
		}
	}
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
	r.Finish(ErrAgentTaskStore)
	for _, tc := range []struct {
		state, reason string
		count         int
	}{{"failed", "storage_error", 1}, {"all", "storage_error", 1}, {"running", "none", 0}, {"failed", "none", 0}, {"failed", "model_output_invalid", 0}} {
		p, e := s.PageWithReason(context.Background(), 10, 0, tc.state, tc.reason)
		if e != nil || len(p.Items) != tc.count {
			t.Fatal(tc, p, e)
		}
		if tc.count > 0 && (p.Items[0].Persisted || p.Items[0].Error != "storage_error") {
			t.Fatal(p)
		}
	}
	p, e := s.PageWithReason(context.Background(), 10, 1, "failed", "storage_error")
	if e != nil || len(p.Items) != 0 {
		t.Fatal("overlay leaked to older page", p, e)
	}
	p, e = s.PageWithReason(context.Background(), 10, 0, "failed", "model_request_failed")
	if e != nil || len(p.Items) != 50 || !p.More {
		t.Fatal("overlay consumed matching page", p, e)
	}
	before, e := strconv.ParseInt(p.Next, 10, 64)
	if e != nil {
		t.Fatal(e)
	}
	older, e := s.PageWithReason(context.Background(), 10, before, "failed", "model_request_failed")
	if e != nil || len(older.Items) != 20 || older.More {
		t.Fatal("overlay lost older matches", older, e)
	}
	if _, e = s.db.Exec(`CREATE TRIGGER reject_task_insert BEFORE INSERT ON agent_tasks BEGIN SELECT RAISE(ABORT,'synthetic'); END`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Begin(20); !errors.Is(e, ErrAgentTaskStore) {
		t.Fatal(e)
	}
	p, e = s.PageWithReason(context.Background(), 20, 0, "failed", "storage_error")
	if e != nil || len(p.Items) != 1 || p.Items[0].Persisted {
		t.Fatal("unsaved task hidden", p, e)
	}
}

func TestTaskReasonRealFailuresAndCorruptPayload(t *testing.T) {
	s := taskFixture(t).tasks
	for _, tc := range []struct {
		err    error
		reason string
	}{{goba.ErrModelRequestFailed, "model_request_failed"}, {goba.ErrActionOutputInvalid, "model_output_invalid"}, {goba.ErrActionOutputLimit, "model_output_limit"}, {ErrAgentReloaded, "settings_changed"}, {ErrAgentRepeatedAction, "repeated_action"}, {errors.New("synthetic"), "execution_error"}} {
		r, e := s.Begin(10)
		if e != nil {
			t.Fatal(e)
		}
		if e = r.Finish(tc.err); e != nil {
			t.Fatal(e)
		}
		p, e := s.PageWithReason(context.Background(), 10, 0, "failed", tc.reason)
		if e != nil || len(p.Items) != 1 || p.Items[0].ID != r.info.ID {
			t.Fatal(tc.reason, p, e)
		}
	}
	if _, e := s.db.Exec(`UPDATE agent_tasks SET payload='invalid-json' WHERE conversation=10`); e != nil {
		t.Fatal(e)
	}
	if _, e := s.PageWithReason(context.Background(), 10, 0, "failed", "other"); !errors.Is(e, ErrAgentTaskStore) {
		t.Fatal("corruption looked empty", e)
	}
}
