package localadmin

import (
	core "github.com/FloatTech/ZeroBot-Plugin/internal/imagegen"
	"strings"
	"testing"
)

func TestImageSettingsAuthAndCSRF(t *testing.T) {
	s := newTestServer(t)
	calls := 0
	s.cfg.ImageGen = func(u *core.Update, b core.Backup) (core.Snapshot, error) {
		calls++
		return core.Snapshot{Revision: strings.Repeat("a", 64), Mode: "openai", KeyConfigured: true}, nil
	}
	expectStatus(t, request(s, "GET", "/api/ai-image", nil, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	expectStatus(t, request(s, "GET", "/api/ai-image", nil, cookie, ""), 200)
	body := map[string]any{"revision": strings.Repeat("a", 64)}
	expectStatus(t, request(s, "POST", "/api/ai-image", body, cookie, "bad"), 403)
	expectStatus(t, request(s, "POST", "/api/ai-image", map[string]any{}, cookie, csrf), 400)
	expectStatus(t, request(s, "POST", "/api/ai-image", body, cookie, csrf), 200)
	if calls != 2 {
		t.Fatal(calls)
	}
}
