package localadmin

import (
	"strings"
	"testing"
)

func TestCTBEndpointAuthorization(t *testing.T) {
	s := newTestServer(t)
	expectStatus(t, request(s, "GET", "/api/ctb", nil, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	expectStatus(t, request(s, "GET", "/api/ctb", nil, cookie, ""), 200)
	expectStatus(t, request(s, "POST", "/api/ctb", map[string]any{"revision": strings.Repeat("a", 64)}, cookie, "bad"), 403)
	expectStatus(t, request(s, "POST", "/api/ctb", map[string]any{}, cookie, csrf), 400)
}
