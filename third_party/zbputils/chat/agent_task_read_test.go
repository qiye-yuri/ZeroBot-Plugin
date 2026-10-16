package chat

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestAgentTaskReadCurrentScopeAndOldTask(t *testing.T) {
	h := taskFixture(t)
	s := h.tasks
	ctx := context.Background()
	task, e := s.Begin(10)
	if e != nil {
		t.Fatal(e)
	}
	count := 3
	if e = s.capture(10, ContextUsage{Available: true, MemorySources: &count}); e != nil {
		t.Fatal(e)
	}
	v, e := s.Read(ctx, 10, task.info.ID)
	if e != nil || v.Status != "running" || v.Version != task.info.Version {
		t.Fatal(e, v)
	}
	*v.Context.MemorySources = 99
	v, e = s.Read(ctx, 10, task.info.ID)
	if e != nil || *v.Context.MemorySources != 3 {
		t.Fatal("read aliases mutable task summary", e, v)
	}
	for _, tc := range []struct {
		store *TaskStore
		grp   int64
	}{{s, -10}, {taskFixture(t).tasks, 10}} {
		if _, e := tc.store.Read(ctx, tc.grp, task.info.ID); !errors.Is(e, ErrAgentTaskMissing) {
			t.Fatal("scope leaked", e)
		}
	}
	if e = task.Finish(nil); e != nil {
		t.Fatal(e)
	}
	ended := task.info
	for i := 0; i < 55; i++ {
		newTask, e := s.Begin(10)
		if e != nil {
			t.Fatal(e)
		}
		if e = newTask.Finish(nil); e != nil {
			t.Fatal(e)
		}
	}
	current, e := s.Begin(10)
	if e != nil {
		t.Fatal(e)
	}
	v, e = s.Read(ctx, 10, ended.ID)
	if e != nil || v.ID != ended.ID || v.Status != "completed" || v.Version != ended.Version {
		t.Fatal("old task outside first page missing", e, v)
	}
	if current.info.Status != "running" || current.cancelled {
		t.Fatal("read changed new task")
	}
	for _, bad := range []string{"", "../path", strings.Repeat("A", 32), strings.Repeat("g", 32), strings.Repeat("a", 31), strings.Repeat("a", 33)} {
		if _, e = s.Read(ctx, 10, bad); !errors.Is(e, ErrAgentTaskConflict) {
			t.Fatal("invalid ID accepted", e)
		}
	}
	if _, e = s.Read(ctx, 0, ended.ID); !errors.Is(e, ErrAgentTaskConflict) {
		t.Fatal(e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	for _, id := range []string{current.info.ID, ended.ID} {
		if _, e = s.Read(cancelled, 10, id); !errors.Is(e, ErrAgentTaskStore) {
			t.Fatal("cancelled read succeeded", e)
		}
	}
	if _, e = s.db.Exec(`UPDATE agent_tasks SET payload='{}' WHERE id=?`, ended.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Read(ctx, 10, ended.ID); !errors.Is(e, ErrAgentTaskStore) {
		t.Fatal("invalid identity returned", e)
	}
	if _, e = s.db.Exec(`DROP TABLE agent_tasks`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Read(ctx, 10, ended.ID); !errors.Is(e, ErrAgentTaskStore) {
		t.Fatal("storage error hidden", e)
	}
}

func TestAgentTaskReadUncertainWritesAndRestart(t *testing.T) {
	for _, event := range []string{"INSERT", "UPDATE"} {
		t.Run(event, func(t *testing.T) {
			h := taskFixture(t)
			s := h.tasks
			if _, e := s.db.Exec(`CREATE TRIGGER read_fault BEFORE ` + event + ` ON agent_tasks BEGIN SELECT RAISE(ABORT,'synthetic'); END`); e != nil {
				t.Fatal(e)
			}
			task, e := s.Begin(10)
			if event == "UPDATE" {
				if e != nil {
					t.Fatal(e)
				}
				e = task.Finish(nil)
			}
			if !errors.Is(e, ErrAgentTaskStore) {
				t.Fatal(e)
			}
			// Admission intentionally returns no runnable handle after INSERT
			// failure. Obtain the visible fault's ID from the public list API.
			page, e := s.Page(context.Background(), 10, 0)
			if e != nil || len(page.Items) != 1 {
				t.Fatal(e, page)
			}
			visible := page.Items[0]
			v, e := s.Read(context.Background(), 10, visible.ID)
			if e != nil || v.Status != "failed" || v.Persisted || v.Error != "storage_error" || v.Version != visible.Version {
				t.Fatal("memory did not override disk", e, v)
			}
		})
	}
	h := taskFixture(t)
	task, e := h.tasks.Begin(10)
	if e != nil {
		t.Fatal(e)
	}
	if e = task.Action("get_group_info"); e != nil {
		t.Fatal(e)
	}
	recovered, e := newTaskStore(h.db, "different-process")
	if e != nil {
		t.Fatal(e)
	}
	v, e := recovered.Read(context.Background(), 10, task.info.ID)
	if e != nil || v.Status != "interrupted" || v.Outcome != "unknown" || !v.Persisted || v.Activity.PlatformActions != 1 {
		t.Fatal("restart recovery missing", e, v)
	}
	version := v.Version
	v, e = recovered.Read(context.Background(), 10, task.info.ID)
	if e != nil || v.Version != version {
		t.Fatal("reading changed version", e, v)
	}
}
