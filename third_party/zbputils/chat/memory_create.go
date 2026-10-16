package chat

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

// CreateManual adds explicit administrator memory. Legacy ownership must be
// resolved first; identical active text reuses existing immutable provenance.
func (s *MemoryStore) CreateManual(ctx context.Context, grp int64, text string) (item MemoryItem, created bool, err error) {
	if grp == 0 || !memoryText(text) {
		return item, false, ErrMemoryInput
	}
	s.legacy.mu.Lock()
	defer s.legacy.mu.Unlock()
	s.history.mu.Lock()
	defer s.history.mu.Unlock()
	mode, e := s.mode(ctx, grp)
	if e != nil {
		return item, false, e
	}
	if mode != "managed" {
		return item, false, ErrMemoryConflict
	}
	cfg, e := s.history.config()
	if e != nil {
		return item, false, ErrMemory
	}
	text = cfg.Redact(strings.TrimSpace(text))
	tx, e := s.history.db.BeginTx(ctx, nil)
	if e != nil {
		return item, false, ErrMemory
	}
	defer tx.Rollback()
	var id, revision, seq int64
	e = tx.QueryRowContext(ctx, `SELECT id,revision,source,source_seq,updated_at,text FROM agent_memories WHERE conversation=? AND active=1 AND text=? ORDER BY id DESC LIMIT 1`, grp, text).Scan(&id, &revision, &item.Source, &seq, &item.UpdatedAt, &item.Text)
	if e == nil {
		item.ID = strconv.FormatInt(id, 10)
		item.Revision = strconv.FormatInt(revision, 10)
		item.SourceSeq = strconv.FormatInt(seq, 10)
		item.Active = true
		item.Text = cfg.Redact(item.Text)
		return item, false, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return MemoryItem{}, false, ErrMemory
	}
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	result, e := tx.ExecContext(ctx, `INSERT INTO agent_memories(conversation,text,source,source_seq,active,revision,created_at,updated_at) VALUES(?,?,'admin',0,1,1,?,?)`, grp, text, stamp, stamp)
	if e != nil {
		return MemoryItem{}, false, ErrMemory
	}
	id, e = result.LastInsertId()
	if e != nil {
		return MemoryItem{}, false, ErrMemory
	}
	if _, e = tx.ExecContext(ctx, `INSERT OR IGNORE INTO agent_memory_mode VALUES(?,'')`, grp); e != nil {
		return MemoryItem{}, false, ErrMemory
	}
	if memoryRevision(ctx, tx, id) != nil || tx.Commit() != nil {
		return MemoryItem{}, false, ErrMemory
	}
	return MemoryItem{ID: strconv.FormatInt(id, 10), Revision: "1", Source: "admin", SourceSeq: "0", Active: true, UpdatedAt: stamp, Text: text}, true, nil
}
