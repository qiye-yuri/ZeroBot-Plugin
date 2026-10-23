package minecraftobserver

import (
	"github.com/tidwall/gjson"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
	"path/filepath"
	"testing"
)

func TestDeliveryPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mc.db")
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	must(initializeDB(path))
	defer func() { dbInstance.sdb.Close() }()
	d := dbInstance
	must(d.newSubscribe("fixture:1", 123, targetTypeUser))
	must(d.newSubscribe("fixture:1", 456, targetTypeGroup))
	must(d.queueStatusChange(&serverStatus{ServerAddr: "fixture:1", Players: "1/20", PingDelay: 1}, message.Message{message.Text("change-one")}))
	calls := map[string]int{}
	fail := true
	call := func(action string, p zero.Params) zero.APIResponse {
		calls[action]++
		if _, ok := p["message"].(message.Message); !ok {
			t.Fatal("nonstandard message")
		}
		if fail && action == "send_private_msg" {
			return zero.APIResponse{Status: "failed"}
		}
		return zero.APIResponse{Status: "ok", Data: gjson.Parse(`{"message_id":1}`)}
	}
	enabled := func(pendingNotice) bool { return true }
	if d.deliverPending(call, enabled) == nil {
		t.Fatal("failure not reported")
	}
	if calls["send_group_msg"] != 1 || calls["send_private_msg"] != 1 {
		t.Fatal(calls)
	}
	must(d.sdb.Close())
	must(initializeDB(path))
	d = dbInstance
	var pending []pendingNotice
	must(d.sdb.Find(&pending).Error)
	if len(pending) != 1 {
		t.Fatal("pending lost across restart")
	}
	fail = false
	must(d.deliverPending(call, enabled))
	must(d.deliverPending(call, enabled))
	if calls["send_group_msg"] != 1 || calls["send_private_msg"] != 2 {
		t.Fatal("duplicate successful recipient", calls)
	}
	must(d.queueStatusChange(&serverStatus{ServerAddr: "fixture:1", Players: "2/20", PingDelay: 1}, message.Message{message.Text("change-two")}))
	must(d.deleteSubscribe("fixture:1", 123, targetTypeUser))
	must(d.deliverPending(call, func(n pendingNotice) bool { return n.TargetType != targetTypeGroup }))
	if calls["send_group_msg"] != 1 || calls["send_private_msg"] != 2 {
		t.Fatal("sent cancelled or disabled subscription")
	}
	var count int
	must(d.sdb.Model(&pendingNotice{}).Count(&count).Error)
	if count != 0 {
		t.Fatal("stale notice not removed")
	}
}

func TestDeliveryReceipt(t *testing.T) {
	for _, tc := range []struct {
		name, status, data string
		code               int64
		ok                 bool
	}{
		{"ok", "ok", `{"message_id":1}`, 0, true},
		{"negative", "ok", `{"message_id":-1}`, 0, true},
		{"failed", "failed", `{"message_id":1}`, 0, false},
		{"code", "ok", `{"message_id":1}`, 1200, false},
		{"zero", "ok", `{"message_id":0}`, 0, false},
		{"absent", "ok", `{}`, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if e := initializeDB(filepath.Join(t.TempDir(), "mc.db")); e != nil {
				t.Fatal(e)
			}
			d := dbInstance
			defer d.sdb.Close()
			if e := d.newSubscribe("fixture:1", 123, targetTypeUser); e != nil {
				t.Fatal(e)
			}
			for i := 0; i < 2; i++ {
				if e := d.queueStatusChange(&serverStatus{ServerAddr: "fixture:1", PingDelay: 1}, message.Message{message.Text("change")}); e != nil {
					t.Fatal(e)
				}
			}
			calls := 0
			e := d.deliverPending(func(string, zero.Params) zero.APIResponse {
				calls++
				return zero.APIResponse{Status: tc.status, RetCode: tc.code, Data: gjson.Parse(tc.data)}
			}, func(pendingNotice) bool { return true })
			var count int
			d.sdb.Model(&pendingNotice{}).Count(&count)
			if tc.ok {
				if e != nil || count != 0 || calls != 2 {
					t.Fatal(e, count, calls)
				}
			} else if e == nil || count != 2 || calls != 1 {
				t.Fatal("failure/order not preserved", e, count, calls)
			}
		})
	}
}

func TestStatusQueueAtomicity(t *testing.T) {
	if e := initializeDB(filepath.Join(t.TempDir(), "mc.db")); e != nil {
		t.Fatal(e)
	}
	d := dbInstance
	defer d.sdb.Close()
	if e := d.newSubscribe("fixture:1", 123, targetTypeUser); e != nil {
		t.Fatal(e)
	}
	if e := d.updateServerStatus(&serverStatus{ServerAddr: "fixture:1", Players: "old"}); e != nil {
		t.Fatal(e)
	}
	if e := d.sdb.DropTable(&pendingNotice{}).Error; e != nil {
		t.Fatal(e)
	}
	if e := d.queueStatusChange(&serverStatus{ServerAddr: "fixture:1", Players: "new"}, message.Message{message.Text("new")}); e == nil {
		t.Fatal("expected write failure")
	}
	status, e := d.getServerStatus("fixture:1")
	if e != nil || status.Players != "old" {
		t.Fatal("baseline committed without notification", status, e)
	}
}

func TestZeroLatencyPersistence(t *testing.T) {
	if e := initializeDB(filepath.Join(t.TempDir(), "mc.db")); e != nil {
		t.Fatal(e)
	}
	d := dbInstance
	defer d.sdb.Close()
	for _, delay := range []int64{0, -1, 0} {
		if e := d.updateServerStatus(&serverStatus{ServerAddr: "fixture:1", PingDelay: delay}); e != nil {
			t.Fatal(e)
		}
		s, e := d.getServerStatus("fixture:1")
		if e != nil || s.PingDelay != delay {
			t.Fatal("latency changed", s, e)
		}
	}
	if e := d.queueStatusChange(&serverStatus{ServerAddr: "fixture:1", PingDelay: 0}, message.Message{message.Text("zero")}); e != nil {
		t.Fatal(e)
	}
	s, e := d.getServerStatus("fixture:1")
	if e != nil || s.PingDelay != 0 {
		t.Fatal("queued zero latency changed", s, e)
	}
}
