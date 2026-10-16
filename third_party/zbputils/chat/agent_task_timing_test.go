package chat

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

func taskStamp(t *testing.T, value string) time.Time {
	t.Helper()
	stamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || stamp.Location() != time.UTC {
		t.Fatalf("invalid UTC task timestamp %q: %v", value, err)
	}
	return stamp
}

// Windows may return the same wall timestamp for consecutive calls. When a
// test needs distinct samples, observe a clock tick before entering the stage.
func waitTaskClockAdvance(t *testing.T, previous time.Time) {
	t.Helper()
	started := time.Now()
	for !time.Now().UTC().After(previous) {
		if time.Since(started) > time.Second {
			t.Fatal("task wall clock did not advance for timing test")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestAgentTaskPhaseTimesAndReadOnlyUpdates(t *testing.T) {
	s := taskFixture(t).tasks
	r, err := s.Begin(10)
	if err != nil {
		t.Fatal(err)
	}
	if r.info.PhaseStartedAt != r.info.StartedAt || r.info.CancelRequestedAt != "" {
		t.Fatal(r.info)
	}
	previous := taskStamp(t, r.info.PhaseStartedAt)
	for _, enter := range []func() error{r.Model, r.Model, func() error { return r.Action("search_history") }, func() error { return r.Result("ok") }, r.FallbackModel} {
		waitTaskClockAdvance(t, previous)
		if err = enter(); err != nil {
			t.Fatal(err)
		}
		stamp := r.info.PhaseStartedAt
		now := taskStamp(t, stamp)
		if !now.After(previous) {
			t.Fatal("new stage did not get a new timestamp", r.info)
		}
		previous = now
		if err = s.capture(10, ContextUsage{Available: true, Records: 1}); err != nil {
			t.Fatal(err)
		}
		if err = s.captureMemory(10, MemoryUsage{Available: true, Mode: "managed"}); err != nil {
			t.Fatal(err)
		}
		v, err := s.Read(context.Background(), 10, r.info.ID)
		if err != nil || v.PhaseStartedAt != stamp {
			t.Fatal("read or metadata reset phase time", err, v)
		}
		page, err := s.Page(context.Background(), 10, 0)
		if err != nil || len(page.Items) != 1 || page.Items[0].PhaseStartedAt != stamp {
			t.Fatal(err, page)
		}
	}
	waitTaskClockAdvance(t, previous)
	if err = r.Finish(nil); err != nil {
		t.Fatal(err)
	}
	if r.info.PhaseStartedAt != r.info.EndedAt || !taskStamp(t, r.info.EndedAt).After(previous) {
		t.Fatal(r.info)
	}
	v, err := s.Read(context.Background(), 10, r.info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Finish(errors.New("late")); err != nil || r.info != v { // Context has a nil pointer in this fixture.
		t.Fatal("duplicate finish rewrote timing", err)
	}
	other, err := s.Begin(-10)
	if err != nil || other.info.CancelRequestedAt != "" {
		t.Fatal("scope inherited a stop time", err)
	}
	other.Finish(nil)
}

func TestAgentTaskFirstStopTimeConcurrentAndInFlight(t *testing.T) {
	s := taskFixture(t).tasks
	r, _ := s.Begin(10)
	if err := r.Action("read_history"); err != nil {
		t.Fatal(err)
	}
	before := r.info
	if _, err := s.Cancel(context.Background(), 10, r.info.ID, "0"); err != ErrAgentTaskConflict || r.info.CancelRequestedAt != "" {
		t.Fatal("rejected stop stamped", err)
	}
	var wg sync.WaitGroup
	stamps := make(chan AgentTaskInfo, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := s.Cancel(context.Background(), 10, before.ID, before.Version)
			if err != nil {
				t.Error(err)
			}
			stamps <- v
		}()
	}
	wg.Wait()
	close(stamps)
	first := r.info.CancelRequestedAt
	taskStamp(t, first)
	for v := range stamps {
		if v.CancelRequestedAt != first || v.PhaseStartedAt != before.PhaseStartedAt {
			t.Fatal("duplicate stop reset time", v)
		}
	}
	if err := r.Model(); err != ErrAgentCancelled || r.info.PhaseStartedAt != before.PhaseStartedAt {
		t.Fatal("cancelled admission changed time", err)
	}
	q := TaskQueries{Available: true, Limit: 3, Used: 1, ReadHistory: 1, Successful: 1}
	if err := r.captureQueries(q); err != nil || r.info.PhaseStartedAt != before.PhaseStartedAt {
		t.Fatal("query summary changed time", err)
	}
	waitTaskClockAdvance(t, taskStamp(t, before.PhaseStartedAt))
	if err := r.Result("ok"); err != nil {
		t.Fatal(err)
	}
	if r.info.CancelRequestedAt != first || !taskStamp(t, r.info.PhaseStartedAt).After(taskStamp(t, before.PhaseStartedAt)) {
		t.Fatal(r.info)
	}
	if err := r.Finish(nil); err != nil {
		t.Fatal(err)
	}
	v, err := s.Cancel(context.Background(), 10, before.ID, before.Version)
	if err != nil || v.CancelRequestedAt != first || v.PhaseStartedAt != v.EndedAt || v.Status != "cancelled" {
		t.Fatal(err, v)
	}
	v, err = s.Read(context.Background(), 10, before.ID)
	if err != nil || v.CancelRequestedAt != first {
		t.Fatal("persisted stop time lost", err, v)
	}
}

func TestAgentTaskTimingRecoveryLegacyAndWriteFailure(t *testing.T) {
	h := taskFixture(t)
	s := h.tasks
	r, _ := s.Begin(10)
	r.Model()
	stopped, err := s.Cancel(context.Background(), 10, r.info.ID, r.info.Version)
	if err != nil {
		t.Fatal(err)
	}
	legacy, _ := s.Begin(-10)
	raw, _ := json.Marshal(legacy.info)
	var old map[string]any
	json.Unmarshal(raw, &old)
	delete(old, "phaseStartedAt")
	delete(old, "cancelRequestedAt")
	raw, _ = json.Marshal(old)
	if _, err = s.db.Exec(`UPDATE agent_tasks SET payload=? WHERE id=?`, string(raw), legacy.info.ID); err != nil {
		t.Fatal(err)
	}
	old["taskId"] = taskID()
	old["status"] = "completed"
	old["phase"] = "finished"
	old["endedAt"] = "2026-10-14T00:00:00Z"
	raw, _ = json.Marshal(old)
	if _, err = s.db.Exec(`INSERT INTO agent_tasks(id,conversation,instance,status,payload) VALUES(?,?,?,?,?)`, old["taskId"], -20, old["instance"], old["status"], string(raw)); err != nil {
		t.Fatal(err)
	}
	recovered, err := newTaskStore(h.db, "timing-restart")
	if err != nil {
		t.Fatal(err)
	}
	v, err := recovered.Read(context.Background(), 10, r.info.ID)
	if err != nil || v.Status != "interrupted" || v.CancelRequestedAt != stopped.CancelRequestedAt || v.PhaseStartedAt != v.EndedAt {
		t.Fatal(err, v)
	}
	taskStamp(t, v.PhaseStartedAt)
	oldValue, err := recovered.Read(context.Background(), -10, legacy.info.ID)
	if err != nil || oldValue.CancelRequestedAt != "" || oldValue.PhaseStartedAt != oldValue.EndedAt {
		t.Fatal("legacy stop invented", err, oldValue)
	}
	oldTerminal, err := recovered.Read(context.Background(), -20, old["taskId"].(string))
	if err != nil || oldTerminal.PhaseStartedAt != "" || oldTerminal.CancelRequestedAt != "" {
		t.Fatal("old terminal timestamps invented", err, oldTerminal)
	}
	again, err := newTaskStore(h.db, "timing-restart-again")
	if err != nil {
		t.Fatal(err)
	}
	v2, err := again.Read(context.Background(), 10, r.info.ID)
	if err != nil || v2.PhaseStartedAt != v.PhaseStartedAt || v2.CancelRequestedAt != v.CancelRequestedAt {
		t.Fatal("repeated recovery rewrote timestamps", err, v2)
	}
	failed, _ := again.Begin(20)
	if _, err = s.db.Exec(`CREATE TRIGGER fail_timing_update BEFORE UPDATE ON agent_tasks BEGIN SELECT RAISE(FAIL,'synthetic'); END`); err != nil {
		t.Fatal(err)
	}
	v, err = again.Cancel(context.Background(), 20, failed.info.ID, failed.info.Version)
	if err != ErrAgentTaskStore || v.Persisted || v.CancelRequestedAt == "" {
		t.Fatal("failed stop was called durable", err, v)
	}
	var saved AgentTaskInfo
	var payload string
	if err = s.db.QueryRow(`SELECT payload FROM agent_tasks WHERE id=?`, failed.info.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal([]byte(payload), &saved); err != nil || saved.CancelRequestedAt != "" || saved.PhaseStartedAt != saved.StartedAt {
		t.Fatal("failed write altered disk", err, saved)
	}
	v, err = again.Read(context.Background(), 20, failed.info.ID)
	if err != nil || v.Persisted || v.CancelRequestedAt == "" {
		t.Fatal("read did not expose uncertain time", err, v)
	}
}

func TestAgentTaskTimingInitialWriteFailure(t *testing.T) {
	s := taskFixture(t).tasks
	if _, err := s.db.Exec(`CREATE TRIGGER fail_timing_insert BEFORE INSERT ON agent_tasks BEGIN SELECT RAISE(FAIL,'synthetic'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Begin(10); err != ErrAgentTaskStore {
		t.Fatal(err)
	}
	page, err := s.Page(context.Background(), 10, 0)
	if err != nil || len(page.Items) != 1 {
		t.Fatal(err, page)
	}
	v := page.Items[0]
	if v.Persisted || v.Status != "failed" || v.Phase != "finished" || v.PhaseStartedAt != v.EndedAt || v.CancelRequestedAt != "" {
		t.Fatal(v)
	}
	taskStamp(t, v.PhaseStartedAt)
	if _, err = s.Begin(10); err != ErrAgentBusy {
		t.Fatal("uncertain initial write released scope", err)
	}
}
