package chat

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestArchiveReadOnlySnapshotIsolationAndReset(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	read := func(bot int64, q ArchiveQuery) ArchivePage {
		t.Helper()
		p, e := readArchive(ctx, root, bot, q, historyConfig)
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	if p := read(456, ArchiveQuery{}); p.Available {
		t.Fatal("missing DB reported available")
	}
	files, _ := os.ReadDir(root)
	if len(files) != 0 {
		t.Fatal("read created files")
	}
	s, e := openAgentHistory(root, 456, 900000, historyConfig)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for i := 0; i < 61; i++ {
		if e = s.Append(10, "event", historyEvent(10, fmt.Sprintf("中文记录-%03d", i))); e != nil {
			t.Fatal(e)
		}
	}
	if e = s.Append(-10, "event", historyEvent(0, "private-only")); e != nil {
		t.Fatal(e)
	}
	p := read(456, ArchiveQuery{Conversation: 10})
	if len(p.Entries) != 50 || !p.More {
		t.Fatalf("page: %+v", p)
	}
	if e = s.Append(10, "event", historyEvent(10, "new-after-snapshot")); e != nil {
		t.Fatal(e)
	}
	second := read(456, ArchiveQuery{Conversation: 10, Snapshot: p.Snapshot, Before: p.Next})
	if len(second.Entries) != 11 || second.More {
		t.Fatalf("second: %+v", second)
	}
	for _, entry := range append(p.Entries, second.Entries...) {
		if strings.Contains(entry.Text, "private-only") || strings.Contains(entry.Text, "new-after") {
			t.Fatal("scope/snapshot leaked")
		}
	}
	list := read(456, ArchiveQuery{})
	if len(list.Conversations) != 2 || list.Conversations[0].Type != "private" || list.Conversations[1].Type != "group" {
		t.Fatalf("scopes: %+v", list)
	}
	other, e := openAgentHistory(root, 789, 900000, historyConfig)
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	if p = read(789, ArchiveQuery{Conversation: 10}); len(p.Entries) != 0 {
		t.Fatal("bot leaked")
	}
	if e = s.reset(); e != nil {
		t.Fatal(e)
	}
	if p = read(456, ArchiveQuery{Conversation: 10}); len(p.Entries) != 0 {
		t.Fatal("reset boundary ignored")
	}
	if p = read(456, ArchiveQuery{Conversation: 10, IncludeResetHistory: true}); len(p.Entries) == 0 {
		t.Fatal("archive inaccessible")
	}
	// Cancellation fails rather than reporting an empty archive.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = readArchive(cancelled, root, 456, ArchiveQuery{Conversation: 10}, historyConfig); e == nil {
		t.Fatal("cancellation ignored")
	}
	if e = os.WriteFile(filepath.Join(root, "999.sqlite"), []byte("broken"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = readArchive(ctx, root, 999, ArchiveQuery{}, historyConfig); e == nil {
		t.Fatal("corruption reported empty")
	}
}

func TestArchiveExactSourceScopeResetAndBounds(t *testing.T) {
	root := t.TempDir()
	h, e := openAgentHistory(root, 456, 900000, historyConfig)
	if e != nil {
		t.Fatal(e)
	}
	defer h.Close()
	for _, text := range []string{"source synthetic-agent-key <img src=x>", strings.Repeat("中文", 3000), strings.Repeat("超限", 15000)} {
		if e = h.Append(10, "event", historyEvent(10, text)); e != nil {
			t.Fatal(e)
		}
	}
	if e = h.reset(); e != nil {
		t.Fatal(e)
	}
	read := func(bot, grp, seq int64, archived bool) ArchivePage {
		t.Helper()
		p, e := readArchive(context.Background(), root, bot, ArchiveQuery{Conversation: grp, Sequence: seq, IncludeResetHistory: archived}, historyConfig)
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	if p := read(456, 10, 1, false); len(p.Entries) != 0 {
		t.Fatal("reset boundary ignored")
	}
	p := read(456, 10, 1, true)
	if len(p.Entries) != 1 || p.Entries[0].Seq != "1" || p.More || p.Scanned != 1 || strings.Contains(p.Entries[0].Text, "synthetic-agent-key") {
		t.Fatal(p)
	}
	if p = read(456, -10, 1, true); len(p.Entries) != 0 {
		t.Fatal("cross-scope source leaked")
	}
	if p = read(789, 10, 1, true); p.Available || len(p.Entries) != 0 {
		t.Fatal("cross-bot source leaked")
	}
	if _, e = os.Stat(filepath.Join(root, "789.sqlite")); !os.IsNotExist(e) {
		t.Fatal("source read created DB")
	}
	p = read(456, 10, 2, true)
	if len(p.Entries) != 1 || !p.Entries[0].Truncated || len(p.Entries[0].Text) > 8000 || !utf8.ValidString(p.Entries[0].Text) {
		t.Fatal("source excerpt bound", p)
	}
	p = read(456, 10, 3, true)
	if len(p.Entries) != 1 || !strings.Contains(p.Entries[0].Text, "未读取正文") {
		t.Fatal("oversized source read", p)
	}
	if p = read(456, 10, 999, true); len(p.Entries) != 0 || !p.Available {
		t.Fatal("missing source not empty")
	}
	for _, q := range []ArchiveQuery{{Sequence: 1}, {Conversation: 10, Sequence: -1}, {Conversation: 10, Sequence: 1, Search: true, Keyword: "x"}, {Conversation: 10, Sequence: 1, Before: 2}} {
		if _, e = readArchive(context.Background(), root, 456, q, historyConfig); e == nil {
			t.Fatal("ambiguous query accepted", q)
		}
	}
}

func TestArchiveLargeDatabaseAndConversationPagination(t *testing.T) {
	root := t.TempDir()
	s, e := openAgentHistory(root, 456, 900000, historyConfig)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	tx, e := s.db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	stmt, e := tx.Prepare("INSERT INTO history(conversation,kind,payload,created_at) VALUES(?,'event',?, 'fixture')")
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 100000; i++ {
		if _, e = stmt.Exec(10, `{"message":"合成大档案"}`); e != nil {
			t.Fatal(e)
		}
	}
	for i := 11; i < 72; i++ {
		if _, e = stmt.Exec(i, `{}`); e != nil {
			t.Fatal(e)
		}
	}
	stmt.Close()
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	started := time.Now()
	p, e := readArchive(context.Background(), root, 456, ArchiveQuery{Conversation: 10, Search: true, Keyword: "不存在"}, historyConfig)
	if e != nil || p.Scanned != 5000 || !p.Partial {
		t.Fatalf("bounded search: %v %+v", e, p)
	}
	t.Logf("100000-record archive bounded search: %s", time.Since(started))
	first, e := readArchive(context.Background(), root, 456, ArchiveQuery{}, historyConfig)
	if e != nil || len(first.Conversations) != 50 || !first.More {
		t.Fatalf("first scopes: %v %+v", e, first)
	}
	second, e := readArchive(context.Background(), root, 456, ArchiveQuery{Snapshot: first.Snapshot, HasAfter: true, AfterConversation: first.Next}, historyConfig)
	if e != nil || len(second.Conversations) != 12 || second.More {
		t.Fatalf("second scopes: %v %+v", e, second)
	}
}

func TestArchiveSearchBudgetsAndRedaction(t *testing.T) {
	root := t.TempDir()
	s, e := openAgentHistory(root, 456, 900000, historyConfig)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	appendEvent := func(text string) {
		t.Helper()
		if e := s.Append(10, "event", historyEvent(10, text)); e != nil {
			t.Fatal(e)
		}
	}
	appendEvent("secret synthetic-agent-key")
	appendEvent("设置AI聊天密钥 synthetic-new-key")
	for i := 0; i < 30; i++ {
		appendEvent("查找中文🙂" + strings.Repeat("中", 2000))
	}
	read := func(word string) ArchivePage {
		t.Helper()
		p, e := readArchive(context.Background(), root, 456, ArchiveQuery{Conversation: 10, Search: true, Keyword: word}, historyConfig)
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	p := read("查找中文🙂")
	n := 0
	for _, item := range p.Entries {
		n += len(item.Text)
		if len(item.Text) > 2000 || !utf8.ValidString(item.Text) {
			t.Fatal("invalid excerpt")
		}
	}
	if n > 16384 || !p.Partial || len(p.Entries) > 20 {
		t.Fatalf("budget %d: %+v", n, p)
	}
	for _, secret := range []string{"synthetic-agent-key", "synthetic-new-key"} {
		if p = read(secret); len(p.Entries) != 0 {
			t.Fatal("secret searchable")
		}
	}
	appendEvent(strings.Repeat("oversized", 10000))
	if p = read("oversized"); !p.Partial || len(p.Entries) != 0 {
		t.Fatal("oversize not skipped")
	}
	if p = read("未读取正文"); !p.Partial || len(p.Entries) != 0 {
		t.Fatal("placeholder matched as historical content")
	}
	// Bulk insert isolates row-bound behavior from fixture encoding overhead.
	tx, e := s.db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 5002; i++ {
		if _, e = tx.Exec("INSERT INTO history(conversation,kind,payload,created_at) VALUES(10,'event','{}','fixture')"); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	if p = read("never-matches"); p.Scanned != 5000 || !p.Partial {
		t.Fatalf("scan limit: %+v", p)
	}
}
