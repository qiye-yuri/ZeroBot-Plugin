package localadmin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/FloatTech/ZeroBot-Plugin/internal/resourcecheck"
)

func TestResourceCheckAuthorization(t *testing.T) {
	s := newTestServer(t)
	s.cfg.Resources = resourcecheck.New(t.TempDir())
	expectStatus(t, request(s, "GET", "/api/resources", nil, nil, ""), 401)
	expectStatus(t, request(s, "POST", "/api/resources/check", map[string]any{}, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	w := request(s, "GET", "/api/resources", nil, cookie, "")
	expectStatus(t, w, 200)
	if !strings.Contains(w.Body.String(), `"report":null`) {
		t.Fatal("GET initiated check")
	}
	expectStatus(t, request(s, "POST", "/api/resources/check", map[string]any{}, cookie, "wrong"), 403)
	expectStatus(t, request(s, "POST", "/api/resources/check", map[string]any{"path": "../private"}, cookie, csrf), 400)
	if s.cfg.Resources.Read().Report != nil {
		t.Fatal("invalid request initiated check")
	}
	w = request(s, "POST", "/api/resources/check", map[string]any{}, cookie, csrf)
	expectStatus(t, w, 200)
	if !strings.Contains(w.Body.String(), "资源清单缺失") {
		t.Fatal("missing manifest result lost")
	}
	expectStatus(t, request(s, "GET", "/api/resources/check", nil, cookie, ""), 405)
	expectStatus(t, request(s, "GET", "/resources.js", nil, nil, ""), 200)
}

func TestResourceCheckHTTPReadOnlyLifecycle(t *testing.T) {
	root := t.TempDir()
	write := func(name string, data []byte) {
		t.Helper()
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	write("Tarot/tarots.json", []byte(`{"test":{"info":{"imgUrl":"test.png"}}}`))
	write("Tarot/test.png", pngData.Bytes())
	write("Wife/wife.json", []byte(`["test.png"]`))
	write("Wife/wives/test.png", pngData.Bytes())
	write("nwife/private.txt", []byte("unrelated fixture"))
	hashes := func() map[string][32]byte {
		t.Helper()
		m := map[string][32]byte{}
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			m[p] = sha256.Sum256(b)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	before := hashes()
	s := newTestServer(t)
	s.cfg.Resources = resourcecheck.New(root)
	cookie, csrf := enroll(t, s)
	read := func(w *httptest.ResponseRecorder) resourcecheck.Snapshot {
		t.Helper()
		expectStatus(t, w, 200)
		var v resourcecheck.Snapshot
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	first := read(request(s, "GET", "/api/resources", nil, cookie, ""))
	if first.Report != nil {
		t.Fatal("read scanned files")
	}
	first = read(request(s, "POST", "/api/resources/check", map[string]any{}, cookie, csrf))
	if first.Busy || first.Report == nil || len(first.Report.Groups) != 2 {
		t.Fatal("invalid complete report")
	}
	tarot, wife := first.Report.Groups[0], first.Report.Groups[1]
	if tarot.Expected != 2 || tarot.Valid != 1 || tarot.Missing != 1 || wife.Valid != 1 {
		t.Fatalf("unexpected report: %+v", first.Report)
	}
	if !reflect.DeepEqual(before, hashes()) {
		t.Fatal("scan changed resource content or created files")
	}
	if strings.Contains(request(s, "GET", "/api/resources", nil, cookie, "").Body.String(), root) {
		t.Fatal("absolute root leaked")
	}
	// A resource change is visible only after an explicitly requested scan.
	write("Wife/wives/test.png", []byte("invalid image"))
	modified := hashes()
	second := read(request(s, "GET", "/api/resources", nil, cookie, ""))
	if !reflect.DeepEqual(first.Report, second.Report) {
		t.Fatal("GET refreshed cached snapshot")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := httptest.NewRequest("POST", "http://"+s.cfg.Addr+"/api/resources/check", strings.NewReader(`{}`)).WithContext(ctx)
	r.RemoteAddr = "127.0.0.1:54321"
	r.AddCookie(cookie)
	r.Header.Set("Origin", "http://"+s.cfg.Addr)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Local-Request", "1")
	r.Header.Set("X-CSRF-Token", csrf)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	expectStatus(t, w, 409)
	second = read(request(s, "GET", "/api/resources", nil, cookie, ""))
	if second.Busy || !reflect.DeepEqual(first.Report, second.Report) {
		t.Fatal("cancel discarded last report or stuck busy")
	}
	second = read(request(s, "POST", "/api/resources/check", map[string]any{}, cookie, csrf))
	if second.Report.Groups[1].Damaged != 1 || second.Report.Groups[1].Files[0].Reason == "" {
		t.Fatal("explicit scan missed damage reason")
	}
	if !reflect.DeepEqual(modified, hashes()) {
		t.Fatal("retry scan modified resources")
	}
	expectStatus(t, request(s, "POST", "/api/logout", map[string]any{}, cookie, csrf), 200)
	expectStatus(t, request(s, "GET", "/api/resources", nil, cookie, ""), 401)
	expectStatus(t, request(s, "POST", "/api/resources/check", map[string]any{}, cookie, csrf), 401)
	s.cfg.Resources = resourcecheck.New(root)
	if s.cfg.Resources.Read().Report != nil {
		t.Fatal("new manager restored a report from disk")
	}
}

func TestResourceCheckHTTPUnavailable(t *testing.T) {
	s := newTestServer(t)
	cookie, csrf := enroll(t, s)
	expectStatus(t, request(s, http.MethodGet, "/api/resources", nil, cookie, ""), 409)
	expectStatus(t, request(s, http.MethodPost, "/api/resources/check", map[string]any{}, cookie, csrf), 409)
}

type delayedResourceWriter struct{ http.ResponseWriter }

func (w delayedResourceWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w delayedResourceWriter) Write(b []byte) (int, error) {
	time.Sleep(120 * time.Millisecond)
	return w.ResponseWriter.Write(b)
}
func TestResourceCheckHTTPWriteDeadline(t *testing.T) {
	s := newTestServer(t)
	s.cfg.Resources = resourcecheck.New(t.TempDir())
	cookie, csrf := enroll(t, s)
	live := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.Handler().ServeHTTP(delayedResourceWriter{w}, r) }))
	live.Config.WriteTimeout = 40 * time.Millisecond
	live.Start()
	defer live.Close()
	r, _ := http.NewRequest(http.MethodPost, live.URL+"/api/resources/check", strings.NewReader(`{}`))
	r.Host = s.cfg.Addr
	r.AddCookie(cookie)
	r.Header.Set("Origin", "http://"+s.cfg.Addr)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Local-Request", "1")
	r.Header.Set("X-CSRF-Token", csrf)
	resp, err := live.Client().Do(r)
	if err != nil {
		t.Fatal("resource scan inherited short response deadline", err)
	}
	defer resp.Body.Close()
	var snapshot resourcecheck.Snapshot
	if err = json.NewDecoder(resp.Body).Decode(&snapshot); err != nil || resp.StatusCode != 200 || snapshot.Report == nil {
		t.Fatal("incomplete response", err)
	}
	if live.Config.WriteTimeout != 40*time.Millisecond {
		t.Fatal("changed unrelated API deadlines")
	}
}
