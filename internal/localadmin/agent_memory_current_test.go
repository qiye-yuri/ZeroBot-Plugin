package localadmin

import (
	"encoding/json"
	"testing"

	"github.com/FloatTech/zbputils/chat"
)

func TestCurrentMemoryAPIScopeAndValidation(t *testing.T) {
	s := newTestServer(t)
	store := &fakeAgentMemory{}
	s.cfg.AgentMemory = func(int64) (AgentMemoryStore, error) { return store, nil }
	path := "/api/agent/memories/current?botId=12345678&type=private&id=10&memoryId=9007199254740993"
	expectStatus(t, request(s, "GET", path, nil, nil, ""), 401)
	cookie, _ := enroll(t, s)
	for _, suffix := range []string{"&after=1", "&state=all", "&revision=1", "&before=1", "&source=admin"} {
		expectStatus(t, request(s, "GET", path+suffix, nil, cookie, ""), 400)
	}
	for _, id := range []string{"", "0", "01", "-1", "9223372036854775808"} {
		expectStatus(t, request(s, "GET", "/api/agent/memories/current?botId=12345678&type=group&id=10&memoryId="+id, nil, cookie, ""), 400)
	}
	expectStatus(t, request(s, "GET", "/api/agent/memories/current?botId=999&type=group&id=10&memoryId=1", nil, cookie, ""), 404)
	if store.calls != 0 {
		t.Fatal("invalid lookup reached store")
	}
	w := request(s, "GET", path, nil, cookie, "")
	expectStatus(t, w, 200)
	var view chat.MemoryView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || len(view.Items) != 1 || view.Items[0].ID != "9007199254740993" || view.Items[0].Active || store.grp != -10 || store.currentID != 9007199254740993 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Body.String(), err)
	}
	store.fail = true
	expectStatus(t, request(s, "GET", path, nil, cookie, ""), 503)
}
