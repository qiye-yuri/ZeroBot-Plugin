package updatewatch

import (
	core "github.com/FloatTech/ZeroBot-Plugin/internal/updatewatch"
	"github.com/FloatTech/zbputils/chat"
	zero "github.com/wdvxdr1123/ZeroBot"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestAdminSettingsRevisionRecipientsAndBackup(t *testing.T) {
	oldConfig, oldState, oldOwners := configPath, statePath, zero.BotConfig.SuperUsers
	defer func() { configPath, statePath, zero.BotConfig.SuperUsers = oldConfig, oldState, oldOwners }()
	configPath = filepath.Join(t.TempDir(), "config.json")
	statePath = filepath.Join(filepath.Dir(configPath), "state.json")
	zero.BotConfig.SuperUsers = []int64{12345, 67890}
	c := core.DefaultConfig()
	c.OwnerID = 12345
	c.BotID = 54321
	if err := core.Save(configPath, c); err != nil {
		t.Fatal(err)
	}
	x, err := AdminSettings("", nil)
	if err != nil {
		t.Fatal(err)
	}
	c.IntervalHours = 6
	y, err := AdminSettings(x.Revision, &c)
	if err != nil {
		t.Fatal(err)
	}
	if y.Config.IntervalHours != 6 || y.Revision == x.Revision {
		t.Fatal("save/revision")
	}
	if _, err = AdminSettings(x.Revision, &c); err == nil {
		t.Fatal("stale write allowed")
	}
	backups, _ := filepath.Glob(filepath.Join(filepath.Dir(configPath), "backups", "*.json"))
	if len(backups) != 1 {
		t.Fatal("backup missing")
	}
	s, _ := core.LoadState(statePath)
	s.OwnerID = c.OwnerID
	s.BotID = c.BotID
	s.Pending = []core.Notice{{Text: "old recipient only"}}
	if err = core.Save(statePath, s); err != nil {
		t.Fatal(err)
	}
	c.OwnerID = 67890
	if _, err = AdminSettings(y.Revision, &c); err == nil {
		t.Fatal("old outbox transferred")
	}
	s.Pending = nil
	if err = core.Save(statePath, s); err != nil {
		t.Fatal(err)
	}
	if _, err = AdminSettings(y.Revision, &c); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	_, err = AdminSettings("", nil)
	mu.Unlock()
	if err == nil {
		t.Fatal("busy lock ignored")
	}
}

func TestAdminRuntimeReadLeavesConfigStateAndBackupsUntouched(t *testing.T) {
	oldConfig, oldState, oldLoader := configPath, statePath, loadModelConfig
	t.Cleanup(func() { configPath, statePath, loadModelConfig = oldConfig, oldState, oldLoader })
	dir := t.TempDir()
	configPath = filepath.Join(dir, "config.json")
	statePath = filepath.Join(dir, "state.json")
	loadModelConfig = func() (chat.AgentConfig, error) { return chat.AgentConfig{}, nil }
	c := core.DefaultConfig()
	c.DailyTime = "09:00"
	if e := core.Save(configPath, c); e != nil {
		t.Fatal(e)
	}
	s := core.State{Seen: map[string]core.Event{"e": {Key: "e", Text: "old"}}, Initialized: map[string]bool{"ftb": true}, Pending: []core.Notice{{Text: "pending"}}, History: []core.Notice{{Text: "sent"}}, LastCheck: time.Date(2026, 10, 24, 1, 0, 0, 0, time.UTC), LastAttempt: time.Date(2026, 10, 24, 1, 0, 0, 0, time.UTC), SourceErrors: map[string]string{"github": "synthetic timeout"}}
	if e := core.Save(statePath, s); e != nil {
		t.Fatal(e)
	}
	if e := core.Save(filepath.Join(dir, "backups", "old.json"), c); e != nil {
		t.Fatal(e)
	}
	snapshot := func() map[string]string {
		values := map[string]string{}
		e := filepath.WalkDir(dir, func(p string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			b, e := os.ReadFile(p)
			values[p] = string(b)
			return e
		})
		if e != nil {
			t.Fatal(e)
		}
		return values
	}
	before := snapshot()
	v, e := AdminSettings("", nil)
	if e != nil {
		t.Fatal(e)
	}
	if v.Pending != 1 || v.Baseline != 1 || !v.LastCheck.Equal(s.LastCheck) || v.SourceErrors["github"] != "synthetic timeout" {
		t.Fatal("incorrect runtime snapshot")
	}
	mu.Lock()
	_, e = AdminSettings("", nil)
	mu.Unlock()
	if e == nil {
		t.Fatal("busy read accepted")
	}
	if _, e = AdminSettings("", nil); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("runtime reads changed persisted state")
	}
	c.IntervalHours++
	if e = core.Save(configPath, c); e != nil {
		t.Fatal(e)
	}
	changed := snapshot()
	fresh, e := AdminSettings("", nil)
	if e != nil || fresh.Revision == v.Revision {
		t.Fatal("revision did not change", e)
	}
	if _, e = AdminSettings(v.Revision, &c); e == nil {
		t.Fatal("stale save accepted")
	}
	if !reflect.DeepEqual(changed, snapshot()) {
		t.Fatal("GET or stale save changed files")
	}
}
