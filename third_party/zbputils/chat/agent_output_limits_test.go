package chat

import (
	"context"
	"encoding/json"
	"errors"
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

func TestAgentRejectedOutputFailsTaskWithoutDispatch(t *testing.T) {
	for _, kind := range []string{"objects", "bytes", "depth", "values", "shared-values", "closing", "truncated", "null", "duplicate-action", "duplicate-params", "duplicate-message"} {
		t.Run(kind, func(t *testing.T) {
			h := taskFixture(t)
			r, err := h.tasks.Begin(10)
			if err != nil {
				t.Fatal(err)
			}
			var output string
			if kind == "objects" {
				output = strings.Repeat(`{"action":"send_group_msg","params":{"group_id":10,"message":"MUST_NOT_SEND"}}`+"\n", 32) + `{"action":"end_action","params":{}}`
			} else if kind == "bytes" {
				output = strings.Repeat("x", (1<<20)+1)
			} else if kind == "depth" || kind == "values" || kind == "shared-values" {
				output = `{"action":"save_memory","params":{"text":"MUST_NOT_SAVE"}}` + `{"action":"send_group_msg","params":{"group_id":10,"message":"MUST_NOT_SEND"}}`
				if kind == "depth" {
					output += `{"action":"send_group_msg","params":{"group_id":10,"extra":` + strings.Repeat("[", 63) + "null" + strings.Repeat("]", 63) + `}}`
				} else {
					count, objects := 10001, 1
					if kind == "shared-values" {
						count, objects = 5001, 2
					}
					for i := 0; i < objects; i++ {
						output += `{"action":"send_group_msg","params":{"group_id":10,"extra":[` + strings.TrimSuffix(strings.Repeat("null,", count), ",") + `]}}`
					}
				}
			} else {
				tail := map[string]string{
					"closing": "]", "truncated": `{"action":`, "null": "null",
					"duplicate-action":  `{"action":"send_group_msg","Action":"end_action"}`,
					"duplicate-params":  `{"action":"send_group_msg","params":{"group_id":10,"group_id":20}}`,
					"duplicate-message": `{"action":"send_group_msg","params":{"message":[{"type":"text","data":{"text":"a","text":"b"}}]}}`,
				}[kind]
				output = `{"action":"save_memory","params":{"text":"MUST_NOT_SAVE"}}` + "\n" + `{"action":"send_group_msg","params":{"group_id":10,"message":"MUST_NOT_SEND"}}` + "\n" + tail
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				calls.Add(1)
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": output}}}})
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
			wantErr, wantCode := goba.ErrActionOutputLimit, "model_output_limit"
			if kind != "objects" && kind != "bytes" && kind != "depth" && kind != "values" && kind != "shared-values" {
				wantErr, wantCode = goba.ErrActionOutputInvalid, "model_output_invalid"
			}
			if !errors.Is(err, wantErr) || executed != 0 || calls.Load() != 1 {
				t.Fatal("limited output partially dispatched or retried", err, executed, calls.Load())
			}
			if e := r.Finish(err); e != nil {
				t.Fatal(e)
			}
			v, e := h.tasks.Read(context.Background(), 10, r.info.ID)
			if e != nil || v.Status != "failed" || v.Error != wantCode || !v.Persisted || v.Activity.ModelPreparations != 1 || v.Activity.LocalActions != 0 || v.Activity.PlatformActions != 0 || v.Results != (TaskResults{Available: true}) || v.Queries.Used != 0 {
				t.Fatal("incorrect failed task summary", e, v)
			}
			var records int
			if e = h.db.QueryRow(`SELECT count(*) FROM history WHERE kind IN ('request','response','end')`).Scan(&records); e != nil || records != 0 {
				t.Fatal("rejected model output entered action archive", e, records)
			}
		})
	}
}
