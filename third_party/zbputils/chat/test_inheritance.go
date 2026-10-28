package chat

import (
	"database/sql"
	"errors"
	"fmt"

	zero "github.com/wdvxdr1123/ZeroBot"
)

// Referencing a known test message inherits its host scope, including QQ echoes.
func (s *agentHistory) inheritedTestRun(grp int64, ctx *zero.Ctx) (string, error) {
	var cutoff int64
	if e := s.db.QueryRow(`SELECT unix_time FROM historical_test_boundary WHERE id=1`).Scan(&cutoff); e != nil && !errors.Is(e, sql.ErrNoRows) {
		return "", e
	}
	if ctx.Event.Time > 0 && ctx.Event.Time <= cutoff {
		return "historical", nil
	}
	ids := []string{}
	if id := fmt.Sprint(ctx.Event.MessageID); id != "0" && id != "<nil>" {
		ids = append(ids, id)
	}
	for _, part := range ctx.Event.Message {
		if part.Type == "reply" && part.Data["id"] != "" {
			ids = append(ids, part.Data["id"])
		}
	}
	for _, id := range ids {
		var run string
		e := s.db.QueryRow(`SELECT run_id FROM test_message_ids WHERE conversation=? AND message_id=?`, grp, id).Scan(&run)
		if e == nil {
			return run, nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return "", e
		}
	}
	return "", nil
}
