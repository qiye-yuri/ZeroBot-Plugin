package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	zero "github.com/wdvxdr1123/ZeroBot"
)

const StateKeyTestRun = zero.StateKeyPrefixKeep + "_chat_test_run__"

var testRunPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,47}$`)

var testIsolationSchema = []string{
	`CREATE TABLE IF NOT EXISTS history_test_records(seq INTEGER PRIMARY KEY,run_id TEXT NOT NULL,root_seq INTEGER NOT NULL,reason TEXT NOT NULL)`,
	`CREATE INDEX IF NOT EXISTS history_test_run ON history_test_records(run_id,seq)`,
	`CREATE TABLE IF NOT EXISTS test_conversations(conversation INTEGER PRIMARY KEY,run_id TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS test_message_ids(conversation INTEGER NOT NULL,message_id TEXT NOT NULL,run_id TEXT NOT NULL,PRIMARY KEY(conversation,message_id))`,
	`CREATE TABLE IF NOT EXISTS memory_test_records(memory_id INTEGER PRIMARY KEY,run_id TEXT NOT NULL,reason TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS legacy_test_memory(conversation INTEGER NOT NULL,text TEXT NOT NULL,reason TEXT NOT NULL,PRIMARY KEY(conversation,text))`,
	`CREATE TABLE IF NOT EXISTS test_isolation_state(id INTEGER PRIMARY KEY,revision INTEGER NOT NULL)`,
	`INSERT OR IGNORE INTO test_isolation_state VALUES(1,0)`,
	`CREATE TABLE IF NOT EXISTS historical_test_boundary(id INTEGER PRIMARY KEY,unix_time INTEGER NOT NULL,max_seq INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS quarantined_summaries(conversation INTEGER NOT NULL,reset_seq INTEGER NOT NULL,through_seq INTEGER NOT NULL,text TEXT NOT NULL,updated_at TEXT NOT NULL,reason TEXT NOT NULL)`,
}

func InitializeTestIsolation(db *sql.DB) error {
	for _, q := range testIsolationSchema {
		if _, e := db.Exec(q); e != nil {
			return e
		}
	}
	return nil
}

func IsTestEvent(ctx *zero.Ctx) bool { v, _ := ctx.State[StateKeyTestRun].(string); return v != "" }
func (s *agentHistory) activeTestRun(grp int64) string {
	if s.testRun != "" {
		return s.testRun
	}
	if s.tasks == nil {
		return ""
	}
	s.tasks.mu.Lock()
	defer s.tasks.mu.Unlock()
	if t := s.tasks.active[grp]; t != nil {
		return t.info.TestRun
	}
	return ""
}
func (s *agentHistory) sourceTestRun(seq int64) (string, error) {
	var run string
	e := s.db.QueryRow(`SELECT run_id FROM history_test_records WHERE seq=?`, seq).Scan(&run)
	if errors.Is(e, sql.ErrNoRows) {
		return "", nil
	}
	return run, e
}

// Test controls are interpreted by the host, never by a model or saved text.
func (s *agentHistory) prepareTestEvent(grp int64, text string, owner bool) (run, reply string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err = s.db.QueryRow(`SELECT run_id FROM test_conversations WHERE conversation=?`, grp).Scan(&run)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return
	}
	if fields[0] != "/测试会话" && fields[0] != "/测试消息" {
		return
	}
	if !owner {
		return run, "测试隔离控制仅限机器人主人。", nil
	}
	if fields[0] == "/测试消息" {
		if len(fields) < 3 || !testRunPattern.MatchString(fields[1]) {
			return run, "用法：/测试消息 批次ID 测试内容（ID仅字母数字、下划线和连字符）。", nil
		}
		return fields[1], "", nil
	}
	if len(fields) == 3 && fields[1] == "开始" && testRunPattern.MatchString(fields[2]) {
		run = fields[2]
		_, err = s.db.Exec(`INSERT INTO test_conversations VALUES(?,?) ON CONFLICT(conversation) DO UPDATE SET run_id=excluded.run_id`, grp, run)
		return run, "测试会话已开启：" + run + "。此会话后续事件与记忆进入测试隔离范围，结束时发送 /测试会话 结束。", err
	}
	if len(fields) == 2 && fields[1] == "结束" {
		if run == "" {
			run = "control"
		}
		_, err = s.db.Exec(`DELETE FROM test_conversations WHERE conversation=?`, grp)
		return run, "测试会话已结束；测试记录保留，不会加入日常记忆或摘要。", err
	}
	if len(fields) == 2 && fields[1] == "状态" {
		if run == "" {
			return "control", "当前为日常会话。", nil
		}
		return run, "当前测试批次：" + run, nil
	}
	return run, "用法：/测试会话 开始 批次ID；/测试会话 结束。", nil
}

func (s *agentHistory) testMemory(run string, grp, seq int64) (*MemoryStore, error) {
	if !testRunPattern.MatchString(run) || s.root == "" {
		return nil, ErrMemoryInput
	}
	s.testMu.Lock()
	defer s.testMu.Unlock()
	if s.testStores == nil {
		s.testStores = map[string]*MemoryStore{}
	}
	child := s.testStores[run]
	if child == nil {
		root := filepath.Join(s.root, "testing", run)
		h, e := openAgentHistory(root, s.bot, s.budget, s.config)
		if e != nil {
			return nil, e
		}
		h.testRun = run
		legacy := &memstorage{root: filepath.Join(root, "legacy")}
		if e = os.MkdirAll(legacy.root, 0700); e != nil {
			h.Close()
			return nil, e
		}
		child = &MemoryStore{h, legacy, s.bot}
		s.testStores[run] = child
	}
	if seq > 0 {
		var payload, stamp string
		if e := s.db.QueryRow(`SELECT payload,created_at FROM history WHERE seq=? AND conversation=? AND kind='event'`, seq, grp).Scan(&payload, &stamp); e != nil {
			return nil, ErrMemoryConflict
		}
		if _, e := child.history.db.Exec(`INSERT OR IGNORE INTO history(seq,conversation,kind,payload,created_at) VALUES(?,?,'event',?,?)`, seq, grp, payload, stamp); e != nil {
			return nil, e
		}
	}
	return child, nil
}
func (s *MemoryStore) testSaveTarget(grp, seq int64) (*MemoryStore, error) {
	if seq == 0 && s.history.testRun == "" && s.history.activeTestRun(grp) != "" {
		return nil, ErrMemoryConflict
	}
	if s.history.testRun != "" || seq == 0 {
		return s, nil
	}
	run, e := s.history.sourceTestRun(seq)
	if e != nil {
		return nil, e
	}
	if run == "" {
		return s, nil
	}
	return s.history.testMemory(run, grp, seq)
}

func (s *MemoryStore) dailyMemoryFilter(alias string) string {
	if s.history.testRun != "" {
		return ""
	}
	return " AND " + alias + "id NOT IN (SELECT memory_id FROM memory_test_records) AND " + alias + "source_seq NOT IN (SELECT seq FROM history_test_records)"
}

func rememberTestMessage(tx *sql.Tx, grp, seq int64, kind, payload, run string) error {
	if run == "" {
		return nil
	}
	var v struct {
		MessageID json.Number `json:"message_id"`
		Data      struct {
			MessageID json.Number `json:"message_id"`
		} `json:"data"`
	}
	d := json.NewDecoder(strings.NewReader(payload))
	d.UseNumber()
	if d.Decode(&v) != nil {
		return nil
	}
	id := v.MessageID.String()
	if kind == "response" {
		id = v.Data.MessageID.String()
		var raw string
		if e := tx.QueryRow(`SELECT payload FROM history WHERE conversation=? AND kind='request' AND seq<? ORDER BY seq DESC LIMIT 1`, grp, seq).Scan(&raw); e != nil {
			if errors.Is(e, sql.ErrNoRows) {
				return nil
			}
			return e
		}
		var request struct {
			Action string `json:"action"`
			Params struct {
				GroupID int64 `json:"group_id"`
				UserID  int64 `json:"user_id"`
			} `json:"params"`
		}
		if json.Unmarshal([]byte(raw), &request) != nil {
			return nil
		}
		if request.Action == "send_group_msg" {
			if request.Params.GroupID > 0 {
				grp = request.Params.GroupID
			}
		} else if request.Action == "send_private_msg" {
			if request.Params.UserID > 0 {
				grp = -request.Params.UserID
			}
		} else {
			return nil
		}
	}
	if id == "" || id == "0" {
		return nil
	}
	_, e := tx.Exec(`INSERT INTO test_message_ids VALUES(?,?,?) ON CONFLICT(conversation,message_id) DO UPDATE SET run_id=excluded.run_id`, grp, id, run)
	if e != nil {
		return e
	}
	if kind != "response" {
		return nil
	}
	// QQ may echo a bot message before its send API returns. Tag that already
	// archived echo too; matching is by platform ID, never by similar text.
	result, e := tx.Exec(`INSERT OR IGNORE INTO history_test_records SELECT seq,?,?,'echo of test message' FROM history WHERE conversation=? AND kind='event' AND json_valid(payload) AND CAST(json_extract(payload,'$.message_id') AS TEXT)=? AND json_extract(payload,'$.user_id')=json_extract(payload,'$.self_id') AND json_extract(payload,'$.self_id')>0`, run, seq, grp, id)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil || n == 0 {
		return e
	}
	// A late classification may invalidate a summary in the target conversation,
	// including a summary already in flight when the echo arrived.
	if _, e = tx.Exec(`INSERT INTO quarantined_summaries SELECT conversation,reset_seq,through_seq,text,updated_at,'late test echo' FROM agent_summaries WHERE conversation=?`, grp); e != nil {
		return e
	}
	if _, e = tx.Exec(`DELETE FROM agent_summaries WHERE conversation=?`, grp); e != nil {
		return e
	}
	_, e = tx.Exec(`UPDATE test_isolation_state SET revision=revision+1 WHERE id=1`)
	return e
}

// Used by platform-history consumers: omit known test events and bot replies.
func TestMessageReadAllowed(bot, grp int64, id, run string) (bool, error) {
	store, e := AgentMemories(bot)
	if e != nil {
		return false, e
	}
	var count, matching int
	e = store.history.db.QueryRow(`SELECT COUNT(*),COALESCE(MAX(conversation=? AND run_id=?),0) FROM test_message_ids WHERE message_id=?`, grp, run, id).Scan(&count, &matching)
	if e != nil {
		return false, e
	}
	return count == 0 || (run != "" && matching != 0), nil
}

func TestIsolationCutoff(bot int64) (int64, error) {
	store, e := AgentMemories(bot)
	if e != nil {
		return 0, e
	}
	var cutoff int64
	e = store.history.db.QueryRow(`SELECT unix_time FROM historical_test_boundary WHERE id=1`).Scan(&cutoff)
	if errors.Is(e, sql.ErrNoRows) {
		return 0, nil
	}
	return cutoff, e
}

func TestMessageIDs(bot, grp int64, scope ...string) (map[string]bool, error) {
	store, e := AgentMemories(bot)
	if e != nil {
		return nil, e
	}
	q := `SELECT message_id FROM test_message_ids WHERE conversation=?`
	args := []any{grp}
	if len(scope) > 0 && scope[0] != "" {
		q += ` AND run_id=?`
		args = append(args, scope[0])
	}
	rows, e := store.history.db.Query(q, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// IsolationMark is an explicit, reviewed source sequence. No keyword heuristic
// is applied here. The migration caller backs up first and exports the evidence.
type IsolationMark struct {
	Seq    int64  `json:"seq"`
	Run    string `json:"run"`
	Root   int64  `json:"root"`
	Reason string `json:"reason"`
}

func IsolateTestRecords(ctx context.Context, db *sql.DB, marks []IsolationMark, memoryIDs []int64) error {
	if e := InitializeTestIsolation(db); e != nil {
		return e
	}
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	groups := map[int64]bool{}
	for _, m := range marks {
		if m.Seq <= 0 || m.Root <= 0 || !testRunPattern.MatchString(m.Run) || m.Reason == "" {
			return ErrMemoryInput
		}
		var grp int64
		var kind, payload string
		if e = tx.QueryRowContext(ctx, `SELECT conversation,kind,payload FROM history WHERE seq=?`, m.Seq).Scan(&grp, &kind, &payload); e != nil {
			return e
		}
		if kind == "reset" {
			return ErrMemoryInput
		}
		inserted, insertErr := tx.ExecContext(ctx, `INSERT OR IGNORE INTO history_test_records VALUES(?,?,?,?)`, m.Seq, m.Run, m.Root, m.Reason)
		if insertErr != nil {
			return insertErr
		}
		n, e := inserted.RowsAffected()
		if e != nil {
			return e
		}
		if n > 0 {
			groups[grp] = true
		}
		if e = rememberTestMessage(tx, grp, m.Seq, kind, payload, m.Run); e != nil {
			return e
		}
	}
	rows, e := tx.QueryContext(ctx, `SELECT DISTINCT m.conversation FROM agent_memories m JOIN history_test_records t ON t.seq=m.source_seq WHERE m.id NOT IN (SELECT memory_id FROM memory_test_records)`)
	if e != nil {
		return e
	}
	for rows.Next() {
		var grp int64
		if e = rows.Scan(&grp); e != nil {
			rows.Close()
			return e
		}
		groups[grp] = true
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT OR IGNORE INTO memory_test_records SELECT m.id,t.run_id,'source is isolated test event' FROM agent_memories m JOIN history_test_records t ON t.seq=m.source_seq`); e != nil {
		return e
	}
	for _, id := range memoryIDs {
		if id <= 0 {
			return ErrMemoryInput
		}
		var grp int64
		if e = tx.QueryRowContext(ctx, `SELECT conversation FROM agent_memories WHERE id=?`, id).Scan(&grp); e != nil {
			return e
		}
		result, e := tx.ExecContext(ctx, `INSERT OR IGNORE INTO memory_test_records VALUES(?,'historical','reviewed test memory')`, id)
		if e != nil {
			return e
		}
		n, e := result.RowsAffected()
		if e != nil {
			return e
		}
		if n > 0 {
			groups[grp] = true
		}
	}
	for grp := range groups {
		if _, e = tx.ExecContext(ctx, `INSERT INTO quarantined_summaries SELECT conversation,reset_seq,through_seq,text,updated_at,'test isolation invalidated earlier summary' FROM agent_summaries WHERE conversation=?`, grp); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `DELETE FROM agent_summaries WHERE conversation=?`, grp); e != nil {
			return e
		}
	}
	if len(groups) > 0 {
		if _, e = tx.ExecContext(ctx, `UPDATE test_isolation_state SET revision=revision+1 WHERE id=1`); e != nil {
			return e
		}
	}
	return tx.Commit()
}

func testHistoryPredicate(run string) string {
	// Run IDs originate from checked host state, never model parameters.
	if run == "" {
		return ` AND seq NOT IN (SELECT seq FROM history_test_records)`
	}
	if !testRunPattern.MatchString(run) {
		return " AND 0"
	}
	return fmt.Sprintf(` AND (kind='reset' OR seq IN (SELECT seq FROM history_test_records WHERE run_id='%s'))`, run)
}

func (s *agentHistory) testLoadTarget(grp int64) (*MemoryStore, error) {
	if s.testRun != "" {
		return nil, nil
	}
	run := s.activeTestRun(grp)
	if run == "" {
		return nil, nil
	}
	s.tasks.mu.Lock()
	t := s.tasks.active[grp]
	var seq int64
	if t != nil {
		seq, _ = strconv.ParseInt(t.info.SourceSeq, 10, 64)
	}
	s.tasks.mu.Unlock()
	return s.testMemory(run, grp, seq)
}
