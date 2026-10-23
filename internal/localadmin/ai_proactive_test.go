package localadmin

import (
	"github.com/FloatTech/zbputils/chat"
	"testing"
)

func TestProactiveGroupConfigPersistenceAndValidation(t *testing.T) {
	s, c, _, _ := aiFixture(t)
	group := aiView(t, s, 101).Groups[0]
	if group.Proactive || group.ProactiveMinutes != 10 || group.FollowupMinutes != 2 {
		t.Fatal("defaults", group)
	}
	on := true
	interval := 10
	window := 3
	update := AIGroupUpdate{Service: "aichatcfg", GroupID: "101", Revision: group.Revision, Proactive: &on, ProactiveMinutes: &interval, FollowupMinutes: &window}
	if err := s.SaveGroup(update); err != nil {
		t.Fatal(err)
	}
	group = aiView(t, s, 101).Groups[0]
	if !group.Proactive || group.ProactiveMinutes != 10 || group.FollowupMinutes != 3 {
		t.Fatal("roundtrip", group)
	}
	if !chat.Storage(c.GetData(101)).Proactive() {
		t.Fatal("runtime cannot read setting")
	}
	update.Revision = group.Revision
	interval = 0
	if s.SaveGroup(update) == nil {
		t.Fatal("zero interval accepted")
	}
	interval = 1441
	if s.SaveGroup(update) == nil {
		t.Fatal("oversize interval accepted")
	}
	interval = 10
	window = 31
	if s.SaveGroup(update) == nil {
		t.Fatal("oversize window accepted")
	}
	window = 3
	update.GroupID = ""
	update.PrivateID = "101"
	if s.SaveGroup(update) == nil {
		t.Fatal("private proactive accepted")
	}
	update.PrivateID = ""
	update.GroupID = "101"
	update.Service = "llm"
	if s.SaveGroup(update) == nil {
		t.Fatal("GPT proactive accepted")
	}
	if !aiView(t, s, 101).Groups[0].Proactive {
		t.Fatal("invalid update damaged settings")
	}
}
