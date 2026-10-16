package localadmin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/chat"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func TestNamedModelsFailureAndMigrationBackupRestore(t *testing.T) {
	s, c, _, backups := aiFixture(t)
	if err := c.SetExtra(map[string]any{"Type": 1, "ModelName": "legacy-local", "API": "http://localhost:11434"}); err != nil {
		t.Fatal(err)
	}
	plugins := NewPluginStore([]*ctrl.Control[*zero.Ctx]{c}, backups)
	backup := NewBackupStore(backups, nil, plugins, NewLogStore())
	id, err := backup.Create("control-db")
	if err != nil {
		t.Fatal(err)
	}
	cfg, revision, _ := s.repository.Read()
	profiles, routes := cfg.ModelCatalog()
	original := c.Manager.BeforeExtra
	c.Manager.BeforeExtra = func() error { return errors.New("synthetic backup failure") }
	if _, err = s.repository.UpdateModels(revision, profiles, routes); err == nil {
		t.Fatal("backup failure ignored")
	}
	cfg, after, _ := s.repository.Read()
	if after != revision || cfg.ModelRoutes != nil {
		t.Fatal("failed migration partially wrote")
	}
	c.Manager.BeforeExtra = original
	if _, err = s.repository.UpdateModels(revision, profiles, routes); err != nil {
		t.Fatal(err)
	}
	restore := previewRestore(t, backup, id)
	if _, err = backup.Restore(context.Background(), restore, "fixture-session"); err != nil {
		t.Fatal(err)
	}
	cfg, _, err = s.repository.Read()
	if err != nil || cfg.ModelRoutes != nil || cfg.ModelName != "legacy-local" {
		t.Fatal("migration rollback lost original", err)
	}
}

func TestNamedModelsMigrationAtomicSelectionSecretsAndRestart(t *testing.T) {
	s, c, path, backups := aiFixture(t)
	if err := c.SetExtra(map[string]any{"ModelName": "same-name", "API": "https://chat.invalid/v1", "Key": "CHAT_PRIVATE_KEY", "AgentModelName": "same-name", "AgentAPI": "https://agent.invalid/v1", "AgentKey": "AGENT_PRIVATE_KEY", "ImageModelName": "image", "ImageAPI": "https://image.invalid/v1", "ImageKey": "IMAGE_PRIVATE_KEY", "Future": "preserved"}); err != nil {
		t.Fatal(err)
	}
	before, revision, _ := c.ExtraJSON()
	cfg, _, _ := s.repository.Read()
	profiles, routes := cfg.ModelCatalog()
	after, afterRevision, _ := c.ExtraJSON()
	if string(before) != string(after) || revision != afterRevision || len(profiles) != 3 {
		t.Fatal("preview wrote or merged distinct providers")
	}
	changes := make([]modelProfileUpdate, 0, len(profiles))
	for _, p := range profiles {
		changes = append(changes, modelProfileUpdate{ID: p.ID, Name: p.Name, Type: p.Type, Model: p.Model, API: p.API, Key: AIKeyChange{Action: "keep"}})
	}
	count, _ := os.ReadDir(backups)
	if err := s.SaveModels(modelsUpdate{Revision: revision, Profiles: changes, Routes: routes}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(backups)
	if len(entries) != len(count)+1 {
		t.Fatal("migration missing atomic backup")
	}
	cfg, rev, _ := s.repository.Read()
	effective, err := cfg.ResolveModels()
	if err != nil || effective.Key != "CHAT_PRIVATE_KEY" || effective.AgentKey != "AGENT_PRIVATE_KEY" || effective.ImageKey != "IMAGE_PRIVATE_KEY" {
		t.Fatal("entry keys mixed", err)
	}
	view := aiView(t, s, 0)
	raw, _ := json.Marshal(view)
	for _, secret := range []string{"CHAT_PRIVATE_KEY", "AGENT_PRIVATE_KEY", "IMAGE_PRIVATE_KEY", "ModelProfiles"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("global view exposes model secrets")
		}
	}
	if err := s.Save(AIUpdate{Revision: rev, Fields: aiFieldsRaw(map[string]any{"ModelName": "ignored"})}); !errors.Is(err, chat.ErrNamedModelFields) {
		t.Fatal("legacy update silently ignored", err)
	}
	changes[0].Name = "改名仍保持引用"
	changes[0].Key = AIKeyChange{Action: "replace", Value: "NEW_PRIVATE_KEY"}
	if err := s.SaveModels(modelsUpdate{Revision: rev, Profiles: changes, Routes: routes}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveModels(modelsUpdate{Revision: rev, Profiles: changes, Routes: routes}); !errors.Is(err, ctrl.ErrConfigConflict) {
		t.Fatal("stale save accepted", err)
	}
	if err := c.Manager.D.Close(); err != nil {
		t.Fatal(err)
	}
	m := ctrl.NewManager[*zero.Ctx](path)
	defer m.D.Close()
	r := chat.NewConfigRepository(m.NewControl("aichatcfg", &ctrl.Options[*zero.Ctx]{Extra: 12345}))
	cfg, _, err = r.Read()
	if err != nil {
		t.Fatal(err)
	}
	effective, err = cfg.ResolveModels()
	if err != nil || effective.Key != "NEW_PRIVATE_KEY" || cfg.ModelProfiles[0].Name != "改名仍保持引用" {
		t.Fatal("restart lost model edit", err)
	}
	raw, _, _ = r.Control.ExtraJSON()
	if !strings.Contains(string(raw), "preserved") {
		t.Fatal("migration lost unknown settings")
	}
}

func TestNamedModelsAPIValidationDeletionAndKeyClear(t *testing.T) {
	s := newTestServer(t)
	store, c, _, _ := aiFixture(t)
	s.cfg.AI = store
	if err := c.SetExtra(map[string]any{"ModelName": "one", "API": "https://one.invalid", "Key": "KEEP_PRIVATE_KEY"}); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, request(s, "GET", "/api/ai-models", nil, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	w := request(s, "GET", "/api/ai-models", nil, cookie, "")
	expectStatus(t, w, 200)
	if strings.Contains(w.Body.String(), "KEEP_PRIVATE_KEY") {
		t.Fatal("secret exposed")
	}
	v := map[string]any{}
	json.Unmarshal(w.Body.Bytes(), &v)
	body := modelsUpdate{Revision: v["revision"].(string), Profiles: []modelProfileUpdate{{ID: "legacy-chat", Name: "聊天", Type: 0, Model: "one", API: "https://one.invalid", Key: AIKeyChange{Action: "keep"}}}, Routes: chat.ModelRoutes{Chat: "legacy-chat"}}
	expectStatus(t, request(s, "POST", "/api/ai-models", body, cookie, "bad"), 403)
	expectStatus(t, request(s, "POST", "/api/ai-models", body, cookie, csrf), 200)
	_, body.Revision, _ = store.repository.Read()
	bad := body
	bad.Profiles = []modelProfileUpdate{}
	expectStatus(t, request(s, "POST", "/api/ai-models", bad, cookie, csrf), 400)
	bad = body
	bad.Profiles = append([]modelProfileUpdate(nil), body.Profiles...)
	bad.Profiles[0].Key = AIKeyChange{Action: "clear", Confirm: true}
	expectStatus(t, request(s, "POST", "/api/ai-models", bad, cookie, csrf), 400)
	body.Profiles = append(body.Profiles, modelProfileUpdate{ID: "ollama", Name: "备用", Type: 1, Model: "local", API: "http://localhost:11434", Key: AIKeyChange{Action: "keep"}})
	body.Routes.Chat = "ollama"
	body.Profiles[0].Key = AIKeyChange{Action: "clear", Confirm: true}
	expectStatus(t, request(s, "POST", "/api/ai-models", body, cookie, csrf), 200)
	cfg, _, _ := store.repository.Read()
	if cfg.ModelProfiles[0].Key != "" {
		t.Fatal("clear failed")
	}
	current, err := cfg.ResolveModels()
	if err != nil || current.Type != 1 || current.AgentAPI != "" || current.ImageAPI != "" {
		t.Fatal("disabled optional entrance fell back", err)
	}
}
