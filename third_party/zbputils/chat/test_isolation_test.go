package chat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func TestTestScopeRoutesMemoryAndConcurrentHistory(t *testing.T) {
	a, _ := memoryFixture(t)
	h := a.history
	normal, e := h.appendWithSequence(10, "event", historyEvent(10, "DAILY_ONLY"))
	if e != nil {
		t.Fatal(e)
	}
	if e = a.SaveFromSequence(10, "DAILY_MEMORY", normal); e != nil {
		t.Fatal(e)
	}
	source, e := h.appendWithSequence(10, "event", historyEvent(10, "TEST_ONLY"), "batch1")
	if e != nil {
		t.Fatal(e)
	}
	run, e := h.tasks.BeginFrom(10, source)
	if e != nil {
		t.Fatal(e)
	}
	if run.info.TestRun != "batch1" {
		t.Fatal("scope not captured")
	}
	if body := historyBody(t, h, 10); strings.Contains(body, "DAILY_ONLY") || !strings.Contains(body, "TEST_ONLY") {
		t.Fatal("mixed history", body)
	}
	if lines, e := a.LoadChecked(10); e != nil || len(lines) != 0 {
		t.Fatal("daily memory reached test", lines, e)
	}
	saved, e := a.SaveLayered(10, "TEST_MEMORY", source, MemorySpec{Kind: "personal", Subject: 123, Attribute: "test.preference"}, false)
	if e != nil || saved.Memory == nil {
		t.Fatal(saved, e)
	}
	child, e := h.testMemory("batch1", 10, source)
	if e != nil {
		t.Fatal(e)
	}
	if child.history.root != filepath.Join(h.root, "testing", "batch1") {
		t.Fatal("test database not separate")
	}
	var count int
	if e = h.db.QueryRow(`SELECT COUNT(*) FROM agent_memories`).Scan(&count); e != nil || count != 1 {
		t.Fatal("test memory persisted in daily db", count, e)
	}
	lines, e := a.LoadChecked(10)
	if e != nil || len(lines) != 1 || !strings.Contains(lines[0], "TEST_MEMORY") {
		t.Fatal("test recall failed", lines, e)
	}
	if e = h.Append(10, "request", &zero.APIRequest{Action: "send_group_msg", Params: zero.Params{"message": "TEST_REPLY"}}); e != nil {
		t.Fatal(e)
	}
	if e = h.Append(10, "response", &goba.APIResponse{Status: "ok", Data: []byte(`{"message_id":12345}`)}); e != nil {
		t.Fatal(e)
	}
	if e = h.Append(10, "event", historyEvent(10, "CONCURRENT_DAILY")); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(historyBody(t, h, 10), "CONCURRENT_DAILY") {
		t.Fatal("normal arrival contaminated active test")
	}
	if e = run.Finish(nil); e != nil {
		t.Fatal(e)
	}
	body := historyBody(t, h, 10)
	if strings.Contains(body, "TEST_") || !strings.Contains(body, "CONCURRENT_DAILY") {
		t.Fatal("test output in daily", body)
	}
	lines, e = a.LoadChecked(10)
	if e != nil || len(lines) != 1 || strings.Contains(lines[0], "TEST_MEMORY") {
		t.Fatal("test memory returned to daily", lines, e)
	}
	q := ArchiveQuery{Conversation: 10, EventsOnly: true, Search: true, Keyword: "TEST_ONLY"}
	page, e := readArchive(context.Background(), h.root, a.bot, q, historyConfig)
	if e != nil || len(page.Entries) != 0 {
		t.Fatal("normal search returned test", page, e)
	}
	q.TestRun = "batch1"
	page, e = readArchive(context.Background(), h.root, a.bot, q, historyConfig)
	if e != nil || len(page.Entries) != 1 {
		t.Fatal("test search failed", page, e)
	}
	q.TestRun = "batch2"
	page, e = readArchive(context.Background(), h.root, a.bot, q, historyConfig)
	if e != nil || len(page.Entries) != 0 {
		t.Fatal("different test batch leaked", page, e)
	}
}

func TestTestControlsRequireOwnerAndPersist(t *testing.T) {
	a, _ := memoryFixture(t)
	h := a.history
	run, reply, e := h.prepareTestEvent(10, "/测试会话 开始 batch1", false)
	if e != nil || run != "" || reply == "" {
		t.Fatal(run, reply, e)
	}
	run, _, e = h.prepareTestEvent(10, "/测试会话 开始 batch1", true)
	if e != nil || run != "batch1" {
		t.Fatal(run, e)
	}
	run, _, e = h.prepareTestEvent(10, "ordinary text", false)
	if e != nil || run != "batch1" {
		t.Fatal("scope not persisted", run, e)
	}
	run, _, e = h.prepareTestEvent(20, "ordinary text", true)
	if e != nil || run != "" {
		t.Fatal("scope leak", run, e)
	}
	run, _, e = h.prepareTestEvent(10, "/测试会话 结束", true)
	if e != nil || run != "batch1" {
		t.Fatal(run, e)
	}
	run, _, e = h.prepareTestEvent(10, "ordinary text", false)
	if e != nil || run != "" {
		t.Fatal("scope not ended", run, e)
	}
	run, _, e = h.prepareTestEvent(10, "/测试消息 oneoff some content", true)
	if e != nil || run != "oneoff" {
		t.Fatal(run, e)
	}
	run, _, e = h.prepareTestEvent(10, "ordinary text", true)
	if e != nil || run != "" {
		t.Fatal("one-off persisted", run, e)
	}
}

func TestRetroactiveIsolationInvalidatesSummaryAndMemory(t *testing.T) {
	a, _ := memoryFixture(t)
	h := a.history
	seq, e := h.appendWithSequence(10, "event", historyEvent(10, "OLD_TEST"))
	if e != nil {
		t.Fatal(e)
	}
	if e = a.SaveFromSequence(10, "OLD_TEST_MEMORY", seq); e != nil {
		t.Fatal(e)
	}
	if _, e = h.db.Exec(`INSERT INTO agent_summaries VALUES(10,0,?,'OLD_TEST_SUMMARY','now')`, seq); e != nil {
		t.Fatal(e)
	}
	if e = IsolateTestRecords(context.Background(), h.db, []IsolationMark{{seq, "historical", seq, "explicit test evidence"}}, nil); e != nil {
		t.Fatal(e)
	}
	if body := historyBody(t, h, 10); strings.Contains(body, "OLD_TEST") {
		t.Fatal("old test still injected", body)
	}
	if lines, e := a.LoadChecked(10); e != nil || len(lines) != 0 {
		t.Fatal("source-linked memory not isolated", lines, e)
	}
	if page, e := a.SearchEnabled(context.Background(), 10, "OLD_TEST", 5); e != nil || len(page.Memories) != 0 {
		t.Fatal("test memory searchable", page, e)
	}
	if page, e := a.ReadEnabled(context.Background(), 10, 1, 1); e != nil || len(page.Memories) != 0 {
		t.Fatal("test memory readable", page, e)
	}
	var n int
	if e = h.db.QueryRow(`SELECT COUNT(*) FROM quarantined_summaries`).Scan(&n); e != nil || n != 1 {
		t.Fatal("summary evidence lost", n, e)
	}
	if e = h.db.QueryRow(`SELECT COUNT(*) FROM history`).Scan(&n); e != nil || n != 1 {
		t.Fatal("archive deleted", n, e)
	}
}

func TestIsolationRejectsInFlightSummary(t *testing.T) {
	h := summaryFixture(t)
	started := make(chan struct{})
	release := make(chan struct{})
	h.scheduleSummary(10, func(SummaryRequest) (string, error) { close(started); <-release; return "STALE_TEST_SUMMARY", nil })
	<-started
	if e := IsolateTestRecords(context.Background(), h.db, []IsolationMark{{1, "historical", 1, "reviewed"}}, nil); e != nil {
		t.Fatal(e)
	}
	close(release)
	waitSummary(t, h)
	var n int
	if e := h.db.QueryRow(`SELECT COUNT(*) FROM agent_summaries`).Scan(&n); e != nil || n != 0 {
		t.Fatal("late tainted summary committed", n, e)
	}
	h.mu.Lock()
	delete(h.summaryRetry, 10)
	h.mu.Unlock()
	next := make(chan string, 1)
	h.scheduleSummary(10, func(r SummaryRequest) (string, error) { next <- r.Records; return "CLEAN", nil })
	input := <-next
	waitSummary(t, h)
	if strings.Contains(input, "event-00") {
		t.Fatal("isolated source reached summary")
	}
}

func TestLegacyIsolationSurvivesMigration(t *testing.T) {
	a, _ := memoryFixture(t)
	if e := os.WriteFile(a.legacyPath(10), []byte("OLD_TEST\nDAILY_FACT\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := a.history.db.Exec(`INSERT INTO legacy_test_memory VALUES(10,'OLD_TEST','reviewed')`); e != nil {
		t.Fatal(e)
	}
	lines, e := a.LoadChecked(10)
	if e != nil || strings.Contains(strings.Join(lines, "\n"), "OLD_TEST") || !strings.Contains(strings.Join(lines, "\n"), "DAILY_FACT") {
		t.Fatal(lines, e)
	}
	p, e := a.Preview(context.Background(), 10)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.Migrate(context.Background(), 10, p.Digest); e != nil {
		t.Fatal(e)
	}
	lines, e = a.LoadChecked(10)
	if e != nil || len(lines) != 1 || !strings.Contains(lines[0], "DAILY_FACT") {
		t.Fatal("migration reintroduced test", lines, e)
	}
}

func TestReapplyingIsolationPreservesLaterCleanSummary(t *testing.T) {
	a, _ := memoryFixture(t)
	h := a.history
	seq, e := h.appendWithSequence(10, "event", historyEvent(10, "old"))
	if e != nil {
		t.Fatal(e)
	}
	marks := []IsolationMark{{seq, "historical", seq, "reviewed"}}
	if e = IsolateTestRecords(context.Background(), h.db, marks, nil); e != nil {
		t.Fatal(e)
	}
	if _, e = h.db.Exec(`INSERT INTO agent_summaries VALUES(10,0,?,'CLEAN_LATER','now')`, seq); e != nil {
		t.Fatal(e)
	}
	if e = IsolateTestRecords(context.Background(), h.db, marks, nil); e != nil {
		t.Fatal(e)
	}
	var text string
	if e = h.db.QueryRow(`SELECT text FROM agent_summaries`).Scan(&text); e != nil || text != "CLEAN_LATER" {
		t.Fatal(text, e)
	}
}

func TestCopyTestArchiveKeepsNormalDataOut(t *testing.T) {
	a, _ := memoryFixture(t)
	h := a.history
	old, e := h.appendWithSequence(10, "event", historyEvent(10, "TEST_ARCHIVE"))
	if e != nil {
		t.Fatal(e)
	}
	if e = a.SaveFromSequence(10, "TEST_MEMORY", old); e != nil {
		t.Fatal(e)
	}
	if _, e = h.appendWithSequence(20, "event", historyEvent(20, "PRIVATE_DAILY")); e != nil {
		t.Fatal(e)
	}
	if e = a.Save(20, "PRIVATE_MEMORY"); e != nil {
		t.Fatal(e)
	}
	if e = IsolateTestRecords(context.Background(), h.db, []IsolationMark{{old, "historical", old, "reviewed"}}, nil); e != nil {
		t.Fatal(e)
	}
	root := filepath.Join(t.TempDir(), "testing")
	if e = CopyTestArchive(context.Background(), h.db, root, a.bot); e != nil {
		t.Fatal(e)
	}
	if e = CopyTestArchive(context.Background(), h.db, root, a.bot); e != nil {
		t.Fatal(e)
	}
	copy, e := openAgentHistory(root, a.bot, 900000, historyConfig)
	if e != nil {
		t.Fatal(e)
	}
	defer copy.Close()
	var count int
	if e = copy.db.QueryRow(`SELECT COUNT(*) FROM history`).Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
	if e = copy.db.QueryRow(`SELECT COUNT(*) FROM agent_memories`).Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
}
