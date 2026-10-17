package localadmin

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/chat"
	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func TestWebSearchSettingsAPIAndPrivacy(t *testing.T) {
	s := newTestServer(t)
	store, c, _, backups := aiFixture(t)
	s.cfg.AI = store
	base := "/api/ai-search?botId=12345678&type=group&id=10"
	expectStatus(t, request(s, "GET", base, nil, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	get := func(path string) map[string]any {
		t.Helper()
		w := request(s, "GET", path, nil, cookie, "")
		expectStatus(t, w, 200)
		v := map[string]any{}
		json.Unmarshal(w.Body.Bytes(), &v)
		return v
	}
	initial := get(base)
	if initial["enabled"] != false || initial["keyConfigured"] != false {
		t.Fatal(initial)
	}
	body := map[string]any{"botId": "12345678", "type": "group", "id": "10", "revision": initial["revision"], "enabled": true, "key": map[string]any{"action": "replace", "value": "synthetic-search-api-key"}, "policy": chat.WebSearchPolicy{Enabled: true, Roles: []string{"owner"}}}
	expectStatus(t, request(s, "POST", "/api/ai-search", body, cookie, "bad"), 403)
	generation := chat.AgentGeneration()
	before, _ := os.ReadDir(backups)
	w := request(s, "POST", "/api/ai-search", body, cookie, csrf)
	expectStatus(t, w, 200)
	if strings.Contains(w.Body.String(), "synthetic-search-api-key") || chat.AgentGeneration() == generation {
		t.Fatal("secret exposed or no reload")
	}
	after, _ := os.ReadDir(backups)
	if len(after) != len(before)+1 {
		t.Fatal("search save not backed up")
	}
	expectStatus(t, request(s, "POST", "/api/ai-search", body, cookie, csrf), 409)
	other := get("/api/ai-search?botId=12345678&type=private&id=10")
	if other["policy"].(map[string]any)["enabled"] != false {
		t.Fatal("group policy leaked to private")
	}
	cfg, _, _ := store.repository.Read()
	if !cfg.SearchAllowed(12345678, 10, goba.PermRoleOwner) || cfg.SearchAllowed(12345678, -10, goba.PermRoleOwner) || cfg.SearchAllowed(87654321, 10, goba.PermRoleOwner) {
		t.Fatal("scope binding changed")
	}
	for _, path := range []string{"/api/ai", "/api/ai-models"} {
		response := request(s, "GET", path, nil, cookie, "")
		expectStatus(t, response, 200)
		if strings.Contains(response.Body.String(), "synthetic-search-api-key") || strings.Contains(response.Body.String(), "WebSearch") {
			t.Fatal("global/model API leaked search configuration")
		}
	}
	if err := store.Save(AIUpdate{Revision: get(base)["revision"].(string), Fields: aiFieldsRaw(map[string]any{"WebSearch": map[string]any{"enabled": false}})}); err == nil {
		t.Fatal("generic save bypassed search scope")
	}
	body["revision"] = get(base)["revision"]
	body["key"] = map[string]any{"action": "clear"}
	expectStatus(t, request(s, "POST", "/api/ai-search", body, cookie, csrf), 400)
	body["key"] = map[string]any{"action": "clear", "confirm": true}
	expectStatus(t, request(s, "POST", "/api/ai-search", body, cookie, csrf), 400)
	body["enabled"] = false
	body["policy"] = chat.WebSearchPolicy{}
	expectStatus(t, request(s, "POST", "/api/ai-search", body, cookie, csrf), 200)
	if v := get(base); v["keyConfigured"] != false || v["enabled"] != false {
		t.Fatal("clear failed")
	}
	if roles, ok := get(base)["policy"].(map[string]any)["roles"].([]any); !ok || len(roles) != 0 {
		t.Fatal("disabled empty roles must render as an empty list")
	}
	for _, path := range []string{"/api/ai-search?botId=999&type=group&id=10", "/api/ai-search?botId=12345678&type=private&id=-10", "/api/ai-search?botId=12345678&type=other&id=10"} {
		r := request(s, "GET", path, nil, cookie, "")
		if r.Code != 400 && r.Code != 404 {
			t.Fatal(path, r.Code)
		}
	}
	// Failed backup must preserve the configuration and revision.
	body["revision"] = get(base)["revision"]
	body["key"] = map[string]any{"action": "replace", "value": "synthetic-search-api-key-2"}
	c.Manager.BeforeExtra = func() error { return errors.New("backup unavailable") }
	expectStatus(t, request(s, "POST", "/api/ai-search", body, cookie, csrf), 500)
	if got := get(base); got["revision"] != body["revision"] || got["keyConfigured"] != false {
		t.Fatal("failed backup mutated state")
	}
}

func TestWebSearchSettingsRestartPreservesOtherConfiguration(t *testing.T) {
	s, c, path, _ := aiFixture(t)
	if err := c.SetExtra(map[string]any{"ModelName": "existing-model", "Key": "synthetic-model-key", "Future": true}); err != nil {
		t.Fatal(err)
	}
	cfg, revision, _ := s.repository.Read()
	if cfg.WebSearch != nil {
		t.Fatal("read migrated old config")
	}
	_, err := s.repository.UpdateWebSearch(revision, chat.WebSearchConfig{Enabled: true, Key: "synthetic-search-key", Scopes: map[string]chat.WebSearchPolicy{"123:10": {Enabled: true, Roles: []string{"owner"}}, "123:-10": {Enabled: true, Roles: []string{"user"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.repository.UpdateWebSearch(revision, chat.WebSearchConfig{}); !errors.Is(err, ctrl.ErrConfigConflict) {
		t.Fatal("stale write accepted", err)
	}
	c.Manager.D.Close()
	m := ctrl.NewManager[*zero.Ctx](path)
	defer m.D.Close()
	c2 := m.NewControl("aichatcfg", &ctrl.Options[*zero.Ctx]{Extra: 12345})
	r := chat.NewConfigRepository(c2)
	cfg, revision, err = r.Read()
	if err != nil || revision == "" || !cfg.SearchAllowed(123, -10, goba.PermRoleUser) || !cfg.SearchAllowed(123, 10, goba.PermRoleOwner) || cfg.Key != "synthetic-model-key" || cfg.ModelName != "existing-model" {
		t.Fatal("restart lost settings", err)
	}
	raw, _, _ := c2.ExtraJSON()
	if !strings.Contains(string(raw), "Future") {
		t.Fatal("unknown field lost")
	}
}
