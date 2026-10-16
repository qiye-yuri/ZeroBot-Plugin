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
