package chat

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func summaryFixture(t *testing.T) *agentHistory {
	t.Helper()
	h, e := openAgentHistory(t.TempDir(), 456, 16384, historyConfig)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { h.Close() })
	for i := 0; i < 24; i++ {
		if e = h.Append(10, "event", historyEvent(10, fmt.Sprintf("event-%02d %s", i, strings.Repeat("内容", 100)))); e != nil {
			t.Fatal(e)
		}
	}
	return h
}
func waitSummary(t *testing.T, h *agentHistory) {
	t.Helper()
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		h.mu.Lock()
		running := h.summarizing[10]
		h.mu.Unlock()
		if !running {
			return
		}
		time.Sleep(time.Millisecond * 5)
	}
	t.Fatal("summary did not finish")
}
func TestSummaryKeepsRecentAndConcurrentMessages(t *testing.T) {
	h := summaryFixture(t)
	started := make(chan SummaryRequest, 1)
	release := make(chan struct{})
	h.scheduleSummary(10, func(r SummaryRequest) (string, error) { started <- r; <-release; return "较早事件已概括", nil })
	var input SummaryRequest
	select {
	case input = <-started:
	case <-time.After(time.Second):
		t.Fatal("not scheduled")
	}
	if !strings.Contains(input.Records, "event-00") || strings.Contains(input.Records, "event-16") {
		t.Fatal("incorrect prefix")
	}
	h.scheduleSummary(10, func(SummaryRequest) (string, error) { t.Error("duplicate summary"); return "", nil })
	if e := h.Append(10, "event", historyEvent(10, "during-summary-new-message")); e != nil {
		t.Fatal(e)
	}
	close(release)
	waitSummary(t, h)
	body := historyBody(t, h, 10)
	for _, want := range []string{"较早事件已概括", "event-16", "event-23", "during-summary-new-message"} {
		if !strings.Contains(body, want) {
			t.Fatal("missing", want)
		}
	}
	if strings.Contains(body, "event-00") {
		t.Fatal("covered prefix repeated")
	}
	var n int
	if e := h.db.QueryRow(`SELECT COUNT(*) FROM history WHERE conversation=10`).Scan(&n); e != nil || n != 25 {
		t.Fatal("archive modified", n, e)
	}
	if strings.Contains(historyBody(t, h, 20), "较早事件") {
		t.Fatal("summary scope leakage")
	}
}
func TestSummaryFailureAndResetKeepArchive(t *testing.T) {
	for _, mode := range []string{"failure", "oversize", "reset"} {
		t.Run(mode, func(t *testing.T) {
			h := summaryFixture(t)
			started := make(chan struct{})
			release := make(chan struct{})
			h.scheduleSummary(10, func(SummaryRequest) (string, error) {
				close(started)
				<-release
				if mode == "failure" {
					return "", errors.New("fixture")
				}
				if mode == "oversize" {
					return strings.Repeat("x", 8193), nil
				}
				return "stale summary", nil
			})
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("not scheduled")
			}
			if mode == "reset" {
				if e := h.reset(); e != nil {
					t.Fatal(e)
				}
			}
			close(release)
			waitSummary(t, h)
			var n int
			if e := h.db.QueryRow(`SELECT COUNT(*) FROM agent_summaries`).Scan(&n); e != nil || n != 0 {
				t.Fatal("invalid result committed", n, e)
			}
			if strings.Contains(historyBody(t, h, 10), "stale summary") {
				t.Fatal("stale injected")
			}
		})
	}
}
func TestSummarySoftThresholdAndRestart(t *testing.T) {
	root := t.TempDir()
	h, e := openAgentHistory(root, 456, 16384, historyConfig)
	if e != nil {
		t.Fatal(e)
	}
	h.scheduleSummary(10, func(SummaryRequest) (string, error) { t.Error("empty summary call"); return "", nil })
	if _, e = h.db.Exec(`INSERT INTO agent_summaries VALUES(10,0,1,'persisted summary','now')`); e != nil {
		t.Fatal(e)
	}
	h.Close()
	h, e = openAgentHistory(root, 456, 16384, historyConfig)
	if e != nil {
		t.Fatal(e)
	}
	defer h.Close()
	if !strings.Contains(historyBody(t, h, 10), "persisted summary") {
		t.Fatal("lost summary after restart")
	}
}
