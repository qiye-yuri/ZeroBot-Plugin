package localadmin

import (
	"strings"
	"testing"
)

func TestMemoryRevisionAPI(t *testing.T) {
	s := newTestServer(t)
	f := &fakeAgentMemory{}
	s.cfg.AgentMemory = func(int64) (AgentMemoryStore, error) { return f, nil }
	base := "/api/agent/memories/revision?botId=12345678&type=private&id=10&memoryId=9007199254740993&revision=9007199254740994"
	expectStatus(t, request(s, "GET", base, nil, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	for _, key := range []string{"memoryId", "revision"} {
		for _, bad := range []string{"", "0", "01", "-1", "9223372036854775808"} {
			value := "9007199254740993"
			if key == "revision" {
				value = "9007199254740994"
			}
			path := strings.Replace(base, key+"="+value, key+"="+bad, 1)
			expectStatus(t, request(s, "GET", path, nil, cookie, ""), 400)
		}
	}
	for _, tail := range []string{"&after=1", "&after=", "&state=all", "&order=desc", "&before=1", "&source=admin", "&from=", "&until="} {
		expectStatus(t, request(s, "GET", base+tail, nil, cookie, ""), 400)
	}
	expectStatus(t, request(s, "GET", strings.Replace(base, "12345678", "999", 1), nil, cookie, ""), 404)
	expectStatus(t, request(s, "POST", "/api/agent/memories/revision", map[string]any{}, cookie, csrf), 405)
	if f.calls != 0 {
		t.Fatal("invalid request reached store", f.calls)
	}
	for _, method := range []string{"GET", "HEAD"} {
		w := request(s, method, base, nil, cookie, "")
		expectStatus(t, w, 200)
		if f.viewRevision != [3]int64{-10, 9007199254740993, 9007199254740994} || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(f, w.Header())
		}
	}
	f.fail = true
	w := request(s, "GET", base, nil, cookie, "")
	expectStatus(t, w, 503)
	if strings.Contains(w.Body.String(), `"items"`) {
		t.Fatal("failed read looked missing")
	}
}
