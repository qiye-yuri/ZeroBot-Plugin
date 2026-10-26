package modelstats

import (
	"encoding/json"
	"testing"
)

func TestEmptySnapshotRecentJSON(t *testing.T) {
	data, err := json.Marshal(New().Read())
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["recent"]) != "[]" {
		t.Fatalf("empty recent must be an available array, got %s", fields["recent"])
	}
}
