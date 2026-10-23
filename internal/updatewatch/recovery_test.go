package updatewatch

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPartialDeliveryResumesInOrderAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Pending = []Notice{{Keys: []string{"a"}, Text: "first"}, {Keys: []string{"b"}, Text: "second"}, {Keys: []string{"c"}, Text: "third"}}
	if err = Save(path, s); err != nil {
		t.Fatal(err)
	}
	var attempted []string
	err = Deliver(&s, path, func(text string) bool {
		attempted = append(attempted, text)
		return text != "second"
	})
	if err != nil || !reflect.DeepEqual(attempted, []string{"first", "second"}) {
		t.Fatalf("unexpected first delivery: %v %v", attempted, err)
	}
	s, err = LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.History) != 1 || s.History[0].Text != "first" || len(s.Pending) != 2 {
		t.Fatalf("lost partial receipt: %+v", s)
	}
	attempted = nil
	if err = Deliver(&s, path, func(text string) bool { attempted = append(attempted, text); return true }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(attempted, []string{"second", "third"}) {
		t.Fatalf("wrong retry order or duplicate: %v", attempted)
	}
	s, err = LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Pending) != 0 || len(s.History) != 3 {
		t.Fatalf("incomplete persisted receipts: %+v", s)
	}
	if err = Deliver(&s, path, func(string) bool { t.Fatal("completed queue resent"); return false }); err != nil {
		t.Fatal(err)
	}
}

func TestPendingAndDeliveredNoticesIncludedInNextJudgment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Initialized["ftb"] = true
	s.History = []Notice{{Keys: []string{"old"}, Text: "delivered"}}
	s.Pending = []Notice{{Keys: []string{"queued"}, Text: "not yet delivered"}}
	input := map[string][]Event{"ftb": {fixture("2.0.0")}}
	calls := 0
	judge := func(_ context.Context, changes []Change, known []Notice) ([]Notice, error) {
		calls++
		if len(changes) != 1 || len(known) != 2 || known[0].Text != "delivered" || known[1].Text != "not yet delivered" {
			t.Fatalf("missing deduplication context: %+v %+v", changes, known)
		}
		return []Notice{}, nil
	}
	if err = Check(context.Background(), &s, path, input, map[string]string{}, judge); err != nil {
		t.Fatal(err)
	}
	s, err = LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = Check(context.Background(), &s, path, input, map[string]string{}, judge); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(s.Pending) != 1 || len(s.History) != 1 {
		t.Fatalf("unchanged event rejudged or queue altered: calls=%d state=%+v", calls, s)
	}
}

func TestAllSourcesFailPreservesBaselineAndOutbox(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	e := fixture("1.0.0")
	s.Initialized["ftb"] = true
	s.Seen[e.Key] = e
	s.Pending = []Notice{{Keys: []string{e.Key}, Text: "queued"}}
	judge := func(context.Context, []Change, []Notice) ([]Notice, error) {
		t.Fatal("unchanged/failed source invoked judge")
		return nil, nil
	}
	if err = Check(context.Background(), &s, path, map[string][]Event{}, map[string]string{"ftb": "synthetic timeout"}, judge); err != nil {
		t.Fatal(err)
	}
	s, err = LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.LastError != "全部来源读取失败" || s.SourceErrors["ftb"] != "synthetic timeout" || !s.Initialized["ftb"] || !reflect.DeepEqual(s.Seen[e.Key], e) || len(s.Pending) != 1 {
		t.Fatalf("failure lost state: %+v", s)
	}
	if err = Check(context.Background(), &s, path, map[string][]Event{"ftb": {e}}, map[string]string{}, judge); err != nil {
		t.Fatal(err)
	}
	s, err = LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.LastError != "" || len(s.SourceErrors) != 0 || len(s.Pending) != 1 || len(s.History) != 0 {
		t.Fatalf("recovery changed queue or kept stale error: %+v", s)
	}
}
