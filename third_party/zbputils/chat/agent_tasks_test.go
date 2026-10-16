package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"

	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func taskFixture(t *testing.T) *agentHistory {
	t.Helper()
	h, e := openAgentHistory(t.TempDir(), 456, 2048, historyConfig)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

func TestAgentTaskFilteredPagingAndUncertainStatus(t *testing.T) {
	h := taskFixture(t)
	s := h.tasks
	for i := 0; i < 125; i++ {
		r, e := s.Begin(10)
		if e != nil {
			t.Fatal(e)
		}
		var result error
		if i%2 == 0 {
			result = errors.New("synthetic failure")
		}
		if e = r.Finish(result); e != nil {
			t.Fatal(e)
		}
	}
	for _, tc := range []struct {
		state string
		want  int
	}{{"all", 125}, {"failed", 63}, {"completed", 62}} {
		seen := map[string]bool{}
		before := int64(0)
		for {
			p, e := s.PageFiltered(context.Background(), 10, before, tc.state)
			if e != nil {
				t.Fatal(e)
			}
			for _, item := range p.Items {
				if seen[item.ID] || (tc.state != "all" && item.Status != tc.state) {
					t.Fatal("filter mismatch or repeated ID")
				}
				seen[item.ID] = true
			}
			if !p.More {
				break
			}
			before, e = strconv.ParseInt(p.Next, 10, 64)
			if e != nil {
				t.Fatal(e)
			}
		}
		if len(seen) != tc.want {
			t.Fatal(tc.state, len(seen))
		}
	}
	if _, e := s.PageFiltered(context.Background(), 10, 0, "bad"); !errors.Is(e, ErrAgentTaskConflict) {
		t.Fatal(e)
	}
	private, _ := s.Begin(-10)
	private.Finish(errors.New("private"))
	p, e := s.PageFiltered(context.Background(), -10, 0, "failed")
	if e != nil || len(p.Items) != 1 {
		t.Fatal("scope mixed")
	}
	r, _ := s.Begin(10)
	if _, e = s.db.Exec(`CREATE TRIGGER reject_task_update BEFORE UPDATE ON agent_tasks BEGIN SELECT RAISE(ABORT,'synthetic'); END`); e != nil {
		t.Fatal(e)
	}
	if e = r.Model(); !errors.Is(e, ErrAgentTaskStore) {
		t.Fatal(e)
	}
	r.Finish(ErrAgentTaskStore)
	p, e = s.PageFiltered(context.Background(), 10, 0, "failed")
	if e != nil || len(p.Items) != 50 || !p.More || p.Items[0].ID != r.info.ID || p.Items[0].Persisted {
		t.Fatal("uncertain terminal omitted", e, p)
	}
	before, _ := strconv.ParseInt(p.Next, 10, 64)
	older, e := s.PageFiltered(context.Background(), 10, before, "failed")
	if e != nil || len(older.Items) != 14 {
		t.Fatal("filtered cursor lost rows", e, len(older.Items))
	}
	for _, x := range older.Items {
		if x.ID == r.info.ID {
			t.Fatal("overlay leaked into older page")
		}
	}
	p, e = s.PageFiltered(context.Background(), 10, 0, "running")
	if e != nil || len(p.Items) != 0 {
		t.Fatal("persisted running status hid failure")
	}
}

func TestAgentTaskFilteredCancellationTransition(t *testing.T) {
	s := taskFixture(t).tasks
	r, _ := s.Begin(10)
	v := taskSnapshot(t, s, 10)
	if _, e := s.Cancel(context.Background(), 10, v.ID, v.Version); e != nil {
		t.Fatal(e)
	}
	p, e := s.PageFiltered(context.Background(), 10, 0, "running")
	if e != nil || len(p.Items) != 0 {
		t.Fatal(e)
	}
	p, e = s.PageFiltered(context.Background(), 10, 0, "cancel_requested")
	if e != nil || len(p.Items) != 1 {
		t.Fatal(e)
	}
	r.Finish(ErrAgentCancelled)
	p, e = s.PageFiltered(context.Background(), 10, 0, "cancelled")
	if e != nil || len(p.Items) != 1 {
		t.Fatal(e)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = s.PageFiltered(cancelled, 10, 0, "cancelled"); e == nil {
		t.Fatal("cancelled read succeeded")
	}
	if _, e = s.db.Exec(`CREATE TRIGGER reject_task_insert BEFORE INSERT ON agent_tasks BEGIN SELECT RAISE(ABORT,'synthetic'); END`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Begin(20); !errors.Is(e, ErrAgentTaskStore) {
		t.Fatal(e)
	}
	p, e = s.PageFiltered(context.Background(), 20, 0, "failed")
	if e != nil || len(p.Items) != 1 || p.Items[0].Persisted {
		t.Fatal("unsaved admission omitted")
	}
	p, e = s.PageFiltered(context.Background(), 20, 0, "running")
	if e != nil || len(p.Items) != 0 {
		t.Fatal("unsaved failure leaked into running filter")
	}
	p, e = s.PageFiltered(context.Background(), 20, 1, "failed")
	if e != nil || len(p.Items) != 0 {
		t.Fatal("unsaved admission appended to older page")
	}
}

func TestAgentTaskMemoryRetrievalContextCounts(t *testing.T) {
	h := taskFixture(t)
	h.budget = 8192
	if e := h.Append(10, "response", &goba.APIResponse{Status: "ok", Data: json.RawMessage(`{"sources":[{"seq":"1"},{"seq":"2"}],"memories":[{"id":"1","revision":"1","text":"synthetic-memory-response"},{"id":"1","revision":"1","text":"same"}]}`)}); e != nil {
		t.Fatal(e)
	}
	if e := h.Append(10, "response", &goba.APIResponse{Status: "ok", Data: json.RawMessage(`{"memories":[{"id":"2","revision":"4","text":"read result"}]}`)}); e != nil {
		t.Fatal(e)
	}
	created := true
	metadata, e := json.Marshal(MemorySaveResult{Mode: "managed", Created: &created, Memory: &MemoryReference{ID: "2", Revision: "4", Source: "agent", SourceSeq: "0"}})
	if e != nil {
		t.Fatal(e)
	}
	if e = h.Append(10, "response", &goba.APIResponse{Status: "ok", Data: metadata}); e != nil {
		t.Fatal(e)
	}
	if e := h.Append(10, "event", &goba.APIResponse{Status: "ok", Data: json.RawMessage(`{"memories":[{"text":"not a response"}]}`)}); e != nil {
		t.Fatal(e)
	}
	r, e := h.tasks.Begin(10)
	if e != nil {
		t.Fatal(e)
	}
	r.Model()
	body := historyBody(t, h, 10)
	v := taskSnapshot(t, h.tasks, 10)
	if !v.Context.Available || v.Context.SearchSources != 2 || v.Context.MemorySources == nil || *v.Context.MemorySources != 3 || !strings.Contains(body, "synthetic-memory-response") {
		t.Fatal(v.Context)
	}
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), "synthetic-memory-response") {
		t.Fatal("summary persisted body")
	}
	if e = r.Finish(nil); e != nil {
		t.Fatal(e)
	}
	s, e := newTaskStore(h.db, "memory-context-restart")
	if e != nil {
		t.Fatal(e)
	}
	v = taskSnapshot(t, s, 10)
	if v.Context.MemorySources == nil || *v.Context.MemorySources != 3 {
		t.Fatal("count not persisted")
	}
	if _, e = h.db.Exec(`UPDATE agent_tasks SET payload=json_remove(payload,'$.context.memorySources') WHERE conversation=10`); e != nil {
		t.Fatal(e)
	}
	if taskSnapshot(t, s, 10).Context.MemorySources != nil {
		t.Fatal("old measurement fabricated")
	}
	h.reset()
	r, _ = h.tasks.Begin(10)
	r.Model()
	historyBody(t, h, 10)
	v = taskSnapshot(t, h.tasks, 10)
	if v.Context.MemorySources == nil || *v.Context.MemorySources != 0 {
		t.Fatal("reset not measured as zero", v.Context)
	}
	if e = r.FallbackModel(); e != nil {
		t.Fatal(e)
	}
	if taskSnapshot(t, h.tasks, 10).Context.MemorySources != nil {
		t.Fatal("fallback retained prior measurement")
	}
}

func TestAgentTaskMemoryRetrievalCountAfterClipping(t *testing.T) {
	h := taskFixture(t)
	h.budget = 128
	data, _ := json.Marshal(map[string]any{"memories": []any{map[string]any{"id": "1", "revision": "1", "text": strings.Repeat("fill", 1000)}}})
	if e := h.Append(10, "response", &goba.APIResponse{Status: "ok", Data: data}); e != nil {
		t.Fatal(e)
	}
	r, _ := h.tasks.Begin(10)
	r.Model()
	historyBody(t, h, 10)
	v := taskSnapshot(t, h.tasks, 10)
	if !v.Context.Clipped || v.Context.MemorySources == nil || *v.Context.MemorySources != 0 {
		t.Fatal("clipped response was counted as complete", v.Context)
	}
}

func TestAgentTaskActivityCountsAndRecovery(t *testing.T) {
	h := taskFixture(t)
	s := h.tasks
	r, e := s.Begin(10)
	if e != nil {
		t.Fatal(e)
	}
	if got := taskSnapshot(t, s, 10).Activity; !got.Available || got.ModelPreparations != 0 {
		t.Fatal(got)
	}
	for i := 0; i < 2; i++ {
		if e = r.Model(); e != nil {
			t.Fatal(e)
		}
	}
	for _, name := range []string{goba.SearchHistory, goba.ReadHistory, goba.SearchMemory, goba.ReadMemory, goba.SVM} {
		if e = r.Action(name); e != nil {
			t.Fatal(e)
		}
		if e = r.Result("ok"); e != nil {
			t.Fatal(e)
		}
	}
	if e = r.FallbackModel(); e != nil {
		t.Fatal(e)
	}
	if e = r.Action("send_group_msg"); e != nil {
		t.Fatal(e)
	}
	want := TaskActivity{Available: true, ModelPreparations: 2, FallbackPreparations: 1, LocalActions: 5, PlatformActions: 1}
	if got := taskSnapshot(t, s, 10).Activity; got != want {
		t.Fatal(got)
	}
	other, e := s.Begin(-10)
	if e != nil {
		t.Fatal(e)
	}
	other.Model()
	other.Finish(nil)
	if taskSnapshot(t, s, 10).Activity != want {
		t.Fatal("scope counters mixed")
	}
	recovered, e := newTaskStore(h.db, "activity-restart")
	if e != nil {
		t.Fatal(e)
	}
	v := taskSnapshot(t, recovered, 10)
	if v.Activity != want || v.Status != "interrupted" || v.Outcome != "unknown" {
		t.Fatal("restart lost counts", v)
	}
	// Older payloads have no measurement, rather than a measured zero.
	if _, e = h.db.Exec(`UPDATE agent_tasks SET payload=json_remove(payload,'$.activity') WHERE conversation=10`); e != nil {
		t.Fatal(e)
	}
	if taskSnapshot(t, recovered, 10).Activity.Available {
		t.Fatal("legacy summary invented")
	}
}

func TestAgentTaskActivityCancellationAndWriteFailure(t *testing.T) {
	s := taskFixture(t).tasks
	r, _ := s.Begin(10)
	r.Model()
	v := taskSnapshot(t, s, 10)
	if _, e := s.Cancel(context.Background(), 10, v.ID, v.Version); e != nil {
		t.Fatal(e)
	}
	if e := r.Model(); !errors.Is(e, ErrAgentCancelled) {
		t.Fatal(e)
	}
	if e := r.Action(goba.SearchHistory); !errors.Is(e, ErrAgentCancelled) {
		t.Fatal(e)
	}
	if e := r.FallbackModel(); !errors.Is(e, ErrAgentCancelled) {
		t.Fatal(e)
	}
	if taskSnapshot(t, s, 10).Activity != v.Activity {
		t.Fatal("cancelled dispatch counted")
	}
	r.Finish(ErrAgentCancelled)
	if taskSnapshot(t, s, 10).Activity != v.Activity {
		t.Fatal("finish changed counters")
	}
	r, _ = s.Begin(10)
	if got := taskSnapshot(t, s, 10).Activity; got.ModelPreparations != 0 || got.LocalActions != 0 {
		t.Fatal("new task inherited counts")
	}
	if _, e := s.db.Exec(`CREATE TRIGGER activity_failure BEFORE UPDATE ON agent_tasks BEGIN SELECT RAISE(ABORT,'synthetic'); END`); e != nil {
		t.Fatal(e)
	}
	if e := r.Action(goba.ReadHistory); !errors.Is(e, ErrAgentTaskStore) {
		t.Fatal(e)
	}
	if taskSnapshot(t, s, 10).Persisted {
		t.Fatal("uncertain counts marked saved")
	}
}

func TestAgentTaskPagesRemainStableDuringNewTasks(t *testing.T) {
	s := taskFixture(t).tasks
	original := map[string]bool{}
	for i := 0; i < 125; i++ {
		r, e := s.Begin(10)
		if e != nil {
			t.Fatal(e)
		}
		original[r.info.ID] = true
		if e = r.Finish(nil); e != nil {
			t.Fatal(e)
		}
	}
	private, _ := s.Begin(-10)
	private.Finish(nil)
	first, e := s.Page(context.Background(), 10, 0)
	if e != nil || len(first.Items) != 50 || !first.More || first.Next == "" {
		t.Fatal(first, e)
	}
	newest, _ := s.Begin(10)
	newest.Finish(nil)
	seen := map[string]bool{}
	page := first
	for {
		for _, v := range page.Items {
			if !original[v.ID] || seen[v.ID] {
				t.Fatal("unexpected or duplicate task", v.ID)
			}
			seen[v.ID] = true
		}
		if !page.More {
			break
		}
		before, e := strconv.ParseInt(page.Next, 10, 64)
		if e != nil {
			t.Fatal(e)
		}
		page, e = s.Page(context.Background(), 10, before)
		if e != nil {
			t.Fatal(e)
		}
	}
	if len(seen) != 125 || len(page.Items) != 25 || page.Next != "" {
		t.Fatal("omitted task", len(seen), page)
	}
	refreshed, e := s.Page(context.Background(), 10, 0)
	if e != nil || refreshed.Items[0].ID != newest.info.ID {
		t.Fatal("refresh missing new task", e)
	}
	isolated, e := s.Page(context.Background(), -10, 0)
	if e != nil || len(isolated.Items) != 1 || isolated.Items[0].ID != private.info.ID {
		t.Fatal("scope leakage", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := s.Page(ctx, 10, 0); e == nil {
		t.Fatal("cancelled query succeeded")
	}
}

func TestAgentTaskPageUnsavedOverlayAndLargeSequence(t *testing.T) {
	s := taskFixture(t).tasks
	// Keep pagination tokens outside JavaScript's safe integer range.
	r, _ := s.Begin(10)
	r.Finish(nil)
	if _, e := s.db.Exec(`UPDATE sqlite_sequence SET seq=9007199254740993 WHERE name='agent_tasks'`); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 50; i++ {
		r, e := s.Begin(10)
		if e != nil {
			t.Fatal(e)
		}
		r.Finish(nil)
	}
	page, e := s.Page(context.Background(), 10, 0)
	if e != nil || page.Next != "9007199254740994" || !page.More {
		t.Fatal(page, e)
	}
	if _, e := s.db.Exec(`CREATE TRIGGER page_insert_failure BEFORE INSERT ON agent_tasks BEGIN SELECT RAISE(ABORT,'synthetic'); END`); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Begin(10); e == nil {
		t.Fatal("expected failure")
	}
	page, e = s.Page(context.Background(), 10, 0)
	if e != nil || len(page.Items) != 50 || page.Items[0].Persisted || !page.More {
		t.Fatal(page, e)
	}
	before, _ := strconv.ParseInt(page.Next, 10, 64)
	older, e := s.Page(context.Background(), 10, before)
	if e != nil || len(older.Items) != 2 || older.More || older.Next != "" {
		t.Fatal(older, e)
	}
	for _, v := range older.Items {
		if !v.Persisted {
			t.Fatal("unsaved overlay repeated on older page")
		}
	}
	empty, e := s.Page(context.Background(), 10, 1)
	if e != nil || len(empty.Items) != 0 || empty.More {
		t.Fatal(empty, e)
	}
}
func taskSnapshot(t *testing.T, s *TaskStore, g int64) AgentTaskInfo {
	t.Helper()
	v, e := s.List(context.Background(), g)
	if e != nil || len(v) == 0 {
		t.Fatalf("list: %v %v", v, e)
	}
	return v[0]
}
func TestAgentTaskAdmissionIsolationAndStaleCancel(t *testing.T) {
	h := taskFixture(t)
	s := h.tasks
	var wg sync.WaitGroup
	var mu sync.Mutex
	var runs []*AgentTask
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := s.Begin(10)
			if e == nil {
				mu.Lock()
				runs = append(runs, r)
				mu.Unlock()
			} else if !errors.Is(e, ErrAgentBusy) {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if len(runs) != 1 {
		t.Fatal(len(runs))
	}
	if _, e := s.Begin(-10); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Begin(20); e != nil {
		t.Fatal(e)
	}
	other := taskFixture(t)
	if _, e := other.tasks.Begin(10); e != nil {
		t.Fatal(e)
	}
	r := runs[0]
	old := taskSnapshot(t, s, 10)
	if e := r.Model(); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Cancel(context.Background(), 10, old.ID, old.Version); !errors.Is(e, ErrAgentTaskConflict) {
		t.Fatal(e)
	}
	if _, e := s.Cancel(context.Background(), -10, old.ID, old.Version); !errors.Is(e, ErrAgentTaskMissing) {
		t.Fatal(e)
	}
	v := taskSnapshot(t, s, 10)
	cancel, e := s.Cancel(context.Background(), 10, v.ID, v.Version)
	if e != nil || cancel.Status != "cancel_requested" {
		t.Fatal(cancel, e)
	}
	again, e := s.Cancel(context.Background(), 10, v.ID, v.Version)
	if e != nil || again.Version != cancel.Version {
		t.Fatal(again, e)
	}
	if e := r.Action("send_group_msg"); !errors.Is(e, ErrAgentCancelled) {
		t.Fatal(e)
	}
	if e := r.Finish(nil); e != nil {
		t.Fatal(e)
	}
	if taskSnapshot(t, s, 10).Status != "cancelled" {
		t.Fatal("wrong terminal")
	}
	fresh, e := s.Begin(10)
	if e != nil {
		t.Fatal(e)
	}
	if v, e := s.Cancel(context.Background(), 10, old.ID, old.Version); e != nil || v.Status != "cancelled" {
		t.Fatal(v, e)
	}
	if fresh.Check() != nil {
		t.Fatal("stale cancel stopped new task")
	}
}
func TestAgentTaskCancelDuringModelAndAction(t *testing.T) {
	for _, phase := range []string{"model", "action"} {
		t.Run(phase, func(t *testing.T) {
			s := taskFixture(t).tasks
			r, _ := s.Begin(10)
			entered := make(chan struct{})
			release := make(chan struct{})
			done := make(chan error, 1)
			calls := 0
			go func() {
				e := RunAgent(func(int) ([]zero.APIRequest, bool, error) {
					if e := r.Model(); e != nil {
						return nil, false, e
					}
					if phase == "model" {
						close(entered)
						<-release
					}
					if e := r.Check(); e != nil {
						return nil, false, e
					}
					return []zero.APIRequest{{Action: "first"}, {Action: "second"}}, false, nil
				}, func(req zero.APIRequest) error {
					if e := r.Action(req.Action); e != nil {
						return e
					}
					calls++
					if phase == "action" {
						close(entered)
						<-release
					}
					return r.Result("ok")
				}, func() bool { return true })
				r.Finish(e)
				done <- e
			}()
			<-entered
			v := taskSnapshot(t, s, 10)
			if _, e := s.Cancel(context.Background(), 10, v.ID, v.Version); e != nil {
				t.Fatal(e)
			}
			if taskSnapshot(t, s, 10).Status != "cancel_requested" {
				t.Fatal("premature terminal")
			}
			close(release)
			e := <-done
			if !errors.Is(e, ErrAgentCancelled) {
				t.Fatal(e)
			}
			want := 0
			if phase == "action" {
				want = 1
			}
			if calls != want {
				t.Fatalf("dispatched %d", calls)
			}
			v = taskSnapshot(t, s, 10)
			if v.Status != "cancelled" || !v.Persisted {
				t.Fatal(v)
			}
			if phase == "action" && v.Outcome != "ok" {
				t.Fatal("lost in-flight receipt")
			}
		})
	}
}
func TestAgentTaskRecoveryUnknownAndBoundedList(t *testing.T) {
	h := taskFixture(t)
	s := h.tasks
	r, _ := s.Begin(10)
	r.Action("send_group_msg")
	var seq int
	var name, path string
	if e := h.db.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); e != nil {
		t.Fatal(e)
	}
	if e := h.Close(); e != nil {
		t.Fatal(e)
	}
	db, e := sql.Open("sqlite3", path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	recovered, e := newTaskStore(db, "new-process")
	if e != nil {
		t.Fatal(e)
	}
	v := taskSnapshot(t, recovered, 10)
	if v.Status != "interrupted" || v.Outcome != "unknown" || v.EndedAt == "" {
		t.Fatal(v)
	}
	for i := 0; i < 55; i++ {
		r, e := recovered.Begin(10)
		if e != nil {
			t.Fatal(e)
		}
		if e = r.Finish(nil); e != nil {
			t.Fatal(e)
		}
	}
	items, e := recovered.List(context.Background(), 10)
	if e != nil || len(items) != 50 {
		t.Fatal(len(items), e)
	}
	if r, e := recovered.Begin(-10); e != nil {
		t.Fatal(e)
	} else {
		r.Finish(ErrAgentReloaded)
	}
	if taskSnapshot(t, recovered, -10).Error != "settings_changed" {
		t.Fatal("missing failure category")
	}
}
func TestAgentTaskWriteFailureFailsClosed(t *testing.T) {
	s := taskFixture(t).tasks
	r, _ := s.Begin(10)
	if _, e := s.db.Exec(`CREATE TRIGGER task_fail BEFORE UPDATE ON agent_tasks BEGIN SELECT RAISE(ABORT,'synthetic'); END`); e != nil {
		t.Fatal(e)
	}
	if e := r.Model(); !errors.Is(e, ErrAgentTaskStore) {
		t.Fatal(e)
	}
	if e := r.Action("send_group_msg"); !errors.Is(e, ErrAgentTaskStore) {
		t.Fatal(e)
	}
	if e := r.Finish(nil); !errors.Is(e, ErrAgentTaskStore) {
		t.Fatal(e)
	}
	v := taskSnapshot(t, s, 10)
	if v.Persisted || v.Status != "failed" || v.Error != "storage_error" {
		t.Fatal(v)
	}
	if _, e := s.Begin(10); !errors.Is(e, ErrAgentBusy) {
		t.Fatal(e)
	}
}
func TestAgentTaskContextCountsAndReset(t *testing.T) {
	h := taskFixture(t)
	for i := 0; i < 20; i++ {
		if e := h.Append(10, "event", historyEvent(10, strings.Repeat("中", 80))); e != nil {
			t.Fatal(e)
		}
	}
	if e := h.Append(10, "response", &goba.APIResponse{Status: "ok", Data: json.RawMessage(`{"sources":[{"seq":"1"},{"seq":"2"}]}`)}); e != nil {
		t.Fatal(e)
	}
	r, _ := h.tasks.Begin(10)
	historyBody(t, h, 10)
	v := taskSnapshot(t, h.tasks, 10)
	if !v.Context.Available || !v.Context.Clipped || v.Context.Records >= 21 || v.Context.SearchSources != 2 {
		t.Fatal(v.Context)
	}
	r.Finish(nil)
	h.reset()
	r, _ = h.tasks.Begin(10)
	historyBody(t, h, 10)
	v = taskSnapshot(t, h.tasks, 10)
	if v.Context.Records != 0 || v.Context.Clipped || v.Context.SearchSources != 0 {
		t.Fatal(v.Context)
	}
	r.Finish(nil)
}

func TestAgentTaskBeginAndTerminalWriteFailure(t *testing.T) {
	for _, phase := range []string{"begin", "finish"} {
		t.Run(phase, func(t *testing.T) {
			s := taskFixture(t).tasks
			var r *AgentTask
			if phase == "finish" {
				r, _ = s.Begin(10)
			}
			event := "INSERT"
			if phase == "finish" {
				event = "UPDATE"
			}
			if _, e := s.db.Exec(`CREATE TRIGGER task_write_failure BEFORE ` + event + ` ON agent_tasks BEGIN SELECT RAISE(ABORT,'synthetic'); END`); e != nil {
				t.Fatal(e)
			}
			var e error
			if phase == "begin" {
				_, e = s.Begin(10)
			} else {
				e = r.Finish(nil)
			}
			if !errors.Is(e, ErrAgentTaskStore) {
				t.Fatal(e)
			}
			v := taskSnapshot(t, s, 10)
			if v.Persisted || v.Status != "failed" {
				t.Fatal(v)
			}
			if _, e := s.Begin(10); !errors.Is(e, ErrAgentBusy) {
				t.Fatal("uncertain write allowed new task", e)
			}
		})
	}
}
