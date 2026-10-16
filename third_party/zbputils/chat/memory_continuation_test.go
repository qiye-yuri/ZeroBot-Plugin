package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func TestMemorySearchContinuationProgressScopeAndCurrentVersion(t *testing.T) {
	a, b := memoryFixture(t)
	ctx := context.Background()
	for _, text := range []string{"needle old", "gap", "needle new"} {
		if err := a.Save(10, text); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.Save(-10, "needle private"); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(10, "needle other bot"); err != nil {
		t.Fatal(err)
	}
	first, err := a.SearchEnabled(ctx, 10, "needle", 1)
	if err != nil || len(first.Memories) != 1 || first.Memories[0].ID != "3" || !first.SearchMore || first.NextBefore != "2" || !first.Partial {
		t.Fatal("must preserve unreturned match after gap", first, err)
	}
	second, err := a.SearchEnabledBefore(ctx, 10, "needle", 1, 2)
	if err != nil || len(second.Memories) != 1 || second.Memories[0].ID != "1" || second.SearchMore || second.NextBefore != "" {
		t.Fatal("continuation skipped old match", second, err)
	}
	if err = a.history.reset(); err != nil {
		t.Fatal(err)
	}
	second, err = a.SearchEnabledBefore(ctx, 10, "needle", 1, 2)
	if err != nil || len(second.Memories) != 1 {
		t.Fatal("chat reset removed memory", second, err)
	}
	if err = a.Update(ctx, 10, 1, 1, "edit", "needle revised"); err != nil {
		t.Fatal(err)
	}
	second, err = a.SearchEnabledBefore(ctx, 10, "needle", 1, 2)
	if err != nil || len(second.Memories) != 1 || second.Memories[0].Revision != "2" || second.Memories[0].Text != "needle revised" {
		t.Fatal("continued query used old revision", second, err)
	}
	stale, err := a.ReadEnabled(ctx, 10, 1, 1)
	if err != nil || len(stale.Memories) != 0 || stale.SearchMore || stale.NextBefore != "" {
		t.Fatal("stale read gained continuation", stale, err)
	}
	if err = a.Update(ctx, 10, 1, 2, "disable", ""); err != nil {
		t.Fatal(err)
	}
	second, err = a.SearchEnabledBefore(ctx, 10, "needle", 1, 2)
	if err != nil || len(second.Memories) != 0 || second.SearchMore || second.NextBefore != "" {
		t.Fatal("continued query exposed disabled memory", second, err)
	}
	if err = os.WriteFile(a.legacyPath(11), []byte("needle legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	legacy, err := a.SearchEnabled(ctx, 11, "needle", 1)
	if err != nil || legacy.Available || legacy.SearchMore || legacy.NextBefore != "" {
		t.Fatal("legacy invented continuation", legacy, err)
	}
}

func TestMemoryContinuationTaskQuotaAndReceiptSurviveReconstruction(t *testing.T) {
	a, _ := memoryFixture(t)
	for _, text := range []string{"needle old", "gap", "needle new"} {
		if err := a.Save(10, text); err != nil {
			t.Fatal(err)
		}
	}
	h, run, ag, search := queryTaskFixture(t)
	bind := func(s *HistorySearch) {
		s.memory = func(ctx context.Context, bot, grp int64, key string, limit int, before int64, source, from, until string) (MemorySearchPage, error) {
			if bot != 456 || grp != 10 {
				t.Fatal("scope lost")
			}
			return a.SearchEnabledBefore(ctx, grp, key, limit, before)
		}
		s.readMemory = func(ctx context.Context, bot, grp, id, revision int64) (MemorySearchPage, error) {
			return a.ReadEnabled(ctx, grp, id, revision)
		}
	}
	bind(search)
	execute := func(s *HistorySearch, req zero.APIRequest) error {
		if err := run.Action(req.Action); err != nil {
			return err
		}
		err := s.Execute(ag, req)
		if resultErr := run.Result(s.Outcome()); resultErr != nil {
			return resultErr
		}
		return err
	}
	readReceipt := func() MemorySearchPage {
		t.Helper()
		var raw string
		var response struct {
			Data MemorySearchPage `json:"data"`
		}
		if err := h.db.QueryRow("SELECT payload FROM history WHERE kind='response' ORDER BY seq DESC LIMIT 1").Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(raw), &response); err != nil {
			t.Fatal(err)
		}
		return response.Data
	}
	if err := execute(search, zero.APIRequest{Action: goba.SearchMemory, Params: zero.Params{"keyword": "needle", "limit": json.Number("1")}}); err != nil {
		t.Fatal(err)
	}
	first := readReceipt()
	if !first.SearchMore || first.NextBefore != "2" {
		t.Fatal(first)
	}
	other, err := NewTaskHistorySearch(456, 10, run)
	if err != nil {
		t.Fatal(err)
	}
	bind(other)
	if err = execute(other, zero.APIRequest{Action: goba.SearchMemory, Params: zero.Params{"keyword": "needle", "limit": json.Number("1"), "before": first.NextBefore}}); err != nil {
		t.Fatal(err)
	}
	second := readReceipt()
	if len(second.Memories) != 1 || second.Memories[0].ID != "1" || second.SearchMore {
		t.Fatal(second)
	}
	if err = execute(other, zero.APIRequest{Action: goba.ReadMemory, Params: zero.Params{"id": second.Memories[0].ID, "revision": second.Memories[0].Revision}}); err != nil {
		t.Fatal(err)
	}
	read := readReceipt()
	if len(read.Memories) != 1 || read.Memories[0].Text != "needle old" || read.SearchMore || read.NextBefore != "" {
		t.Fatal(read)
	}
	if err = execute(other, zero.APIRequest{Action: goba.SearchMemory, Params: zero.Params{"keyword": "needle"}}); err == nil {
		t.Fatal("continued queries reset quota")
	}
	if err = run.Finish(errors.New("quota")); err != nil {
		t.Fatal(err)
	}
	v, err := h.tasks.Read(context.Background(), 10, run.info.ID)
	if err != nil || v.Queries.Used != 3 || v.Queries.SearchMemory != 2 || v.Queries.ReadMemory != 1 || v.Queries.Successful != 3 || v.Queries.Denied != 1 || v.Status != "failed" {
		t.Fatal(v, err)
	}
}

func TestMemorySearchContinuationEmptyScanAndLargeID(t *testing.T) {
	a, _ := memoryFixture(t)
	tx, err := a.history.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := 0; i < 5002; i++ {
		text := "gap"
		if i == 0 {
			text = "needle old"
		}
		if _, err = tx.Exec(`INSERT INTO agent_memories(id,conversation,text,source,source_seq,active,revision,created_at,updated_at) VALUES(?,10,?,'agent',0,1,1,'fixture','fixture')`, 9007199254740993+int64(i), text); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	page, err := a.SearchEnabled(context.Background(), 10, "needle", 1)
	if err != nil || len(page.Memories) != 0 || page.Scanned != 5000 || !page.SearchMore || page.NextBefore != "9007199254740995" {
		t.Fatal("empty window lacks exact next position", page, err)
	}
	before, _ := strconv.ParseInt(page.NextBefore, 10, 64)
	page, err = a.SearchEnabledBefore(context.Background(), 10, "needle", 1, before)
	if err != nil || len(page.Memories) != 1 || page.Memories[0].ID != "9007199254740993" || page.SearchMore || page.NextBefore != "" {
		t.Fatal("continued empty window lost match", page, err)
	}
}

func TestMemorySearchContinuationByteCapsAndPartialOnly(t *testing.T) {
	for _, kind := range []string{"scan", "results", "oversized", "clipped"} {
		t.Run(kind, func(t *testing.T) {
			a, _ := memoryFixture(t)
			count := 1
			text := "needle " + strings.Repeat("字", 1000)
			switch kind {
			case "scan":
				count = 80
				text = strings.Repeat("x", 60000)
			case "results":
				count = 15
			case "oversized":
				text = "needle " + strings.Repeat("x", 70000)
			}
			tx, err := a.history.db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			for i := 0; i < count; i++ {
				if _, err = tx.Exec(`INSERT INTO agent_memories(conversation,text,source,source_seq,active,revision,created_at,updated_at) VALUES(10,?,'agent',0,1,1,'fixture','fixture')`, text+fmt.Sprint(i)); err != nil {
					t.Fatal(err)
				}
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			page, err := a.SearchEnabled(context.Background(), 10, "needle", 20)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "scan" || kind == "results" {
				if !page.SearchMore || page.NextBefore == "" || !page.Partial {
					t.Fatal("byte cap missing continuation", page)
				}
				before, _ := strconv.ParseInt(page.NextBefore, 10, 64)
				next, err := a.SearchEnabledBefore(context.Background(), 10, "needle", 20, before)
				if err != nil {
					t.Fatal(err)
				}
				if kind == "scan" && page.Scanned+next.Scanned != count {
					t.Fatal("skipped scanned rows")
				}
				if kind == "results" {
					seen := map[string]bool{}
					for _, item := range append(page.Memories, next.Memories...) {
						if seen[item.ID] {
							t.Fatal("duplicate result")
						}
						seen[item.ID] = true
					}
					if len(seen) != count {
						t.Fatal("unreturned match lost", len(seen))
					}
				}
			} else if !page.Partial || page.SearchMore || page.NextBefore != "" {
				t.Fatal("partial excerpt invented more records", page)
			}
		})
	}
}
