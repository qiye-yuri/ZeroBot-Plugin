package localadmin

import (
	"strings"
	"testing"
)

func TestAIKeyChangeContract(t *testing.T) {
	for _, action := range []string{"keep", "replace", "clear", "", "unknown"} {
		for _, value := range []string{"", "synthetic-new-key"} {
			for _, confirm := range []bool{false, true} {
				change := AIKeyChange{Action: action, Value: value, Confirm: confirm}
				got, err := change.apply("synthetic-old-key")
				valid := action == "keep" && value == "" && !confirm || action == "replace" && value != "" && !confirm || action == "clear" && value == "" && confirm
				if (err == nil) != valid {
					t.Fatalf("action=%q value-present=%v confirm=%v: %v", action, value != "", confirm, err)
				}
				if err != nil {
					if strings.Contains(err.Error(), "synthetic-") || got != "" {
						t.Fatal("invalid operation exposed a key")
					}
					continue
				}
				want := map[string]string{"keep": "synthetic-old-key", "replace": "synthetic-new-key", "clear": ""}[action]
				if got != want {
					t.Fatalf("%s did not preserve/replace/clear the key", action)
				}
			}
		}
	}
}
