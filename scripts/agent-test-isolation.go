//go:build ignore

// Extract a user-authorized historical time range, then apply its immutable plan.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/FloatTech/zbputils/chat"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var dbFile = flag.String("db", "", "one account history database")
var output = flag.String("out", "", "private local output directory")
var cutoff = flag.String("before", "", "user-authorized inclusive RFC3339 boundary; required for export")
var applyPlan = flag.String("apply", "", "reviewed plan JSON; stop service and back up first")
var legacyDir = flag.String("legacy-dir", "", "optional existing legacy memory directory to snapshot")

type record struct {
	Seq          int64  `json:"seq"`
	Conversation int64  `json:"conversation"`
	Kind         string `json:"kind"`
	Time         string `json:"time"`
	Payload      string `json:"payload"`
	Text         string `json:"text,omitempty"`
	SHA          string `json:"sha256"`
	Root         int64  `json:"root"`
}
type memory struct {
	ID, Conversation, Revision int64
	Text, SHA                  string
}
type legacy struct {
	Name, SHA    string
	Conversation int64
	Lines        []string
}
type plan struct {
	Database string
	Snapshot int64
	Cutoff   string
	Records  []record
	Memories []memory
	Legacy   []legacy
}

func hash(s string) string { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }
func textOf(raw string) (string, bool) {
	var e struct {
		Post    string          `json:"post_type"`
		Message json.RawMessage `json:"message"`
	}
	if json.Unmarshal([]byte(raw), &e) != nil || e.Post != "message" {
		return "", false
	}
	var text string
	if json.Unmarshal(e.Message, &text) == nil {
		return text, true
	}
	var parts []struct {
		Type string `json:"type"`
		Data struct {
			Text string `json:"text"`
		} `json:"data"`
	}
	if json.Unmarshal(e.Message, &parts) == nil {
		for _, p := range parts {
			if p.Type == "text" {
				text += p.Data.Text
			}
		}
	}
	return text, true
}
func writeJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, b, 0600)
}
func main() {
	flag.Parse()
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	if *dbFile == "" || *output == "" {
		return errors.New("-db and -out required")
	}
	absolute, e := filepath.Abs(*dbFile)
	if e != nil {
		return e
	}
	*dbFile = absolute
	if e := os.MkdirAll(*output, 0700); e != nil {
		return e
	}
	mode := "ro"
	if *applyPlan != "" {
		mode = "rw"
	}
	db, e := sql.Open("sqlite3", "file:/"+filepath.ToSlash(*dbFile)+"?mode="+mode)
	if e != nil {
		return e
	}
	defer db.Close()
	if *applyPlan != "" {
		return apply(db)
	}
	if _, e := os.Stat(filepath.Join(*output, "plan.json")); e == nil {
		return errors.New("output plan already exists; use a new export directory")
	}
	before, e := time.Parse(time.RFC3339Nano, *cutoff)
	if e != nil {
		return errors.New("explicit -before boundary required")
	}
	p := plan{Database: filepath.Base(*dbFile), Cutoff: before.UTC().Format(time.RFC3339Nano), Records: []record{}, Memories: []memory{}, Legacy: []legacy{}}
	rows, e := db.Query(`SELECT seq,conversation,kind,created_at,payload FROM history ORDER BY seq`)
	if e != nil {
		return e
	}
	roots := map[int64]int64{}
	messages := 0
	for rows.Next() {
		var r record
		if e = rows.Scan(&r.Seq, &r.Conversation, &r.Kind, &r.Time, &r.Payload); e != nil {
			rows.Close()
			return e
		}
		stamp, e := time.Parse(time.RFC3339Nano, r.Time)
		if e != nil {
			rows.Close()
			return errors.New("unknown timestamp requires manual review")
		}
		if stamp.After(before) {
			continue
		}
		r.SHA = hash(r.Payload)
		p.Snapshot = r.Seq
		if r.Kind == "event" {
			var message bool
			r.Text, message = textOf(r.Payload)
			roots[r.Conversation] = r.Seq
			if message {
				messages++
			}
		}
		r.Root = roots[r.Conversation]
		if r.Root == 0 {
			r.Root = r.Seq
		}
		p.Records = append(p.Records, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	rows, e = db.Query(`SELECT id,conversation,revision,text,updated_at FROM agent_memories ORDER BY id`)
	if e != nil {
		return e
	}
	for rows.Next() {
		var m memory
		var stamp string
		if e = rows.Scan(&m.ID, &m.Conversation, &m.Revision, &m.Text, &stamp); e != nil {
			rows.Close()
			return e
		}
		t, e := time.Parse(time.RFC3339Nano, stamp)
		if e != nil {
			rows.Close()
			return errors.New("unknown memory timestamp requires manual review")
		}
		if t.After(before) {
			continue
		}
		m.SHA = hash(m.Text)
		p.Memories = append(p.Memories, m)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if *legacyDir != "" {
		files, e := filepath.Glob(filepath.Join(*legacyDir, "*.txt"))
		if e != nil {
			return e
		}
		for _, file := range files {
			info, e := os.Lstat(file)
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				return errors.New("invalid legacy file")
			}
			grp, e := strconv.ParseInt(strings.TrimSuffix(filepath.Base(file), ".txt"), 10, 64)
			if e != nil {
				return e
			}
			if info.ModTime().After(before) {
				return errors.New("legacy file changed after the authorized boundary")
			}
			b, e := os.ReadFile(file)
			if e != nil {
				return e
			}
			entry := legacy{Name: filepath.Base(file), SHA: hash(string(b)), Conversation: grp}
			for _, line := range strings.Split(string(b), "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					entry.Lines = append(entry.Lines, line)
				}
			}
			p.Legacy = append(p.Legacy, entry)
		}
	}
	if e = writeJSON(filepath.Join(*output, "plan.json"), p); e != nil {
		return e
	}
	f, e := os.OpenFile(filepath.Join(*output, "messages-and-results.jsonl"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	enc := json.NewEncoder(f)
	for _, r := range p.Records {
		if e = enc.Encode(r); e != nil {
			f.Close()
			return e
		}
	}
	if e = f.Close(); e != nil {
		return e
	}
	var report strings.Builder
	fmt.Fprintf(&report, "# 历史测试资料（本机私有）\n\n用户授权：截至 %s 的消息均视为测试。冻结档案边界 #%d。\n\n", p.Cutoff, p.Snapshot)
	for _, r := range p.Records {
		if r.Kind != "event" {
			continue
		}
		text, ok := textOf(r.Payload)
		if !ok {
			continue
		}
		if text == "" {
			text = "（非文字消息，完整结构见 JSONL）"
		}
		fmt.Fprintf(&report, "## #%d · 会话 %d · %s\n\n%s\n\n", r.Seq, r.Conversation, r.Time, text)
	}
	if e = os.WriteFile(filepath.Join(*output, "report.md"), []byte(report.String()), 0600); e != nil {
		return e
	}
	result := map[string]any{"applied": false, "messages": messages, "records": len(p.Records), "memories": len(p.Memories), "legacyFiles": len(p.Legacy), "snapshot": p.Snapshot, "cutoff": p.Cutoff}
	if e = writeJSON(filepath.Join(*output, "summary.json"), result); e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
func apply(db *sql.DB) error {
	b, e := os.ReadFile(*applyPlan)
	if e != nil {
		return e
	}
	var p plan
	if e = json.Unmarshal(b, &p); e != nil {
		return e
	}
	if p.Database != filepath.Base(*dbFile) {
		return errors.New("plan database mismatch")
	}
	if _, e = time.Parse(time.RFC3339Nano, p.Cutoff); e != nil {
		return e
	}
	marks := []chat.IsolationMark{}
	for _, r := range p.Records {
		var raw, stamp, kind string
		var grp int64
		if e = db.QueryRow(`SELECT payload,conversation,created_at,kind FROM history WHERE seq=?`, r.Seq).Scan(&raw, &grp, &stamp, &kind); e != nil {
			return e
		}
		if grp != r.Conversation || stamp != r.Time || kind != r.Kind || hash(raw) != r.SHA || r.Seq > p.Snapshot {
			return errors.New("source differs from frozen plan")
		}
		if kind != "reset" {
			marks = append(marks, chat.IsolationMark{Seq: r.Seq, Run: "historical", Root: r.Root, Reason: "user-authorized historical test boundary " + p.Cutoff})
		}
	}
	ids := []int64{}
	for _, m := range p.Memories {
		var text string
		var rev, grp int64
		if e = db.QueryRow(`SELECT text,revision,conversation FROM agent_memories WHERE id=?`, m.ID).Scan(&text, &rev, &grp); e != nil {
			return e
		}
		if hash(text) != m.SHA || rev != m.Revision || grp != m.Conversation {
			return errors.New("memory changed since export")
		}
		ids = append(ids, m.ID)
	}
	for _, l := range p.Legacy {
		if *legacyDir == "" || filepath.Base(l.Name) != l.Name {
			return errors.New("legacy directory required")
		}
		b, e := os.ReadFile(filepath.Join(*legacyDir, l.Name))
		if e != nil || hash(string(b)) != l.SHA {
			return errors.New("legacy file changed since export")
		}
	}
	if e = chat.IsolateTestRecords(context.Background(), db, marks, ids); e != nil {
		return e
	}
	tx, e := db.Begin()
	if e != nil {
		return e
	}
	boundary, _ := time.Parse(time.RFC3339Nano, p.Cutoff)
	if _, e = tx.Exec(`INSERT INTO historical_test_boundary VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET unix_time=MAX(unix_time,excluded.unix_time),max_seq=MAX(max_seq,excluded.max_seq)`, boundary.Unix(), p.Snapshot); e != nil {
		tx.Rollback()
		return e
	}
	for _, l := range p.Legacy {
		for _, line := range l.Lines {
			if _, e = tx.Exec(`INSERT OR IGNORE INTO legacy_test_memory VALUES(?,?,'user-authorized historical test boundary')`, l.Conversation, line); e != nil {
				tx.Rollback()
				return e
			}
		}
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	bot, e := strconv.ParseInt(strings.TrimSuffix(p.Database, ".sqlite"), 10, 64)
	if e != nil {
		return e
	}
	if e = chat.CopyTestArchive(context.Background(), db, filepath.Join(filepath.Dir(*dbFile), "testing", "historical"), bot); e != nil {
		return e
	}
	result := map[string]any{"applied": true, "records": len(marks), "memories": len(ids), "legacyFiles": len(p.Legacy), "snapshot": p.Snapshot, "cutoff": p.Cutoff, "time": time.Now().UTC().Format(time.RFC3339Nano)}
	if e = writeJSON(filepath.Join(*output, "applied.json"), result); e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
