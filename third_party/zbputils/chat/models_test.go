package chat

import "testing"

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
