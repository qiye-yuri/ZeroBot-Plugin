package chat

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// Read returns one task's current summary in the selected conversation. It
// neither changes task state nor waits for a model or platform action to finish.
func (s *TaskStore) Read(ctx context.Context, grp int64, id string) (AgentTaskInfo, error) {
	decoded, e := hex.DecodeString(id)
	if grp == 0 || e != nil || len(decoded) != 16 || hex.EncodeToString(decoded) != id {
		return AgentTaskInfo{}, ErrAgentTaskConflict
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return AgentTaskInfo{}, ErrAgentTaskStore
	}
	if task := s.active[grp]; task != nil && task.info.ID == id {
		// Memory is authoritative when a failed write leaves stale disk state.
		value := task.info
		if value.Context.MemorySources != nil {
			count := *value.Context.MemorySources
			value.Context.MemorySources = &count
		}
		return value, nil
	}
	var raw string
	e = s.db.QueryRowContext(ctx, `SELECT payload FROM agent_tasks WHERE conversation=? AND id=?`, grp, id).Scan(&raw)
	if errors.Is(e, sql.ErrNoRows) {
		return AgentTaskInfo{}, ErrAgentTaskMissing
	}
	var value AgentTaskInfo
	if e != nil || json.Unmarshal([]byte(raw), &value) != nil || value.ID != id {
		return AgentTaskInfo{}, ErrAgentTaskStore
	}
	return value, nil
}
