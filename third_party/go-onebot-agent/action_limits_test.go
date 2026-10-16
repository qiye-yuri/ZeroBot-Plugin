package goba

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
)

type actionLimitMemory struct{ saved atomic.Int32 }

func (m *actionLimitMemory) Save(int64, string) error { m.saved.Add(1); return nil }
func (*actionLimitMemory) Load(int64) []string        { return nil }

func actionLimitBatch(count int, end bool) string {
	parts := []string{}
	for i := 0; i < count; i++ {
		parts = append(parts, `{"action":"send_group_msg","params":{"group_id":10,"message":"`+strconv.Itoa(i)+`"}}`)
	}
	if end {
		parts = append(parts, `{"action":"end_action","params":{}}`)
	}
	return strings.Join(parts, "\n")
}

func TestActionObjectPreflightBoundaries(t *testing.T) {
	for _, tc := range []struct {
		input   string
		limited bool
	}{{actionLimitBatch(32, false), false}, {actionLimitBatch(31, true), false}, {actionLimitBatch(32, true), true}, {actionLimitBatch(33, false), true}, {strings.Repeat("{}\n", 32), false}, {strings.Repeat("{}\n", 33), true}, {`{"action":"end_action"}` + "\n" + actionLimitBatch(40, false), false}} {
		err := checkActionObjects(tc.input)
		if errors.Is(err, ErrActionOutputLimit) != tc.limited {
			t.Fatal("preflight mismatch", tc.limited, err)
		}
	}
}

func TestActionLimitsRejectBeforeAutomaticWrites(t *testing.T) {
	prefix := `{"action":"send_group_msg","params":{"group_id":10,"message":"`
	suffix := `"}}`
	exact := prefix + strings.Repeat("x", agentActionOutputBytes-len(prefix)-len(suffix)) + suffix
	for _, tc := range []struct {
		name, output   string
		limited, ended bool
		requests       int
	}{{"exact-bytes", exact, false, false, 1}, {"byte-overflow", exact + " ", true, false, 0}, {"exact-objects", actionLimitBatch(32, false), false, false, 32}, {"end-within-limit", actionLimitBatch(31, true), false, true, 31}, {"object-overflow", actionLimitBatch(32, true), true, false, 0}, {"fenced-overflow", "```json\n" + actionLimitBatch(33, false) + "\n```", true, false, 0}, {"auto-save-prefix", `{"action":"save_memory","params":{"text":"MUST_NOT_SAVE"}}` + "\n" + actionLimitBatch(31, true), true, false, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": tc.output}}}})
			}))
			defer server.Close()
			memory := &actionLimitMemory{}
			history := &preparationHistory{}
			manual := !tc.limited
			ag := NewAgent(&Config{}, 16, 8, time.Hour, "", memory, manual, manual)
			ag.SetHistoryStore(history)
			api := deepinfra.NewAPI(server.URL, "")
			api.SetHTTPClient(server.Client())
			reqs, err := ag.GetAction(api, model.NewOpenAI("synthetic", "", 0, 1, 128), 10, PermRoleOwner, 0, false)
			if tc.limited {
				if !errors.Is(err, ErrActionOutputLimit) || len(reqs) != 0 || history.appends.Load() != 0 || memory.saved.Load() != 0 {
					t.Fatal("limited output caused partial writes", err, len(reqs), history.appends.Load(), memory.saved.Load())
				}
			} else if len(reqs) != tc.requests || (tc.ended && !errors.Is(err, io.EOF)) || (!tc.ended && err != nil) {
				t.Fatal("valid output changed", err, len(reqs))
			}
		})
	}
}

func TestActionSyntaxPreflight(t *testing.T) {
	for _, tc := range []struct {
		name, output string
		invalid      bool
	}{
		{"empty-compatible", " \n\t", false},
		{"empty-objects-compatible", "{}\n{}", false},
		{"concatenated", actionLimitBatch(2, false), false},
		{"number-compatible", `{"action":"send_group_msg","params":{"group_id":9007199254740993,"value":1e999}}`, false},
		{"first-end-compatible", `{"action":"end_action"}]ignored`, false},
		{"closing-array", actionLimitBatch(1, false) + "]", true},
		{"closing-object", actionLimitBatch(1, false) + "}", true},
		{"truncated", actionLimitBatch(1, false) + `{"action":`, true},
		{"prose", actionLimitBatch(1, false) + " explanation", true},
		{"null", "null", true},
		{"array", "[]", true},
		{"number", "1", true},
		{"string", `"action"`, true},
		{"wrong-action-type", `{"action":3}`, true},
		{"wrong-params-type", `{"action":"send_group_msg","params":[]}`, true},
		{"invalid-utf8", `{"action":"` + string([]byte{0xff}) + `"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkActionObjects(tc.output)
			if (tc.invalid && !errors.Is(err, ErrActionOutputInvalid)) || (!tc.invalid && err != nil) {
				t.Fatal("unexpected syntax validation", err)
			}
		})
	}
}

func TestMalformedActionBatchRejectsBeforeAutomaticWrites(t *testing.T) {
	prefix := `{"action":"save_memory","params":{"text":"MUST_NOT_SAVE"}}` + "\n" + actionLimitBatch(1, false)
	for _, tail := range []string{"]", "}", `{"action":`, " explanation", "null", "[]", `{"action":3}`, `{"action":"send_group_msg","params":[]}`,
		`{"action":"send_group_msg","action":"end_action"}`,
		`{"action":"send_group_msg","params":{"group_id":10,"group_id":20}}`,
		`{"action":"send_group_msg","params":{"message":[{"type":"text","data":{"text":"a","text":"b"}}]}}`,
		`{"action":"send_group_msg","params":{"group_id":10},"Params":{"message":"x"}}`,
	} {
		for _, manual := range []bool{false, true} {
			t.Run(strconv.FormatBool(manual)+"/"+tail, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": "```json\n" + prefix + "\n" + tail + "\n```"}}}})
				}))
				defer server.Close()
				memory := &actionLimitMemory{}
				history := &preparationHistory{}
				ag := NewAgent(&Config{}, 16, 8, time.Hour, "", memory, manual, manual)
				ag.SetHistoryStore(history)
				api := deepinfra.NewAPI(server.URL, "")
				api.SetHTTPClient(server.Client())
				reqs, err := ag.GetAction(api, model.NewOpenAI("synthetic", "", 0, 1, 128), 10, PermRoleOwner, 0, false)
				if !errors.Is(err, ErrActionOutputInvalid) || reqs != nil || history.appends.Load() != 0 || memory.saved.Load() != 0 {
					t.Fatal("malformed batch partially applied", err, len(reqs), history.appends.Load(), memory.saved.Load())
				}
			})
		}
	}
}
