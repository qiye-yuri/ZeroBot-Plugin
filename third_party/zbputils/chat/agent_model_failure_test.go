package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func TestAgentModelRequestFailuresPersistWithoutDispatch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{{"unauthorized", 401, "SYNTHETIC_PROVIDER_FAILURE_PRIVATE"}, {"unavailable", 503, "SYNTHETIC_PROVIDER_FAILURE_PRIVATE"}, {"invalid-protocol", 200, "SYNTHETIC_PROVIDER_FAILURE_PRIVATE"}, {"empty-protocol", 200, ""},
		{"missing-output", 200, `{"error":{"message":"SYNTHETIC_PROVIDER_FAILURE_PRIVATE"}}`},
		{"unsupported-output", 200, `{"choices":[{"message":{"content":[{"type":"text","text":"{\"action\":\"save_memory\",\"params\":{\"text\":\"MUST_NOT_SAVE\"}}"},{"type":"private-unknown-type","text":"SYNTHETIC_PROVIDER_FAILURE_PRIVATE"}]}}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := taskFixture(t)
			r, err := h.tasks.Begin(10)
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				calls.Add(1)
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer server.Close()
			ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", preparationTestMemory{}, true, true)
			ag.SetHistoryStore(h)
			api := deepinfra.NewAPI(server.URL, "")
			api.SetHTTPClient(server.Client())
			executed := 0
			err = RunAgent(func(iter int) ([]zero.APIRequest, bool, error) {
				if err := r.Model(); err != nil {
					return nil, false, err
				}
				return NextAgentActionsChecked(&ag, false, iter, api, model.NewOpenAI("agent", "", 0, 1, 128, ""), 10, goba.PermRoleUser, r.Check)
			}, func(zero.APIRequest) error { executed++; return nil }, func() bool { return true })
			if !errors.Is(err, goba.ErrModelRequestFailed) || executed != 0 || calls.Load() != 1 {
				t.Fatal("model failure dispatched or retried", err, executed, calls.Load())
			}
			if e := r.Finish(err); e != nil {
				t.Fatal(e)
			}
			v, e := h.tasks.Read(context.Background(), 10, r.info.ID)
			raw, _ := json.Marshal(v)
			if e != nil || v.Status != "failed" || v.Error != "model_request_failed" || !v.Persisted || v.Activity.ModelPreparations != 1 || v.Activity.FallbackPreparations != 0 || v.Activity.LocalActions != 0 || v.Activity.PlatformActions != 0 || v.Results != (TaskResults{Available: true}) || v.Queries.Used != 0 || strings.Contains(string(raw), "SYNTHETIC_PROVIDER_FAILURE_PRIVATE") {
				t.Fatal("incorrect model failure summary", e, v)
			}
			var records int
			if e = h.db.QueryRow(`SELECT count(*) FROM history WHERE kind IN ('request','response','end')`).Scan(&records); e != nil || records != 0 {
				t.Fatal("failed model request created action records", e, records)
			}
		})
	}
}

func TestAgentModelFailureCategoryPriority(t *testing.T) {
	for _, tc := range []struct {
		name, status, code string
		cause              error
	}{
		{"model", "failed", "model_request_failed", goba.ErrModelRequestFailed},
		{"settings", "failed", "settings_changed", errors.Join(goba.ErrModelRequestFailed, ErrAgentReloaded)},
		{"size", "failed", "model_output_limit", errors.Join(goba.ErrModelRequestFailed, goba.ErrActionOutputLimit)},
		{"unrelated", "failed", "execution_error", errors.New("synthetic local failure")},
		{"cancel", "cancelled", "", goba.ErrModelRequestFailed},
		{"storage", "failed", "storage_error", goba.ErrModelRequestFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := taskFixture(t)
			r, err := h.tasks.Begin(10)
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "cancel" {
				if _, err = h.tasks.Cancel(context.Background(), 10, r.info.ID, r.info.Version); err != nil {
					t.Fatal(err)
				}
			}
			if tc.name == "storage" {
				if _, err = h.db.Exec(`CREATE TRIGGER fail_model_finish BEFORE UPDATE ON agent_tasks BEGIN SELECT RAISE(FAIL,'synthetic'); END`); err != nil {
					t.Fatal(err)
				}
			}
			err = r.Finish(tc.cause)
			if (tc.name == "storage" && err != ErrAgentTaskStore) || (tc.name != "storage" && err != nil) || r.info.Status != tc.status || r.info.Error != tc.code {
				t.Fatal("model failure masked higher priority state", err, r.info)
			}
		})
	}
}
