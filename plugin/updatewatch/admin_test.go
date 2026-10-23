package updatewatch

import (
	core "github.com/FloatTech/ZeroBot-Plugin/internal/updatewatch"
	zero "github.com/wdvxdr1123/ZeroBot"
	"path/filepath"
	"testing"
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
