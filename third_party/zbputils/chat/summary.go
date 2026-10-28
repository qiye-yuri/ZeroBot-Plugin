package chat

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"strings"
	"time"
	"unicode/utf8"
)

const summarySchema = `CREATE TABLE IF NOT EXISTS agent_summaries (conversation INTEGER PRIMARY KEY, reset_seq INTEGER NOT NULL, through_seq INTEGER NOT NULL, text TEXT NOT NULL, updated_at TEXT NOT NULL)`

// SummaryRequest is an immutable prefix snapshot. New messages remain outside it.
type SummaryRequest struct{ Previous, Records string }
type Summarizer func(SummaryRequest) (string, error)

// ScheduleAgentSummary does no network work on the caller's goroutine.
func ScheduleAgentSummary(bot, conversation int64, summarize Summarizer) {
	agentsMu.Lock()
	h := histories[bot]
	agentsMu.Unlock()
	if h != nil {
		h.scheduleSummary(conversation, summarize)
	}
}

func (s *agentHistory) scheduleSummary(grp int64, summarize Summarizer) {
	if summarize == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.summarizing == nil {
		s.summarizing = map[int64]bool{}
		s.summaryRetry = map[int64]time.Time{}
	}
	if s.summarizing[grp] || time.Now().Before(s.summaryRetry[grp]) {
		return
	}
	// Reserve a full recent tail and avoid tiny/unsuitable context configurations.
	if s.budget < 16384 {
		return
	}
	var reset, through int64
	if s.db.QueryRow(`SELECT COALESCE(MAX(seq),0) FROM history WHERE conversation=? AND kind='reset'`, grp).Scan(&reset) != nil {
		return
	}
	var previous string
	err := s.db.QueryRow(`SELECT through_seq,text FROM agent_summaries WHERE conversation=? AND reset_seq=?`, grp, reset).Scan(&through, &previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return
	}
	if through < reset {
		through = reset
	}
	var size int64
	if s.db.QueryRow(`SELECT COALESCE(SUM(length(CAST(payload AS BLOB))+64),0) FROM history WHERE conversation=? AND seq>?`, grp, through).Scan(&size) != nil || size+int64(len(previous)) < int64(s.budget)*3/5 {
		return
	}
	// Keep at least eight whole message turns. The prefix ends immediately before
	// an event, so a pending tool request/response is never split by compaction.
	var tail int64
	err = s.db.QueryRow(`SELECT seq FROM history WHERE conversation=? AND seq>? AND kind='event' ORDER BY seq DESC LIMIT 1 OFFSET 7`, grp, through).Scan(&tail)
	if err != nil {
		return
	}
	rows, err := s.db.Query(`SELECT seq,kind,payload FROM history WHERE conversation=? AND seq>? AND seq<? ORDER BY seq LIMIT 2048`, grp, through, tail)
	if err != nil {
		return
	}
	var input strings.Builder
	var appended int64
	boundary := through
	candidate := ""
	for rows.Next() {
		var seq int64
		var kind, payload string
		if rows.Scan(&seq, &kind, &payload) != nil {
			rows.Close()
			return
		}
		if kind == "event" && input.Len() > 0 {
			candidate = input.String()
			boundary = seq - 1
		}
		line := fmt.Sprintf("[#%d %s] %s\n", seq, kind, compactHistoryMedia(payload))
		if input.Len()+len(line) > 48000 {
			break
		}
		input.WriteString(line)
		appended = seq
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return
	}
	// If the whole prefix fits, include the last complete turn as well.
	var last int64
	if s.db.QueryRow(`SELECT COALESCE(MAX(seq),0) FROM history WHERE conversation=? AND seq>? AND seq<?`, grp, through, tail).Scan(&last) != nil {
		return
	}
	// A bounded query must never claim coverage of an unexamined record.
	if appended > through && appended == last {
		candidate = input.String()
		boundary = last
	}
	if candidate == "" || boundary <= through {
		return
	}
	s.summarizing[grp] = true
	generation := AgentGeneration()
	go func() {
		result, e := summarize(SummaryRequest{Previous: previous, Records: candidate})
		s.mu.Lock()
		defer s.mu.Unlock()
		defer delete(s.summarizing, grp)
		s.summaryRetry[grp] = time.Now().Add(time.Minute)
		result = strings.TrimSpace(result)
		if e != nil || result == "" || len(result) > 8192 || !utf8.ValidString(result) || generation != AgentGeneration() {
			logrus.Warnln("[chat] 滚动摘要未采纳，保留旧摘要与原始档案。")
			return
		}
		cfg, e := s.config()
		if e != nil {
			return
		}
		result = cfg.Redact(result)
		// Reset and previous-prefix checks make late results harmless.
		_, e = s.db.Exec(`INSERT INTO agent_summaries(conversation,reset_seq,through_seq,text,updated_at)
   SELECT ?,?,?,?,? WHERE COALESCE((SELECT MAX(seq) FROM history WHERE conversation=? AND kind='reset'),0)=?
   ON CONFLICT(conversation) DO UPDATE SET reset_seq=excluded.reset_seq,through_seq=excluded.through_seq,text=excluded.text,updated_at=excluded.updated_at
   WHERE agent_summaries.through_seq<=? OR agent_summaries.reset_seq<>excluded.reset_seq`, grp, reset, boundary, result, time.Now().UTC().Format(time.RFC3339Nano), grp, reset, through)
		if e != nil {
			logrus.Warnln("[chat] 滚动摘要保存失败，保留原始档案。")
		}
	}()
}
