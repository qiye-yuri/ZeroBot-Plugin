package chat

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/FloatTech/floatbox/file"
)

var ErrMemory = errors.New("Agent 记忆存储不可用")
var ErrMemoryConflict = errors.New("记忆或旧文件归属已变化，请重新读取")
var ErrMemoryInput = errors.New("记忆参数无效")

var memorySchema = []string{
	`CREATE TABLE IF NOT EXISTS agent_memory_mode (conversation INTEGER PRIMARY KEY, imported_hash TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS agent_memories (id INTEGER PRIMARY KEY AUTOINCREMENT, conversation INTEGER NOT NULL, text TEXT NOT NULL, source TEXT NOT NULL, source_seq INTEGER NOT NULL, active INTEGER NOT NULL, revision INTEGER NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
	`CREATE INDEX IF NOT EXISTS agent_memory_scope ON agent_memories(conversation,id)`,
	`CREATE TABLE IF NOT EXISTS agent_memory_revisions (memory_id INTEGER NOT NULL, revision INTEGER NOT NULL, text TEXT NOT NULL, active INTEGER NOT NULL, source TEXT NOT NULL, source_seq INTEGER NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(memory_id,revision))`,
}

type MemoryStore struct {
	history *agentHistory
	legacy  *memstorage
	bot     int64
}
type MemoryItem struct {
	Kind      string `json:"kind,omitempty"`
	Subject   string `json:"subject,omitempty"`
	Attribute string `json:"attribute,omitempty"`
	ID        string `json:"id"`
	Text      string `json:"text"`
	Source    string `json:"source"`
	SourceSeq string `json:"sourceSeq"`
	Active    bool   `json:"active"`
	Revision  string `json:"revision"`
	UpdatedAt string `json:"updatedAt"`
}

// MemoryReference identifies a saved version without copying its text.
type MemoryReference struct {
	ID        string `json:"id"`
	Revision  string `json:"revision"`
	Source    string `json:"source"`
	SourceSeq string `json:"sourceSeq"`
}

// Legacy text files have no independently addressable records. Created and
// Memory are omitted in that mode rather than inventing an ID or dedup result.
type MemorySaveResult struct {
	Mode    string           `json:"mode"`
	Created *bool            `json:"created,omitempty"`
	Memory  *MemoryReference `json:"memory,omitempty"`
}
type MemoryView struct {
	Mode  string       `json:"mode"`
	Items []MemoryItem `json:"items"`
	More  bool         `json:"more"`
	Next  string       `json:"next"`
}
type MemoryPreview struct {
	Digest string `json:"digest"`
	Count  int    `json:"count"`
	Bytes  int    `json:"bytes"`
	Mode   string `json:"mode"`
}
type memoryClaim struct {
	Bot    int64  `json:"bot"`
	Digest string `json:"digest"`
}

// AgentMemories opens the account's schema under the same registry used by AgentOf.
// The host must authorize bot against its connected-account registry first.
func AgentMemories(bot int64) (*MemoryStore, error) {
	if bot <= 0 {
		return nil, ErrMemoryInput
	}
	agentsMu.Lock()
	defer agentsMu.Unlock()
	legacy, e := atomicgetmemstorage("aichat")
	if e != nil {
		return nil, ErrMemory
	}
	h := histories[bot]
	if h == nil {
		h, e = openAgentHistory(filepath.Join(file.BOTPATH, "data", "aichat", "history"), bot, agentContextBytes, CurrentConfig)
		if e != nil {
			return nil, ErrMemory
		}
		histories[bot] = h
	}
	return &MemoryStore{history: h, legacy: legacy, bot: bot}, nil
}

func memoryText(text string) bool {
	return strings.TrimSpace(text) != "" && len(text) <= 4000 && utf8.ValidString(text) && !strings.ContainsAny(text, "\r\n\x00")
}
func memoryDigest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func (s *MemoryStore) legacyPath(grp int64) string {
	return filepath.Join(s.legacy.root, strconv.FormatInt(grp, 10)+".txt")
}
func (s *MemoryStore) claimPath(grp int64) string {
	return filepath.Join(s.legacy.root, strconv.FormatInt(grp, 10)+".owner.json")
}

func readMemoryFile(p string, limit int64) ([]byte, error) {
	info, e := os.Lstat(p)
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > limit {
		return nil, ErrMemory
	}
	f, e := os.Open(p)
	if e != nil {
		return nil, ErrMemory
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil || int64(len(b)) > limit {
		return nil, ErrMemory
	}
	return b, nil
}

// Caller holds legacy.mu, then history.mu; no reverse lock order is used.
func (s *MemoryStore) mode(ctx context.Context, grp int64) (string, error) {
	if s.legacy.root == "" || s.bot <= 0 {
		return "", ErrMemory
	}
	var hash string
	e := s.history.db.QueryRowContext(ctx, "SELECT imported_hash FROM agent_memory_mode WHERE conversation=?", grp).Scan(&hash)
	if e == nil {
		return "managed", nil
	}
	if e != sql.ErrNoRows {
		return "", ErrMemory
	}
	b, e := readMemoryFile(s.claimPath(grp), 1024)
	if e == nil {
		var c memoryClaim
		if json.Unmarshal(b, &c) != nil || c.Bot <= 0 {
			return "", ErrMemory
		}
		if c.Bot != s.bot {
			return "claimed_elsewhere", nil
		}
		return "pending", nil
	}
	if !os.IsNotExist(e) {
		return "", ErrMemory
	}
	_, e = readMemoryFile(s.legacyPath(grp), 1<<20)
	if os.IsNotExist(e) {
		return "managed", nil
	}
	if e != nil {
		return "", ErrMemory
	}
	return "legacy", nil
}

func (s *MemoryStore) View(ctx context.Context, grp, after, id int64) (MemoryView, error) {
	return s.ViewFiltered(ctx, grp, after, id, "all")
}

// ViewFiltered filters current rows; revision history is always unfiltered.
func (s *MemoryStore) ViewFiltered(ctx context.Context, grp, after, id int64, state string) (MemoryView, error) {
	return s.view(ctx, grp, after, id, state, false, 0)
}

// ViewVersionsNewest pages revisions from newest to oldest. Before is an
// exclusive revision boundary; zero starts at the latest revision.
func (s *MemoryStore) ViewVersionsNewest(ctx context.Context, grp, id, before int64) (MemoryView, error) {
	return s.view(ctx, grp, before, id, "all", true, 0)
}

// ViewRevision is a management read of one saved revision, including disabled
// history. It does not change the current version or the model's read access.
func (s *MemoryStore) ViewRevision(ctx context.Context, grp, id, revision int64) (MemoryView, error) {
	if id <= 0 || revision <= 0 {
		return MemoryView{Items: []MemoryItem{}}, ErrMemoryInput
	}
	return s.view(ctx, grp, 0, id, "all", false, revision)
}

func (s *MemoryStore) view(ctx context.Context, grp, after, id int64, state string, newest bool, revision int64) (MemoryView, error) {
	v := MemoryView{Items: []MemoryItem{}}
	active := -1
	switch state {
	case "all":
	case "active":
		active = 1
	case "disabled":
		active = 0
	default:
		return v, ErrMemoryInput
	}
	if id != 0 && state != "all" {
		return v, ErrMemoryInput
	}
	if grp == 0 || after < 0 || id < 0 || (newest && id == 0) {
		return v, ErrMemoryInput
	}
	s.legacy.mu.Lock()
	defer s.legacy.mu.Unlock()
	s.history.mu.Lock()
	defer s.history.mu.Unlock()
	mode, e := s.mode(ctx, grp)
	if e != nil {
		return v, e
	}
	v.Mode = mode
	if mode != "managed" {
		return v, nil
	}
	cfg, e := s.history.config()
	if e != nil {
		return v, ErrMemory
	}
	query := `SELECT id,text,source,source_seq,active,revision,updated_at FROM agent_memories WHERE conversation=? AND id>? AND (?=-1 OR active=?) ORDER BY id LIMIT 51`
	args := []any{grp, after, active, active}
	if id != 0 {
		query = `SELECT m.id,r.text,r.source,r.source_seq,r.active,r.revision,r.updated_at FROM agent_memory_revisions r JOIN agent_memories m ON m.id=r.memory_id WHERE m.conversation=? AND m.id=? AND r.revision>? ORDER BY r.revision LIMIT 51`
		args = []any{grp, id, after}
		if newest {
			query = `SELECT m.id,r.text,r.source,r.source_seq,r.active,r.revision,r.updated_at FROM agent_memory_revisions r JOIN agent_memories m ON m.id=r.memory_id WHERE m.conversation=? AND m.id=?`
			args = []any{grp, id}
			if after > 0 {
				query += ` AND r.revision<?`
				args = append(args, after)
			}
			query += ` ORDER BY r.revision DESC LIMIT 51`
		}
		if revision > 0 {
			query = `SELECT m.id,r.text,r.source,r.source_seq,r.active,r.revision,r.updated_at FROM agent_memory_revisions r JOIN agent_memories m ON m.id=r.memory_id WHERE m.conversation=? AND m.id=? AND r.revision=?`
			args = []any{grp, id, revision}
		}
	}
	rows, e := s.history.db.QueryContext(ctx, query, args...)
	if e != nil {
		return v, ErrMemory
	}
	defer rows.Close()
	for rows.Next() {
		var item MemoryItem
		var num, seq, rev int64
		var active int
		if rows.Scan(&num, &item.Text, &item.Source, &seq, &active, &rev, &item.UpdatedAt) != nil {
			return v, ErrMemory
		}
		if len(v.Items) == 50 {
			v.More = true
			break
		}
		item.ID = strconv.FormatInt(num, 10)
		item.SourceSeq = strconv.FormatInt(seq, 10)
		item.Revision = strconv.FormatInt(rev, 10)
		item.Active = active == 1
		item.Text = cfg.Redact(item.Text)
		v.Items = append(v.Items, item)
		if id == 0 {
			v.Next = item.ID
		} else {
			v.Next = item.Revision
		}
	}
	if rows.Err() != nil {
		return v, ErrMemory
	}
	rows.Close()
	for i := range v.Items {
		if e := s.memoryMetadata(ctx, &v.Items[i]); e != nil {
			return v, e
		}
	}
	return v, nil
}

func memoryRevision(ctx context.Context, tx *sql.Tx, id int64) error {
	_, e := tx.ExecContext(ctx, `INSERT INTO agent_memory_revisions SELECT id,revision,text,active,source,source_seq,updated_at FROM agent_memories WHERE id=?`, id)
	return e
}

func (s *MemoryStore) Update(ctx context.Context, grp, id, revision int64, action, text string) error {
	return s.update(ctx, grp, id, revision, action, text, 0)
}

// Revert copies an older revision into a new revision without changing active state.
func (s *MemoryStore) Revert(ctx context.Context, grp, id, revision, target int64) error {
	if target <= 0 || target >= revision {
		return ErrMemoryInput
	}
	return s.update(ctx, grp, id, revision, "revert", "", target)
}

func (s *MemoryStore) update(ctx context.Context, grp, id, revision int64, action, text string, target int64) error {
	if grp == 0 || id <= 0 || revision <= 0 || (action != "edit" && action != "disable" && action != "restore" && !(action == "revert" && target > 0)) || (action == "edit" && !memoryText(text)) || (action != "edit" && text != "") {
		return ErrMemoryInput
	}
	s.legacy.mu.Lock()
	defer s.legacy.mu.Unlock()
	s.history.mu.Lock()
	defer s.history.mu.Unlock()
	mode, e := s.mode(ctx, grp)
	if e != nil {
		return e
	}
	if mode != "managed" {
		return ErrMemoryConflict
	}
	cfg, e := s.history.config()
	if e != nil {
		return ErrMemory
	}
	tx, e := s.history.db.BeginTx(ctx, nil)
	if e != nil {
		return ErrMemory
	}
	defer tx.Rollback()
	var result sql.Result
	if action == "restore" {
		var conflict bool
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_memory_layers old JOIN agent_memory_layers other ON old.kind=other.kind AND old.subject=other.subject AND old.attribute=other.attribute JOIN agent_memories m ON m.id=other.memory_id WHERE old.memory_id=? AND m.id<>? AND m.active=1 AND (m.conversation=? OR old.kind='permanent'))`, id, id, grp).Scan(&conflict)
		if e != nil {
			return ErrMemory
		}
		if conflict {
			return ErrMemoryConflict
		}
	}
	if action == "revert" {
		var seq int64
		e = tx.QueryRowContext(ctx, `SELECT r.text,r.source_seq FROM agent_memory_revisions r JOIN agent_memories m ON m.id=r.memory_id WHERE m.conversation=? AND m.id=? AND m.revision=? AND r.revision=?`, grp, id, revision, target).Scan(&text, &seq)
		if errors.Is(e, sql.ErrNoRows) {
			return ErrMemoryConflict
		}
		if e != nil {
			return ErrMemory
		}
		result, e = tx.ExecContext(ctx, `UPDATE agent_memories SET text=?,source='admin',source_seq=?,revision=revision+1,updated_at=? WHERE conversation=? AND id=? AND revision=?`, cfg.Redact(text), seq, time.Now().UTC().Format(time.RFC3339Nano), grp, id, revision)
	} else if action == "edit" {
		result, e = tx.ExecContext(ctx, `UPDATE agent_memories SET text=?,source='admin',source_seq=0,revision=revision+1,updated_at=? WHERE conversation=? AND id=? AND revision=?`, cfg.Redact(strings.TrimSpace(text)), time.Now().UTC().Format(time.RFC3339Nano), grp, id, revision)
	} else {
		active := 0
		if action == "restore" {
			active = 1
		}
		result, e = tx.ExecContext(ctx, `UPDATE agent_memories SET active=?,revision=revision+1,updated_at=? WHERE conversation=? AND id=? AND revision=? AND active!=?`, active, time.Now().UTC().Format(time.RFC3339Nano), grp, id, revision, active)
	}
	if e != nil {
		return ErrMemory
	}
	n, e := result.RowsAffected()
	if e != nil {
		return ErrMemory
	}
	if n != 1 {
		return ErrMemoryConflict
	}
	if memoryRevision(ctx, tx, id) != nil || tx.Commit() != nil {
		return ErrMemory
	}
	return nil
}

func (s *MemoryStore) Save(grp int64, text string) error { return s.SaveFromSequence(grp, text, 0) }

// SaveFrom retains source compatibility for callers holding only a provider
// message ID. Such IDs do not establish a unique archive source.
// Deprecated: use SaveFromSequence with the host-captured event sequence.
func (s *MemoryStore) SaveFrom(grp int64, text, _ string) error {
	return s.SaveFromSequence(grp, text, 0)
}

// SaveFromSequence binds the exact host-recorded trigger, never a model-selected
// source or a guess based on the latest event. Zero explicitly means unknown.
func (s *MemoryStore) SaveFromSequence(grp int64, text string, sourceSeq int64) error {
	_, err := s.SaveFromSequenceResult(grp, text, sourceSeq)
	return err
}

// SaveFromSequenceResult returns only metadata after a committed insertion or
// scoped dedup read. A later edit may invalidate the returned revision.
func (s *MemoryStore) SaveFromSequenceResult(grp int64, text string, sourceSeq int64) (result MemorySaveResult, err error) {
	if grp == 0 || sourceSeq < 0 || !memoryText(text) {
		return result, ErrMemoryInput
	}
	s.legacy.mu.Lock()
	defer s.legacy.mu.Unlock()
	s.history.mu.Lock()
	defer s.history.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if sourceSeq > 0 {
		var found bool
		if e := s.history.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM history WHERE conversation=? AND seq=? AND kind='event')`, grp, sourceSeq).Scan(&found); e != nil {
			return result, ErrMemory
		}
		if !found {
			return result, ErrMemoryConflict
		}
	}
	mode, e := s.mode(ctx, grp)
	if e != nil {
		return result, e
	}
	if mode == "legacy" { // Existing unassigned files retain the old behavior until explicitly migrated.
		cfg, e := s.history.config()
		if e != nil {
			return result, ErrMemory
		}
		if e = s.legacy.saveLockedWithConfig(grp, text, cfg); e != nil {
			return result, e
		}
		return MemorySaveResult{Mode: "legacy"}, nil
	}
	if mode == "pending" {
		return result, ErrMemoryConflict
	}
	// A file assigned to another robot cannot be read or appended; this robot gets its own new store.
	cfg, e := s.history.config()
	if e != nil {
		return result, ErrMemory
	}
	tx, e := s.history.db.BeginTx(ctx, nil)
	if e != nil {
		return result, ErrMemory
	}
	defer tx.Rollback()
	// Compare exactly what would be persisted, under the same lock/transaction as
	// insertion. Repeated saves do not rewrite provenance or refresh recency.
	text = cfg.Redact(strings.TrimSpace(text))
	var id, revision, seq int64
	var source string
	e = tx.QueryRowContext(ctx, `SELECT id,revision,source,source_seq FROM agent_memories WHERE conversation=? AND active=1 AND text=? ORDER BY id DESC LIMIT 1`, grp, text).Scan(&id, &revision, &source, &seq)
	if e == nil {
		if id <= 0 || revision <= 0 || seq < 0 || (source != "agent" && source != "admin" && source != "legacy") {
			return result, ErrMemory
		}
		created := false
		return MemorySaveResult{Mode: "managed", Created: &created, Memory: &MemoryReference{ID: strconv.FormatInt(id, 10), Revision: strconv.FormatInt(revision, 10), Source: source, SourceSeq: strconv.FormatInt(seq, 10)}}, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return result, ErrMemory
	}
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	r, e := tx.ExecContext(ctx, `INSERT INTO agent_memories(conversation,text,source,source_seq,active,revision,created_at,updated_at) VALUES(?,?,'agent',?,1,1,?,?)`, grp, text, sourceSeq, stamp, stamp)
	if e != nil {
		return result, ErrMemory
	}
	id, e = r.LastInsertId()
	if e != nil || id <= 0 {
		return result, ErrMemory
	}
	if _, e = tx.ExecContext(ctx, `INSERT OR IGNORE INTO agent_memory_mode VALUES(?,'')`, grp); e != nil {
		return result, ErrMemory
	}
	if memoryRevision(ctx, tx, id) != nil || tx.Commit() != nil {
		return result, ErrMemory
	}
	created := true
	return MemorySaveResult{Mode: "managed", Created: &created, Memory: &MemoryReference{ID: strconv.FormatInt(id, 10), Revision: "1", Source: "agent", SourceSeq: strconv.FormatInt(sourceSeq, 10)}}, nil
}

func (s *MemoryStore) Load(grp int64) []string { v, _ := s.LoadChecked(grp); return v }
func (s *MemoryStore) LoadChecked(grp int64) (result []string, err error) {
	usage := MemoryUsage{}
	defer func() {
		if err != nil {
			usage = MemoryUsage{Mode: usage.Mode, Reason: "read_error"}
		}
		if grp != 0 && s.history.tasks != nil {
			if e := s.history.tasks.captureMemory(grp, usage); e != nil {
				result, err = nil, e
			}
		}
	}()
	if grp == 0 {
		return nil, ErrMemoryInput
	}
	s.legacy.mu.Lock()
	defer s.legacy.mu.Unlock()
	s.history.mu.Lock()
	defer s.history.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	mode, e := s.mode(ctx, grp)
	if e != nil {
		return nil, e
	}
	if mode == "pending" {
		return nil, ErrMemoryConflict
	}
	usage = MemoryUsage{Available: true, Mode: mode}
	if mode == "claimed_elsewhere" {
		return nil, nil
	}
	cfg, e := s.history.config()
	if e != nil {
		return nil, ErrMemory
	}
	if mode == "legacy" {
		b, e := readMemoryFile(s.legacyPath(grp), 1<<20)
		if e != nil {
			return nil, ErrMemory
		}
		return []string{cfg.Redact(strings.TrimSpace(string(b)))}, nil
	}
	result, usage, err = s.recall(ctx, grp, cfg)
	return result, err
}

func (s *MemoryStore) previewLocked(ctx context.Context, grp int64) (MemoryPreview, []byte, []string, error) {
	p := MemoryPreview{}
	mode, e := s.mode(ctx, grp)
	if e != nil {
		return p, nil, nil, e
	}
	p.Mode = mode
	if mode != "legacy" && mode != "pending" {
		return p, nil, nil, ErrMemoryConflict
	}
	b, e := readMemoryFile(s.legacyPath(grp), 1<<20)
	if e != nil || !utf8.Valid(b) {
		return p, nil, nil, ErrMemory
	}
	lines := []string{}
	for _, line := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !memoryText(line) || len(lines) >= 1000 {
			return p, nil, nil, ErrMemoryInput
		}
		lines = append(lines, line)
	}
	p.Digest = memoryDigest(b)
	p.Bytes = len(b)
	p.Count = len(lines)
	return p, b, lines, nil
}
func (s *MemoryStore) Preview(ctx context.Context, grp int64) (MemoryPreview, error) {
	if grp == 0 {
		return MemoryPreview{}, ErrMemoryInput
	}
	s.legacy.mu.Lock()
	defer s.legacy.mu.Unlock()
	s.history.mu.Lock()
	defer s.history.mu.Unlock()
	p, _, _, e := s.previewLocked(ctx, grp)
	return p, e
}

func privateMemoryFile(p string, b []byte) error {
	f, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	good := false
	defer func() {
		f.Close()
		if !good {
			_ = os.Remove(p)
		}
	}()
	if protectHistoryPath(p, false) != nil {
		return ErrMemory
	}
	if _, e = f.Write(b); e != nil {
		return ErrMemory
	}
	if f.Sync() != nil {
		return ErrMemory
	}
	if f.Close() != nil {
		return ErrMemory
	}
	good = true
	return nil
}

func (s *MemoryStore) Migrate(ctx context.Context, grp int64, digest string) error {
	if grp == 0 || len(digest) != 64 {
		return ErrMemoryInput
	}
	s.legacy.mu.Lock()
	defer s.legacy.mu.Unlock()
	s.history.mu.Lock()
	defer s.history.mu.Unlock()
	var imported string
	e := s.history.db.QueryRowContext(ctx, `SELECT imported_hash FROM agent_memory_mode WHERE conversation=?`, grp).Scan(&imported)
	if e == nil {
		if imported == digest {
			return nil
		}
		return ErrMemoryConflict
	}
	if e != sql.ErrNoRows {
		return ErrMemory
	}
	p, b, lines, e := s.previewLocked(ctx, grp)
	if e != nil {
		return e
	}
	if p.Digest != digest {
		return ErrMemoryConflict
	}
	cfg, e := s.history.config()
	if e != nil {
		return ErrMemory
	}
	backup := filepath.Join(s.legacy.root, strconv.FormatInt(grp, 10)+"."+digest+".bak")
	if e = privateMemoryFile(backup, b); e != nil {
		old, readErr := readMemoryFile(backup, 1<<20)
		if !os.IsExist(e) || readErr != nil || memoryDigest(old) != digest {
			return ErrMemory
		}
	}
	claim := s.claimPath(grp)
	claimData, _ := json.Marshal(memoryClaim{s.bot, digest})
	created := false
	if e = privateMemoryFile(claim, claimData); e == nil {
		created = true
	} else {
		old, re := readMemoryFile(claim, 1024)
		var c memoryClaim
		if !os.IsExist(e) || re != nil || json.Unmarshal(old, &c) != nil || c.Bot != s.bot || c.Digest != digest {
			return ErrMemoryConflict
		}
	}
	keepClaim := false
	defer func() {
		if created && !keepClaim {
			_ = os.Remove(claim)
		}
	}()
	tx, e := s.history.db.BeginTx(ctx, nil)
	if e != nil {
		return ErrMemory
	}
	defer tx.Rollback()
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	for _, line := range lines {
		r, e := tx.ExecContext(ctx, `INSERT INTO agent_memories(conversation,text,source,source_seq,active,revision,created_at,updated_at) VALUES(?,?,'legacy',0,1,1,?,?)`, grp, cfg.Redact(line), stamp, stamp)
		if e != nil {
			return ErrMemory
		}
		id, e := r.LastInsertId()
		if e != nil || memoryRevision(ctx, tx, id) != nil {
			return ErrMemory
		}
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO agent_memory_mode VALUES(?,?)`, grp, digest); e != nil {
		return ErrMemory
	}
	latest, e := readMemoryFile(s.legacyPath(grp), 1<<20)
	if e != nil || memoryDigest(latest) != digest {
		return ErrMemoryConflict
	}
	// Once commit is attempted, an I/O error may leave its outcome uncertain.
	// Keep ownership reserved; only the same bot may retry/reconcile it.
	keepClaim = true
	if tx.Commit() != nil {
		return ErrMemory
	}
	return nil
}
