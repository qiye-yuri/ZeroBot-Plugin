package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/FloatTech/floatbox/file"
)

// ArchiveQuery is a read-only, bounded query. Conversation zero lists scopes.
// Snapshot zero starts a new snapshot; Before is an exclusive sequence cursor.
type ArchiveQuery struct {
	TestRun string // Host-bound test scope. Never accepted from model parameters.
	// Sequence selects a single bounded record within the same scope/reset rules.
	Sequence int64
	// Context includes up to this many neighboring events on each side of an
	// exact event source (0..3), within the same transaction/scope/reset boundary.
	Context           int
	Conversation      int64
	Snapshot          int64
	Before            int64
	AfterConversation int64
	HasAfter          bool
	Keyword           string
	Search            bool
	// SearchLimit zero preserves the default 20-result search cap.
	SearchLimit         int
	IncludeResetHistory bool
	// SearchFrom is inclusive; SearchUntil is exclusive. Both use stored archive
	// creation time and filter within the existing bounded search scan.
	SearchFrom  time.Time
	SearchUntil time.Time
	// SearchSender matches the top-level user_id of saved message events only.
	// Zero means unrestricted; missing/invalid metadata must not be inferred.
	SearchSender int64
	// SearchEventType matches the saved event's top-level post_type.
	SearchEventType string
	// EventsOnly prevents local search actions/results from matching their own keyword.
	EventsOnly bool
}

type ArchiveEntry struct {
	Seq       string `json:"seq"`
	Kind      string `json:"kind"`
	Time      string `json:"time"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
}
type ArchiveConversation struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}
type ArchivePage struct {
	Entries       []ArchiveEntry        `json:"entries"`
	Conversations []ArchiveConversation `json:"conversations"`
	Snapshot      int64                 `json:"-"`
	Next          int64                 `json:"-"`
	More          bool                  `json:"more"`
	Partial       bool                  `json:"partial"`
	Scanned       int                   `json:"scanned"`
	Available     bool                  `json:"available"`
	SearchMore    bool                  `json:"searchMore"`
	NextBefore    string                `json:"nextBefore,omitempty"`
}

var ErrArchive = errors.New("会话档案暂不可读取")

func ValidArchiveEventType(value string) bool {
	return value == "" || value == "message" || value == "notice" || value == "request"
}

// ParseArchiveSearchRange accepts optional absolute instants, retaining nanosecond
// precision. A zero time is reserved for an omitted boundary.
func ParseArchiveSearchRange(from, until string) (start, end time.Time, err error) {
	for _, bound := range []struct {
		text   string
		target *time.Time
	}{{from, &start}, {until, &end}} {
		if bound.text == "" {
			continue
		}
		stamp, e := time.Parse(time.RFC3339Nano, bound.text)
		if e != nil || stamp.IsZero() {
			return time.Time{}, time.Time{}, ErrArchive
		}
		*bound.target = stamp.UTC()
	}
	if !start.IsZero() && !end.IsZero() && !start.Before(end) {
		return time.Time{}, time.Time{}, ErrArchive
	}
	return start, end, nil
}

// ReadAgentArchive never creates a database, initializes an Agent or calls a model.
// Callers must authorize the bot ID against their runtime account registry first.
func ReadAgentArchive(ctx context.Context, bot int64, q ArchiveQuery) (ArchivePage, error) {
	return readArchive(ctx, filepath.Join(file.BOTPATH, "data", "aichat", "history"), bot, q, CurrentConfig)
}

func readArchive(ctx context.Context, root string, bot int64, q ArchiveQuery, config func() (AgentConfig, error)) (page ArchivePage, err error) {
	page.Entries = []ArchiveEntry{}
	page.Conversations = []ArchiveConversation{}
	if !ValidArchiveEventType(q.SearchEventType) || (q.SearchEventType != "" && (!q.Search || q.Conversation == 0)) {
		return page, ErrArchive
	}
	if q.SearchSender < 0 || (q.SearchSender > 0 && (!q.Search || q.Conversation == 0)) {
		return page, ErrArchive
	}
	if q.Context < 0 || q.Context > 3 || (q.Context > 0 && (q.Sequence <= 0 || !q.EventsOnly || q.IncludeResetHistory)) {
		return page, ErrArchive
	}
	if q.SearchLimit < 0 || q.SearchLimit > 20 || (q.SearchLimit != 0 && !q.Search) {
		return page, ErrArchive
	}
	if (!q.SearchFrom.IsZero() || !q.SearchUntil.IsZero()) && (!q.Search || q.Conversation == 0) {
		return page, ErrArchive
	}
	if !q.SearchFrom.IsZero() && !q.SearchUntil.IsZero() && !q.SearchFrom.Before(q.SearchUntil) {
		return page, ErrArchive
	}
	if q.Sequence < 0 || (q.Sequence > 0 && (q.Conversation == 0 || q.Search || q.Before != 0 || q.HasAfter)) {
		return page, ErrArchive
	}
	q.Keyword = strings.TrimSpace(q.Keyword)
	if bot <= 0 || q.Snapshot < 0 || q.Before < 0 || (q.Search && (q.Conversation == 0 || (q.Keyword == "" && q.SearchSender == 0 && q.SearchFrom.IsZero() && q.SearchUntil.IsZero() && q.SearchEventType == "") || utf8.RuneCountInString(q.Keyword) > 128)) {
		return page, ErrArchive
	}
	// Reject redirected archive roots/files. Parent project junctions remain valid.
	for _, p := range []string{root, filepath.Join(root, strconv.FormatInt(bot, 10)+".sqlite")} {
		info, e := os.Lstat(p)
		if os.IsNotExist(e) {
			return page, nil
		}
		if e != nil || info.Mode()&os.ModeSymlink != 0 {
			return page, ErrArchive
		}
		if p == root && !info.IsDir() || p != root && !info.Mode().IsRegular() {
			return page, ErrArchive
		}
	}
	abs, e := filepath.Abs(filepath.Join(root, strconv.FormatInt(bot, 10)+".sqlite"))
	if e != nil {
		return page, ErrArchive
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	if !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path
	}
	u.RawQuery = "mode=ro&_pragma=busy_timeout(1000)"
	db, e := sql.Open("sqlite3", u.String())
	if e != nil {
		return page, ErrArchive
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, e := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return page, ErrArchive
	}
	defer tx.Rollback()
	page.Available = true
	page.Snapshot = q.Snapshot
	if page.Snapshot == 0 {
		if tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(seq),0) FROM history").Scan(&page.Snapshot) != nil {
			return page, ErrArchive
		}
	}
	if q.Conversation == 0 {
		rows, e := tx.QueryContext(ctx, "SELECT DISTINCT conversation FROM history WHERE seq<=? AND conversation!=0 AND (?=0 OR conversation>?) ORDER BY conversation LIMIT 51", page.Snapshot, q.HasAfter, q.AfterConversation)
		if e != nil {
			return page, ErrArchive
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if rows.Scan(&id) != nil || id == -9223372036854775808 {
				return page, ErrArchive
			}
			if len(page.Conversations) == 50 {
				page.More = true
				break
			}
			page.Next = id
			kind := "group"
			if id < 0 {
				kind = "private"
				id = -id
			}
			page.Conversations = append(page.Conversations, ArchiveConversation{kind, strconv.FormatInt(id, 10)})
		}
		if rows.Err() != nil {
			return page, ErrArchive
		}
		return page, nil
	}
	cfg, e := config()
	if e != nil {
		return page, ErrArchive
	}
	reset := int64(0)
	if !q.IncludeResetHistory {
		if tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(seq),0) FROM history WHERE conversation=? AND kind='reset' AND seq<=?", q.Conversation, page.Snapshot).Scan(&reset) != nil {
			return page, ErrArchive
		}
	}
	limit := 51
	var hasIsolation int
	if tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='history_test_records'`).Scan(&hasIsolation) != nil {
		return page, ErrArchive
	}
	if q.TestRun != "" && (!testRunPattern.MatchString(q.TestRun) || hasIsolation == 0) {
		return page, nil
	}
	historySource := "history"
	if hasIsolation != 0 {
		historySource = "(SELECT * FROM history WHERE 1" + testHistoryPredicate(q.TestRun) + ") AS history"
	}
	if q.Search {
		limit = 5001
	}
	query := "SELECT seq,kind,created_at,CASE WHEN length(CAST(payload AS BLOB))<=65536 THEN payload ELSE '' END FROM history WHERE conversation=? AND seq<=? AND seq>? AND (?=0 OR seq<?) AND (?=0 OR kind='event') ORDER BY seq DESC LIMIT ?"
	args := []any{q.Conversation, page.Snapshot, reset, q.Before, q.Before, q.EventsOnly, limit}
	if q.Sequence > 0 {
		query = "SELECT seq,kind,created_at,CASE WHEN length(CAST(payload AS BLOB))<=65536 THEN payload ELSE '' END FROM history WHERE seq=? AND conversation=? AND seq<=? AND seq>? AND (?=0 OR kind='event')"
		args = []any{q.Sequence, q.Conversation, page.Snapshot, reset, q.EventsOnly}
	}
	if q.Context > 0 {
		var exists int
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+historySource+" WHERE seq=? AND conversation=? AND kind='event' AND seq<=? AND seq>?)", q.Sequence, q.Conversation, page.Snapshot, reset).Scan(&exists); err != nil {
			return page, ErrArchive
		}
		if exists == 0 {
			return page, nil
		} // Never return neighbors for an unverified anchor.
		query = `WITH older AS (SELECT seq FROM history WHERE conversation=? AND kind='event' AND seq<=? AND seq>? ORDER BY seq DESC LIMIT ?),
		newer AS (SELECT seq FROM history WHERE conversation=? AND kind='event' AND seq>? AND seq<=? ORDER BY seq ASC LIMIT ?)
		SELECT seq,kind,created_at,CASE WHEN length(CAST(payload AS BLOB))<=65536 THEN payload ELSE '' END FROM history
		WHERE seq IN (SELECT seq FROM older UNION ALL SELECT seq FROM newer) ORDER BY seq DESC`
		args = []any{q.Conversation, q.Sequence, reset, q.Context + 1, q.Conversation, q.Sequence, page.Snapshot, q.Context}
	}
	query = strings.ReplaceAll(query, "FROM history", "FROM "+historySource)
	rows, e := tx.QueryContext(ctx, query, args...)
	if e != nil {
		return page, ErrArchive
	}
	defer rows.Close()
	bytesRead, total := 0, 0
	searchLimit := q.SearchLimit
	if searchLimit == 0 {
		searchLimit = 20
	}
	lastScanned := int64(0)
	continueBefore := func(seq int64) {
		page.Partial = true
		if seq > 0 {
			page.SearchMore = true
			page.NextBefore = strconv.FormatInt(seq, 10)
		}
	}
	for rows.Next() {
		if e = ctx.Err(); e != nil {
			return page, ErrArchive
		}
		var seq int64
		var kind, stamp, raw string
		if rows.Scan(&seq, &kind, &stamp, &raw) != nil {
			return page, ErrArchive
		}
		if !q.Search && len(page.Entries) == 50 {
			page.More = true
			break
		}
		if q.Search && (page.Scanned == 5000 || bytesRead+len(raw) > 4*1024*1024) {
			continueBefore(lastScanned)
			break
		}
		previousScanned := lastScanned
		page.Scanned++
		lastScanned = seq
		bytesRead += len(raw)
		if (q.SearchSender > 0 || q.SearchEventType != "") && kind != "event" {
			continue
		}
		if q.Search && (!q.SearchFrom.IsZero() || !q.SearchUntil.IsZero()) {
			created, err := time.Parse(time.RFC3339Nano, stamp)
			if err != nil {
				page.Partial = true // Unknown timestamps cannot prove a range match.
				continue
			}
			if (!q.SearchFrom.IsZero() && created.Before(q.SearchFrom)) || (!q.SearchUntil.IsZero() && !created.Before(q.SearchUntil)) {
				continue
			}
		}
		text := "[单条记录超过 64 KiB，未读取正文]"
		truncated := raw == ""
		if raw != "" {
			var value any
			dec := json.NewDecoder(strings.NewReader(raw))
			dec.UseNumber()
			if dec.Decode(&value) != nil {
				return page, ErrArchive
			}
			if q.SearchEventType != "" {
				match, unknown := archiveEventTypeMatches(value, q.SearchEventType)
				if unknown {
					page.Partial = true
				}
				if !match {
					continue
				}
			}
			if q.SearchSender > 0 {
				match, unknown := archiveSenderMatches(value, q.SearchSender)
				if unknown {
					page.Partial = true
				}
				if !match {
					continue
				}
			}
			if containsKeySetting(value) {
				text = "[密钥设置内容已隐藏]"
			} else {
				b, e := json.Marshal(redactHistoryValue(value, cfg))
				if e != nil {
					return page, ErrArchive
				}
				text = string(b)
			}
		}
		if q.Search {
			if truncated {
				page.Partial = true
				continue // The placeholder is UI metadata, not archived content.
			}
			at := strings.Index(text, q.Keyword)
			if at < 0 {
				continue
			}
			if len(page.Entries) == searchLimit {
				continueBefore(previousScanned)
				break
			}
			start := at - 200
			if start < 0 {
				start = 0
			}
			for start > 0 && !utf8.RuneStart(text[start]) {
				start--
			}
			text = text[start:]
			truncated = truncated || start > 0
		}
		budget := 2000
		if !q.Search {
			budget = 8000
		}
		if q.Context > 0 && seq != q.Sequence {
			budget = 2000
		}
		if (q.Search || q.Context > 0) && total+budget > 16384 {
			budget = 16384 - total
			if budget <= 0 {
				if q.Search {
					continueBefore(previousScanned)
				} else {
					page.Partial = true
				}
				break
			}
		}
		if len(text) > budget {
			n := budget
			for n > 0 && !utf8.ValidString(text[:n]) {
				n--
			}
			text = text[:n]
			truncated = true
		}
		total += len(text)
		if (q.Search || q.Context > 0) && truncated {
			page.Partial = true
		}
		page.Next = seq
		page.Entries = append(page.Entries, ArchiveEntry{strconv.FormatInt(seq, 10), kind, stamp, text, truncated})
	}
	if rows.Err() != nil {
		return page, ErrArchive
	}
	return page, nil
}

func archiveEventTypeMatches(value any, eventType string) (match, unknown bool) {
	event, ok := value.(map[string]any)
	if !ok {
		return false, true
	}
	postType, ok := event["post_type"].(string)
	if !ok || postType == "" {
		return false, true
	}
	return postType == eventType, false
}

func archiveSenderMatches(value any, sender int64) (match, unknown bool) {
	message, ok := value.(map[string]any)
	if !ok {
		return false, true
	}
	post, ok := message["post_type"].(string)
	if !ok || post == "" {
		return false, true
	}
	if post != "message" {
		return false, false
	}
	var text string
	switch id := message["user_id"].(type) {
	case json.Number:
		text = string(id)
	case string:
		text = id
	default:
		return false, true
	}
	id, err := strconv.ParseInt(text, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != text {
		return false, true
	}
	return id == sender, false
}
