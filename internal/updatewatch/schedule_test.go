package updatewatch

import (
	"testing"
	"time"
)

func TestDailyBeijingSchedule(t *testing.T) {
	at := func(s string) time.Time {
		v, e := time.Parse(time.RFC3339, s)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	c := DefaultConfig()
	c.DailyTime = "09:00"
	c.ScheduleChangedAt = at("2026-10-20T23:00:00+08:00")
	last := at("2026-10-20T00:35:00+08:00")
	for _, tc := range []struct {
		now  string
		want bool
	}{
		{"2026-10-20T23:01:00+08:00", false},
		{"2026-10-21T00:59:59Z", false},
		{"2026-10-21T01:00:00Z", true},
		{"2026-10-21T12:00:00+08:00", true},
	} {
		if got := Due(c, last, at(tc.now)); got != tc.want {
			t.Fatalf("%s: %v", tc.now, got)
		}
	}
	if Due(c, at("2026-10-21T09:01:00+08:00"), at("2026-10-21T12:00:00+08:00")) {
		t.Fatal("duplicate daily run")
	}
	c.DailyTime = "25:00"
	if c.Validate() == nil {
		t.Fatal("invalid time accepted")
	}
	c.DailyTime = ""
	if Due(c, last, last.Add(23*time.Hour)) || !Due(c, last, last.Add(24*time.Hour)) {
		t.Fatal("interval compatibility")
	}
}
