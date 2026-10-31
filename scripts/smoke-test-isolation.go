//go:build ignore

// Complete test-scope lifecycle against a built bot, local model and synthetic OneBot.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
	_ "modernc.org/sqlite"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var binary = flag.String("binary-name", "zerobot-test-isolation.exe", "built candidate under .local/bin")

func main() {
	flag.Parse()
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	root, e := os.Getwd()
	if e != nil {
		return e
	}
	parent := filepath.Join(root, ".local/test-isolation-smoke")
	if e = os.MkdirAll(parent, 0700); e != nil {
		return e
	}
	out, e := os.MkdirTemp(parent, "run-")
	if e != nil {
		return e
	}
	var mu sync.Mutex
	calls := 0
	testRounds := 0
	failures := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "invalid body", 400)
			return
		}
		raw, _ := json.Marshal(body)
		text := string(raw)
		mu.Lock()
		defer mu.Unlock()
		calls++
		reply := ""
		switch {
		case strings.Contains(text, "[密钥设置内容已隐藏]"):
			if strings.Contains(text, "credential-fixture-") {
				failures = append(failures, "credential command reached model")
			}
			reply = `{"action":"send_private_msg","params":{"user_id":12345678,"message":"CREDENTIAL_HIDDEN"}}` + `{"action":"end_action","params":{}}`
		case strings.Contains(text, "ISOLATION_DAILY_CHECK"):
			if strings.Contains(text, "ISOLATION_TEST_SAVE") || strings.Contains(text, "TEST_MEMORY") || strings.Contains(text, "batch1") || strings.Contains(text, "credential-fixture-") {
				failures = append(failures, "test data in daily request")
			}
			if !strings.Contains(text, "DAILY_MEMORY") {
				failures = append(failures, "daily memory missing")
			}
			reply = `{"action":"send_private_msg","params":{"user_id":12345678,"message":"DAILY_OK"}}` + `{"action":"end_action","params":{}}`
		case strings.Contains(text, "ISOLATION_SECOND"):
			if strings.Contains(text, "TEST_MEMORY") || strings.Contains(text, "DAILY_MEMORY") || strings.Contains(text, "ISOLATION_TEST_SAVE") {
				failures = append(failures, "another scope in batch2")
			}
			reply = `{"action":"send_private_msg","params":{"user_id":12345678,"message":"SECOND_OK"}}` + `{"action":"end_action","params":{}}`
		case strings.Contains(text, "ISOLATION_TEST_SAVE"):
			if strings.Contains(text, "DAILY_MEMORY") {
				failures = append(failures, "daily memory in test request")
			}
			testRounds++
			if testRounds == 1 {
				reply = `{"action":"save_memory","params":{"text":"TEST_MEMORY","kind":"personal","subject":"12345678","attribute":"test.value"}}`
			} else {
				if !strings.Contains(text, `\"mode\":\"test\"`) {
					failures = append(failures, "test save receipt missing")
				}
				reply = `{"action":"send_private_msg","params":{"user_id":12345678,"message":"TEST_OK"}}` + `{"action":"end_action","params":{}}`
			}
		default:
			failures = append(failures, "control command reached model")
			reply = `{"action":"end_action","params":{}}`
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": reply}}}})
	}))
	defer server.Close()
	program := filepath.Join(root, ".local/bin", *binary)
	h, e := qa.New(root, program, out)
	if e != nil {
		return e
	}
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	bot, e := h.Start(ctx, "test-isolation")
	if e != nil {
		return e
	}
	defer bot.Stop()
	c := qa.NewClient(h.Address)
	if _, e = c.Auth(h.AdminDir); e != nil {
		return e
	}
	v, e := c.Checked("GET", "/api/plugins", nil)
	if e != nil {
		return e
	}
	for _, raw := range v["plugins"].([]any) {
		p := raw.(map[string]any)
		if p["name"] == "aichat" {
			_, e = c.Checked("POST", "/api/plugin-switch", map[string]any{"name": "aichat", "groupId": "0", "mode": "enabled", "revision": p["state"].(map[string]any)["revision"]})
			if e != nil {
				return e
			}
		}
	}
	v, e = c.Checked("GET", "/api/ai", nil)
	if e != nil {
		return e
	}
	_, e = c.Checked("POST", "/api/ai", map[string]any{"revision": v["global"].(map[string]any)["revision"], "fields": map[string]any{"API": server.URL, "ModelName": "isolation", "Type": 0, "AgentAPI": server.URL, "AgentModelName": "isolation", "AgentType": 0}, "keys": map[string]any{"Key": map[string]any{"action": "replace", "value": "synthetic"}, "AgentKey": map[string]any{"action": "replace", "value": "synthetic"}}})
	if e != nil {
		return e
	}
	v, e = c.Checked("GET", "/api/ai?private=12345678", nil)
	if e != nil {
		return e
	}
	for _, raw := range v["groups"].([]any) {
		g := raw.(map[string]any)
		if g["service"] == "aichatcfg" {
			_, e = c.Checked("POST", "/api/ai-group", map[string]any{"service": "aichatcfg", "privateId": "12345678", "revision": g["revision"], "temperature": 20, "rate": 100, "useAgent": true, "respondAt": true, "voiceOutput": false})
			if e != nil {
				return e
			}
		}
	}
	scope := "botId=87654321&type=private&id=12345678"
	if _, e = c.Checked("GET", "/api/agent/memories?"+scope, nil); e != nil {
		return e
	}
	if _, e = c.Checked("POST", "/api/agent/memories/create", map[string]any{"botId": "87654321", "type": "private", "id": "12345678", "text": "DAILY_MEMORY"}); e != nil {
		return e
	}
	send := func(input, want string) error {
		reply, _, e := h.SendWithContext(ctx, qa.Owner, input)
		if e != nil {
			return e
		}
		if !strings.Contains(reply, want) {
			return fmt.Errorf("unexpected reply for %s: %s", input, reply)
		}
		return nil
	}
	if e = send("/测试会话 开始 batch1", "已开启"); e != nil {
		return e
	}
	if e = send("ISOLATION_TEST_SAVE", "TEST_OK"); e != nil {
		return e
	}
	// Wait for the task's terminal transition before ending the test scope.
	for i := 0; i < 100; i++ {
		v, e = c.Checked("GET", "/api/agent/tasks?"+scope, nil)
		if e != nil {
			return e
		}
		items := v["items"].([]any)
		if len(items) > 0 && items[0].(map[string]any)["status"] == "completed" {
			if items[0].(map[string]any)["testRun"] != "batch1" {
				return errors.New("task scope not persisted")
			}
			break
		}
		if i == 99 {
			return errors.New("test task did not finish")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if e = send("/测试会话 结束", "已结束"); e != nil {
		return e
	}
	if e = send("ISOLATION_DAILY_CHECK", "DAILY_OK"); e != nil {
		return e
	}
	time.Sleep(100 * time.Millisecond)
	if e = send("/测试消息 batch2 ISOLATION_SECOND", "SECOND_OK"); e != nil {
		return e
	}
	// Quote commands rather than execute credential setters. Each scope is
	// independent so the model must observe the newly redacted message.
	for i, command := range []string{"设置AI画图密钥", "设置b站cookie "} {
		if e = send(fmt.Sprintf("/测试消息 credential%d 引用设置命令：%scredential-fixture-%d", i, command, i), "CREDENTIAL_HIDDEN"); e != nil {
			return e
		}
		time.Sleep(100 * time.Millisecond)
	}
	if e = send("ISOLATION_DAILY_CHECK", "DAILY_OK"); e != nil {
		return e
	}
	v, e = c.Checked("GET", "/api/agent/memories?"+scope, nil)
	if e != nil {
		return e
	}
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "TEST_MEMORY") || !strings.Contains(string(b), "DAILY_MEMORY") {
		return errors.New("daily memory storage mixed")
	}
	db, e := sql.Open("sqlite3", "file:/"+filepath.ToSlash(filepath.Join(out, "data/aichat/history/testing/batch1/87654321.sqlite"))+"?mode=ro")
	if e != nil {
		return e
	}
	defer db.Close()
	var n int
	if e = db.QueryRow(`SELECT COUNT(*) FROM agent_memories WHERE text='TEST_MEMORY'`).Scan(&n); e != nil || n != 1 {
		return errors.New("independent test memory missing")
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 7 || len(failures) > 0 {
		return fmt.Errorf("unexpected calls %d or scope failures %v", calls, failures)
	}
	data, e := os.ReadFile(program)
	if e != nil {
		return e
	}
	hash := sha256.Sum256(data)
	result := map[string]any{"passed": true, "realModelCalls": 0, "realQQMessagesSent": false, "modelCalls": calls, "testControlsLocal": true, "testMemorySeparateDatabase": true, "dailyContextClean": true, "batch2Isolated": true, "credentialCommandsHiddenFromModel": true, "source": "built-binary", "binarySHA256": hex.EncodeToString(hash[:])}
	if e = qa.WriteJSON(filepath.Join(parent, "latest.json"), result); e != nil {
		return e
	}
	if e = qa.WriteJSON(filepath.Join(out, "result.json"), result); e != nil {
		return e
	}
	b, _ = json.MarshalIndent(result, "", "  ")
	fmt.Println(string(b))
	return nil
}
