package chat

import (
	"context"
	"encoding/json"
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

func TestAgentSearchContinuationModelReadsOlderSourceWithinSharedQuota(t *testing.T) {
	a, _ := memoryFixture(t)
	a.history.budget = 8192
	for i, text := range []string{"needle OLDER_TARGET " + strings.Repeat("旧", 1100), "gap", "needle NEWER_TARGET"} {
		if _, err := a.history.db.Exec("INSERT INTO history(seq,conversation,kind,payload,created_at) VALUES(?,10,'event',?,?)", 9007199254740993+int64(i), `{"message":"`+text+`"}`, "2026-10-15T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 30; i++ {
		if err := a.history.Append(10, "event", historyEvent(10, strings.Repeat("近期", 200))); err != nil {
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
		if q.Search && q.SearchLimit != 1 {
			t.Error("limit not applied in archive query", q.SearchLimit)
		}
		return readArchive(ctx, filepath.Join(filepath.Dir(a.legacy.root), "history"), bot, q, historyConfig)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		reply := `{"action":"search_history","params":{"keyword":"needle","limit":1}}`
		if n > 1 {
			var raw string
			var response struct {
				Data struct {
					Sources    []ArchiveEntry `json:"sources"`
					SearchMore bool           `json:"searchMore"`
					NextBefore string         `json:"nextBefore"`
				} `json:"data"`
			}
			if err := a.history.db.QueryRow("SELECT payload FROM history WHERE conversation=10 AND kind='response' ORDER BY seq DESC LIMIT 1").Scan(&raw); err != nil || json.Unmarshal([]byte(raw), &response) != nil || len(response.Data.Sources) != 1 {
				t.Error("actual response unavailable", err)
				http.Error(w, "fixture", 500)
				return
			}
			data := response.Data
			source := data.Sources[0]
			switch n {
			case 2:
				if source.Seq != "9007199254740995" || !data.SearchMore || data.NextBefore != "9007199254740994" {
					t.Error("continuation lost exact scanned position", data)
				}
				raw, _ := json.Marshal(zero.APIRequest{Action: goba.SearchHistory, Params: zero.Params{"keyword": "needle", "limit": 1, "before": data.NextBefore}})
				reply = string(raw)
			case 3:
				if source.Seq != "9007199254740993" || !strings.Contains(source.Text, "OLDER_TARGET") || data.SearchMore || data.NextBefore != "" {
					t.Error("older source missing or false continuation", data)
				}
				raw, _ := json.Marshal(zero.APIRequest{Action: goba.ReadHistory, Params: zero.Params{"seq": source.Seq}})
				reply = string(raw)
			case 4:
				if source.Seq != "9007199254740993" || source.Truncated || !strings.Contains(source.Text, strings.Repeat("旧", 1100)) {
					t.Error("continued source was not read exactly")
				}
				reply = `{"action":"end_action","params":{}}`
			default:
				t.Error("unexpected model iteration", n)
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": reply}}}})
	}))
	defer server.Close()
	api := deepinfra.NewAPI(server.URL, "")
	api.SetHTTPClient(server.Client())
	err = RunAgent(func(iter int) ([]zero.APIRequest, bool, error) {
		return NextAgentActionsChecked(&ag, false, iter, api, model.NewOpenAI("synthetic", "", 0, 1, 512, ""), 10, goba.PermRoleUser, run.Check)
	}, func(req zero.APIRequest) error {
		if req.Action != goba.SearchHistory && req.Action != goba.ReadHistory {
			t.Fatal("local retrieval forwarded")
		}
		if err := run.Action(req.Action); err != nil {
			return err
		}
		if req.Action == goba.ReadHistory {
			var err error
			search, err = NewTaskHistorySearch(456, 10, run)
			if err != nil {
				return err
			}
			search.read = func(ctx context.Context, bot int64, q ArchiveQuery) (ArchivePage, error) {
				return readArchive(ctx, filepath.Join(filepath.Dir(a.legacy.root), "history"), bot, q, historyConfig)
			}
		}
		if err := search.Execute(&ag, req); err != nil {
			return err
		}
		return run.Result(search.Outcome())
	}, func() bool { return true })
	if err != nil || calls.Load() != 4 {
		t.Fatal(err, calls.Load())
	}
	if err = run.Finish(nil); err != nil {
		t.Fatal(err)
	}
	v, err := a.history.tasks.Read(context.Background(), 10, run.info.ID)
	if err != nil || v.Queries.Used != 3 || v.Queries.Successful != 3 || v.Queries.SearchHistory != 2 || v.Queries.ReadHistory != 1 || v.Status != "completed" {
		t.Fatal("continued task quota/state not persisted", v, err)
	}
}
