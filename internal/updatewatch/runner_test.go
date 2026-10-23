package updatewatch

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(version string) Event {
	return Event{Key: "version:" + version, Date: "2026-10-19", Version: version, Stage: "已发布", Text: "Important official change: fixes dedicated server crash.", URLs: []string{ftbPage}}
}
func TestBaselineChangesRestartAndOutbox(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, e := LoadState(path)
	if e != nil {
		t.Fatal(e)
	}
	calls := 0
	judge := func(_ context.Context, ch []Change, _ []Notice) ([]Notice, error) {
		calls++
		return []Notice{{Keys: []string{ch[0].Current.Key}, Text: "更新通知"}}, nil
	}
	input := map[string][]Event{"ftb": {fixture("1.0.0")}}
	check := func() {
		t.Helper()
		if e := Check(context.Background(), &s, path, input, map[string]string{}, judge); e != nil {
			t.Fatal(e)
		}
	}
	check()
	if calls != 0 || len(s.Pending) != 0 {
		t.Fatal("baseline must be silent")
	}
	input["ftb"] = append(input["ftb"], fixture("1.1.0"))
	check()
	if calls != 1 || len(s.Pending) != 1 {
		t.Fatal("new event not queued")
	}
	if e = Deliver(&s, path, func(string) bool { return false }); e != nil {
		t.Fatal(e)
	}
	s, e = LoadState(path)
	if e != nil {
		t.Fatal(e)
	}
	check()
	if calls != 1 || len(s.Pending) != 1 {
		t.Fatal("restart lost outbox or rejudged")
	}
	sent := 0
	if e = Deliver(&s, path, func(string) bool { sent++; return true }); e != nil {
		t.Fatal(e)
	}
	s, e = LoadState(path)
	if e != nil {
		t.Fatal(e)
	}
	check()
	_ = Deliver(&s, path, func(string) bool { sent++; return true })
	if sent != 1 || len(s.History) != 1 {
		t.Fatal("duplicate notification")
	}
}
func TestFailedJudgeDoesNotConsumeChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, _ := LoadState(path)
	s.Initialized["ftb"] = true
	input := map[string][]Event{"ftb": {fixture("1.4.0")}}
	err := Check(context.Background(), &s, path, input, map[string]string{}, func(context.Context, []Change, []Notice) ([]Notice, error) {
		return nil, errors.New("model unavailable")
	})
	if err == nil || len(s.Seen) != 0 || len(s.Pending) != 0 {
		t.Fatal("model failure consumed event")
	}
}
func TestSourceRecoveryAndConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, _ := LoadState(path)
	judge := func(context.Context, []Change, []Notice) ([]Notice, error) {
		t.Fatal("must not judge source baseline/conflict")
		return nil, nil
	}
	_ = Check(context.Background(), &s, path, map[string][]Event{}, map[string]string{"ftb": "HTTP 503"}, judge)
	if s.Initialized["ftb"] {
		t.Fatal("failed source initialized")
	}
	_ = Check(context.Background(), &s, path, map[string][]Event{"ftb": {fixture("1.0.0")}}, map[string]string{}, judge)
	a, b := fixture("1.1.0"), fixture("1.1.0")
	b.Date = "2026-10-18"
	_ = Check(context.Background(), &s, path, map[string][]Event{"ftb": {a}, "curseforge": {b}}, map[string]string{}, judge)
	if _, ok := s.Seen[a.Key]; ok {
		t.Fatal("conflicting event accepted")
	}
}
func TestDecisionEvidenceAndFormat(t *testing.T) {
	ch := []Change{{Current: fixture("1.4.0")}}
	good := `{"findings":[{"indices":[0],"summary":"修复服务器崩溃","impact":"提升服务器稳定性","evidence":["fixes dedicated server crash."]}]}`
	n, e := ParseDecision(good, ch)
	if e != nil || len(n) != 1 || !strings.Contains(n[0].Text, ftbPage) {
		t.Fatalf("%v %v", n, e)
	}
	for _, raw := range []string{`{}`, `{"findings":null}`, strings.Replace(good, "fixes dedicated server crash.", "invented official evidence here", 1), strings.Replace(good, `[0]`, `[99]`, 1), strings.Replace(good, "提升服务器稳定性", "[CQ:at,qq=all]", 1)} {
		if _, e = ParseDecision(raw, ch); e == nil {
			t.Fatalf("accepted invalid decision %s", raw)
		}
	}
	if n, e = ParseDecision(`{"findings":[]}`, ch); e != nil || len(n) != 0 {
		t.Fatal("valid quiet decision rejected")
	}
}
func TestConfigAndSourceAllowlist(t *testing.T) {
	c := DefaultConfig()
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	c.IntervalHours = 0
	if c.Validate() == nil {
		t.Fatal("zero interval")
	}
	for _, s := range []string{"http://github.com/FTBTeam", "https://github.com.evil.test/", "https://github.com:9000/x", "https://user:secret@github.com/x", "https://127.0.0.1/x"} {
		if allowedURL(s) == nil {
			t.Fatal(s)
		}
	}
}
