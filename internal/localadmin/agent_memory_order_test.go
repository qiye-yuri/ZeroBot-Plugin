package localadmin

import (
	"testing"
)

func TestMemoryVersionOrderAPI(t *testing.T) {
	s := newTestServer(t)
	f := &fakeAgentMemory{}
	s.cfg.AgentMemory = func(int64) (AgentMemoryStore, error) { return f, nil }
	cookie, csrf := enroll(t, s)
	base := "/api/agent/memories?botId=12345678&type=private&id=10&memoryId=9007199254740993"
	expectStatus(t, request(s, "GET", base+"&order=desc", nil, nil, ""), 401)
	for _, suffix := range []string{"&order=invalid", "&order=DESC", "&order=desc&state=active", "&order=desc&after=-1"} {
		expectStatus(t, request(s, "GET", base+suffix, nil, cookie, ""), 400)
	}
	for _, path := range []string{"/api/agent/memories?botId=12345678&type=private&id=10&order=desc", "/api/agent/memories/current?botId=12345678&type=private&id=10&memoryId=1&order=desc", "/api/agent/memories/read?botId=12345678&type=private&id=10&memoryId=1&revision=1&order=asc"} {
		expectStatus(t, request(s, "GET", path, nil, cookie, ""), 400)
	}
	body := map[string]any{"botId": "12345678", "type": "private", "id": "10", "keyword": "needle"}
	expectStatus(t, request(s, "POST", "/api/agent/memories/search?order=desc", body, cookie, csrf), 400)
	if f.calls != 0 {
		t.Fatal("invalid order reached memory storage", f.calls)
	}
	w := request(s, "GET", base+"&order=desc&after=9007199254740994", nil, cookie, "")
	expectStatus(t, w, 200)
	if f.newest != [3]int64{-10, 9007199254740993, 9007199254740994} || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(f.newest, w)
	}
	f.newest = [3]int64{}
	for _, suffix := range []string{"", "&order=", "&order=asc"} {
		expectStatus(t, request(s, "GET", base+suffix, nil, cookie, ""), 200)
		if f.newest != [3]int64{} {
			t.Fatal("old default switched ordering")
		}
	}
	f.fail = true
	expectStatus(t, request(s, "GET", base+"&order=desc", nil, cookie, ""), 503)
}
