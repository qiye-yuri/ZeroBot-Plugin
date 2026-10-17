package chat

import (
	"encoding/json"
	"github.com/FloatTech/zbputils/modelconfig"
	"testing"
)

func TestIndependentParametersInheritanceAndSnapshot(t *testing.T) {
	max, top, empty := uint(600), float32(.3), ""
	c := AgentConfig{MaxN: 2048, TopP: .8, ReasoningEffort: "high", ModelProfiles: []ModelProfile{
		{ID: "chat", Model: "chat", API: "http://chat.invalid", Key: "key", Parameters: &modelconfig.Parameters{MaxN: &max, ReasoningEffort: &empty}},
		{ID: "agent", Model: "agent", API: "http://agent.invalid", Key: "key", Parameters: &modelconfig.Parameters{TopP: &top}},
	}, ModelRoutes: &ModelRoutes{Chat: "chat", Agent: "agent", Image: "chat"}}
	resolved, err := c.ResolveModels()
	if err != nil {
		t.Fatal(err)
	}
	want := modelconfig.RequestParameters{MaxN: 600, TopP: .8, ReasoningEffort: ""}
	if resolved.RequestParams("") != want || resolved.RequestParams("Image") != want {
		t.Fatal("shared profile lost overrides")
	}
	if p := resolved.RequestParams("Agent"); p.MaxN != 2048 || p.TopP != .3 || p.ReasoningEffort != "high" {
		t.Fatal(p)
	}
	max = 999
	top = .5
	if resolved.RequestParams("") != want || resolved.RequestParams("Agent").TopP != .3 {
		t.Fatal("snapshot aliases mutable parameters")
	}
	if resolved.MaxN != 2048 || resolved.TopP != .8 || resolved.ReasoningEffort != "high" {
		t.Fatal("overrides rewrote globals")
	}
	c.ModelProfiles[0].Parameters = nil
	next, err := c.ResolveModels()
	if err != nil {
		t.Fatal(err)
	}
	if next.RequestParams("") != c.GlobalParameters() {
		t.Fatal("removing override failed")
	}
	for _, raw := range []string{`{}`, `{"parameters":null}`, `{"parameters":{"maxN":null,"topP":null,"reasoningEffort":null}}`} {
		var p ModelProfile
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			t.Fatal(err)
		}
		if p.Parameters.Resolve(c.GlobalParameters()) != c.GlobalParameters() {
			t.Fatal("missing/null must inherit")
		}
	}
	legacy := AgentConfig{ReasoningEffort: "low"}
	legacy, err = legacy.ResolveModels()
	if err != nil {
		t.Fatal(err)
	}
	if p := legacy.RequestParams("Agent"); p.MaxN != 4096 || p.TopP != .9 || p.ReasoningEffort != "low" {
		t.Fatal("legacy defaults changed", p)
	}
}

func TestModelCatalogDedupResolutionAndSnapshot(t *testing.T) {
	c := AgentConfig{Type: 0, ModelName: "same", API: "https://one.invalid", Key: "first", AgentModelName: "same", AgentAPI: "https://one.invalid", AgentKey: "first", ImageModelName: "same", ImageAPI: "https://one.invalid", ImageKey: "second"}
	profiles, routes := c.ModelCatalog()
	if len(profiles) != 2 || routes.Chat != routes.Agent || routes.Image == routes.Chat {
		t.Fatal("migration dedup ignored full configuration")
	}
	c.ModelProfiles = profiles
	c.ModelRoutes = &routes
	snapshot, err := c.ResolveModels()
	if err != nil {
		t.Fatal(err)
	}
	c.ModelProfiles[0].Model = "new"
	c.ModelProfiles[0].Key = "new-key"
	if snapshot.ModelName != "same" || snapshot.AgentKey != "first" || snapshot.ImageKey != "second" {
		t.Fatal("resolved request snapshot changed")
	}
	c.ModelRoutes.Agent = "missing"
	if _, err = c.ResolveModels(); err == nil {
		t.Fatal("missing explicit selection fell back")
	}
	c.ModelRoutes.Agent = ""
	c.ModelRoutes.Image = ""
	resolved, err := c.ResolveModels()
	if err != nil || resolved.AgentAPI != "" || resolved.ImageAPI != "" {
		t.Fatal("optional disabled route fell back")
	}
}

func TestGPTSummarySnapshotUsesItsOwnGlobalInheritance(t *testing.T) {
	chatMax, gptTop := uint(900), float32(.4)
	c := AgentConfig{MaxN: 2048, TopP: .8, ReasoningEffort: "high", ModelProfiles: []ModelProfile{
		{ID: "chat", Name: "聊天", API: "http://chat.invalid", Model: "chat", Key: "chat-key", Parameters: &modelconfig.Parameters{MaxN: &chatMax}},
		{ID: "gpt", Name: "GPT", API: "http://gpt.invalid", Model: "gpt", Key: "gpt-key", Parameters: &modelconfig.Parameters{TopP: &gptTop}},
	}, ModelRoutes: &ModelRoutes{Chat: "chat", GPT: "gpt"}}
	snapshot, err := c.ResolveModels()
	if err != nil {
		t.Fatal(err)
	}
	c.ModelProfiles[1].Key = "changed"
	gptTop = .1
	gpt := snapshot.ForModel("GPT")
	summary := snapshot.ForModel("Summary")
	if gpt.ModelName != "gpt" || gpt.Key != "gpt-key" || gpt.RequestParams("").MaxN != 2048 || gpt.RequestParams("").TopP != .4 {
		t.Fatal("GPT inherited chat overrides or lost snapshot", gpt.RequestParams(""))
	}
	if summary.ModelName != "chat" || summary.RequestParams("").MaxN != 900 {
		t.Fatal("summary did not inherit chat")
	}
	if snapshot.ModelName != "chat" || snapshot.RequestParams("").MaxN != 900 || snapshot.MaxN != 2048 {
		t.Fatal("selection changed snapshot/global")
	}
	legacy, err := (AgentConfig{ModelName: "legacy", MaxN: 1234}).ResolveModels()
	if err != nil || legacy.ForModel("GPT").ModelName != "legacy" || legacy.ForModel("Summary").RequestParams("").MaxN != 1234 {
		t.Fatal("legacy changed", err)
	}
}
