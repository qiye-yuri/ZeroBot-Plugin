package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Metadata is immutable for a memory ID; all text changes use existing revisions.
const memoryLayerSchema = `CREATE TABLE IF NOT EXISTS agent_memory_layers (memory_id INTEGER PRIMARY KEY, kind TEXT NOT NULL, subject INTEGER NOT NULL, attribute TEXT NOT NULL)`

var memoryAttribute = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,79}$`)

func (s *MemoryStore) memoryMetadata(ctx context.Context, item *MemoryItem) error {
	if e := s.history.db.QueryRowContext(ctx, `SELECT run_id FROM memory_test_records WHERE memory_id=?`, item.ID).Scan(&item.TestRun); e != nil && !errors.Is(e, sql.ErrNoRows) {
		return ErrMemory
	}
	var subject int64
	err := s.history.db.QueryRowContext(ctx, `SELECT kind,subject,attribute FROM agent_memory_layers WHERE memory_id=?`, item.ID).Scan(&item.Kind, &subject, &item.Attribute)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return ErrMemory
	}
	if subject > 0 {
		item.Subject = strconv.FormatInt(subject, 10)
	}
	return nil
}

type MemorySpec struct {
	Kind, Attribute   string
	Subject, Revision int64
}

// SaveLayered stores explicit facts or role states. Only a host-authorized owner
// can promote knowledge to permanent/shared scope. Personal records remain scoped.
func (s *MemoryStore) SaveLayered(grp int64, text string, seq int64, spec MemorySpec, owner bool) (MemorySaveResult, error) {
	target, routeErr := s.testSaveTarget(grp, seq)
	if routeErr != nil {
		return MemorySaveResult{}, routeErr
	}
	if target != s {
		v, e := target.SaveLayered(grp, text, seq, spec, owner)
		v.Mode = "test"
		v.TestRun = target.history.testRun
		return v, e
	}
	if spec.Kind == "" || (spec.Kind == "long" && spec.Attribute == "") {
		if spec.Subject != 0 || spec.Revision != 0 || spec.Attribute != "" {
			return MemorySaveResult{}, ErrMemoryInput
		}
		return s.SaveFromSequenceResult(grp, text, seq)
	}
	if grp == 0 || seq <= 0 || !memoryText(text) || !memoryAttribute.MatchString(spec.Attribute) || spec.Revision < 0 {
		return MemorySaveResult{}, ErrMemoryInput
	}
	if spec.Kind != "long" && spec.Kind != "personal" && spec.Kind != "permanent" {
		return MemorySaveResult{}, ErrMemoryInput
	}
	if (spec.Kind == "personal" && spec.Subject <= 0) || (spec.Kind != "personal" && spec.Subject != 0) || (spec.Kind == "permanent" && !owner) {
		return MemorySaveResult{}, ErrMemoryInput
	}
	s.legacy.mu.Lock()
	defer s.legacy.mu.Unlock()
	s.history.mu.Lock()
	defer s.history.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	mode, e := s.mode(ctx, grp)
	if e != nil {
		return MemorySaveResult{}, e
	}
	if mode == "legacy" || mode == "pending" {
		return MemorySaveResult{}, ErrMemoryConflict
	}
	cfg, e := s.history.config()
	if e != nil {
		return MemorySaveResult{}, ErrMemory
	}
	text = cfg.Redact(strings.TrimSpace(text))
	spec.Attribute = strings.TrimSpace(spec.Attribute)
	tx, e := s.history.db.BeginTx(ctx, nil)
	if e != nil {
		return MemorySaveResult{}, ErrMemory
	}
	defer tx.Rollback()
	var payload string
	if e = tx.QueryRowContext(ctx, `SELECT payload FROM history WHERE conversation=? AND seq=? AND kind='event'`, grp, seq).Scan(&payload); e != nil {
		return MemorySaveResult{}, ErrMemoryConflict
	}
	if spec.Kind == "personal" {
		var ev struct {
			UserID int64 `json:"user_id"`
		}
		if json.Unmarshal([]byte(payload), &ev) != nil || ev.UserID != spec.Subject {
			return MemorySaveResult{}, ErrMemoryInput
		}
	}
	// Stable attributes identify changes of a fact, not merely similar sentences.
	var id, rev, sourceSeq int64
	var old, source string
	scope := grp
	if spec.Kind == "permanent" {
		scope = 0
	}
	e = tx.QueryRowContext(ctx, `SELECT m.id,m.revision,m.text,m.source,m.source_seq FROM agent_memories m JOIN agent_memory_layers l ON l.memory_id=m.id WHERE (m.conversation=? OR (?=0 AND l.kind='permanent')) AND l.kind=? AND l.subject=? AND l.attribute=? AND m.active=1 ORDER BY m.id DESC LIMIT 1`, scope, scope, spec.Kind, spec.Subject, spec.Attribute).Scan(&id, &rev, &old, &source, &sourceSeq)
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	created := false
	if e == nil {
		if old == text {
			return MemorySaveResult{Mode: "managed", Created: &created, Memory: &MemoryReference{ID: strconv.FormatInt(id, 10), Revision: strconv.FormatInt(rev, 10), Source: source, SourceSeq: strconv.FormatInt(sourceSeq, 10)}}, nil
		}
		if spec.Revision != rev {
			return MemorySaveResult{}, ErrMemoryConflict
		}
		// Shared records retain their original management/source scope; cross-scope
		// mutation needs a new explicit save in that original scope.
		result, err := tx.ExecContext(ctx, `UPDATE agent_memories SET text=?,source='agent',source_seq=?,revision=revision+1,updated_at=? WHERE id=? AND conversation=? AND revision=?`, text, seq, stamp, id, grp, rev)
		if err != nil {
			return MemorySaveResult{}, ErrMemory
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return MemorySaveResult{}, ErrMemoryConflict
		}
		rev++
	} else if errors.Is(e, sql.ErrNoRows) {
		if spec.Revision != 0 {
			return MemorySaveResult{}, ErrMemoryConflict
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO agent_memories(conversation,text,source,source_seq,active,revision,created_at,updated_at) VALUES(?,?,'agent',?,1,1,?,?)`, grp, text, seq, stamp, stamp)
		if err != nil {
			return MemorySaveResult{}, ErrMemory
		}
		id, e = result.LastInsertId()
		if e != nil {
			return MemorySaveResult{}, ErrMemory
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO agent_memory_layers VALUES(?,?,?,?)`, id, spec.Kind, spec.Subject, spec.Attribute); e != nil {
			return MemorySaveResult{}, ErrMemory
		}
		rev = 1
		created = true
	} else {
		return MemorySaveResult{}, ErrMemory
	}
	if _, e = tx.ExecContext(ctx, `INSERT OR IGNORE INTO agent_memory_mode VALUES(?,'')`, grp); e != nil || memoryRevision(ctx, tx, id) != nil || tx.Commit() != nil {
		return MemorySaveResult{}, ErrMemory
	}
	return MemorySaveResult{Mode: "managed", Created: &created, Memory: &MemoryReference{ID: strconv.FormatInt(id, 10), Revision: strconv.FormatInt(rev, 10), Source: "agent", SourceSeq: strconv.FormatInt(seq, 10)}}, nil
}

func memoryTerms(text string) map[string]bool {
	terms := map[string]bool{}
	runes := []rune(strings.ToLower(text))
	if len(runes) > 2048 {
		runes = runes[:2048]
	}
	for _, w := range strings.FieldsFunc(string(runes), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		r := []rune(w)
		if len(r) < 2 {
			continue
		}
		terms[w] = true
		for i := 1; i < len(r); i++ {
			if unicode.Is(unicode.Han, r[i-1]) && unicode.Is(unicode.Han, r[i]) {
				terms[string(r[i-1:i+1])] = true
			}
		}
	}
	return terms
}

// Called with history.mu held. A bounded lexical recall avoids an embedding
// service and sends no private messages to a second provider for ranking.
func (s *MemoryStore) recall(ctx context.Context, grp int64, cfg AgentConfig) ([]string, MemoryUsage, error) {
	usage := MemoryUsage{Available: true, Mode: "managed", BudgetBytes: 30000, Limit: 100}
	var payload string
	e := s.history.db.QueryRowContext(ctx, `SELECT payload FROM history WHERE conversation=? AND kind='event' AND seq NOT IN (SELECT seq FROM history_test_records) AND seq>COALESCE((SELECT MAX(seq) FROM history WHERE conversation=? AND kind='reset'),0) ORDER BY seq DESC LIMIT 1`, grp, grp).Scan(&payload)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return nil, usage, e
	}
	var ev struct {
		UserID  int64 `json:"user_id"`
		Message []struct {
			Type string `json:"type"`
			Data struct {
				Text string `json:"text"`
			} `json:"data"`
		} `json:"message"`
	}
	_ = json.Unmarshal([]byte(payload), &ev)
	query := ""
	for _, p := range ev.Message {
		if p.Type == "text" {
			query += " " + p.Data.Text
		}
	}
	terms := memoryTerms(query)
	rows, e := s.history.db.QueryContext(ctx, `SELECT m.id,m.revision,m.text,COALESCE(l.kind,'long'),COALESCE(l.subject,0),COALESCE(l.attribute,'') FROM agent_memories m LEFT JOIN agent_memory_layers l ON l.memory_id=m.id WHERE m.active=1`+s.dailyMemoryFilter("m.")+` AND (m.conversation=? OR l.kind='permanent') ORDER BY m.id DESC LIMIT 1001`, grp)
	if e != nil {
		return nil, usage, e
	}
	type candidate struct {
		id, rev, subject      int64
		text, kind, attribute string
		score                 int
	}
	all := []candidate{}
	for rows.Next() {
		var c candidate
		if e = rows.Scan(&c.id, &c.rev, &c.text, &c.kind, &c.subject, &c.attribute); e != nil {
			rows.Close()
			return nil, usage, e
		}
		if len(all) == 1000 {
			usage.Clipped = true
			usage.Reason = "scan_limit"
			break
		}
		if c.kind == "personal" && c.subject == ev.UserID {
			c.score += 100
		}
		if c.kind == "permanent" {
			c.score += 5
		}
		for t := range memoryTerms(c.text) {
			if terms[t] {
				c.score++
			}
		}
		all = append(all, c)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, usage, e
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].score > all[j].score })
	out := []string{}
	seen := map[string]bool{}
	for _, c := range all {
		// Do not collapse identical statements about different people or attributes.
		key := fmt.Sprintf("%s/%d/%s/%s", c.kind, c.subject, c.attribute, strings.TrimSpace(c.text))
		if seen[key] {
			continue
		}
		seen[key] = true
		line := fmt.Sprintf("[记忆 #%d v%d] %s", c.id, c.rev, cfg.Redact(c.text))
		if c.attribute != "" {
			line = fmt.Sprintf("[记忆 #%d v%d %s subject=%d attribute=%s] %s", c.id, c.rev, c.kind, c.subject, c.attribute, cfg.Redact(c.text))
		}
		if len(out) == 100 {
			usage.Clipped = true
			usage.Reason = "record_limit"
			break
		}
		if usage.Bytes+len(line) > 30000 {
			usage.Clipped = true
			usage.Reason = "byte_limit"
			continue
		}
		out = append(out, line)
		usage.Bytes += len(line)
	}
	// Ranked selection, chronological presentation is unnecessary: each record
	// carries an explicit revision and contradictory attributes require CAS.
	usage.Records = len(out)
	return out, usage, nil
}
