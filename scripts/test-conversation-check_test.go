package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
)

var checkBinary = flag.String("validation-binary", "", "final isolated binary under .local/bin")

func TestConversationCheckIsolatedRuntime(t *testing.T) {
	if *checkBinary == "" {
		t.Skip("supply -validation-binary after the final build")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(root) == "scripts" {
		root = filepath.Dir(root)
	}
	if err = os.MkdirAll(filepath.Join(root, ".local/r7-runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	out, err := os.MkdirTemp(filepath.Join(root, ".local/r7-runtime"), "run-")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("ARTIFACTS=" + out)
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected provider request", 500)
	}))
	defer provider.Close()
	h, err := qa.New(root, filepath.Join(root, ".local/bin", *checkBinary), out)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	bot, err := h.Start(ctx, "initial")
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
	check := func() map[string]any { return api("GET", "/api/agent/conversation-check?"+query, nil) }
	initial := check()
	if initial["models"].(map[string]any)["named"] != false {
		t.Fatal("fixture was not legacy mode")
	}
	models := api("GET", "/api/ai-models", nil)
	profiles := []any{}
	for _, name := range []string{"chat", "agent", "image", "gpt", "summary"} {
		profiles = append(profiles, map[string]any{"id": name, "name": "synthetic " + name, "type": 0, "model": "r7-" + name, "api": provider.URL, "key": map[string]any{"action": "replace", "value": "synthetic-" + name}, "parameters": map[string]any{"maxN": 2048}})
	}
	api("POST", "/api/ai-models", map[string]any{"revision": models["revision"], "profiles": profiles, "routes": map[string]string{"chat": "chat", "agent": "agent", "image": "image", "gpt": "gpt", "summary": "summary"}})
	persona := api("GET", "/api/ai-persona?"+query, nil)
	api("POST", "/api/ai-persona", map[string]any{"botId": "87654321", "type": "private", "id": "12345678", "revision": persona["revision"], "mode": "custom", "character": "synthetic-private-body", "sex": "女"})
	search := api("GET", "/api/ai-search?"+query, nil)
	api("POST", "/api/ai-search", map[string]any{"botId": "87654321", "type": "private", "id": "12345678", "revision": search["revision"], "enabled": true, "key": map[string]any{"action": "replace", "value": "synthetic-search-key"}, "policy": map[string]any{"enabled": true, "roles": []string{"owner"}}})
	parameters := api("GET", "/api/ai?private=12345678", nil)
	for _, raw := range parameters["groups"].([]any) {
		p := raw.(map[string]any)
		if p["service"] == "aichatcfg" {
			api("POST", "/api/ai-group", map[string]any{"service": "aichatcfg", "privateId": "12345678", "revision": p["revision"], "rate": 17, "temperature": 80, "useAgent": true})
		}
	}
	plugins := api("GET", "/api/plugins", nil)
	for _, raw := range plugins["plugins"].([]any) {
		p := raw.(map[string]any)
		if p["name"] == "aichat" {
			api("POST", "/api/plugin-switch", map[string]any{"name": "aichat", "groupId": "0", "mode": "enabled", "revision": p["state"].(map[string]any)["revision"]})
		}
	}
	beforeDB, err := os.ReadFile(filepath.Join(out, "data/control/plugins.db"))
	if err != nil {
		t.Fatal(err)
	}
	beforeBackups, _ := os.ReadDir(filepath.Join(h.AdminDir, "backups"))
	saved := check()
	if saved["persona"].(map[string]any)["mode"] != "custom" || saved["search"].(map[string]any)["policy"].(map[string]any)["enabled"] != true || saved["conversation"].([]any)[0].(map[string]any)["rate"] != float64(17) {
		t.Fatal("saved state not reflected", saved)
	}
	entries := saved["models"].(map[string]any)["entries"].([]any)
	if len(entries) != 5 || entries[1].(map[string]any)["model"] != "r7-gpt" || entries[2].(map[string]any)["applicable"] != false || saved["plugins"].([]any)[0].(map[string]any)["service"] != "aichat" || saved["plugins"].([]any)[0].(map[string]any)["state"].(map[string]any)["source"] != "global" {
		t.Fatal("runtime routes or plugin mismatch", saved)
	}
	other := api("GET", "/api/agent/conversation-check?botId=87654321&type=group&id=12345678", nil)
	if other["persona"].(map[string]any)["mode"] != "inherit" || other["search"].(map[string]any)["policy"].(map[string]any)["enabled"] != false {
		t.Fatal("private scope leaked to group")
	}
	// R8 follows the summary scope through the existing read-only search endpoint.
	for _, scope := range []string{"private", "group"} {
		form := api("GET", "/api/ai-search?botId=87654321&type="+scope+"&id=12345678", nil)
		if form["botId"] != "87654321" || form["type"] != scope || form["id"] != "12345678" || form["policy"].(map[string]any)["enabled"] != (scope == "private") {
			t.Fatal("search shortcut scope mismatch", scope)
		}
	}
	afterDB, _ := os.ReadFile(filepath.Join(out, "data/control/plugins.db"))
	afterBackups, _ := os.ReadDir(filepath.Join(h.AdminDir, "backups"))
	if !bytes.Equal(beforeDB, afterDB) || len(beforeBackups) != len(afterBackups) || calls.Load() != 0 {
		t.Fatal("check wrote configuration, created backups or called model")
	}
	raw, _ := json.Marshal(saved)
	for _, secret := range []string{"synthetic-search-key", "synthetic-private-body", provider.URL, "synthetic-chat"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatal("private content exposed")
		}
	}
	bot.Stop()
	bot, err = h.Start(ctx, "restarted")
	if err != nil {
		t.Fatal(err)
	}
	defer bot.Stop()
	c = qa.NewClient(h.Address)
	if _, err = c.Auth(h.AdminDir); err != nil {
		t.Fatal(err)
	}
	restarted := check()
	normalize := func(v map[string]any) {
		delete(v, "revision")
		delete(v, "observedAt")
		for _, raw := range v["conversation"].([]any) {
			delete(raw.(map[string]any), "revision")
		}
	}
	normalize(saved)
	normalize(restarted)
	if !reflect.DeepEqual(saved, restarted) || calls.Load() != 0 {
		t.Fatal("restart changed summary or invoked model")
	}
	if err = qa.WriteJSON(filepath.Join(out, "result.json"), map[string]any{"passed": true, "isolated": true, "realQQMessagesSent": false, "providerCalls": calls.Load(), "savedConfigurationReflected": true, "privateGroupIsolation": true, "readOnly": true, "restartPersisted": true}); err != nil {
		t.Fatal(err)
	}
}
