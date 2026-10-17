package goba

import (
	"encoding/json"
	"errors"
	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebSearchPerCallPermissionAndPrompt(t *testing.T) {
	ag := NewAgent(&Config{}, 16, 8, time.Hour, "", &actionLimitMemory{}, false, false)
	ag.SetHistoryStore(&preparationHistory{})
	wantEnabled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct{ Role, Content string }
		}
		json.NewDecoder(r.Body).Decode(&body)
		has := strings.Contains(body.Messages[0].Content, "|search_web|")
		if has != wantEnabled {
			t.Error("per-call prompt permission leaked", has, wantEnabled)
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": `{"action":"search_web","params":{"query":"public"}}`}}}})
	}))
	defer server.Close()
	api := deepinfra.NewAPI(server.URL, "")
	api.SetHTTPClient(server.Client())
	for _, role := range []PermRole{PermRoleOwner, PermRoleAdmin, PermRoleUser} {
		for _, enabled := range []bool{false, true, false} {
			wantEnabled = enabled
			reqs, err := ag.GetActionChecked(api, model.NewOpenAI("fixture", "", 0, 1, 1024), 10, role, 1, false, func() error { return nil }, Config{WebSearch: enabled})
			if enabled {
				if err != nil || len(reqs) != 1 || reqs[0].Action != SearchWeb {
					t.Fatal(err, reqs)
				}
			} else if !errors.Is(err, ErrPermissionDenied) || len(reqs) != 0 {
				t.Fatal("unauthorized search accepted", err, reqs)
			}
		}
	}
}
