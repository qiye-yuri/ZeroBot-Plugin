package chat

import (
	"database/sql"
	"errors"
)

var ErrMessageRecalled = errors.New("原消息已撤回，不能继续读取或处理")

// Original records remain available for local audit. Model-facing reads omit them.
const recallPredicate = ` AND seq NOT IN (SELECT seq FROM recalled_history)`

func initializeRecall(db *sql.DB) error {
	tx, e := db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, q := range []string{
		`CREATE INDEX IF NOT EXISTS history_recall_lookup ON history(conversation, json_extract(CASE WHEN json_valid(payload) THEN payload ELSE '{}' END,'$.notice_type'), CAST(json_extract(CASE WHEN json_valid(payload) THEN payload ELSE '{}' END,'$.message_id') AS TEXT)) WHERE kind='event'`,
		`CREATE VIEW IF NOT EXISTS recalled_history AS SELECT h.seq FROM history h WHERE h.kind='event' AND json_extract(CASE WHEN json_valid(h.payload) THEN h.payload ELSE '{}' END,'$.post_type')='message' AND EXISTS (SELECT 1 FROM history r WHERE r.conversation=h.conversation AND r.kind='event' AND json_extract(CASE WHEN json_valid(r.payload) THEN r.payload ELSE '{}' END,'$.notice_type') IN ('group_recall','friend_recall') AND CAST(json_extract(CASE WHEN json_valid(r.payload) THEN r.payload ELSE '{}' END,'$.message_id') AS TEXT)=CAST(json_extract(CASE WHEN json_valid(h.payload) THEN h.payload ELSE '{}' END,'$.message_id') AS TEXT) AND COALESCE((SELECT run_id FROM history_test_records WHERE seq=r.seq),'')=COALESCE((SELECT run_id FROM history_test_records WHERE seq=h.seq),''))`,
		`CREATE TABLE IF NOT EXISTS recall_migration(id INTEGER PRIMARY KEY)`,
		`INSERT INTO quarantined_summaries SELECT conversation,reset_seq,through_seq,text,updated_at,'message recall migration' FROM agent_summaries WHERE NOT EXISTS(SELECT 1 FROM recall_migration) AND conversation IN (SELECT conversation FROM history WHERE seq IN (SELECT seq FROM recalled_history))`,
		`DELETE FROM agent_summaries WHERE NOT EXISTS(SELECT 1 FROM recall_migration) AND conversation IN (SELECT conversation FROM history WHERE seq IN (SELECT seq FROM recalled_history))`,
		`INSERT OR IGNORE INTO recall_migration VALUES(1)`,
	} {
		if _, e = tx.Exec(q); e != nil {
			return e
		}
	}
	return tx.Commit()
}

func invalidateRecalledSummary(tx *sql.Tx, grp int64) error {
	for _, q := range []string{
		`INSERT INTO quarantined_summaries SELECT conversation,reset_seq,through_seq,text,updated_at,'message recalled' FROM agent_summaries WHERE conversation=?`,
		`DELETE FROM agent_summaries WHERE conversation=?`,
	} {
		if _, e := tx.Exec(q, grp); e != nil {
			return e
		}
	}
	_, e := tx.Exec(`UPDATE test_isolation_state SET revision=revision+1 WHERE id=1`)
	return e
}

func (s *agentHistory) messageRecallCheck(grp int64, id, run string) error {
	var found bool
	e := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM history r WHERE conversation=? AND kind='event' AND json_extract(CASE WHEN json_valid(payload) THEN payload ELSE '{}' END,'$.notice_type') IN ('group_recall','friend_recall') AND CAST(json_extract(CASE WHEN json_valid(payload) THEN payload ELSE '{}' END,'$.message_id') AS TEXT)=? AND COALESCE((SELECT run_id FROM history_test_records WHERE seq=r.seq),'')=?)`, grp, id, run).Scan(&found)
	if e != nil {
		return e
	}
	if found {
		return ErrMessageRecalled
	}
	return nil
}

// MessageRecallCheck fails closed if the archive cannot be read.
func MessageRecallCheck(bot, grp int64, id, run string) error {
	store, e := AgentMemories(bot)
	if e != nil {
		return e
	}
	return store.history.messageRecallCheck(grp, id, run)
}
