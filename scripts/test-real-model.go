//go:build ignore

// Opt-in real-provider checks against the deployed binary and synthetic OneBot.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
	"github.com/FloatTech/zbputils/modelconfig"
	_ "modernc.org/sqlite"
)

var execute = flag.Bool("run", false, "opt in to billable real-provider requests using locally configured credentials")
var binaryName = flag.String("binary-name", "zerobot.exe", "executable under .local/bin")
var caseNames = flag.String("cases", "gpt,chat,agent,memory-save,history,memory,vision", "comma-separated cases")

type provider struct {
	API, ModelName, Key string
	Type                int
}

func (p provider) ready() bool {
	return p.Type >= 0 && p.Type <= 2 && p.API != "" && p.ModelName != "" && (p.Key != "" || p.Type == 1)
}

type providerSnapshot struct {
	Mode      string
	Providers map[string]provider
}
type caseResult struct {
	Name    string         `json:"name"`
	Passed  bool           `json:"passed"`
	Seconds float64        `json:"seconds"`
	Failure string         `json:"failure,omitempty"`
	Task    map[string]any `json:"task,omitempty"`
}

func readProviders(root string) (providerSnapshot, error) {
	name := filepath.ToSlash(filepath.Join(root, "data/control/plugins.db"))
	db, err := sql.Open("sqlite3", "file:/"+name+"?mode=ro")
	if err != nil {
		return providerSnapshot{}, err
	}
	defer db.Close()
	rows, err := db.Query("SELECT ext FROM __resp WHERE ext IS NOT NULL AND ext NOT IN ('','-')")
	if err != nil {
		return providerSnapshot{}, err
	}
	defer rows.Close()
	var found json.RawMessage
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return providerSnapshot{}, err
		}
		var cfg map[string]json.RawMessage
		if json.Unmarshal([]byte(raw), &cfg) != nil || (cfg["ModelName"] == nil && cfg["ModelRoutes"] == nil && cfg["ModelProfiles"] == nil) {
			continue
		}
		if found != nil {
			return providerSnapshot{}, errors.New("ambiguous AI configuration")
		}
		found = json.RawMessage(raw)
	}
	if err = rows.Err(); err != nil {
		return providerSnapshot{}, err
	}
	if found == nil {
		return providerSnapshot{}, errors.New("AI configuration missing")
	}
	var cfg struct {
		ModelRoutes   *modelconfig.Routes
		ModelProfiles []modelconfig.Profile
	}
	if json.Unmarshal(found, &cfg) != nil {
		return providerSnapshot{}, errors.New("invalid provider configuration")
	}
	mode := "legacy"
	if cfg.ModelRoutes != nil {
		mode = "named"
	}
	result := map[string]provider{}
	if cfg.ModelRoutes != nil {
		selected, err := modelconfig.Select(cfg.ModelProfiles, *cfg.ModelRoutes)
		if err != nil {
			return providerSnapshot{}, errors.New("invalid model routes; check saved AI settings")
		}
		for role, p := range selected {
			result[role] = provider{API: p.API, ModelName: p.Model, Key: p.Key, Type: p.Type}
		}
	} else {
		if cfg.ModelProfiles != nil {
			return providerSnapshot{}, errors.New("named models have no routes")
		}
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(found, &fields)
		for _, role := range []string{"", "Agent", "Image"} {
			var p provider
			for field, target := range map[string]any{"API": &p.API, "ModelName": &p.ModelName, "Key": &p.Key, "Type": &p.Type} {
				if raw := fields[role+field]; raw != nil && json.Unmarshal(raw, target) != nil {
					return providerSnapshot{}, errors.New("invalid legacy provider configuration")
				}
			}
			result[role] = p
		}
	}
	return providerSnapshot{Mode: mode, Providers: result}, nil
}

// Validate the entire selection before starting an instance or admitting HTTP.
func selectedCases(text string, providers map[string]provider) ([]string, error) {
	names := []string{}
	seen := map[string]bool{}
	for _, name := range strings.Split(text, ",") {
		name = strings.TrimSpace(name)
		var required []string
		switch name {
		case "gpt", "chat":
			required = []string{""}
		case "agent", "memory-save", "history", "memory":
			required = []string{"Agent"}
		case "vision":
			required = []string{"Agent", "Image"}
		default:
			return nil, errors.New("unknown or empty case name")
		}
		if seen[name] {
			return nil, errors.New("duplicate case name")
		}
		seen[name] = true
		for _, role := range required {
			if !providers[role].ready() {
				return nil, fmt.Errorf("case %s requires a configured %s model", name, map[string]string{"": "chat", "Agent": "Agent", "Image": "vision"}[role])
			}
		}
		names = append(names, name)
	}
	return names, nil
}

func main() {
	flag.Parse()
	if filepath.Base(*binaryName) != *binaryName || filepath.Ext(*binaryName) != ".exe" {
		fmt.Fprintln(os.Stderr, "invalid binary filename")
		os.Exit(2)
	}
	root, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	snapshot, err := readProviders(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cannot read AI provider configuration.")
		os.Exit(1)
	}
	providers := snapshot.Providers
	summary := map[string]any{}
	for role, p := range providers {
		if role == "" {
			role = "Chat"
		}
		summary[role] = map[string]any{"model": p.ModelName, "type": p.Type, "enabled": p.API != "" || p.ModelName != "", "ready": p.ready()}
	}
	names, selectionErr := selectedCases(*caseNames, providers)
	if !*execute {
		view := map[string]any{"mode": snapshot.Mode, "preflight": summary, "casesReady": selectionErr == nil, "realModelCalls": 0, "note": "Add -run to make real billable requests; only synthetic data will be sent."}
		if selectionErr != nil {
			view["selectionError"] = selectionErr.Error()
		}
		b, _ := json.MarshalIndent(view, "", "  ")
		fmt.Println(string(b))
		return
	}
	if selectionErr != nil {
		fmt.Fprintln(os.Stderr, selectionErr)
		os.Exit(1)
	}
	parent := filepath.Join(root, ".local/real-model")
	if err = os.MkdirAll(parent, 0700); err != nil {
		panic(err)
	}
	out, err := os.MkdirTemp(parent, "run-")
	if err != nil {
		panic(err)
	}
	result := map[string]any{"passed": false, "startedAt": time.Now().Format(time.RFC3339), "realProvider": true, "syntheticOneBot": true, "realQQMessagesSent": false, "realChatDataRead": false, "dailyConfigurationWrites": 0, "providers": summary}
	result["configurationMode"] = snapshot.Mode
	result["configurationSnapshot"] = "resolved once before starting the isolated instance; projected into its legacy fields; named-route persistence is covered by N2 integration"
	fmt.Println("ARTIFACTS=" + out)
	err = run(root, out, providers, names, result)
	result["completedAt"] = time.Now().Format(time.RFC3339)
	result["passed"] = err == nil
	if err != nil {
		result["failure"] = err.Error()
	}
	if saveErr := qa.WriteJSON(filepath.Join(out, "result.json"), result); saveErr != nil {
		fmt.Fprintln(os.Stderr, "Cannot save real-model result.")
		os.Exit(1)
	}
	b, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(b))
	if err != nil {
		os.Exit(1)
	}
}

func run(root, out string, providers map[string]provider, names []string, result map[string]any) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	binary := filepath.Join(root, ".local/bin", *binaryName)
	program, err := os.ReadFile(binary)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(program)
	result["binarySHA256"] = hex.EncodeToString(hash[:])
	program = nil
	h, err := qa.New(root, binary, out)
	if err != nil {
		return err
	}
	defer h.Close()
	// These private action cases exercise the existing bot-owner permission set.
	configBytes, err := os.ReadFile(h.ConfigPath)
	if err != nil {
		return err
	}
	var config map[string]any
	if err = json.Unmarshal(configBytes, &config); err != nil {
		return err
	}
	owners := []int64{qa.Owner}
	for index := range names {
		owners = append(owners, int64(23456000+index))
	}
	config["zero"].(map[string]any)["super_users"] = owners
	if err = qa.WriteJSON(h.ConfigPath, config); err != nil {
		return err
	}
	result["agentRole"] = "synthetic bot owner in private conversation"
	bot, err := h.Start(ctx, "real-model")
	if err != nil {
		return err
	}
	defer func() { bot.Stop(); result["testProcessStopped"] = true }()
	c := qa.NewClient(h.Address)
	if _, err = c.Auth(h.AdminDir); err != nil {
		return err
	}
	fields := map[string]any{"SystemP": "你是软件验收助手。严格完成当前合成测试，不编造工具结果。", "AgentChar": "你是软件验收助手。优先按用户要求使用本地工具，工具成功后简短回复；不做额外动作。", "AgentSex": "unknown", "MaxN": 4096, "TopP": 0.9, "NoSystemP": false}
	keys := map[string]any{}
	for p, v := range providers {
		fields[p+"API"], fields[p+"ModelName"], fields[p+"Type"] = v.API, v.ModelName, v.Type
		if v.Key != "" {
			keys[p+"Key"] = map[string]any{"action": "replace", "value": v.Key}
		}
	}
	value, err := c.Checked("GET", "/api/ai", nil)
	if err != nil {
		return err
	}
	if _, err = c.Checked("POST", "/api/ai", map[string]any{"revision": value["global"].(map[string]any)["revision"], "fields": fields, "keys": keys}); err != nil {
		return err
	}
	defer func() {
		v, e := c.Checked("GET", "/api/overview", nil)
		if e != nil {
			if err == nil {
				err = e
			}
			return
		}
		result["modelRequests"] = v["modelRequests"]
		result["replyTasks"] = v["replyTasks"]
		stats := v["modelRequests"].(map[string]any)
		complete := stats["started"] == stats["completed"] && stats["active"] == float64(0) && stats["incomplete"] == float64(0) && stats["failed"] == float64(0)
		result["httpResponsesCompleted"] = complete
		if err == nil && !complete {
			err = errors.New("functional replies passed but model HTTP responses did not all complete")
		}
	}()
	plugins, err := c.Checked("GET", "/api/plugins", nil)
	if err != nil {
		return err
	}
	for _, raw := range plugins["plugins"].([]any) {
		p := raw.(map[string]any)
		if p["name"] == "aichat" {
			_, err = c.Checked("POST", "/api/plugin-switch", map[string]any{"name": "aichat", "groupId": "0", "mode": "enabled", "revision": p["state"].(map[string]any)["revision"]})
			if err != nil {
				return err
			}
			break
		}
	}
	random := make([]byte, 5)
	if _, err = rand.Read(random); err != nil {
		return err
	}
	nonce := hex.EncodeToString(random)
	results := []caseResult{}
	for index, name := range names {
		fmt.Println("RUN " + name)
		start := time.Now()
		item := caseResult{Name: name}
		uid := int64(23456000 + index)
		agent := name != "chat" && name != "gpt"
		if err = configure(c, uid, agent); err == nil {
			item.Task, err = checkCase(ctx, c, h, out, uid, name, nonce)
		}
		item.Passed = err == nil
		item.Seconds = time.Since(start).Seconds()
		if err != nil {
			item.Failure = err.Error()
		}
		results = append(results, item)
		result["cases"] = results
		if e := qa.WriteJSON(filepath.Join(out, "progress.json"), map[string]any{"cases": results, "updatedAt": time.Now().Format(time.RFC3339)}); e != nil {
			return e
		}
		fmt.Printf("CASE %s passed=%t seconds=%.2f\n", name, item.Passed, item.Seconds)
	}
	for _, item := range results {
		if !item.Passed {
			return errors.New("one or more real-model cases failed; see per-case results")
		}
	}
	if len(results) == 0 {
		return errors.New("no cases selected")
	}
	return nil
}

func configure(c *qa.Client, uid int64, agent bool) error {
	v, err := c.Checked("GET", fmt.Sprintf("/api/ai?private=%d", uid), nil)
	if err != nil {
		return err
	}
	for _, raw := range v["groups"].([]any) {
		g := raw.(map[string]any)
		if g["service"] != "aichatcfg" {
			continue
		}
		_, err = c.Checked("POST", "/api/ai-group", map[string]any{"service": "aichatcfg", "privateId": fmt.Sprint(uid), "revision": g["revision"], "temperature": 20, "rate": 100, "useAgent": agent, "respondAt": true, "voiceOutput": false})
		return err
	}
	return errors.New("AI conversation configuration missing")
}

func scope(uid int64) map[string]any {
	return map[string]any{"botId": "87654321", "type": "private", "id": fmt.Sprint(uid)}
}
func scopeQuery(uid int64) string { return fmt.Sprintf("botId=87654321&type=private&id=%d", uid) }

func task(c *qa.Client, uid int64) (map[string]any, error) {
	for n := 0; n < 100; n++ {
		v, e := c.Checked("GET", "/api/agent/tasks?"+scopeQuery(uid), nil)
		if e != nil {
			return nil, e
		}
		if items, ok := v["items"].([]any); ok && len(items) > 0 {
			t := items[0].(map[string]any)
			state, _ := t["status"].(string)
			if state != "running" && state != "cancelling" && state != "starting" {
				return t, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, errors.New("Agent task did not reach a terminal state")
}

func checkCase(ctx context.Context, c *qa.Client, h *qa.Harness, out string, uid int64, name, nonce string) (map[string]any, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 100*time.Second)
	defer cancel()
	expected := "REAL_" + strings.ReplaceAll(name, "-", "_") + "_" + nonce
	prompt := "请只输出以下字符串，不要增加标点或解释：" + expected
	var segments []map[string]any
	if name != "gpt" && name != "chat" {
		// Initialize this synthetic account's store even when running history alone.
		if _, e := c.Checked("GET", "/api/agent/memories?"+scopeQuery(uid), nil); e != nil {
			return nil, e
		}
	}
	switch name {
	case "gpt":
		prompt = "/gpt " + prompt
	case "chat":
	case "agent":
		prompt = "请通过 send_private_msg 向当前发言者回复这条合成验收字符串：" + expected + "。只输出协议要求的动作 JSON，不要直接输出自然语言；回复后 end_action。"
	case "memory-save":
		prompt = "请先执行 save_memory，把这条合成验收记忆保存到本会话：验收星球代号=" + expected + "。保存成功后通过 send_private_msg 回复该代号，最后 end_action。所有步骤只输出动作 JSON。"
	case "history":
		db, e := sql.Open("sqlite3", filepath.Join(out, "data/aichat/history/87654321.sqlite"))
		if e != nil {
			return nil, e
		}
		defer db.Close()
		payload, _ := json.Marshal(map[string]any{"post_type": "message", "message_type": "private", "user_id": uid, "message": "合成历史索引 HISTORY_" + nonce + " 的秘密代号是 " + expected, "sender": map[string]any{"user_id": uid}})
		if _, e = db.Exec("INSERT INTO history(conversation,kind,payload,created_at) VALUES(?,?,?,?)", -uid, "event", string(payload), time.Now().Add(-time.Hour).Format(time.RFC3339Nano)); e != nil {
			return nil, e
		}
		prompt = "请先调用 search_history 查询关键词 HISTORY_" + nonce + "，然后用返回的来源序号调用 read_history，最后通过 send_private_msg 回复查到的秘密代号并 end_action。所有步骤只输出动作 JSON，不能省略工具步骤。"
	case "memory":
		body := scope(uid)
		body["text"] = "MEMORY_" + nonce + " 合成验收资料：" + strings.Repeat("这是一段用于检查精确读取的合成说明。", 40) + "最终秘密代号=" + expected
		if _, e := c.Checked("POST", "/api/agent/memories/create", body); e != nil {
			return nil, e
		}
		// More than 100 newer items keep the target out of direct memory loading.
		for i := 0; i < 101; i++ {
			body["text"] = fmt.Sprintf("无关合成记忆 %d", i)
			if _, e := c.Checked("POST", "/api/agent/memories/create", body); e != nil {
				return nil, e
			}
		}
		prompt = "请先调用 search_memory 搜索关键词 MEMORY_" + nonce + "，然后使用返回的准确编号和修订号调用 read_memory 读取完整正文，最后通过 send_private_msg 回复正文末尾的最终秘密代号并 end_action。所有步骤只输出动作 JSON，不能省略工具步骤。"
	case "vision":
		img := image.NewRGBA(image.Rect(0, 0, 48, 48))
		for y := 0; y < 48; y++ {
			for x := 0; x < 48; x++ {
				img.Set(x, y, color.RGBA{R: 255, A: 255})
			}
		}
		var data bytes.Buffer
		if e := png.Encode(&data, img); e != nil {
			return nil, e
		}
		prompt = "请根据图片描述，通过 send_private_msg 回复图片的主要颜色并 end_action，不要编造未看到的内容。所有步骤只输出动作 JSON。"
		segments = []map[string]any{{"type": "text", "data": map[string]string{"text": prompt}}, {"type": "image", "data": map[string]string{"file": "base64://" + base64.StdEncoding.EncodeToString(data.Bytes())}}}
	default:
		return nil, errors.New("unknown case name")
	}
	reply, _, e := h.SendWithContext(checkCtx, uid, prompt, segments...)
	if e != nil {
		return nil, errors.New("model reply deadline exceeded or synthetic connection failed")
	}
	if e = qa.WriteJSON(filepath.Join(out, name+"-reply.json"), map[string]any{"reply": reply}); e != nil {
		return nil, e
	}
	if name == "chat" || name == "gpt" {
		if !strings.Contains(reply, expected) {
			return nil, errors.New("real chat reply did not contain the expected nonce")
		}
		return nil, nil
	}
	t, e := task(c, uid)
	if e != nil {
		return nil, e
	}
	if t["status"] != "completed" {
		return t, fmt.Errorf("Agent task ended with status %v and reason %v", t["status"], t["error"])
	}
	if activity, ok := t["activity"].(map[string]any); !ok || activity["fallbackPreparations"] != float64(0) {
		return t, errors.New("ordinary-chat fallback cannot count as an Agent action success")
	}
	if name == "vision" {
		if !strings.Contains(reply, "红") && !strings.Contains(strings.ToLower(reply), "red") {
			return t, errors.New("vision did not identify the synthetic red image")
		}
		return t, nil
	}
	if !strings.Contains(reply, expected) {
		return t, errors.New("real Agent reply did not contain the expected nonce")
	}
	if name == "history" || name == "memory" {
		q, _ := t["queries"].(map[string]any)
		a, b := "searchHistory", "readHistory"
		if name == "memory" {
			a, b = "searchMemory", "readMemory"
		}
		if q[a] == nil || q[b] == nil || q[a].(float64) < 1 || q[b].(float64) < 1 {
			return t, errors.New("required retrieval and exact-read tools were not both executed")
		}
	}
	if name == "memory-save" {
		v, e := c.Checked("GET", "/api/agent/memories?"+scopeQuery(uid), nil)
		if e != nil {
			return t, e
		}
		saved := false
		for _, raw := range v["items"].([]any) {
			m := raw.(map[string]any)
			text, _ := m["text"].(string)
			if strings.Contains(text, expected) {
				saved = true
			}
		}
		if !saved {
			return t, errors.New("Agent claimed a memory save without a stored matching record")
		}
	}
	return t, nil
}
