package localadmin

import (
	"fmt"
	core "github.com/FloatTech/ZeroBot-Plugin/internal/updatewatch"
	"strings"
	"testing"
)

func TestUpdateWatchAuthorizationAndSave(t *testing.T) {
	s := newTestServer(t)
	calls := 0
	s.cfg.UpdateWatch = func(rev string, c *core.Config) (core.AdminSnapshot, error) {
		calls++
		return core.AdminSnapshot{Config: core.DefaultConfig(), Revision: strings.Repeat("a", 64)}, nil
	}
	expectStatus(t, request(s, "GET", "/api/updatewatch", nil, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	expectStatus(t, request(s, "GET", "/api/updatewatch", nil, cookie, ""), 200)
	body := map[string]any{"revision": strings.Repeat("a", 64), "config": core.DefaultConfig()}
	expectStatus(t, request(s, "POST", "/api/updatewatch", body, cookie, "wrong"), 403)
	expectStatus(t, request(s, "POST", "/api/updatewatch", map[string]any{}, cookie, csrf), 400)
	expectStatus(t, request(s, "POST", "/api/updatewatch", body, cookie, csrf), 200)
	if calls != 2 {
		t.Fatal("unauthorized or invalid request reached plugin")
	}
}

func TestUpdateWatchRuntimeReadDelegationAndBusy(t *testing.T) {
	s := newTestServer(t)
	cookie, _ := enroll(t, s)
	calls := 0
	busy := true
	s.cfg.UpdateWatch = func(rev string, c *core.Config) (core.AdminSnapshot, error) {
		calls++
		if rev != "" || c != nil {
			t.Fatal("GET passed write arguments")
		}
		if busy {
			return core.AdminSnapshot{}, fmt.Errorf("正在检查来源或模型，请稍后重试")
		}
		return core.AdminSnapshot{Revision: strings.Repeat("a", 64), Pending: 2, Baseline: 3}, nil
	}
	expectStatus(t, request(s, "GET", "/api/updatewatch", nil, nil, ""), 401)
	if calls != 0 {
		t.Fatal("unauthenticated read reached plugin")
	}
	expectStatus(t, request(s, "GET", "/api/updatewatch", nil, cookie, ""), 409)
	busy = false
	w := request(s, "GET", "/api/updatewatch", nil, cookie, "")
	expectStatus(t, w, 200)
	if calls != 2 || !strings.Contains(w.Body.String(), "\"pending\":2") {
		t.Fatal("retry snapshot missing")
	}
}
