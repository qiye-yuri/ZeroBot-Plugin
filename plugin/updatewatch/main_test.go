package updatewatch

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	core "github.com/FloatTech/ZeroBot-Plugin/internal/updatewatch"
	"github.com/FloatTech/zbputils/chat"
	_ "modernc.org/sqlite"
)

func TestModelDecisionUsesConfiguredRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "summary-model" || len(body.Messages) != 2 || !strings.Contains(body.Messages[0].Content, "不可信数据") {
			t.Errorf("wrong model or rules: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"findings\":[]}"}}]}`))
	}))
	defer server.Close()
	old := loadModelConfig
	defer func() { loadModelConfig = old }()
	loadModelConfig = func() (chat.AgentConfig, error) {
		return (chat.AgentConfig{ModelProfiles: []chat.ModelProfile{{ID: "chat", Name: "聊天", API: server.URL, Key: "synthetic", Model: "chat-model"}, {ID: "summary", Name: "总结", API: server.URL, Key: "synthetic", Model: "summary-model"}}, ModelRoutes: &chat.ModelRoutes{Chat: "chat", Summary: "summary"}}).ResolveModels()
	}
	if n, e := judge("Summary")(context.Background(), nil, nil); e != nil || len(n) != 0 {
		t.Fatalf("%v %v", n, e)
	}
}

// Opt-in real model check: reads only the configured model from a read-only DB,
// uses one already fetched public release, and never sends a QQ message.
func TestRealModelPublicRelease(t *testing.T) {
	dbPath := os.Getenv("UPDATEWATCH_TEST_DB")
	evidencePath := os.Getenv("UPDATEWATCH_TEST_EVIDENCE")
	if dbPath == "" || evidencePath == "" {
		t.Skip("explicit real-model test not requested")
	}
	db, e := sql.Open("sqlite3", "file:"+dbPath+"?mode=ro")
	if e != nil {
		t.Fatal("config DB unavailable")
	}
	defer db.Close()
	rows, e := db.Query("SELECT ext FROM __resp WHERE ext IS NOT NULL AND ext NOT IN ('','-')")
	if e != nil {
		t.Fatal("config query failed")
	}
	defer rows.Close()
	var selected chat.AgentConfig
	found := 0
	for rows.Next() {
		var raw string
		if rows.Scan(&raw) != nil {
			t.Fatal("config scan failed")
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal([]byte(raw), &fields) != nil || (fields["ModelName"] == nil && fields["ModelRoutes"] == nil) {
			continue
		}
		if json.Unmarshal([]byte(raw), &selected) != nil {
			t.Fatal("config invalid")
		}
		found++
	}
	if found != 1 {
		t.Fatal("ambiguous model configuration")
	}
	selected, e = selected.ResolveModels()
	if e != nil {
		t.Fatal("model routes invalid")
	}
	old := loadModelConfig
	defer func() { loadModelConfig = old }()
	loadModelConfig = func() (chat.AgentConfig, error) { return selected, nil }
	b, e := os.ReadFile(evidencePath)
	if e != nil {
		t.Fatal(e)
	}
	var sources map[string][]core.Event
	if json.Unmarshal(b, &sources) != nil || len(sources["ftb"]) == 0 {
		t.Fatal("no fetched official release")
	}
	event := sources["ftb"][0]
	var reply string
	oldInspector := inspectModelReply
	defer func() { inspectModelReply = oldInspector }()
	inspectModelReply = func(s string) { reply = s }
	result, e := judge("Summary")(context.Background(), []core.Change{{Current: event}}, nil)
	if e != nil {
		t.Log(reply)
		t.Fatal(e)
	}
	if len(result) != 1 {
		t.Fatalf("expected significant release decision, got %d", len(result))
	}
	t.Logf("Verified Chinese summary with original evidence: version=%s notices=%d (no QQ send)", event.Version, len(result))
	previous := event
	event.Text += "\n"
	repeated, e := judge("Summary")(context.Background(), []core.Change{{Current: event, Previous: &previous}}, result)
	if e != nil || len(repeated) != 0 {
		t.Fatalf("semantic duplicate not suppressed: count=%d error=%v", len(repeated), e)
	}
}
