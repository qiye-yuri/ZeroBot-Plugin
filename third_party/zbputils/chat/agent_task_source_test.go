package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"testing"

	"github.com/FloatTech/zbputils/control"
	"github.com/RomiChan/syncx"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func TestAgentTaskSourceScopePersistenceAndUnknown(t *testing.T) {
	h := taskFixture(t)
	first, e := h.appendWithSequence(10, "event", historyEvent(10, "first same message ID"))
	if e != nil {
		t.Fatal(e)
	}
	second, e := h.appendWithSequence(10, "event", historyEvent(10, "second same message ID"))
	if e != nil || second <= first {
		t.Fatal(e, first, second)
	}
	task, e := h.tasks.BeginFrom(10, first)
	if e != nil {
		t.Fatal(e)
	}
	if task.info.SourceSeq != strconv.FormatInt(first, 10) {
		t.Fatal("linked later repeated message ID", task.info)
	}
	if e = task.Finish(nil); e != nil {
		t.Fatal(e)
	}
	v, e := h.tasks.Read(context.Background(), 10, task.info.ID)
	if e != nil || v.SourceSeq != task.info.SourceSeq {
		t.Fatal(e, v)
	}
	for _, source := range []int64{first, second} {
		if _, e = h.tasks.BeginFrom(-10, source); !errors.Is(e, ErrAgentTaskConflict) {
			t.Fatal("cross-scope source accepted", e)
		}
	}
	request, e := h.appendWithSequence(10, "response", historyEvent(10, "not a trigger"))
	if e != nil {
		t.Fatal(e)
	}
	for _, source := range []int64{-1, request, 999999} {
		if _, e = h.tasks.BeginFrom(10, source); !errors.Is(e, ErrAgentTaskConflict) {
			t.Fatal("invalid source accepted", e)
		}
	}
	if e = h.reset(); e != nil {
		t.Fatal(e)
	}
	task, e = h.tasks.BeginFrom(10, second)
	if e != nil {
		t.Fatal("reset invalidated real trigger", e)
	}
	if e = task.Action("get_group_info"); e != nil {
		t.Fatal(e)
	}
	recovered, e := newTaskStore(h.db, "source-restart")
	if e != nil {
		t.Fatal(e)
	}
	v, e = recovered.Read(context.Background(), 10, task.info.ID)
	if e != nil || v.SourceSeq != strconv.FormatInt(second, 10) || v.Status != "interrupted" {
		t.Fatal(e, v)
	}
	unknown, e := recovered.Begin(-10)
	if e != nil || unknown.info.SourceSeq != "0" {
		t.Fatal(e, unknown)
	}
	if e = unknown.Finish(nil); e != nil {
		t.Fatal(e)
	}
	// Existing payloads have no source field and must not receive a guessed one.
	raw, _ := json.Marshal(unknown.info)
	var old map[string]any
	json.Unmarshal(raw, &old)
	delete(old, "sourceSeq")
	raw, _ = json.Marshal(old)
	if _, e = h.db.Exec(`UPDATE agent_tasks SET payload=? WHERE id=?`, string(raw), unknown.info.ID); e != nil {
		t.Fatal(e)
	}
	v, e = recovered.Read(context.Background(), -10, unknown.info.ID)
	if e != nil || v.SourceSeq != "" {
		t.Fatal("legacy source invented", e, v)
	}
	if _, e = h.db.Exec(`DROP TABLE history`); e != nil {
		t.Fatal(e)
	}
	if _, e = recovered.BeginFrom(20, 1); !errors.Is(e, ErrAgentTaskStore) {
		t.Fatal("source lookup failure hidden", e)
	}
}

func TestAgentEventExactSequenceConcurrencyAndBinding(t *testing.T) {
	h := taskFixture(t)
	// Exercise a sequence larger than JavaScript's exact integer range.
	if _, e := h.db.Exec(`INSERT INTO history(seq,conversation,kind,payload,created_at) VALUES(9007199254740993,10,'event','{}','fixture')`); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			text := "event " + strconv.Itoa(i)
			seq, e := h.appendWithSequence(10, "event", historyEvent(10, text))
			if e != nil {
				t.Error(e)
				return
			}
			var raw string
			if e = h.db.QueryRow(`SELECT payload FROM history WHERE seq=?`, seq).Scan(&raw); e != nil {
				t.Error(e)
				return
			}
			var event struct{ Message json.RawMessage }
			if json.Unmarshal([]byte(raw), &event) != nil {
				t.Error("invalid event")
				return
			}
			var segments []struct{ Data map[string]string }
			json.Unmarshal(event.Message, &segments)
			if len(segments) != 1 || segments[0].Data["text"] != text || seq <= 9007199254740993 {
				t.Error("sequence belongs to another insert", seq)
			}
		}(i)
	}
	wg.Wait()
	seq, e := h.appendWithSequence(10, "event", historyEvent(10, "last"))
	if e != nil {
		t.Fatal(e)
	}
	task, e := h.tasks.BeginFrom(10, seq)
	if e != nil || task.info.SourceSeq != strconv.FormatInt(seq, 10) {
		t.Fatal("sequence precision lost", e, task)
	}
	ctx := &zero.Ctx{Event: &zero.Event{PostType: "message", SelfID: 456, GroupID: 10, UserID: 123}}
	mp := &syncx.Map[string, any]{}
	ctx.State = map[string]any{control.StateKeySyncxState: mp}
	mp.Store(stateKeyAgentEventSource, agentEventSource{event: ctx.Event, bot: 456, conversation: 10, sequence: seq})
	if AgentEventSequence(ctx) != seq {
		t.Fatal("bound source lost")
	}
	copyEvent := *ctx.Event
	other := &zero.Ctx{Event: &copyEvent, State: ctx.State}
	if AgentEventSequence(other) != 0 {
		t.Fatal("binding reused for another event")
	}
	ctx.Event.SelfID = 789
	if AgentEventSequence(ctx) != 0 {
		t.Fatal("bot override accepted")
	}
	ctx.Event.SelfID = 456
	ctx.Event.GroupID = 0
	if AgentEventSequence(ctx) != 0 {
		t.Fatal("private scope override accepted")
	}
	mp.Store(stateKeyAgentEventSource, agentEventSource{event: ctx.Event, bot: 456, conversation: -123, sequence: seq})
	if AgentEventSequence(ctx) != seq {
		t.Fatal("private binding not supported")
	}
	ctx.Event.PostType = "notice"
	if AgentEventSequence(ctx) != 0 {
		t.Fatal("non-message task source accepted")
	}
	if AgentEventSequence(nil) != 0 {
		t.Fatal("nil context source")
	}
	if _, e = h.db.Exec(`CREATE TRIGGER source_failure BEFORE INSERT ON history BEGIN SELECT RAISE(ABORT,'synthetic'); END`); e != nil {
		t.Fatal(e)
	}
	seq, e = h.appendWithSequence(10, "event", historyEvent(10, "failed"))
	if seq != 0 || !errors.Is(e, errAgentHistory) {
		t.Fatal("failed insert gained a sequence", seq, e)
	}
}
