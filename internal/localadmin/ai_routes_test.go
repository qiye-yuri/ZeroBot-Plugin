package localadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/chat"
	"github.com/FloatTech/zbputils/modelconfig"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func TestFiveModelRoutesPersistenceDeletionInheritanceAndRestore(t *testing.T) {
	store, c, file, backups := aiFixture(t)
	legacy := chat.AgentConfig{ModelProfiles: []chat.ModelProfile{{ID: "chat", Name: "聊天", API: "https://chat.invalid", Model: "chat", Key: "chat-secret"}}, ModelRoutes: &chat.ModelRoutes{Chat: "chat"}, MaxN: 2048}
	if err := c.SetExtra(legacy); err != nil {
		t.Fatal(err)
	}
	plugins := NewPluginStore([]*ctrl.Control[*zero.Ctx]{c}, backups)
	backup := NewBackupStore(backups, nil, plugins, NewLogStore())
	oldID, err := backup.Create("control-db")
	if err != nil {
		t.Fatal(err)
	}
	before, rev, _ := c.ExtraJSON()
	s := newTestServer(t)
	s.cfg.AI = store
	cookie, csrf := enroll(t, s)
	read := request(s, "GET", "/api/ai-models", nil, cookie, "")
	expectStatus(t, read, 200)
	var view struct{ Entries map[string]modelRouteView }
	json.Unmarshal(read.Body.Bytes(), &view)
	after, _, _ := c.ExtraJSON()
	if string(before) != string(after) || !view.Entries["gpt"].Inherited || view.Entries["gpt"].ProfileID != "chat" || !view.Entries["summary"].Inherited {
		t.Fatal("old named JSON read migrated or lost inheritance")
	}
	profiles := []modelProfileUpdate{}
	for i, id := range []string{"chat", "gpt", "summary", "agent", "image"} {
		max := uint(500 + i*100)
		profiles = append(profiles, modelProfileUpdate{ID: id, Name: id, Model: id, API: "https://" + id + ".invalid", Key: AIKeyChange{Action: "replace", Value: id + "-secret"}, Parameters: &modelconfig.Parameters{MaxN: &max}})
	}
	body := modelsUpdate{Revision: rev, Profiles: profiles, Routes: chat.ModelRoutes{Chat: "chat", GPT: "gpt", Summary: "summary", Agent: "agent", Image: "image"}}
	w := request(s, "POST", "/api/ai-models", body, cookie, csrf)
	expectStatus(t, w, 200)
	if err = json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"chat", "gpt", "summary", "agent", "image"} {
		v := view.Entries[id]
		if v.ProfileID != id || v.Inherited || v.Parameters.MaxN != uint(500+i*100) || strings.Contains(w.Body.String(), id+"-secret") {
			t.Fatal("wrong explicit route or leaked key", id)
		}
	}
	_, rev, _ = store.repository.Read()
	body.Revision = rev
	for _, id := range []string{"gpt", "summary"} {
		bad := body
		bad.Profiles = nil
		for _, p := range profiles {
			if p.ID != id {
				bad.Profiles = append(bad.Profiles, p)
			}
		}
		if err = store.SaveModels(bad); err == nil {
			t.Fatal("deleted referenced profile", id)
		}
		_, afterRev, _ := store.repository.Read()
		if afterRev != rev {
			t.Fatal("failed deletion changed revision")
		}
	}
	selectedID, err := backup.Create("control-db")
	if err != nil {
		t.Fatal(err)
	}
	body.Routes.GPT, body.Routes.Summary = "", ""
	if err = store.SaveModels(body); err != nil {
		t.Fatal(err)
	}
	cfg, _, _ := store.repository.Read()
	cfg, err = cfg.ResolveModels()
	if err != nil || cfg.ForModel("GPT").ModelName != "chat" || cfg.ForModel("Summary").RequestParams("").MaxN != 500 {
		t.Fatal("restore inheritance", err)
	}
	if _, err = backup.Restore(context.Background(), previewRestore(t, backup, selectedID), "fixture-session"); err != nil {
		t.Fatal(err)
	}
	if err = c.Manager.D.Close(); err != nil {
		t.Fatal(err)
	}
	manager := ctrl.NewManager[*zero.Ctx](file)
	defer manager.D.Close()
	repository := chat.NewConfigRepository(manager.NewControl("aichatcfg", &ctrl.Options[*zero.Ctx]{Extra: 12345}))
	cfg, _, err = repository.Read()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = cfg.ResolveModels()
	if err != nil || cfg.ForModel("GPT").ModelName != "gpt" || cfg.ForModel("Summary").ModelName != "summary" || cfg.ForModel("Summary").RequestParams("").MaxN != 700 {
		t.Fatal("restart/backup lost routes", err)
	}
	plugins = NewPluginStore([]*ctrl.Control[*zero.Ctx]{repository.Control}, backups)
	backup = NewBackupStore(backups, nil, plugins, NewLogStore())
	if _, err = backup.Restore(context.Background(), previewRestore(t, backup, oldID), "fixture-session"); err != nil {
		t.Fatal(err)
	}
	cfg, _, err = repository.Read()
	if err != nil || cfg.ModelRoutes.GPT != "" || cfg.ModelRoutes.Summary != "" {
		t.Fatal("old three-route backup restore", err)
	}
}

func TestFourDiagnosticRoutesUseSelectedProfilesAcrossProtocols(t *testing.T) {
	for _, kind := range []chat.ModelType{0, 1, 2} {
		t.Run(kind.String(), func(t *testing.T) {
			calls := 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				entry := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")[0]
				index := map[string]int{"chat": 0, "gpt": 1, "summary": 2, "agent": 3}[entry]
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
					return
				}
				if kind == 0 && r.Header.Get("Authorization") != "Bearer key-"+entry {
					t.Error("wrong credential")
				}
				if kind == 2 {
					if r.URL.Query().Get("key") != "key-"+entry || !strings.Contains(r.URL.Path, "chosen-"+entry) {
						t.Error("wrong GenAI route")
					}
					payload = payload["generationConfig"].(map[string]any)
					if payload["maxOutputTokens"] != float64(128) || payload["topP"] != []float64{.2, .3, .4, .5}[index] {
						t.Error("wrong GenAI parameters", payload)
					}
				} else {
					if payload["model"] != "chosen-"+entry || payload["max_tokens"] != float64(128) || payload["top_p"] != []float64{.2, .3, .4, .5}[index] {
						t.Error("wrong selected diagnostic", payload)
					}
				}
				responses := []string{`{"choices":[{"message":{"content":"OK"}}]}`, `{"message":{"content":"OK"}}`, `{"candidates":[{"content":{"parts":[{"text":"OK"}]}}]}`}
				fmt.Fprint(w, responses[int(kind)])
			}))
			defer provider.Close()
			s := newTestServer(t)
			store, c, _, _ := aiFixture(t)
			s.cfg.AI = store
			cfg := chat.AgentConfig{MaxN: 4096, TopP: .9, ModelRoutes: &chat.ModelRoutes{Chat: "chat", GPT: "gpt", Summary: "summary", Agent: "agent"}}
			for i, entry := range []string{"chat", "gpt", "summary", "agent"} {
				max, top := uint(1000+i*100), []float32{.2, .3, .4, .5}[i]
				cfg.ModelProfiles = append(cfg.ModelProfiles, chat.ModelProfile{ID: entry, Name: entry, Type: kind, Model: "chosen-" + entry, API: provider.URL + "/" + entry, Key: chat.ModelKey("key-" + entry), Parameters: &modelconfig.Parameters{MaxN: &max, TopP: &top}})
			}
			if err := c.SetExtra(cfg); err != nil {
				t.Fatal(err)
			}
			before, rev, _ := c.ExtraJSON()
			cookie, csrf := enroll(t, s)
			for _, entry := range []string{"chat", "gpt", "summary", "agent"} {
				w := request(s, "POST", "/api/ai-test", map[string]any{"entry": entry, "revision": rev}, cookie, csrf)
				expectStatus(t, w, 200)
				var result modelProbeResult
				json.Unmarshal(w.Body.Bytes(), &result)
				if !result.Success || result.Model != "chosen-"+entry {
					t.Fatal(w.Body.String())
				}
			}
			after, afterRev, _ := c.ExtraJSON()
			if calls != 4 || string(before) != string(after) || rev != afterRev {
				t.Fatal("diagnostic retried or changed configuration")
			}
		})
	}
}
