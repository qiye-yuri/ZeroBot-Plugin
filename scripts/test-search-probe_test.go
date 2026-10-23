package main

import (
	"bytes"
	"context"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
)

var probeBinary = flag.String("validation-binary", "", "final isolated binary under .local/bin")

func TestSearchProbeIsolatedRuntime(t *testing.T) {
	if *probeBinary == "" {
		t.Skip("supply -validation-binary after the final build")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(root) == "scripts" {
		root = filepath.Dir(root)
	}
	dir := filepath.Join(root, ".local/r9-runtime")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	out, err := os.MkdirTemp(dir, "run-")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("ARTIFACTS=" + out)
	// Refuse the HTTPS tunnel locally: the final binary exercises its real
	// transport and error path without connecting to a paid search service.
	var calls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "CONNECT" && r.Host == "api.deepseek.com:443" {
			calls.Add(1)
		}
		http.Error(w, "synthetic blocked tunnel", http.StatusBadGateway)
	}))
	defer proxy.Close()
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	h, err := qa.New(root, filepath.Join(root, ".local/bin", *probeBinary), out)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	bot, err := h.Start(ctx, "probe")
	if err != nil {
		t.Fatal(err)
	}
	defer bot.Stop()
	c := qa.NewClient(h.Address)
	if _, err = c.Auth(h.AdminDir); err != nil {
		t.Fatal(err)
	}
	api := func(method, path string, body any) map[string]any {
		t.Helper()
		v, e := c.Checked(method, path, body)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	const query = "botId=87654321&type=private&id=12345678"
	search := api("GET", "/api/ai-search?"+query, nil)
	saved := api("POST", "/api/ai-search", map[string]any{"botId": "87654321", "type": "private", "id": "12345678", "revision": search["revision"], "enabled": false, "key": map[string]any{"action": "replace", "value": "synthetic-search-key"}, "policy": map[string]any{"enabled": false, "roles": []string{"owner"}}})
	file := filepath.Join(out, "data/control/plugins.db")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	beforeBackups, _ := os.ReadDir(filepath.Join(h.AdminDir, "backups"))
	beforeSent := h.LastSentMessage()
	result := api("POST", "/api/ai-search-test", map[string]any{"revision": saved["revision"]})
	if result["category"] != "network_error" || result["resultCount"] != float64(0) || result["revision"] != saved["revision"] || calls.Load() != 1 {
		t.Fatal("unexpected diagnostic", result, calls.Load())
	}
	after, _ := os.ReadFile(file)
	afterBackups, _ := os.ReadDir(filepath.Join(h.AdminDir, "backups"))
	state := api("GET", "/api/ai-search?"+query, nil)
	if !bytes.Equal(before, after) || len(beforeBackups) != len(afterBackups) || !bytes.Equal(beforeSent, h.LastSentMessage()) || state["enabled"] != false || state["policy"].(map[string]any)["enabled"] != false || state["revision"] != saved["revision"] {
		t.Fatal("probe changed configuration or sent a message")
	}
	if err = qa.WriteJSON(filepath.Join(out, "result.json"), map[string]any{"passed": true, "isolated": true, "realQQMessagesSent": false, "externalSearchCalls": 0, "localProxyAttempts": calls.Load(), "readOnly": true, "globalAndScopeRemainDisabled": true, "networkFailureClassified": true}); err != nil {
		t.Fatal(err)
	}
}
