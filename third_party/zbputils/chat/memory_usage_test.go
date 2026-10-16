package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
	goba "github.com/fumiama/go-onebot-agent"
)

func TestMemoryUsageBoundsAndPersistence(t *testing.T) {
	for _, tc := range []struct {
		name              string
		count, size, want int
		reason            string
	}{
		{"empty", 0, 1, 0, ""}, {"exact", 100, 1, 100, ""}, {"count", 101, 1, 100, "record_limit"}, {"bytes", 10, 3900, 7, "byte_limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := memoryFixture(t)
			for i := 0; i < tc.count; i++ {
				if e := a.Save(10, fmt.Sprintf("%d-%s", i, strings.Repeat("x", tc.size))); e != nil {
					t.Fatal(e)
				}
			}
			r, e := a.history.tasks.Begin(10)
			if e != nil {
				t.Fatal(e)
			}
			if e = r.Model(); e != nil {
				t.Fatal(e)
			}
			lines, e := a.LoadChecked(10)
			if e != nil {
				t.Fatal(e)
			}
			u := taskSnapshot(t, a.history.tasks, 10).Memory
			bytes := 0
			for _, line := range lines {
				bytes += len(line)
			}
			if !u.Available || u.Mode != "managed" || u.Records != tc.want || u.Bytes != bytes || u.BudgetBytes != 30000 || u.Limit != 100 || u.Reason != tc.reason || u.Clipped != (tc.reason != "") {
				t.Fatalf("usage %+v", u)
			}
			// Other scopes and robots cannot replace the active task's summary.
			if _, e = a.LoadChecked(-10); e != nil {
				t.Fatal(e)
			}
			if _, e = b.LoadChecked(10); e != nil {
				t.Fatal(e)
			}
			if taskSnapshot(t, a.history.tasks, 10).Memory != u {
				t.Fatal("scope leaked")
			}
			if e = r.Finish(nil); e != nil {
				t.Fatal(e)
			}
			reopened, e := newTaskStore(a.history.db, "usage-restart")
			if e != nil {
				t.Fatal(e)
			}
			if taskSnapshot(t, reopened, 10).Memory != u {
				t.Fatal("summary not persisted")
			}
		})
	}
}

func TestMemoryUsageModesFailuresAndRoundReset(t *testing.T) {
	a, b := memoryFixture(t)
	ctx := context.Background()
	if e := os.WriteFile(a.legacyPath(10), []byte("legacy"), 0600); e != nil {
		t.Fatal(e)
	}
	r, _ := a.history.tasks.Begin(10)
	if e := r.Model(); e != nil {
		t.Fatal(e)
	}
	if _, e := a.LoadChecked(10); e != nil {
		t.Fatal(e)
	}
	if u := taskSnapshot(t, a.history.tasks, 10).Memory; !u.Available || u.Mode != "legacy" || u.Records != 0 {
		t.Fatal(u)
	}
	p, e := a.Preview(ctx, 10)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.Migrate(ctx, 10, p.Digest); e != nil {
		t.Fatal(e)
	}
	other, _ := b.history.tasks.Begin(10)
	other.Model()
	if _, e = b.LoadChecked(10); e != nil {
		t.Fatal(e)
	}
	if u := taskSnapshot(t, b.history.tasks, 10).Memory; !u.Available || u.Mode != "claimed_elsewhere" {
		t.Fatal(u)
	}
	if e = r.Model(); e != nil {
		t.Fatal(e)
	}
	if taskSnapshot(t, a.history.tasks, 10).Memory.Available {
		t.Fatal("new round retained old usage")
	}
	if e = a.Update(ctx, 10, 1, 1, "disable", ""); e != nil {
		t.Fatal(e)
	}
	if _, e = a.LoadChecked(10); e != nil {
		t.Fatal(e)
	}
	if u := taskSnapshot(t, a.history.tasks, 10).Memory; !u.Available || u.Records != 0 {
		t.Fatal("disabled counted", u)
	}
	if e = r.FallbackModel(); e != nil {
		t.Fatal(e)
	}
	if _, e = a.LoadChecked(10); e != nil {
		t.Fatal(e)
	}
	if taskSnapshot(t, a.history.tasks, 10).Memory.Available {
		t.Fatal("fallback mislabeled")
	}
	r.Model()
	if _, e = a.history.db.Exec("DROP TABLE agent_memories"); e != nil {
		t.Fatal(e)
	}
	if _, e = a.LoadChecked(10); e == nil {
		t.Fatal("read failure hidden")
	}
	if u := taskSnapshot(t, a.history.tasks, 10).Memory; u.Available || u.Reason != "read_error" {
		t.Fatal(u)
	}
}

func TestMemoryUsageSaveFailureStopsPreparation(t *testing.T) {
	a, _ := memoryFixture(t)
	r, _ := a.history.tasks.Begin(10)
	r.Model()
	if _, e := a.history.db.Exec(`CREATE TRIGGER reject_usage BEFORE UPDATE ON agent_tasks BEGIN SELECT RAISE(ABORT,'synthetic'); END`); e != nil {
		t.Fatal(e)
	}
	if lines, e := a.LoadChecked(10); !errors.Is(e, ErrAgentTaskStore) || lines != nil {
		t.Fatal("unrecorded input allowed", e)
	}
	if e := r.Check(); !errors.Is(e, ErrAgentTaskStore) {
		t.Fatal(e)
	}
}

func TestMemoryUsageMatchesModelPreparation(t *testing.T) {
	a, _ := memoryFixture(t)
	if e := a.Save(10, "synthetic-memory-K29"); e != nil {
		t.Fatal(e)
	}
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	if e := a.history.Append(10, "event", historyEvent(10, "hello")); e != nil {
		t.Fatal(e)
	}
	r, _ := a.history.tasks.Begin(10)
	r.Model()
	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		data, _ := io.ReadAll(req.Body)
		requestBody = string(data)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": "{\"action\":\"end_action\",\"params\":{}}"}}}})
	}))
	defer server.Close()
	api := deepinfra.NewAPI(server.URL, "fixture")
	api.SetHTTPClient(server.Client())
	if _, _, e := NextAgentActions(&ag, false, 0, api, model.NewOpenAI("fixture", "", .7, 1, 512, ""), 10, goba.PermRoleUser); e != nil {
		t.Fatal(e)
	}
	v := taskSnapshot(t, a.history.tasks, 10)
	if !strings.Contains(requestBody, "synthetic-memory-K29") || !v.Memory.Available || v.Memory.Records != 1 || !v.Context.Available {
		t.Fatalf("preparation not captured: %+v", v)
	}
	encoded, _ := json.Marshal(v)
	if strings.Contains(string(encoded), "synthetic-memory-K29") {
		t.Fatal("task persisted plaintext")
	}
}
