package updatewatch

import "time"

// Due uses Beijing time independently of the host's configured timezone.
func Due(c Config, last, now time.Time) bool {
	if c.DailyTime == "" {
		return last.IsZero() || now.Sub(last) >= time.Duration(c.IntervalHours)*time.Hour
	}
	hm, err := time.Parse("15:04", c.DailyTime)
	if err != nil {
		return false
	}
	local := now.In(time.FixedZone("Asia/Shanghai", 8*60*60))
	slot := time.Date(local.Year(), local.Month(), local.Day(), hm.Hour(), hm.Minute(), 0, 0, local.Location())
	return !now.Before(slot) && last.Before(slot) && c.ScheduleChangedAt.Before(slot)
}
