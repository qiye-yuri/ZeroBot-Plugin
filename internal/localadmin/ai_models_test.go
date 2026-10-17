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
	"github.com/FloatTech/zbputils/modelconfig"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func TestModelParametersSaveRejectRestartAndRestore(t *testing.T) {
	s, c, file, backups := aiFixture(t)
	if err := c.SetExtra(map[string]any{"ModelName": "legacy", "API": "https://chat.invalid", "Key": "PRIVATE_KEY", "MaxN": 2048, "TopP": .8, "ReasoningEffort": "high"}); err != nil {
		t.Fatal(err)
	}
	plugins := NewPluginStore([]*ctrl.Control[*zero.Ctx]{c}, backups)
	backup := NewBackupStore(backups, nil, plugins, NewLogStore())
	id, err := backup.Create("control-db")
	if err != nil {
		t.Fatal(err)
	}
	_, revision, _ := s.repository.Read()
	max, top, empty := uint(700), float32(.25), ""
	body := modelsUpdate{Revision: revision, Profiles: []modelProfileUpdate{{ID: "one", Name: "模型", Model: "chosen", API: "https://chat.invalid", Key: AIKeyChange{Action: "replace", Value: "PRIVATE_KEY"}, Parameters: &modelconfig.Parameters{MaxN: &max, TopP: &top, ReasoningEffort: &empty}}}, Routes: chat.ModelRoutes{Chat: "one", Agent: "one", Image: "one"}}
	generation := chat.AgentGeneration()
	if err = s.SaveModels(body); err != nil {
		t.Fatal(err)
	}
	if chat.AgentGeneration() <= generation {
		t.Fatal("parameter save did not invalidate Agent/image cache")
	}
	cfg, rev, _ := s.repository.Read()
	resolved, err := cfg.ResolveModels()
	if err != nil {
		t.Fatal(err)
	}
	if p := resolved.RequestParams("Image"); p.MaxN != 700 || p.TopP != .25 || p.ReasoningEffort != "" {
		t.Fatal(p)
	}
	if cfg.MaxN != 2048 || cfg.TopP != .8 || cfg.ReasoningEffort != "high" {
		t.Fatal("globals changed")
	}
	if err = s.SaveModels(body); !errors.Is(err, ctrl.ErrConfigConflict) {
		t.Fatal("stale parameter save accepted", err)
	}
	body.Revision = rev
	before, _, _ := c.ExtraJSON()
	for _, raw := range []string{`{"maxN":0}`, `{"maxN":131073}`, `{"topP":0}`, `{"topP":-0.1}`, `{"topP":1.01}`} {
		var p modelconfig.Parameters
		if err = json.Unmarshal([]byte(raw), &p); err != nil {
			t.Fatal(err)
		}
		body.Profiles[0].Parameters = &p
		if err = s.SaveModels(body); err == nil {
			t.Fatal("invalid parameters accepted", raw)
		}
		after, afterRev, _ := c.ExtraJSON()
		if string(before) != string(after) || afterRev != rev {
			t.Fatal("invalid save changed configuration")
		}
	}
	parametersBackup, err := backup.Create("control-db")
	if err != nil {
		t.Fatal(err)
	}
	body.Profiles[0].Parameters = nil
	if err = s.SaveModels(body); err != nil {
		t.Fatal(err)
	}
	cleared, _, _ := s.repository.Read()
	cleared, err = cleared.ResolveModels()
	if err != nil || cleared.RequestParams("") != cleared.GlobalParameters() {
		t.Fatal("restore inheritance", err)
	}
	if _, err = backup.Restore(context.Background(), previewRestore(t, backup, parametersBackup), "fixture-session"); err != nil {
		t.Fatal(err)
	}
	// A newly opened repository must retain explicit empty reasoning, not inherit.
	if err = c.Manager.D.Close(); err != nil {
		t.Fatal(err)
	}
	manager := ctrl.NewManager[*zero.Ctx](file)
	defer manager.D.Close()
	repository := chat.NewConfigRepository(manager.NewControl("aichatcfg", &ctrl.Options[*zero.Ctx]{Extra: 12345}))
	reread, _, err := repository.Read()
	if err != nil {
		t.Fatal(err)
	}
	if reread.ModelProfiles[0].Parameters.ReasoningEffort == nil || *reread.ModelProfiles[0].Parameters.ReasoningEffort != "" {
		t.Fatal("restart lost explicit empty")
	}
	plugins = NewPluginStore([]*ctrl.Control[*zero.Ctx]{repository.Control}, backups)
	backup = NewBackupStore(backups, nil, plugins, NewLogStore())
	// Restore the complete pre-R5 snapshot through the existing backup flow.
	restore := previewRestore(t, backup, id)
	if _, err = backup.Restore(context.Background(), restore, "fixture-session"); err != nil {
		t.Fatal(err)
	}
	restored, _, err := repository.Read()
	if err != nil || restored.ModelRoutes != nil || restored.MaxN != 2048 {
		t.Fatal("restore lost legacy globals", err)
	}
}

func TestModelParameterHTTPViewsAndValidation(t *testing.T) {
	s := newTestServer(t)
	store, c, _, _ := aiFixture(t)
	s.cfg.AI = store
	if err := c.SetExtra(map[string]any{"ModelName": "legacy", "API": "https://chat.invalid", "Key": "PRIVATE_KEY", "MaxN": 2048, "TopP": .8, "ReasoningEffort": "high"}); err != nil {
		t.Fatal(err)
	}
	cookie, csrf := enroll(t, s)
	_, revision, _ := store.repository.Read()
	profile := map[string]any{"id": "one", "name": "模型", "type": 0, "model": "chosen", "api": "https://chat.invalid", "key": map[string]any{"action": "replace", "value": "PRIVATE_KEY"}, "parameters": map[string]any{"maxN": 600, "topP": .35, "reasoningEffort": ""}}
	body := map[string]any{"revision": revision, "profiles": []any{profile}, "routes": chat.ModelRoutes{Chat: "one", Agent: "one"}}
	w := request(s, "POST", "/api/ai-models", body, cookie, csrf)
	expectStatus(t, w, 200)
	var view struct {
		Revision          string
		Profiles          []modelProfileView
		ParameterDefaults modelconfig.RequestParameters
	}
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(w.Body.String(), "PRIVATE_KEY") {
		t.Fatal("view leaked key")
	}
	if p := view.Profiles[0]; p.Parameters.ReasoningEffort == nil || *p.Parameters.ReasoningEffort != "" || p.EffectiveParameters.MaxN != 600 || p.EffectiveParameters.TopP != .35 || p.EffectiveParameters.ReasoningEffort != "" || view.ParameterDefaults.MaxN != 2048 {
		t.Fatal("view lost values or sources", view)
	}
	body["revision"] = view.Revision
	for _, p := range []any{map[string]any{"maxN": 1.5}, map[string]any{"maxN": -1}, map[string]any{"topP": "0.5"}, map[string]any{"reasoningEffort": false}} {
		profile["parameters"] = p
		expectStatus(t, request(s, "POST", "/api/ai-models", body, cookie, csrf), 400)
	}
	profile["parameters"] = map[string]any{"maxN": nil, "topP": nil, "reasoningEffort": nil}
	w = request(s, "POST", "/api/ai-models", body, cookie, csrf)
	expectStatus(t, w, 200)
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Profiles[0].EffectiveParameters != view.ParameterDefaults {
		t.Fatal("null fields did not inherit")
	}
}

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
