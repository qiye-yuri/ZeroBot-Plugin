package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func queryTaskFixture(t *testing.T) (*agentHistory, *AgentTask, *goba.Agent, *HistorySearch) {
	t.Helper()
	h := taskFixture(t)
	r, e := h.tasks.Begin(10)
	if e != nil {
		t.Fatal(e)
	}
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", nil, true, true)
	ag.SetHistoryStore(h)
	s, e := NewTaskHistorySearch(456, 10, r)
	if e != nil {
		t.Fatal(e)
	}
	return h, r, &ag, s
}

func TestAgentTaskQueryQuotaCoveragePersistenceAndBinding(t *testing.T) {
	h, r, ag, search := queryTaskFixture(t)
	ctx := context.Background()
	for _, scope := range [][2]int64{{789, 10}, {456, -10}, {0, 10}} {
		if _, e := NewTaskHistorySearch(scope[0], scope[1], r); e != ErrAgentTaskConflict {
			t.Fatal("query task scope accepted", scope, e)
		}
	}
	search.read = func(_ context.Context, bot int64, q ArchiveQuery) (ArchivePage, error) {
		v, e := h.tasks.Read(ctx, 10, r.info.ID)
		if e != nil || v.Queries.Used != 1 || v.Queries.Successful != 0 || v.Queries.SearchHistory != 1 || bot != 456 || q.Conversation != 10 {
			t.Fatal("quota was not saved before query", e, v)
		}
		return ArchivePage{Available: true, Entries: []ArchiveEntry{{Seq: "1", Text: "private query body"}, {Seq: "2", Text: "another"}}}, nil
	}
	search.readMemory = func(context.Context, int64, int64, int64, int64) (MemorySearchPage, error) {
		return MemorySearchPage{Available: true, Partial: true}, nil
	}
	reqs := []zero.APIRequest{
		{Action: goba.SearchHistory, Params: zero.Params{"keyword": "private query keyword", "limit": json.Number("1")}},
		{Action: goba.ReadMemory, Params: zero.Params{"id": "1", "revision": "1"}},
		{Action: goba.SearchMemory, Params: zero.Params{"keyword": "", "limit": json.Number("1")}},
	}
	for _, req := range reqs {
		if e := r.Action(req.Action); e != nil {
			t.Fatal(e)
		}
		if e := search.Execute(ag, req); e != nil {
			t.Fatal(e)
		}
		if e := r.Result(search.Outcome()); e != nil {
			t.Fatal(e)
		}
	}
	// Reconstructing the host helper retains the task-wide quota.
	other, e := NewTaskHistorySearch(456, 10, r)
	if e != nil || other.Usage() != search.Usage() {
		t.Fatal("quota reset by helper reconstruction", e)
	}
	if e = r.Action(goba.ReadHistory); e != nil {
		t.Fatal(e)
	}
	e = other.Execute(ag, zero.APIRequest{Action: goba.ReadHistory, Params: zero.Params{"seq": "1"}})
	if e == nil || other.Outcome() != "local_error" {
		t.Fatal("fourth query accepted", e)
	}
	if e = r.Result(other.Outcome()); e != nil {
		t.Fatal(e)
	}
	want := TaskQueries{Available: true, Used: 3, Limit: 3, SearchHistory: 1, SearchMemory: 1, ReadMemory: 1, Successful: 2, Empty: 1, Partial: 2, Errors: 1, Denied: 1}
	if !want.valid() || r.info.Queries != want {
		t.Fatal("quota/coverage mismatch", r.info.Queries)
	}
	if e = r.Finish(errors.New("query quota reached")); e != nil {
		t.Fatal(e)
	}
	v, e := h.tasks.Read(ctx, 10, r.info.ID)
	if e != nil || v.Queries != want {
		t.Fatal("query summary not persisted", e, v)
	}
	page, e := h.tasks.PageFiltered(ctx, 10, 0, "failed")
	if e != nil || len(page.Items) != 1 || page.Items[0].Queries != want {
		t.Fatal(e, page)
	}
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), "private query") {
		t.Fatal("query text retained in task summary")
	}
	if _, e = NewTaskHistorySearch(456, 10, r); e != ErrAgentTaskConflict {
		t.Fatal("finished task helper accepted", e)
	}
	newRun, _ := h.tasks.Begin(10)
	if newRun.info.Queries != (TaskQueries{Available: true, Limit: 3}) {
		t.Fatal("new task inherited quota")
	}
	private, _ := h.tasks.Begin(-10)
	if private.info.Queries != newRun.info.Queries {
		t.Fatal("private task inherited quota")
	}
	newRun.Finish(nil)
	private.Finish(nil)
}

func TestAgentTaskQueryFailuresDoNotClaimSuccessfulCoverage(t *testing.T) {
	for _, stage := range []string{"quota write", "response write", "summary write", "query read"} {
		t.Run(stage, func(t *testing.T) {
			h, r, ag, search := queryTaskFixture(t)
			if e := r.Action(goba.SearchHistory); e != nil {
				t.Fatal(e)
			}
			calls := 0
			search.read = func(context.Context, int64, ArchiveQuery) (ArchivePage, error) {
				calls++
				if stage == "summary write" {
					if _, e := h.db.Exec(`CREATE TRIGGER reject_query_summary BEFORE UPDATE ON agent_tasks BEGIN SELECT RAISE(ABORT,'fixture'); END`); e != nil {
						t.Fatal(e)
					}
				}
				if stage == "query read" {
					return ArchivePage{}, errors.New("sensitive fixture path")
				}
				return ArchivePage{Available: true, Entries: []ArchiveEntry{{Seq: "1", Text: "fixture body"}}}, nil
			}
			if stage == "quota write" {
				if _, e := h.db.Exec(`CREATE TRIGGER reject_query_summary BEFORE UPDATE ON agent_tasks BEGIN SELECT RAISE(ABORT,'fixture'); END`); e != nil {
					t.Fatal(e)
				}
			}
			if stage == "response write" {
				if _, e := h.db.Exec(`CREATE TRIGGER reject_query_response BEFORE INSERT ON history WHEN NEW.kind='response' BEGIN SELECT RAISE(ABORT,'fixture'); END`); e != nil {
					t.Fatal(e)
				}
			}
			e := search.Execute(ag, zero.APIRequest{Action: goba.SearchHistory, Params: zero.Params{"keyword": "fixture"}})
			if stage == "query read" && e != nil {
				t.Fatal(e)
			}
			if stage != "query read" && e == nil {
				t.Fatal("write failure ignored")
			}
			if search.Outcome() != "local_error" {
				t.Fatal("unsaved response reported success", stage)
			}
			v, readErr := h.tasks.Read(context.Background(), 10, r.info.ID)
			if readErr != nil || v.Queries.Used != 1 {
				t.Fatal(readErr, v)
			}
			if stage == "quota write" && calls != 0 {
				t.Fatal("query ran after quota write failure")
			}
			if stage == "quota write" || stage == "summary write" {
				if v.Persisted {
					t.Fatal("uncertain summary reported durable")
				}
			} else if !v.Persisted || v.Queries.Successful != 0 || v.Queries.Errors != 1 {
				t.Fatal("failure counted as successful response", v)
			}
			if stage == "summary write" {
				if _, e = h.db.Exec(`DROP TRIGGER reject_query_summary`); e != nil {
					t.Fatal(e)
				}
				recovered, e := newTaskStore(h.db, "query-summary-restart")
				if e != nil {
					t.Fatal(e)
				}
				v, e = recovered.Read(context.Background(), 10, r.info.ID)
				if e != nil || v.Queries.Used != 1 || v.Queries.Successful != 0 || v.Queries.Errors != 0 || v.Status != "interrupted" {
					t.Fatal("unwritten coverage invented at restart", e, v)
				}
			}
		})
	}
}

func TestAgentTaskQueryCancellationLegacyAndInvalidSnapshots(t *testing.T) {
	h, r, ag, search := queryTaskFixture(t)
	search.read = func(context.Context, int64, ArchiveQuery) (ArchivePage, error) {
		if _, e := h.tasks.Cancel(context.Background(), 10, r.info.ID, r.info.Version); e != nil {
			t.Fatal(e)
		}
		return ArchivePage{Available: true}, nil
	}
	if e := r.Action(goba.ReadHistory); e != nil {
		t.Fatal(e)
	}
	if e := search.Execute(ag, zero.APIRequest{Action: goba.ReadHistory, Params: zero.Params{"seq": "1"}}); e != nil {
		t.Fatal(e)
	}
	if e := r.Result(search.Outcome()); e != nil {
		t.Fatal(e)
	}
	if e := r.Finish(ErrAgentCancelled); e != nil {
		t.Fatal(e)
	}
	v, e := h.tasks.Read(context.Background(), 10, r.info.ID)
	if e != nil || v.Status != "cancelled" || v.Queries.Successful != 1 || v.Queries.Empty != 1 {
		t.Fatal("cancelled in-flight usage lost", e, v)
	}
	// Old summaries remain unmeasured after recovery.
	legacy, _ := h.tasks.Begin(10)
	var payload map[string]any
	raw, _ := json.Marshal(legacy.info)
	json.Unmarshal(raw, &payload)
	delete(payload, "queries")
	raw, _ = json.Marshal(payload)
	if _, e = h.db.Exec(`UPDATE agent_tasks SET payload=? WHERE id=?`, string(raw), legacy.info.ID); e != nil {
		t.Fatal(e)
	}
	recovered, e := newTaskStore(h.db, "legacy-query-restart")
	if e != nil {
		t.Fatal(e)
	}
	v, e = recovered.Read(context.Background(), 10, legacy.info.ID)
	if e != nil || v.Queries != (TaskQueries{}) {
		t.Fatal("legacy query counters inferred", e, v)
	}
	for _, q := range []TaskQueries{{Available: true, Limit: 4}, {Available: true, Limit: 3, Used: 1}, {Available: true, Limit: 3, Empty: 1}, {Available: true, Limit: 3, Errors: -1}, {Available: true, Limit: 3, Used: 4, SearchHistory: 4}} {
		if q.valid() {
			t.Fatal("invalid query summary accepted", q)
		}
	}
}
