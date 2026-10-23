package localadmin

import (
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
