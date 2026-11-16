package localadmin

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/chat"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func TestPersonaPersistenceScopeConflictAndBackup(t *testing.T) {
	s, c, path, backups := aiFixture(t)
	if err := c.SetExtra(map[string]any{"AgentChar": "global", "AgentSex": "女", "Future": "keep"}); err != nil {
		t.Fatal(err)
	}
	cfg, revision, _ := s.repository.Read()
	if p, custom := cfg.Persona(123, 10); custom || p.Character != "global" {
		t.Fatal(p, custom)
	}
	before, _ := os.ReadDir(backups)
	generation := chat.AgentGeneration()
	p := chat.AgentPersona{Character: "group custom", Sex: "自由文本"}
	next, err := s.repository.UpdatePersona(revision, 123, 10, &p)
	if err != nil {
		t.Fatal(err)
	}
	if chat.AgentGeneration() != generation {
		t.Fatal("persona edit stopped existing tasks")
	}
	after, _ := os.ReadDir(backups)
	if len(after) != len(before)+1 {
		t.Fatal("missing backup")
	}
	if _, err = s.repository.UpdatePersona(revision, 123, -10, &p); !errors.Is(err, ctrl.ErrConfigConflict) {
		t.Fatal("stale revision accepted", err)
	}
	cfg, _, _ = s.repository.Read()
	for _, scope := range [][2]int64{{123, -10}, {124, 10}, {123, 11}} {
		if actual, custom := cfg.Persona(scope[0], scope[1]); custom || actual.Character != "global" {
			t.Fatal("scope leaked", scope, actual)
		}
	}
	if _, err = s.repository.UpdatePersona(next, 123, -10, &chat.AgentPersona{Character: "private"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.repository.UpdatePersona(next, 123, 10, nil); !errors.Is(err, ctrl.ErrConfigConflict) {
		t.Fatal("lost concurrent persona")
	}
	if err = c.Manager.D.Close(); err != nil {
		t.Fatal(err)
	}
	m := ctrl.NewManager[*zero.Ctx](path)
	defer m.D.Close()
	r := chat.NewConfigRepository(m.NewControl("aichatcfg", &ctrl.Options[*zero.Ctx]{Extra: 12345}))
	cfg, revision, err = r.Read()
	if err != nil {
		t.Fatal(err)
	}
	if p, custom := cfg.Persona(123, 10); !custom || p.Character != "group custom" {
		t.Fatal("restart lost override")
	}
	if _, err = r.UpdatePersona(revision, 123, 10, nil); err != nil {
		t.Fatal(err)
	}
	cfg, _, _ = r.Read()
	if p, custom := cfg.Persona(123, 10); custom || p.Character != "global" {
		t.Fatal("inherit failed")
	}
	if p, custom := cfg.Persona(123, -10); !custom || p.Character != "private" {
		t.Fatal("restore affected private")
	}
}

func TestPersonaAPIAuthValidationAndGlobalPrivacy(t *testing.T) {
	s := newTestServer(t)
	store, _, _, _ := aiFixture(t)
	s.cfg.AI = store
	base := "/api/ai-persona?botId=12345678&type=group&id=10"
	expectStatus(t, request(s, "GET", base, nil, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	get := func() map[string]any {
		w := request(s, "GET", base, nil, cookie, "")
		expectStatus(t, w, 200)
		v := map[string]any{}
		json.Unmarshal(w.Body.Bytes(), &v)
		return v
	}
	v := get()
	body := map[string]any{"botId": "12345678", "type": "group", "id": "10", "revision": v["revision"], "mode": "custom", "character": " custom persona ", "sex": "女"}
	expectStatus(t, request(s, "POST", "/api/ai-persona", body, cookie, "bad"), 403)
	expectStatus(t, request(s, "POST", "/api/ai-persona", body, cookie, csrf), 200)
	if actual := get(); actual["mode"] != "custom" || actual["character"] != "custom persona" {
		t.Fatal(actual)
	}
	expectStatus(t, request(s, "POST", "/api/ai-persona", body, cookie, csrf), 409)
	w := request(s, "GET", "/api/ai", nil, cookie, "")
	if strings.Contains(w.Body.String(), "Personas") || strings.Contains(w.Body.String(), "custom persona") {
		t.Fatal("global view exposes other scopes")
	}
	body["revision"] = get()["revision"]
	body["character"] = "  "
	expectStatus(t, request(s, "POST", "/api/ai-persona", body, cookie, csrf), 400)
	body["mode"] = "inherit"
	expectStatus(t, request(s, "POST", "/api/ai-persona", body, cookie, csrf), 200)
	if get()["mode"] != "inherit" {
		t.Fatal("inherit failed")
	}
	for _, path := range []string{"/api/ai-persona?botId=999&type=group&id=10", "/api/ai-persona?botId=12345678&type=private&id=-10"} {
		w := request(s, "GET", path, nil, cookie, "")
		if w.Code != 400 && w.Code != 404 {
			t.Fatal(w.Code)
		}
	}
}

func TestPersonaCardCopiesAcrossGroups(t *testing.T) {
	s := newTestServer(t)
	store, _, _, _ := aiFixture(t)
	s.cfg.AI = store
	cookie, csrf := enroll(t, s)
	get := func(group string) map[string]any {
		t.Helper()
		w := request(s, "GET", "/api/ai-persona?botId=12345678&type=group&id="+group, nil, cookie, "")
		expectStatus(t, w, 200)
		var v map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	post := func(group, mode string, fields map[string]any, status int) {
		t.Helper()
		fields["botId"], fields["type"], fields["id"], fields["mode"] = "12345678", "group", group, mode
		if _, ok := fields["revision"]; !ok {
			fields["revision"] = get(group)["revision"]
		}
		expectStatus(t, request(s, "POST", "/api/ai-persona", fields, cookie, csrf), status)
	}
	post("10", "custom", map[string]any{"character": "鲸鱼人设", "sex": "鱼"}, 200)
	old := get("10")["revision"]
	post("10", "saveCard", map[string]any{"name": "小鲸鱼"}, 200)
	post("10", "saveCard", map[string]any{"name": "小鲸鱼"}, 409)
	post("20", "applyCard", map[string]any{"cardIndex": 0, "revision": old}, 409)
	post("20", "applyCard", map[string]any{"cardIndex": 1}, 400)
	post("20", "applyCard", map[string]any{"cardIndex": 0}, 200)
	if v := get("20"); v["character"] != "鲸鱼人设" || v["sex"] != "鱼" || v["mode"] != "custom" {
		t.Fatal(v)
	}
	post("20", "custom", map[string]any{"character": "本群专用"}, 200)
	if v := get("10"); v["character"] != "鲸鱼人设" {
		t.Fatal(v)
	}
	post("10", "inherit", map[string]any{}, 200)
	post("30", "applyCard", map[string]any{"cardIndex": 0}, 200)
	if get("30")["character"] != "鲸鱼人设" || get("20")["character"] != "本群专用" {
		t.Fatal("card or group changed through a shared reference")
	}
	w := request(s, "GET", "/api/ai", nil, cookie, "")
	if strings.Contains(w.Body.String(), "小鲸鱼") {
		t.Fatal("card leaked into global settings")
	}
}

func TestPersonaCardBotPrivateScopeAndSnapshot(t *testing.T) {
	s := newTestServer(t)
	store, c, _, _ := aiFixture(t)
	s.cfg.AI = store
	s.cfg.Overview = func() Overview { return Overview{BotAccounts: []string{"12345678", "87654321"}} }
	if err := c.SetExtra(map[string]any{"AgentChar": "global snapshot", "AgentSex": "neutral"}); err != nil {
		t.Fatal(err)
	}
	cookie, csrf := enroll(t, s)
	get := func(bot, kind, id string) map[string]any {
		t.Helper()
		w := request(s, "GET", "/api/ai-persona?botId="+bot+"&type="+kind+"&id="+id, nil, cookie, "")
		expectStatus(t, w, 200)
		var v map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	post := func(bot, kind, id, mode string, extra map[string]any, status int) {
		t.Helper()
		body := map[string]any{"botId": bot, "type": kind, "id": id, "mode": mode, "revision": get(bot, kind, id)["revision"]}
		for k, v := range extra {
			body[k] = v
		}
		expectStatus(t, request(s, "POST", "/api/ai-persona", body, cookie, csrf), status)
	}
	generation := chat.AgentGeneration()
	post("12345678", "group", "10", "saveCard", map[string]any{"name": "inherited"}, 200)
	if chat.AgentGeneration() != generation {
		t.Fatal("saving card interrupted active tasks")
	}
	if get("12345678", "group", "10")["mode"] != "inherit" {
		t.Fatal("saving card changed the source override")
	}
	if len(get("87654321", "group", "10")["cards"].([]any)) != 0 {
		t.Fatal("other bot can read card")
	}
	post("87654321", "group", "10", "applyCard", map[string]any{"cardIndex": 0}, 400)
	post("12345678", "group", "20", "applyCard", nil, 400)
	post("12345678", "group", "20", "applyCard", map[string]any{"cardIndex": -1}, 400)
	post("87654321", "group", "10", "saveCard", map[string]any{"name": "inherited"}, 200)
	_, revision, err := store.repository.Read()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.repository.Update(revision, aiFieldsRaw(map[string]any{"AgentChar": "changed global"})); err != nil {
		t.Fatal(err)
	}
	generation = chat.AgentGeneration()
	post("12345678", "private", "20", "applyCard", map[string]any{"cardIndex": 0}, 200)
	if chat.AgentGeneration() != generation {
		t.Fatal("applying card interrupted active tasks")
	}
	if v := get("12345678", "private", "20"); v["character"] != "global snapshot" || v["mode"] != "custom" {
		t.Fatal("card followed the global edit", v)
	}
	for _, scope := range [][3]string{{"12345678", "group", "20"}, {"87654321", "private", "20"}} {
		if v := get(scope[0], scope[1], scope[2]); v["mode"] != "inherit" || v["character"] != "changed global" {
			t.Fatal("copy leaked into another scope", v)
		}
	}
}

func TestPersonaCardBackupFailureAndValidationAreAtomic(t *testing.T) {
	s := newTestServer(t)
	store, c, path, backups := aiFixture(t)
	s.cfg.AI = store
	if err := c.SetExtra(map[string]any{"AgentChar": "saved persona", "Key": "synthetic-card-secret"}); err != nil {
		t.Fatal(err)
	}
	cookie, csrf := enroll(t, s)
	post := func(mode string, fields map[string]any) int {
		t.Helper()
		_, revision, err := store.repository.Read()
		if err != nil {
			t.Fatal(err)
		}
		body := map[string]any{"botId": "12345678", "type": "group", "id": "10", "revision": revision, "mode": mode}
		for k, v := range fields {
			body[k] = v
		}
		w := request(s, "POST", "/api/ai-persona", body, cookie, csrf)
		if strings.Contains(w.Body.String(), "synthetic-card-secret") {
			t.Fatal("secret exposed")
		}
		return w.Code
	}
	_, unchanged, _ := store.repository.Read()
	for _, name := range []string{"", "   ", "bad\nname", strings.Repeat("字", 65), "synthetic-card-secret"} {
		if status := post("saveCard", map[string]any{"name": name}); status != 400 {
			t.Fatal("invalid name accepted", status)
		}
	}
	if _, revision, _ := store.repository.Read(); revision != unchanged {
		t.Fatal("invalid save changed configuration")
	}
	originalHook := c.Manager.BeforeExtra
	defer func() { c.Manager.BeforeExtra = originalHook }()
	c.Manager.BeforeExtra = func() error { return errors.New("synthetic backup failure") }
	if post("saveCard", map[string]any{"name": "saved"}) != 500 {
		t.Fatal("failed backup accepted")
	}
	if cfg, revision, _ := store.repository.Read(); len(cfg.PersonaCards) != 0 || revision != unchanged {
		t.Fatal("failed backup changed card library")
	}
	c.Manager.BeforeExtra = originalHook
	before, _ := os.ReadDir(backups)
	if post("saveCard", map[string]any{"name": "saved", "character": "untrusted request draft"}) != 200 {
		t.Fatal("card save failed")
	}
	after, _ := os.ReadDir(backups)
	if len(after) != len(before)+1 {
		t.Fatal("card save missing backup")
	}
	_, unchanged, _ = store.repository.Read()
	c.Manager.BeforeExtra = func() error { return errors.New("synthetic backup failure") }
	if post("applyCard", map[string]any{"cardIndex": 0}) != 500 {
		t.Fatal("failed apply backup accepted")
	}
	if cfg, revision, _ := store.repository.Read(); len(cfg.Personas) != 0 || revision != unchanged {
		t.Fatal("failed apply changed conversation")
	}
	c.Manager.BeforeExtra = originalHook
	if err := store.Save(AIUpdate{Revision: unchanged, Fields: aiFieldsRaw(map[string]any{"PersonaCards": []any{}})}); !errors.Is(err, errAIValidation) {
		t.Fatal("global API bypassed card protection", err)
	}
	if err := c.Manager.D.Close(); err != nil {
		t.Fatal(err)
	}
	m := ctrl.NewManager[*zero.Ctx](path)
	defer m.D.Close()
	r := chat.NewConfigRepository(m.NewControl("aichatcfg", &ctrl.Options[*zero.Ctx]{Extra: 12345}))
	cfg, _, err := r.Read()
	if err != nil || len(cfg.PersonaCards) != 1 || cfg.PersonaCards[0].Character != "saved persona" {
		t.Fatal("restart lost saved snapshot", err)
	}
}
