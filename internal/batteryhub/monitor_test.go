package batteryhub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestThresholdSequence(t *testing.T) {
	state := 0
	for _, tc := range []struct{ percent, alert int }{
		{20, 0}, {10, 0}, {9, 1}, {8, 0}, {6, 0}, {5, 2}, {4, 0}, {6, 0}, {5, 0}, {-1, 0}, {101, 0}, {10, 0}, {5, 2},
	} {
		next, alert := Next(state, &tc.percent)
		if alert != tc.alert {
			t.Fatalf("percent %d state %d: alert %d want %d", tc.percent, state, alert, tc.alert)
		}
		state = next
	}
	if next, alert := Next(2, nil); next != 2 || alert != 0 {
		t.Fatal("disconnect reset latch")
	}
}

func TestPersistenceAndRetry(t *testing.T) {
	count := 0
	success := false
	m := &monitor{state: map[string]int{}, path: filepath.Join(t.TempDir(), "state.json"), send: func(string) bool { count++; return success }}
	low, critical := 9, 5
	readings := []Reading{{"headset", &low}, {"mouse", &critical}}
	if err := m.update(readings); err != nil {
		t.Fatal(err)
	}
	if len(m.state) != 0 {
		t.Fatal("failed sends consumed alerts")
	}
	success = true
	if err := m.update(readings); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(m.path)
	if err != nil {
		t.Fatal(err)
	}
	m.state = nil
	if err = json.Unmarshal(data, &m.state); err != nil {
		t.Fatal(err)
	}
	if err = m.update(readings); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("restart duplicated sends: %d", count)
	}
	if m.state["headset"] != 1 || m.state["mouse"] != 2 {
		t.Fatal("devices shared latch")
	}
}
