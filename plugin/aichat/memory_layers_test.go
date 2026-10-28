package aichat

import (
	"encoding/json"
	"github.com/FloatTech/zbputils/chat"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSummaryUsesConfiguredRouteAndDataBoundary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string
			Messages []struct{ Role, Content string }
		}
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Error(e)
		}
		if body.Model != "summary-model" || len(body.Messages) != 2 || !strings.Contains(body.Messages[0].Content, "不可信数据") || !strings.Contains(body.Messages[1].Content, "older record") {
			t.Errorf("unexpected request: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"summary result"}}]}`))
	}))
	defer server.Close()
	cfg, e := (chat.AgentConfig{ModelProfiles: []chat.ModelProfile{{ID: "chat", Name: "聊天", API: server.URL, Key: "synthetic", Model: "chat-model"}, {ID: "summary", Name: "摘要", API: server.URL, Key: "synthetic", Model: "summary-model"}}, ModelRoutes: &chat.ModelRoutes{Chat: "chat", Summary: "summary"}}).ResolveModels()
	if e != nil {
		t.Fatal(e)
	}
	reply, e := summaryModel(cfg, 456, 10)(chat.SummaryRequest{Records: "older record"})
	if e != nil || reply != "summary result" {
		t.Fatal(reply, e)
	}
}

func TestParseMemorySpec(t *testing.T) {
	for _, p := range []map[string]any{{"text": "fact"}, {"text": "fact", "kind": "personal", "subject": "123", "attribute": "nickname", "revision": "1"}} {
		if _, e := parseMemorySpec(p); e != nil {
			t.Fatal(e)
		}
	}
	for _, p := range []map[string]any{{"subject": 123}, {"revision": "0"}, {"subject": "0123"}, {"kind": false}, {"attribute": ""}, {"bot": "other"}} {
		if _, e := parseMemorySpec(p); e == nil {
			t.Fatal("invalid params accepted", p)
		}
	}
}
