package chat

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// CopyTestArchive creates a separate evidence database. Daily rows remain
// immutable and excluded by their tags; copying is idempotent and recoverable.
func CopyTestArchive(ctx context.Context, source *sql.DB, root string, bot int64) error {
	dest, e := openAgentHistory(root, bot, agentContextBytes, func() (AgentConfig, error) { return AgentConfig{}, nil })
	if e != nil {
		return e
	}
	defer dest.Close()
	tx, e := dest.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, q := range []struct{ table, query string }{
		{"history", `SELECT seq,conversation,kind,payload,created_at FROM history WHERE seq IN (SELECT seq FROM history_test_records)`},
		{"history_test_records", `SELECT seq,run_id,root_seq,reason FROM history_test_records`},
		{"test_message_ids", `SELECT conversation,message_id,run_id FROM test_message_ids`},
		{"legacy_test_memory", `SELECT conversation,text,reason FROM legacy_test_memory`},
		{"agent_memories", `SELECT id,conversation,text,source,source_seq,active,revision,created_at,updated_at FROM agent_memories WHERE id IN (SELECT memory_id FROM memory_test_records)`},
		{"agent_memory_revisions", `SELECT memory_id,revision,text,active,source,source_seq,updated_at FROM agent_memory_revisions WHERE memory_id IN (SELECT memory_id FROM memory_test_records)`},
		{"agent_memory_layers", `SELECT memory_id,kind,subject,attribute FROM agent_memory_layers WHERE memory_id IN (SELECT memory_id FROM memory_test_records)`},
		{"agent_memory_mode", `SELECT DISTINCT conversation,'' FROM agent_memories WHERE id IN (SELECT memory_id FROM memory_test_records)`},
		{"quarantined_summaries", `SELECT conversation,reset_seq,through_seq,text,updated_at,reason FROM quarantined_summaries`},
	} {
		rows, e := source.QueryContext(ctx, q.query)
		if e != nil {
			return e
		}
		columns, e := rows.Columns()
		if e != nil {
			rows.Close()
			return e
		}
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if e = rows.Scan(pointers...); e != nil {
				rows.Close()
				return e
			}
			// Quarantined summaries have no natural row key. Avoid re-copy duplicates.
			insert := fmt.Sprintf("INSERT OR IGNORE INTO %s VALUES(%s)", q.table, strings.TrimSuffix(strings.Repeat("?,", len(columns)), ","))
			if q.table == "quarantined_summaries" {
				insert = `INSERT INTO quarantined_summaries SELECT ?,?,?,?,?,? WHERE NOT EXISTS(SELECT 1 FROM quarantined_summaries WHERE conversation=? AND through_seq=? AND updated_at=?)`
				values = append(values, values[0], values[2], values[4])
			}
			if _, e = tx.ExecContext(ctx, insert, values...); e != nil {
				rows.Close()
				return e
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
	}
	return tx.Commit()
}
