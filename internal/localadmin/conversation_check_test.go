package localadmin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/chat"
	"github.com/FloatTech/zbputils/modelconfig"
	zero "github.com/wdvxdr1123/ZeroBot"
)

const checkBot = "9007199254740993"
const checkID = "9007199254740995"

func conversationCheckFixture(t *testing.T) (*Server, *ctrl.Control[*zero.Ctx], string, string) {
	t.Helper()
	store, c, path, backups := aiFixture(t)
	s := newTestServer(t)
	s.cfg.AI = store
	aichat := c.Manager.NewControl("aichat", nil)
	s.cfg.PluginControl = NewPluginStore([]*ctrl.Control[*zero.Ctx]{c, store.groups["llm"], aichat}, backups)
	s.cfg.Overview = func() Overview { return Overview{BotAccounts: []string{checkBot, "12345678"}} }
	if err := c.SetExtra(map[string]any{"API": "https://example.invalid", "Key": "synthetic-private-key", "ModelName": "legacy", "AgentChar": "private-persona-body", "SystemP": "private-system-body"}); err != nil {
		t.Fatal(err)
	}
	return s, c, path, backups
}

func decodeCheck(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	expectStatus(t, w, 200)
	var v map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestConversationCheckScopeReadOnlyPrivacyAndRestart(t *testing.T) {
	s, c, path, backups := conversationCheckFixture(t)
	_, revision, _ := s.cfg.AI.repository.Read()
	const bot, id = int64(9007199254740993), int64(9007199254740995)
	if _, err := s.cfg.AI.repository.UpdatePersona(revision, bot, id, &chat.AgentPersona{Character: "custom-body"}); err != nil {
		t.Fatal(err)
	}
	_, revision, _ = s.cfg.AI.repository.Read()
	if _, err := s.cfg.AI.repository.UpdateWebSearch(revision, chat.WebSearchConfig{Enabled: true, Key: "synthetic-search-key", Scopes: map[string]chat.WebSearchPolicy{chat.WebSearchScope(bot, id): {Enabled: true, Roles: []string{"owner", "admin"}}, chat.WebSearchScope(bot, -id): {Roles: []string{}}}}); err != nil {
		t.Fatal(err)
	}
	if err := c.SetData(id, int64(chat.BitmapNagt)|25); err != nil {
		t.Fatal(err)
	}
	if err := s.cfg.PluginControl.controls["aichat"].Disable(-id); err != nil {
		t.Fatal(err)
	}
	if err := c.Manager.Response(-id); err != nil {
		t.Fatal(err)
	}
	cookie, _ := enroll(t, s)
	base := "/api/agent/conversation-check?botId="
	read := func(account, kind string) map[string]any {
		return decodeCheck(t, request(s, "GET", base+account+"&type="+kind+"&id="+checkID, nil, cookie, ""))
	}
	beforeDB, _ := os.ReadFile(path)
	beforeBackups, _ := os.ReadDir(backups)
	group, private, other := read(checkBot, "group"), read(checkBot, "private"), read("12345678", "group")
	if group["scope"].(map[string]any)["id"] != checkID || group["observedAt"] == "" || group["revision"] == "" {
		t.Fatal("scope or provenance missing", group)
	}
	if group["persona"].(map[string]any)["mode"] != "custom" || private["persona"].(map[string]any)["mode"] != "inherit" || other["persona"].(map[string]any)["mode"] != "inherit" {
		t.Fatal("persona scope crossed")
	}
	if group["response"].(map[string]any)["effectiveEnabled"] != false || private["response"].(map[string]any)["source"] != "conversation" {
		t.Fatal("private response treated as positive group")
	}
	if group["conversation"].([]any)[0].(map[string]any)["rate"] != float64(25) || !reflect.DeepEqual(group["conversation"], other["conversation"]) {
		t.Fatal("shared parameters changed by bot")
	}
	if private["conversation"].([]any)[0].(map[string]any)["rate"] != float64(0) || private["plugins"].([]any)[0].(map[string]any)["state"].(map[string]any)["effectiveEnabled"] != false {
		t.Fatal("private switch or parameters crossed")
	}
	if group["search"].(map[string]any)["policy"].(map[string]any)["enabled"] != true || other["search"].(map[string]any)["policy"].(map[string]any)["enabled"] != false || len(private["search"].(map[string]any)["policy"].(map[string]any)["roles"].([]any)) != 0 {
		t.Fatal("search scope crossed")
	}
	for _, v := range []map[string]any{group, private, other} {
		raw, _ := json.Marshal(v)
		for _, hidden := range []string{"synthetic-private-key", "synthetic-search-key", "private-persona-body", "private-system-body", "custom-body", "example.invalid"} {
			if bytes.Contains(raw, []byte(hidden)) {
				t.Fatal("private content exposed", hidden)
			}
		}
	}
	afterDB, _ := os.ReadFile(path)
	afterBackups, _ := os.ReadDir(backups)
	if !bytes.Equal(beforeDB, afterDB) || len(beforeBackups) != len(afterBackups) {
		t.Fatal("read changed database or created backup")
	}
	if err := c.Manager.D.Close(); err != nil {
		t.Fatal(err)
	}
	m := ctrl.NewManager[*zero.Ctx](path)
	defer m.D.Close()
	controls := []*ctrl.Control[*zero.Ctx]{m.NewControl("aichatcfg", &ctrl.Options[*zero.Ctx]{Extra: 12345}), m.NewControl("llm", nil), m.NewControl("aichat", nil)}
	s.cfg.AI = NewAIStore(controls)
	s.cfg.PluginControl = NewPluginStore(controls, backups)
	restarted := read(checkBot, "group")
	delete(restarted, "observedAt")
	delete(group, "observedAt")
	// Configuration revisions are process-specific; values and sources persist.
	delete(restarted, "revision")
	delete(group, "revision")
	for _, v := range []map[string]any{restarted, group} {
		for _, row := range v["conversation"].([]any) {
			delete(row.(map[string]any), "revision")
		}
	}
	if !reflect.DeepEqual(group, restarted) {
		t.Fatal("restart changed saved summary", group, restarted)
	}
}

func TestConversationCheckResponseCombinationsAndSwitchSources(t *testing.T) {
	for _, global := range []bool{false, true} {
		for _, local := range []bool{false, true} {
			s, c, _, _ := conversationCheckFixture(t)
			if local {
				if err := c.Manager.Response(10); err != nil {
					t.Fatal(err)
				}
			}
			if global {
				if err := c.Manager.Response(0); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.cfg.PluginControl.controls["aichat"].Disable(10); err != nil {
				t.Fatal(err)
			}
			cookie, _ := enroll(t, s)
			v := decodeCheck(t, request(s, "GET", "/api/agent/conversation-check?botId="+checkBot+"&type=group&id=10", nil, cookie, ""))
			r := v["response"].(map[string]any)
			if r["globalEnabled"] != global || r["conversationEnabled"] != local || r["effectiveEnabled"] != (global || local) {
				t.Fatal(r)
			}
			plugins := v["plugins"].([]any)
			if plugins[0].(map[string]any)["state"].(map[string]any)["source"] != "group" || plugins[1].(map[string]any)["state"].(map[string]any)["source"] != "default" {
				t.Fatal(plugins)
			}
		}
	}
}

func TestConversationCheckModelSelectionAndInvalidReference(t *testing.T) {
	max := uint(2000)
	top := float32(.5)
	reason := ""
	c := chat.AgentConfig{MaxN: 4096, TopP: .9, ModelName: "must-not-fallback", ModelRoutes: &chat.ModelRoutes{Chat: "chat", Summary: "summary", Agent: "agent"}, ModelProfiles: []chat.ModelProfile{
		{ID: "chat", Name: "chat", API: "https://example.invalid", Model: "chat", Key: "chat-key", Parameters: &modelconfig.Parameters{MaxN: &max}},
		{ID: "summary", Name: "summary", API: "https://example.invalid", Model: "summary", Key: "summary-key", Parameters: &modelconfig.Parameters{TopP: &top, ReasoningEffort: &reason}},
		{ID: "agent", Name: "agent", API: "https://example.invalid", Model: "agent", Key: "agent-key"},
	}}
	v := conversationModels(c, true)
	rows := v["entries"].([]conversationModelCheck)
	if v["error"] != "" || len(rows) != 5 || rows[1].Model != "chat" || !rows[1].Inherited || rows[1].Parameters.MaxN != max || rows[1].ParameterSources["maxN"] != "model" || rows[2].Applicable || rows[2].Model != "summary" || rows[2].Parameters.TopP != top || rows[2].ParameterSources["reasoningEffort"] != "model" || rows[4].Enabled {
		t.Fatal(v)
	}
	c.ModelRoutes.GPT = "missing"
	v = conversationModels(c, false)
	if v["error"] == "" || len(v["entries"].([]conversationModelCheck)) != 0 {
		t.Fatal("invalid reference fell back", v)
	}
}

func TestConversationCheckAuthErrorsAndUnregisteredPlugin(t *testing.T) {
	s, c, _, _ := conversationCheckFixture(t)
	base := "/api/agent/conversation-check?botId=" + checkBot + "&type=group&id=10"
	expectStatus(t, request(s, "GET", base, nil, nil, ""), 401)
	cookie, _ := enroll(t, s)
	for _, tail := range []string{"&id=0", "&id=01", "&id=9223372036854775808", "&type=all", "&botId=0"} {
		// Replace, rather than append duplicate query fields.
		parts := strings.Split(strings.TrimPrefix(tail, "&"), "=")
		r := httptest.NewRequest("GET", "http://"+s.cfg.Addr+base, nil)
		q := r.URL.Query()
		q.Set(parts[0], parts[1])
		expectStatus(t, request(s, "GET", r.URL.Path+"?"+q.Encode(), nil, cookie, ""), 400)
	}
	expectStatus(t, request(s, "GET", strings.Replace(base, checkBot, "42", 1), nil, cookie, ""), 404)
	r := httptest.NewRequest("GET", "http://"+s.cfg.Addr+base, nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://other.invalid")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	expectStatus(t, w, http.StatusForbidden)
	delete(s.cfg.PluginControl.controls, "llm")
	v := decodeCheck(t, request(s, "GET", base, nil, cookie, ""))
	if v["plugins"].([]any)[1].(map[string]any)["registered"] != false {
		t.Fatal("unregistered shown as disabled")
	}
	s.cfg.AI.unified = true
	v = decodeCheck(t, request(s, "GET", base, nil, cookie, ""))
	entries := v["plugins"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["service"] != "aichat" {
		t.Fatal("unified AI diagnostics exposed a retired plugin switch", entries)
	}
	if len(v["conversation"].([]any)) != 2 {
		t.Fatal("unified diagnostics lost legacy conversation parameters")
	}
	if err := c.SetExtra(map[string]any{"ModelRoutes": map[string]string{"chat": "missing"}}); err != nil {
		t.Fatal(err)
	}
	v = decodeCheck(t, request(s, "GET", base, nil, cookie, ""))
	if v["models"].(map[string]any)["error"] == "" || v["persona"] == nil {
		t.Fatal("bad model hid other settings", v)
	}
	c.Manager.Lock()
	_, err := c.Manager.D.Exec("DROP TABLE __resp")
	c.Manager.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, request(s, "GET", base, nil, cookie, ""), 500)
}
