package modelconfig

import (
	"encoding/json"
	"testing"
)

func TestGPTSummaryInheritanceAndExplicitReferences(t *testing.T) {
	profiles := []Profile{{ID: "chat", API: "http://chat.invalid", Model: "chat", Key: "chat-key"}, {ID: "gpt", API: "http://gpt.invalid", Model: "gpt", Key: "gpt-key"}, {ID: "summary", API: "http://summary.invalid", Model: "summary", Key: "summary-key"}}
	var routes Routes
	if err := json.Unmarshal([]byte(`{"chat":"chat","agent":"","image":""}`), &routes); err != nil {
		t.Fatal(err)
	}
	selected, err := Select(profiles, routes)
	if err != nil || selected["GPT"].ID != "chat" || selected["Summary"].ID != "chat" || selected["Agent"].ID != "" {
		t.Fatal(selected, err)
	}
	routes.GPT, routes.Summary = "gpt", "summary"
	selected, err = Select(profiles, routes)
	if err != nil || selected["GPT"].ID != "gpt" || selected["Summary"].ID != "summary" || selected[""].ID != "chat" {
		t.Fatal(selected, err)
	}
	for _, role := range []string{"gpt", "summary"} {
		bad := routes
		if role == "gpt" {
			bad.GPT = "missing"
		} else {
			bad.Summary = "missing"
		}
		if _, err = Select(profiles, bad); err == nil {
			t.Fatal("invalid explicit reference inherited", role)
		}
	}
	raw, err := json.Marshal(routes)
	if err != nil {
		t.Fatal(err)
	}
	var restored Routes
	if err = json.Unmarshal(raw, &restored); err != nil || restored != routes {
		t.Fatal("route round trip", err)
	}
}
