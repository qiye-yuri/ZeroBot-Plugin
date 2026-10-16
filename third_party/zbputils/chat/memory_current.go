package chat

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
)

// ReadCurrent locates one current managed memory, including disabled items.
// It does not expose old revisions or unassigned legacy text to the model.
func (s *MemoryStore) ReadCurrent(ctx context.Context, grp, id int64) (v MemoryView, err error) {
	v.Items = []MemoryItem{}
	if grp == 0 || id <= 0 {
		return v, ErrMemoryInput
	}
	s.legacy.mu.Lock()
	defer s.legacy.mu.Unlock()
	s.history.mu.Lock()
	defer s.history.mu.Unlock()
	mode, err := s.mode(ctx, grp)
	if err != nil {
		return v, err
	}
	v.Mode = mode
	if mode != "managed" {
		return v, nil
	}
	cfg, err := s.history.config()
	if err != nil {
		return v, ErrMemory
	}
	var item MemoryItem
	var seq, revision int64
	var active int
	err = s.history.db.QueryRowContext(ctx, `SELECT text,source,source_seq,active,revision,updated_at FROM agent_memories WHERE conversation=? AND id=?`, grp, id).Scan(&item.Text, &item.Source, &seq, &active, &revision, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return v, nil
	}
	if err != nil {
		return v, ErrMemory
	}
	item.ID = strconv.FormatInt(id, 10)
	item.Revision = strconv.FormatInt(revision, 10)
	item.SourceSeq = strconv.FormatInt(seq, 10)
	item.Active = active == 1
	item.Text = cfg.Redact(item.Text)
	v.Items = append(v.Items, item)
	return v, nil
}
