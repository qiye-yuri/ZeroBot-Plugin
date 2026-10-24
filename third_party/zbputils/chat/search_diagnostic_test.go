package chat

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSearchWebDiagnosticCancellationAndRedirect(t *testing.T) {
	for _, redirect := range []bool{false, true} {
		calls := 0
		client := &http.Client{Transport: searchTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if !redirect {
				<-r.Context().Done()
				return nil, r.Context().Err()
			}
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://example.com/secret"}}, Body: io.NopCloser(strings.NewReader("PRIVATE_REDIRECT"))}, nil
		})}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		page, d := SearchWeb(ctx, client, "SYNTHETIC_KEY", "fixed public query")
		cancel()
		want := "network_error"
		if redirect {
			want = "http_error"
		}
		if d.Category != want || page.Status != "error" || calls != 1 {
			t.Fatal(d, page, calls)
		}
	}
}
