package chat

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
	goba "github.com/fumiama/go-onebot-agent"
)

func TestPersonaTaskSnapshotReachesModelWithoutChangingSharedAgent(t *testing.T) {
	h := taskFixture(t)
	ag := goba.NewAgent(&goba.Config{Chars: "LEGACY_PERSONA"}, 16, 8, time.Hour, "", preparationTestMemory{}, true, true)
	ag.SetHistoryStore(h)
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"{\"action\":\"end_action\"}"}}]}`)
	}))
	defer server.Close()
	api := deepinfra.NewAPI(server.URL, "")
	api.SetHTTPClient(server.Client())
	cfg := AgentConfig{AgentChar: "GLOBAL_PERSONA", Personas: map[string]AgentPersona{personaKey(1, 10): {Character: "SNAPSHOT_PERSONA", Sex: "女"}}}
	p, _ := cfg.Persona(1, 10)
	character := goba.Config{Chars: p.Character, Sex: p.Sex}
	cfg.Personas[personaKey(1, 10)] = AgentPersona{Character: "NEW_PERSONA"}
	for i := 0; i < 2; i++ {
		if _, _, err := NextAgentActionsChecked(&ag, false, i, api, model.NewOpenAI("agent", "", 0, 1, 128, ""), 10, goba.PermRoleUser, nil, character); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := NextAgentActionsChecked(&ag, false, 0, api, model.NewOpenAI("agent", "", 0, 1, 128, ""), 10, goba.PermRoleUser, nil); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 3 {
		t.Fatal(len(bodies))
	}
	for _, body := range bodies[:2] {
		if !strings.Contains(body, "SNAPSHOT_PERSONA") || strings.Contains(body, "NEW_PERSONA") || strings.Contains(body, "LEGACY_PERSONA") {
			t.Fatal("task changed or mixed persona")
		}
	}
	if !strings.Contains(bodies[2], "LEGACY_PERSONA") || strings.Contains(bodies[2], "SNAPSHOT_PERSONA") {
		t.Fatal("shared agent was mutated")
	}
}
