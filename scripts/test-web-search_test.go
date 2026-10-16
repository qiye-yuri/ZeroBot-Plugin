// Run with test-real-model.go and test-real-model_test.go to reuse the isolated
// binary harness and saved-revision helpers. Real Search is separately opt-in.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
)

var realSearchConfig = flag.String("real-search-config", "", "opt-in: private JSON snapshot with aiConfig official DeepSeek API/Key; one billable search, synthetic model/OneBot")

type searchRuntimePage struct {
	Status  string                        `json:"status"`
	Results []struct{ Title, URL string } `json:"results"`
}

func searchToolPage(raw []byte) searchRuntimePage {
	var body struct {
		Messages []struct{ Content json.RawMessage }
	}
	json.Unmarshal(raw, &body)
	for _, m := range body.Messages {
		var content any
		json.Unmarshal(m.Content, &content)
		texts := []string{}
		switch value := content.(type) {
		case string:
			texts = append(texts, value)
		case []any:
			for _, part := range value {
				if p, ok := part.(map[string]any); ok {
					if text, ok := p["text"].(string); ok {
						texts = append(texts, text)
					}
				}
			}
		}
		for _, text := range texts {
			for _, line := range strings.Split(text, "\n") {
				var response struct{ Data searchRuntimePage }
				if json.Unmarshal([]byte(line), &response) == nil && response.Data.Status != "" {
					return response.Data
				}
			}
		}
	}
	return searchRuntimePage{}
}

func TestWebSearchIsolatedRuntime(t *testing.T) {
	if *validationBinary == "" {
		t.Skip("supply -validation-binary for final binary integration")
	}
	for _, tc := range []struct {
		name            string
		enabled, unsent bool
	}{{"disabled", false, false}, {"enabled", true, false}, {"tool-error-without-send", true, true}} {
		t.Run(tc.name, func(t *testing.T) { webSearchRuntime(t, tc.enabled, "synthetic-search-runtime-key", false, tc.unsent) })
	}
}
func TestWebSearchRealProviderRuntime(t *testing.T) {
	if *validationBinary == "" || *realSearchConfig == "" {
		t.Skip("real Search requires explicit -validation-binary and -real-search-config")
	}
	credentialPath := *realSearchConfig
	if !filepath.IsAbs(credentialPath) {
		root, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(root) == "scripts" {
			root = filepath.Dir(root)
		}
		credentialPath = filepath.Join(root, credentialPath)
	}
	raw, err := os.ReadFile(credentialPath)
	if err != nil {
		t.Fatal("private configuration snapshot unavailable")
	}
	var source struct{ AIConfig struct{ API, Key string } }
	if json.Unmarshal(raw, &source) != nil {
		t.Fatal("invalid private snapshot")
	}
	u, err := url.Parse(source.AIConfig.API)
	if err != nil || u.Scheme != "https" || u.Hostname() != "api.deepseek.com" || source.AIConfig.Key == "" {
		t.Fatal("snapshot does not contain an official DeepSeek credential")
	}
	webSearchRuntime(t, true, source.AIConfig.Key, true, false)
}

func webSearchRuntime(t *testing.T, enabled bool, key string, real, unsent bool) {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(root) == "scripts" {
		root = filepath.Dir(root)
	}
	base := filepath.Join(root, ".local/n3/runtime")
	os.MkdirAll(base, 0700)
	out, err := os.MkdirTemp(base, "run-")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("ARTIFACTS=" + out)
	report := map[string]any{"realSearch": real, "syntheticModel": true, "syntheticOneBot": true, "realQQMessagesSent": false, "enabled": enabled}
	defer func() {
		report["passed"] = !t.Failed()
		if err := qa.WriteJSON(filepath.Join(out, "result.json"), report); err != nil {
			t.Error(err)
		}
	}()
	var mu sync.Mutex
	rounds := 0
	page := searchRuntimePage{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		rounds++
		reply := `{"action":"search_web","params":{"query":"Go programming language official documentation"}}`
		if !real {
			reply = `{"action":"search_web","params":{"query":"public","group_id":99}}`
		}
		if rounds > 1 {
			page = searchToolPage(body)
			reply = fmt.Sprintf(`{"action":"send_private_msg","params":{"user_id":%d,"message":"N3 搜索处理完成"}}`+`{"action":"end_action"}`, qa.Owner)
			if unsent {
				reply = `{"action":"end_action"}`
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": reply}}}})
	}))
	defer server.Close()
	h, err := qa.New(root, filepath.Join(root, ".local/bin", *validationBinary), out)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	bot, err := h.Start(ctx, "search")
	if err != nil {
		t.Fatal(err)
	}
	defer bot.Stop()
	c := qa.NewClient(h.Address)
	if _, err = c.Auth(h.AdminDir); err != nil {
		t.Fatal(err)
	}
	v, err := c.Checked("GET", "/api/ai", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Checked("POST", "/api/ai", map[string]any{"revision": v["global"].(map[string]any)["revision"], "fields": map[string]any{"Type": 0, "ModelName": "synthetic", "API": server.URL, "AgentType": 0, "AgentModelName": "synthetic", "AgentAPI": server.URL, "MaxN": 2048}, "keys": map[string]any{"Key": map[string]any{"action": "replace", "value": "synthetic-model-key"}, "AgentKey": map[string]any{"action": "replace", "value": "synthetic-model-key"}}})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := c.Checked("GET", "/api/ai-search?"+scopeQuery(qa.Owner), nil)
	if err != nil {
		t.Fatal(err)
	}
	body := scope(qa.Owner)
	body["revision"], body["enabled"], body["key"], body["policy"] = settings["revision"], enabled, map[string]any{"action": "replace", "value": key}, map[string]any{"enabled": true, "roles": []string{"owner"}}
	if _, err = c.Checked("POST", "/api/ai-search", body); err != nil {
		t.Fatal(err)
	}
	if err = configure(c, qa.Owner, true); err != nil {
		t.Fatal(err)
	}
	plugins, err := c.Checked("GET", "/api/plugins", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range plugins["plugins"].([]any) {
		p := raw.(map[string]any)
		if p["name"] == "aichat" {
			_, err = c.Checked("POST", "/api/plugin-switch", map[string]any{"name": "aichat", "groupId": "0", "mode": "enabled", "revision": p["state"].(map[string]any)["revision"]})
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	var reply string
	if enabled {
		reply, _, err = h.SendWithContext(ctx, qa.Owner, "请搜索公开资料进行合成验收")
	} else {
		// A rejected action must not produce a platform send; allow the existing
		// short harness timeout instead of waiting for a reply that must not exist.
		reply, _, err = h.Send(qa.Owner, "请搜索公开资料进行合成验收")
	}
	if err != nil {
		t.Fatal("isolated message did not receive a response", err)
	}
	qa.WriteJSON(filepath.Join(out, "reply.json"), map[string]any{"reply": reply})
	info, err := task(c, qa.Owner)
	if err != nil {
		t.Fatal(err)
	}
	report["task"] = info
	mu.Lock()
	received, modelRounds := page, rounds
	mu.Unlock()
	report["modelRounds"], report["resultCount"] = modelRounds, len(received.Results)
	if !enabled {
		if info["status"] != "failed" || modelRounds != 1 || reply != "" {
			t.Fatal("disabled forged search not rejected", info["status"], modelRounds)
		}
		return
	}
	if unsent && !strings.Contains(reply, "本次未取得搜索资料") {
		t.Fatal("early end lost the tool error")
	}
	if info["status"] != "completed" || modelRounds != 2 {
		t.Fatal("search task did not complete", info["status"], modelRounds)
	}
	usage := info["webSearch"].(map[string]any)
	if real {
		if received.Status != "ok" || len(received.Results) != 5 || usage["used"] != float64(1) || usage["successful"] != float64(1) {
			t.Fatal("real search did not return five results", received.Status, len(received.Results), usage)
		}
		for _, item := range received.Results {
			if !strings.Contains(reply, item.URL) {
				t.Fatal("actual source omitted from platform reply")
			}
		}
		qa.WriteJSON(filepath.Join(out, "results.json"), received)
	} else {
		if received.Status != "error" || usage["used"] != float64(0) || usage["denied"] != float64(1) || strings.Contains(reply, "搜索来源") {
			t.Fatal("invalid scope parameters reached search or invented sources", received.Status, usage)
		}
	}
	stats, err := c.Checked("GET", "/api/overview", nil)
	if err != nil {
		t.Fatal(err)
	}
	modelUsage := stats["modelRequests"].(map[string]any)
	report["modelRequests"] = modelUsage
	if modelUsage["started"] != float64(2) || modelUsage["completed"] != float64(2) || modelUsage["failed"] != float64(0) {
		t.Fatal("search incorrectly counted as model request", modelUsage)
	}
}
