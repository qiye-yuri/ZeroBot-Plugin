package query

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Go caches environment proxy settings, so each scenario gets a fresh process.
func TestQueryEnvironmentProxy(t *testing.T) {
	var hits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "query-proxy.invalid" {
			http.Error(w, "unexpected fixture request", 503)
			return
		}
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":200,"data":{"data":{"records":[]}}}`))
	}))
	defer proxy.Close()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"proxy", "bypass", "direct"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, exe, "-test.run=^TestQueryProxyChild$")
			for _, value := range os.Environ() {
				key, _, _ := strings.Cut(value, "=")
				switch strings.ToUpper(key) {
				case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "REQUEST_METHOD", "QUERY_PROXY_CHILD", "QUERY_PROXY_EXPECTED":
					continue
				}
				cmd.Env = append(cmd.Env, value)
			}
			cmd.Env = append(cmd.Env, "QUERY_PROXY_CHILD="+mode, "QUERY_PROXY_EXPECTED="+proxy.URL)
			if mode != "direct" {
				cmd.Env = append(cmd.Env, "HTTP_PROXY="+proxy.URL, "HTTPS_PROXY="+proxy.URL)
			}
			if mode == "bypass" {
				cmd.Env = append(cmd.Env, "NO_PROXY=query-proxy.invalid")
			}
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s: %v\n%s", mode, err, out)
			}
		})
	}
	if hits.Load() != 1 {
		t.Fatalf("proxy requests=%d, want 1", hits.Load())
	}
}

func TestQueryProxyChild(t *testing.T) {
	mode := os.Getenv("QUERY_PROXY_CHILD")
	if mode == "" {
		return
	}
	client := newDanmakuClient()
	defer client.CloseIdleConnections()
	transport := client.Transport.(*http.Transport)
	if transport.Proxy == nil {
		if mode == "proxy" {
			t.Fatal("query transport ignores configured environment proxy")
		}
		return // A nil proxy selector is a valid direct route in these scenarios.
	}
	for _, scheme := range []string{"http", "https"} {
		req, err := http.NewRequest(http.MethodGet, scheme+"://query-proxy.invalid/records", nil)
		if err != nil {
			t.Fatal(err)
		}
		selected, err := transport.Proxy(req)
		if err != nil {
			t.Fatal(err)
		}
		if mode == "proxy" {
			if selected == nil || selected.String() != os.Getenv("QUERY_PROXY_EXPECTED") {
				t.Fatal("configured proxy not selected")
			}
		} else if selected != nil {
			t.Fatal("direct/bypass still uses proxy")
		}
	}
	if mode == "proxy" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, err := getDanmaku(ctx, client, "http://query-proxy.invalid/records"); err != nil {
			t.Fatal("synthetic proxy query failed", err)
		}
	}
}
