package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func TestAgentSearchTimeParametersAndScope(t *testing.T) {
	a, _ := memoryFixture(t)
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	for _, params := range []zero.Params{
		{"keyword": "needle", "from": 123}, {"keyword": "needle", "until": nil},
		{"keyword": "needle", "from": "2026-10-15"}, {"keyword": "needle", "until": "2026-10-15T00:00:00"},
		{"keyword": "needle", "from": "0001-01-01T00:00:00Z"},
		{"keyword": "needle", "from": "2026-10-16T00:00:00Z", "until": "2026-10-15T00:00:00Z"},
		{"keyword": "needle", "from": "2026-10-15T00:00:00Z", "until": "2026-10-15T08:00:00+08:00"},
		{"keyword": "needle", "from": "2026-10-15T00:00:00Z", "botId": "789"},
	} {
		s := NewHistorySearch(456, -10)
		s.read = func(context.Context, int64, ArchiveQuery) (ArchivePage, error) {
			t.Fatal("invalid range read")
			return ArchivePage{}, nil
		}
		if err := s.Execute(&ag, zero.APIRequest{Action: goba.SearchHistory, Params: params}); err != nil || s.Outcome() != "local_error" || s.Usage().Used != 1 || s.Usage().Errors != 1 {
			t.Fatalf("invalid range accepted %v %v", params, err)
		}
	}
	for _, tc := range []struct {
		action string
		params zero.Params
	}{
		{goba.ReadHistory, zero.Params{"seq": "1", "from": ""}},
		{goba.ReadMemory, zero.Params{"id": "1", "revision": "1", "from": ""}},
	} {
		s := NewHistorySearch(456, -10)
		s.read = func(context.Context, int64, ArchiveQuery) (ArchivePage, error) {
			t.Fatal("range accepted on another action")
			return ArchivePage{}, nil
		}
		s.memory = func(context.Context, int64, int64, string, int, int64, string, string, string) (MemorySearchPage, error) {
			t.Fatal("memory range accepted")
			return MemorySearchPage{}, nil
		}
		s.readMemory = func(context.Context, int64, int64, int64, int64) (MemorySearchPage, error) {
			t.Fatal("memory read range accepted")
			return MemorySearchPage{}, nil
		}
		if err := s.Execute(&ag, zero.APIRequest{Action: tc.action, Params: tc.params}); err != nil || s.Outcome() != "local_error" {
			t.Fatal(tc.action, err)
		}
	}
	for _, bounds := range [][2]string{{"2026-10-15T08:00:00.000000001+08:00", "2026-10-15T00:00:00.000000002Z"}, {"", "2026-10-15T00:00:00Z"}, {"2026-10-15T00:00:00Z", ""}, {"", ""}} {
		params := zero.Params{"before": "9007199254740993", "from": bounds[0], "until": bounds[1]}
		if bounds[0] == "" && bounds[1] == "" {
			params["keyword"] = "needle"
		}
		s := NewHistorySearch(456, -10)
		s.read = func(_ context.Context, bot int64, q ArchiveQuery) (ArchivePage, error) {
			from, until, err := ParseArchiveSearchRange(bounds[0], bounds[1])
			if err != nil || bot != 456 || q.Conversation != -10 || q.IncludeResetHistory || !q.EventsOnly || !q.Search || q.Before != 9007199254740993 || !q.SearchFrom.Equal(from) || !q.SearchUntil.Equal(until) {
				t.Fatal("scope/range lost", q)
			}
			return ArchivePage{Entries: []ArchiveEntry{}, Partial: true}, nil
		}
		if err := s.Execute(&ag, zero.APIRequest{Action: goba.SearchHistory, Params: params}); err != nil || s.Outcome() != "ok" || s.Usage().Empty != 1 || s.Usage().Partial != 1 {
			t.Fatal(err, s.Usage())
		}
	}
}

func TestAgentSearchTimeRoundTripUsesActualBoundedSource(t *testing.T) {
	a, _ := memoryFixture(t)
	a.history.budget = 8192
	for _, row := range []struct {
		scope             int64
		kind, stamp, text string
	}{
		{10, "event", "2026-10-15T00:00:00Z", "TIME_TARGET different subject"},
		{10, "event", "2026-10-15T00:00:00.000000001Z", "needle FUTURE_DISTRACTOR"},
		{-10, "event", "2026-10-15T00:00:00Z", "needle PRIVATE_DISTRACTOR"},
		{10, "response", "2026-10-15T00:00:00Z", "needle TOOL_DISTRACTOR"},
	} {
		if _, err := a.history.db.Exec("INSERT INTO history(conversation,kind,payload,created_at) VALUES(?,?,?,?)", row.scope, row.kind, `{"message":"`+row.text+`"}`, row.stamp); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 40; i++ {
		if err := a.history.Append(10, "event", historyEvent(10, strings.Repeat("近期", 150))); err != nil {
			t.Fatal(err)
		}
	}
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	run, err := a.history.tasks.Begin(10)
	if err != nil {
		t.Fatal(err)
	}
	search, err := NewTaskHistorySearch(456, 10, run)
	if err != nil {
		t.Fatal(err)
	}
	search.read = func(ctx context.Context, bot int64, q ArchiveQuery) (ArchivePage, error) {
		return readArchive(ctx, filepath.Join(filepath.Dir(a.legacy.root), "history"), bot, q, historyConfig)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		n := calls.Add(1)
		body := string(raw)
		reply := `{"action":"search_history","params":{"limit":1,"before":"9007199254740993","from":"2026-10-15T08:00:00+08:00","until":"2026-10-15T00:00:00.000000001Z"}}`
		if n == 1 && strings.Contains(body, "TIME_TARGET") {
			t.Error("old source leaked into initial context")
		}
		if n == 2 {
			if !strings.Contains(body, "TIME_TARGET") || strings.Contains(body, "DISTRACTOR") || !strings.Contains(body, `\"from\":\"2026-10-15T00:00:00Z\"`) || !strings.Contains(body, `\"until\":\"2026-10-15T00:00:00.000000001Z\"`) {
				t.Error("filtered source/applied bounds absent or distractor leaked")
			}
			reply = `{"action":"end_action","params":{}}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": reply}}}})
	}))
	defer server.Close()
	api := deepinfra.NewAPI(server.URL, "")
	api.SetHTTPClient(server.Client())
	err = RunAgent(func(iter int) ([]zero.APIRequest, bool, error) {
		return NextAgentActionsChecked(&ag, false, iter, api, model.NewOpenAI("synthetic", "", 0, 1, 512, ""), 10, goba.PermRoleUser, run.Check)
	}, func(req zero.APIRequest) error {
		if req.Action != goba.SearchHistory {
			t.Fatal("local query forwarded")
		}
		if err := run.Action(req.Action); err != nil {
			return err
		}
		if err := search.Execute(&ag, req); err != nil {
			return err
		}
		return run.Result(search.Outcome())
	}, func() bool { return true })
	if err != nil || calls.Load() != 2 {
		t.Fatal(err, calls.Load())
	}
	if err = run.Finish(nil); err != nil {
		t.Fatal(err)
	}
	v, err := a.history.tasks.Read(context.Background(), 10, run.info.ID)
	if err != nil || v.Status != "completed" || v.Queries.Used != 1 || v.Queries.Successful != 1 || v.Queries.SearchHistory != 1 || v.Queries.Empty != 0 {
		t.Fatal(v, err)
	}
}

func TestAgentSearchTimeQuotaSurvivesReconstructionAndFailure(t *testing.T) {
	h, run, ag, s := queryTaskFixture(t)
	s.read = func(context.Context, int64, ArchiveQuery) (ArchivePage, error) {
		return ArchivePage{}, errors.New("private provider path")
	}
	for _, req := range []zero.APIRequest{
		{Action: goba.SearchHistory, Params: zero.Params{"keyword": "needle", "from": "bad"}},
		{Action: goba.SearchHistory, Params: zero.Params{"keyword": "needle", "until": "2026-10-15T00:00:00Z"}},
		{Action: goba.SearchMemory, Params: zero.Params{"keyword": "needle", "until": ""}},
	} {
		if err := run.Action(req.Action); err != nil {
			t.Fatal(err)
		}
		if err := s.Execute(ag, req); err != nil {
			t.Fatal(err)
		}
		if err := run.Result(s.Outcome()); err != nil {
			t.Fatal(err)
		}
	}
	other, err := NewTaskHistorySearch(456, 10, run)
	if err != nil {
		t.Fatal(err)
	}
	if err = run.Action(goba.SearchHistory); err != nil {
		t.Fatal(err)
	}
	if err = other.Execute(ag, zero.APIRequest{Action: goba.SearchHistory, Params: zero.Params{"keyword": "needle", "from": ""}}); err == nil {
		t.Fatal("range reset quota")
	}
	if err = run.Result(other.Outcome()); err != nil {
		t.Fatal(err)
	}
	if err = run.Finish(errors.New("stop")); err != nil {
		t.Fatal(err)
	}
	v, err := h.tasks.Read(context.Background(), 10, run.info.ID)
	if err != nil || v.Queries.Used != 3 || v.Queries.Errors != 3 || v.Queries.Denied != 1 || v.Queries.Successful != 0 {
		t.Fatal(v, err)
	}
}
