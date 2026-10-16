package chat

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

func TestAgentTaskResultsSettleOnceAndPersist(t *testing.T) {
	s := taskFixture(t).tasks
	r, e := s.Begin(10)
	if e != nil {
		t.Fatal(e)
	}
	want := TaskResults{Available: true}
	if r.info.Results != want {
		t.Fatal(r.info.Results)
	}
	version := r.info.Version
	if e = r.Result("ok"); e != ErrAgentTaskConflict || r.info.Version != version {
		t.Fatal("unadmitted result counted", e)
	}
	for _, outcome := range []string{"ok", "ok", "rejected", "unknown", "local_error"} {
		if e = r.Model(); e != nil {
			t.Fatal(e)
		}
		if e = r.Action("send_group_msg"); e != nil {
			t.Fatal(e)
		}
		before := r.info
		for _, next := range []func() error{r.Model, r.FallbackModel, func() error { return r.Action("send_private_msg") }, func() error { return r.Result("invalid") }} {
			if e = next(); e != ErrAgentTaskConflict || r.info.Version != before.Version || r.info.Results != before.Results {
				t.Fatal("in-flight state overwritten", e, r.info)
			}
		}
		if e = r.Result(outcome); e != nil {
			t.Fatal(e)
		}
		switch outcome {
		case "ok":
			want.OK++
		case "rejected":
			want.Rejected++
		case "unknown":
			want.Unknown++
		case "local_error":
			want.LocalErrors++
		}
		version = r.info.Version
		if e = r.Result(outcome); e != ErrAgentTaskConflict || r.info.Results != want || r.info.Version != version {
			t.Fatal("duplicate result counted", e, r.info)
		}
	}
	if e = r.Finish(errors.New("fixture")); e != nil {
		t.Fatal(e)
	}
	if e = r.Finish(nil); e != nil {
		t.Fatal(e)
	}
	v, e := s.Read(context.Background(), 10, r.info.ID)
	if e != nil || v.Results != want || v.Status != "failed" {
		t.Fatal("persisted results lost", e, v)
	}
	page, e := s.PageFiltered(context.Background(), 10, 0, "failed")
	if e != nil || len(page.Items) != 1 || page.Items[0].Results != want {
		t.Fatal(e, page)
	}
	// A returned value cannot alter internal state.
	v.Results.OK = 999
	v, e = s.Read(context.Background(), 10, r.info.ID)
	if e != nil || v.Results != want {
		t.Fatal(e, v)
	}
	other, e := s.Begin(-10)
	if e != nil || other.info.Results != (TaskResults{Available: true}) {
		t.Fatal("scope inherited results", e)
	}
	other.Finish(nil)
	newRun, e := s.Begin(10)
	if e != nil || newRun.info.Results != (TaskResults{Available: true}) {
		t.Fatal("new task inherited results", e)
	}
	newRun.Finish(nil)
}

func TestAgentTaskResultsCancellationConcurrencyAndPendingFinish(t *testing.T) {
	s := taskFixture(t).tasks
	r, _ := s.Begin(10)
	if e := r.Action("send_group_msg"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Cancel(context.Background(), 10, r.info.ID, r.info.Version); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	results := make(chan error, 24)
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- r.Result("ok") }()
	}
	wg.Wait()
	close(results)
	accepted := 0
	for e := range results {
		if e == nil {
			accepted++
		} else if e != ErrAgentTaskConflict {
			t.Fatal(e)
		}
	}
	if accepted != 1 {
		t.Fatal("receipt settled more than once", accepted)
	}
	if e := r.Finish(ErrAgentCancelled); e != nil {
		t.Fatal(e)
	}
	v, e := s.Read(context.Background(), 10, r.info.ID)
	if e != nil || v.Status != "cancelled" || v.Results != (TaskResults{Available: true, OK: 1}) {
		t.Fatal("cancellation lost in-flight receipt", e, v)
	}
	if _, e = s.Cancel(context.Background(), 10, v.ID, "1"); e != nil {
		t.Fatal(e)
	}
	if e = r.Result("ok"); e != ErrAgentTaskConflict {
		t.Fatal(e)
	}
	pending, _ := s.Begin(10)
	if e = pending.Action("send_group_msg"); e != nil {
		t.Fatal(e)
	}
	if e = pending.Finish(errors.New("receipt missing")); e != nil {
		t.Fatal(e)
	}
	pending.Finish(nil)
	v, e = s.Read(context.Background(), 10, pending.info.ID)
	if e != nil || v.Outcome != "unknown" || v.Results != (TaskResults{Available: true, Unknown: 1}) {
		t.Fatal("missing receipt not counted once", e, v)
	}
	modelOnly, _ := s.Begin(10)
	modelOnly.Model()
	modelOnly.Finish(errors.New("model failure"))
	if modelOnly.info.Results != (TaskResults{Available: true}) {
		t.Fatal("model failure counted as an action result")
	}
}

func TestAgentTaskResultsRecoveryLegacyAndUncertainWrite(t *testing.T) {
	t.Run("recovery and legacy", func(t *testing.T) {
		h := taskFixture(t)
		r, _ := h.tasks.Begin(10)
		r.Action("send_group_msg")
		r.Result("ok")
		r.Action("send_group_msg")
		legacy, _ := h.tasks.Begin(-10)
		legacy.Action("send_private_msg")
		var old map[string]any
		raw, _ := json.Marshal(legacy.info)
		if e := json.Unmarshal(raw, &old); e != nil {
			t.Fatal(e)
		}
		delete(old, "results")
		raw, _ = json.Marshal(old)
		if _, e := h.db.Exec(`UPDATE agent_tasks SET payload=? WHERE id=?`, string(raw), legacy.info.ID); e != nil {
			t.Fatal(e)
		}
		recovered, e := newTaskStore(h.db, "results-restart")
		if e != nil {
			t.Fatal(e)
		}
		v, e := recovered.Read(context.Background(), 10, r.info.ID)
		if e != nil || v.Status != "interrupted" || v.Results != (TaskResults{Available: true, OK: 1, Unknown: 1}) {
			t.Fatal(e, v)
		}
		again, e := newTaskStore(h.db, "results-restart-again")
		if e != nil {
			t.Fatal(e)
		}
		v, e = again.Read(context.Background(), 10, r.info.ID)
		if e != nil || v.Results.Unknown != 1 {
			t.Fatal("recovery counted twice", e, v)
		}
		v, e = again.Read(context.Background(), -10, legacy.info.ID)
		if e != nil || v.Outcome != "unknown" || v.Results != (TaskResults{}) {
			t.Fatal("legacy results inferred", e, v)
		}
	})
	t.Run("uncertain result write", func(t *testing.T) {
		h := taskFixture(t)
		r, _ := h.tasks.Begin(10)
		r.Action("send_group_msg")
		if _, e := h.db.Exec(`CREATE TRIGGER reject_results BEFORE UPDATE ON agent_tasks BEGIN SELECT RAISE(ABORT,'fixture'); END`); e != nil {
			t.Fatal(e)
		}
		if e := r.Result("ok"); e != ErrAgentTaskStore {
			t.Fatal(e)
		}
		v, e := h.tasks.Read(context.Background(), 10, r.info.ID)
		if e != nil || v.Persisted || v.Results.OK != 1 {
			t.Fatal("uncertain receipt reported durable", e, v)
		}
		if e = r.Finish(nil); e != ErrAgentTaskStore {
			t.Fatal(e)
		}
		if _, e = h.db.Exec(`DROP TRIGGER reject_results`); e != nil {
			t.Fatal(e)
		}
		recovered, e := newTaskStore(h.db, "results-after-failure")
		if e != nil {
			t.Fatal(e)
		}
		v, e = recovered.Read(context.Background(), 10, r.info.ID)
		if e != nil || !v.Persisted || v.Results != (TaskResults{Available: true, Unknown: 1}) {
			t.Fatal("unwritten success invented after restart", e, v)
		}
	})
}
