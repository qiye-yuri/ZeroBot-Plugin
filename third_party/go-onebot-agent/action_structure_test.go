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

// Six values besides the null leaves: root, action, params, group_id,
// message and extra array. Field names themselves are not JSON values.
func structureValueAction(leaves int) string {
	return `{"action":"send_group_msg","params":{"group_id":10,"message":"ok"},"extra":[` + strings.TrimSuffix(strings.Repeat("null,", leaves), ",") + `]}`
}

func structureDepthAction(arrays int) string {
	return `{"action":"send_group_msg","params":{"group_id":10,"message":"ok","extra":` + strings.Repeat("[", arrays) + "null" + strings.Repeat("]", arrays) + `}}`
}

func TestActionStructureBoundariesAndSharedBudget(t *testing.T) {
	end := `{"action":"end_action","params":{}}`
	for _, tc := range []struct {
		name, raw string
		limited   bool
	}{
		{"exact-depth", structureDepthAction(62), false}, {"depth-overflow", structureDepthAction(63), true},
		{"formerly-unbounded-depth", structureDepthAction(1000), true},
		{"exact-values", structureValueAction(9994), false}, {"values-overflow", structureValueAction(9995), true},
		{"shared-exact", structureValueAction(4994) + structureValueAction(4994), false},
		{"shared-overflow", structureValueAction(4995) + structureValueAction(4995), true},
		{"end-counted-exact", structureValueAction(9991) + end, false}, {"end-counted-overflow", structureValueAction(9994) + end, true},
		{"end-tail-ignored", end + structureDepthAction(1000) + structureValueAction(11000), false},
		{"end-object-itself-limited", `{"action":"end_action","extra":` + strings.Repeat("[", 64) + "null" + strings.Repeat("]", 64) + `}`, true},
		{"opaque-text", `{"action":"send_group_msg","params":{"message":"` + strings.Repeat("[]{}", 12000) + `","group_id":9007199254740993}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := checkActionObjects(tc.raw)
			if (tc.limited && !errors.Is(e, ErrActionOutputLimit)) || (!tc.limited && e != nil) {
				t.Fatal(e)
			}
		})
	}
	// Count object member values too, not only array leaves.
	var fields strings.Builder
	fields.WriteString(`{"action":"send_group_msg","params":{},"extra":{`)
	for i := 0; i < 10000; i++ {
		if i > 0 {
			fields.WriteByte(',')
		}
		fields.WriteString(`"` + strconv.Itoa(i) + `":0`)
	}
	fields.WriteString(`}}`)
	if e := checkActionObjects(fields.String()); !errors.Is(e, ErrActionOutputLimit) {
		t.Fatal("object members bypassed value budget", e)
	}
}

func TestActionStructureRejectsAutomaticPrefixesAndRecovers(t *testing.T) {
	prefix := `{"action":"save_memory","params":{"text":"MUST_NOT_SAVE"}}` + `{"action":"send_group_msg","params":{"group_id":10,"message":"MUST_NOT_SEND"}}`
	for _, tail := range []string{structureDepthAction(63), structureValueAction(10000), structureValueAction(4995) + structureValueAction(4995)} {
		t.Run("reject-and-recover", func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				output := prefix + tail
				if calls.Add(1) > 1 {
					output = structureDepthAction(62) + `{"action":"end_action","params":{}}`
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": output}}}})
			}))
			defer server.Close()
			history := &preparationHistory{}
			memory := &actionLimitMemory{}
			ag := NewAgent(&Config{}, 16, 8, time.Hour, "", memory, false, false)
			ag.SetHistoryStore(history)
			api := deepinfra.NewAPI(server.URL, "")
			api.SetHTTPClient(server.Client())
			p := model.NewOpenAI("synthetic", "", 0, 1, 128)
			reqs, e := ag.GetActionChecked(api, p, 10, PermRoleOwner, 1, false, func() error { return nil })
			if !errors.Is(e, ErrActionOutputLimit) || len(reqs) != 0 || history.appends.Load() != 0 || memory.saved.Load() != 0 {
				t.Fatal("partial writes", e, len(reqs), history.appends.Load(), memory.saved.Load())
			}
			reqs, e = ag.GetActionChecked(api, p, 10, PermRoleOwner, 2, false, func() error { return nil })
			if !errors.Is(e, io.EOF) || len(reqs) != 1 || memory.saved.Load() != 0 || calls.Load() != 2 {
				t.Fatal("normal recovery failed", e, len(reqs))
			}
		})
	}
}
