package aiimage

import (
	core "github.com/FloatTech/ZeroBot-Plugin/internal/imagegen"
	sql "github.com/FloatTech/sqlite"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func budgetFixture(t *testing.T, limit, cost string) *storage {
	t.Helper()
	s := &storage{}
	if err := s.open(filepath.Join(t.TempDir(), "image.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.db.Close() })
	snap, err := s.admin(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Quality != "medium" || snap.MonthlyBudgetUSD != "100.000000" {
		t.Fatal("migration defaults", snap)
	}
	u := &core.Update{Revision: snap.Revision, Mode: "openai", URL: "http://127.0.0.1:8000/images/generations", Model: "synthetic", MonthlyBudgetUSD: limit, CustomCostUSD: cost}
	u.Key.Action = "keep"
	if _, err = s.admin(u, func(*sql.Sqlite) error { return nil }); err != nil {
		t.Fatal(err)
	}
	return s
}
func TestBudgetConcurrentPersistenceAndMonth(t *testing.T) {
	s := budgetFixture(t, "1", "0.1")
	now := time.Date(2026, 10, 31, 15, 59, 59, 0, time.UTC)
	var wg sync.WaitGroup
	var admitted atomic.Int32
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _, _, entry, err := s.reserve("test", now)
			if err == nil {
				admitted.Add(1)
				if err = s.finish(entry, true); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	if admitted.Load() != 10 {
		t.Fatalf("admitted=%d", admitted.Load())
	}
	if _, _, _, _, _, err := s.reserve("blocked", now); err == nil {
		t.Fatal("exhausted request admitted")
	}
	// A new storage instance (restart) reads the same independent budget ledger.
	other := &storage{}
	if err := other.open(s.path); err != nil {
		t.Fatal(err)
	}
	defer other.db.Close()
	if _, _, _, _, _, err := other.reserve("blocked after restart", now); err == nil {
		t.Fatal("restart reset budget")
	}
	if _, _, _, _, _, err := other.reserve("new month", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	old, _ := s.budgetLocked(now, usd)
	newMonth, _ := s.budgetLocked(now.Add(time.Second), usd)
	if old.Month != "2026-10" || newMonth.Month != "2026-11" || old.ChargedUSD != "1.000000" || newMonth.ChargedUSD != "0.100000" {
		t.Fatal(old, newMonth)
	}
}
func TestBudgetFailureAndPriceChanges(t *testing.T) {
	s := budgetFixture(t, "0.2", "0.1")
	now := time.Now()
	_, _, _, _, entry, err := s.reserve("test", now)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.finish(entry, false); err != nil {
		t.Fatal(err)
	}
	v, _ := s.admin(nil, nil)
	if v.Budget.UncertainUSD != "0.100000" || v.Budget.ChargedUSD != "0.100000" {
		t.Fatal(v)
	}
	// Changing key/model does not reset money already reserved.
	if err = s.updateField("密钥", "new-key"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, _, err = s.reserve("last", now); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, _, err = s.reserve("blocked", now); err == nil {
		t.Fatal("failure refunded")
	}
}
func TestBudgetZeroUnknownAndInvalid(t *testing.T) {
	s := budgetFixture(t, "0", "0")
	if _, _, _, _, _, err := s.reserve("test", time.Now()); err == nil {
		t.Fatal("zero budget admitted")
	}
	unknown := budgetFixture(t, "100", "")
	if _, _, _, _, _, err := unknown.reserve("test", time.Now()); err == nil {
		t.Fatal("unknown price admitted")
	}
	for _, v := range []string{"-1", "NaN", "1e2", "0.0000001", "100001", "999999999999999999999999999"} {
		if _, err := parseDollars(v, 100000*usd); err == nil {
			t.Fatal(v)
		}
	}
}
func TestPromptDimensionsAndOfficialPrices(t *testing.T) {
	for _, tc := range []struct {
		input, size string
		ok          bool
	}{{"--size=1536x1024 水彩猫", "1536x1024", true}, {"水彩猫 --size=1024×1536", "1024x1536", true}, {"4K横图水彩猫", "1024x1024", true}, {"猫 --size=3840x2160", "3840x2160", true}, {"--size=1024x1024", "", false}, {"猫 --size=1024x1024 --size=1536x1024", "", false}} {
		_, size, err := parseImagePrompt(tc.input, "1024x1024")
		if (err == nil) != tc.ok || (tc.ok && size != tc.size) {
			t.Fatal(tc, err, size)
		}
	}
	cfg := imageConfig{APIURL: "https://api.openai.com/v1/images/generations", ModelName: "gpt-image-2"}
	settings := defaultSettings()
	price, _, err := quoteImage(cfg, "openai", "cat", settings)
	if err != nil || price != 68351 {
		t.Fatal(price, err)
	}
	cfg.ModelName = "gpt-image-2.5-unknown"
	if _, _, err = quoteImage(cfg, "openai", "cat", settings); err == nil {
		t.Fatal("unverified price admitted")
	}
	settings.CustomCost = "0"
	if _, _, err = quoteImage(cfg, "openai", "cat", settings); err == nil {
		t.Fatal("official custom override admitted")
	}
}
func TestBudgetLoweringAndCorruption(t *testing.T) {
	s := budgetFixture(t, "1", "0.6")
	if _, _, _, _, _, err := s.reserve("test", time.Now()); err != nil {
		t.Fatal(err)
	}
	v, _ := s.admin(nil, nil)
	u := &core.Update{Revision: v.Revision, Mode: v.Mode, URL: v.URL, Model: v.Model, MonthlyBudgetUSD: "0.5", CustomCostUSD: "0.6"}
	u.Key.Action = "keep"
	saved, err := s.admin(u, func(*sql.Sqlite) error { return nil })
	if err != nil || !saved.Budget.Stopped || saved.Budget.RemainingUSD != "0.000000" {
		t.Fatal(saved, err)
	}
	if _, _, _, _, _, err = s.reserve("test", time.Now()); err == nil {
		t.Fatal("lower limit ignored")
	}
	database, err := s.ledger()
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec("DROP TABLE image_budget")
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec("CREATE TABLE image_budget (bad TEXT)")
	database.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, _, err = s.reserve("test", time.Now()); err == nil {
		t.Fatal("broken ledger allowed call")
	}
}

func TestBudgetSharedDatabaseAndConfigRestore(t *testing.T) {
	s := budgetFixture(t, "0.5", "0.1")
	other := &storage{}
	if err := other.open(s.path); err != nil {
		t.Fatal(err)
	}
	defer other.db.Close()
	var wg sync.WaitGroup
	var admitted atomic.Int32
	now := time.Now()
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			target := s
			if i%2 == 1 {
				target = other
			}
			if _, _, _, _, _, err := target.reserve("test", now); err == nil {
				admitted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if admitted.Load() > 5 || admitted.Load() == 0 {
		t.Fatal(admitted.Load())
	}
	view, err := s.budgetLocked(now, 500000)
	if err != nil {
		t.Fatal(err)
	}
	amount, _ := parseDollars(view.ChargedUSD, usd)
	if amount != int64(admitted.Load())*100000 {
		t.Fatal(view)
	}
	// Replacing configuration rows (as a settings restore does) cannot erase ledger.
	if _, err = s.db.Exec("DELETE FROM render_settings"); err != nil {
		t.Fatal(err)
	}
	after, err := s.admin(nil, nil)
	if err != nil || after.Budget.ChargedUSD != view.ChargedUSD {
		t.Fatal(after, err)
	}
}
func TestBudgetCustomPriceInvalidatedByChat(t *testing.T) {
	s := budgetFixture(t, "100", "0.1")
	if err := s.updateField("模型名", "changed"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, _, err := s.reserve("test", time.Now()); err == nil {
		t.Fatal("reused custom quote for changed model")
	}
}
