package aichat

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
)

type toolBody struct {
	io.Reader
	closed bool
}

func (b *toolBody) Close() error { b.closed = true; return nil }

type brokenToolBody struct{}

func (brokenToolBody) Read([]byte) (int, error) { return 0, errors.New("synthetic-private-detail") }

// Exercise the public feature paths so a future caller cannot accidentally
// bypass the shared redirect, cancellation or bounded-read policy.
func TestDeepSeekToolFeatureTransportBoundaries(t *testing.T) {
	for _, feature := range []struct {
		name    string
		timeout time.Duration
		run     func(context.Context, *http.Client) (string, error)
	}{
		{"link", 90 * time.Second, func(ctx context.Context, c *http.Client) (string, error) {
			return readLink(ctx, linkFixture(), false, 456, 10, goba.PermRoleOwner, "https://example.com/a", c)
		}},
		{"search", 180 * time.Second, func(ctx context.Context, c *http.Client) (string, error) {
			return runDeepSearch(ctx, linkFixture(), deepSeekToolEvent(), goba.PermRoleOwner, false, "问题", c)
		}},
	} {
		for _, scenario := range []string{"redirect", "oversize", "broken", "cancelled"} {
			t.Run(feature.name+"/"+scenario, func(t *testing.T) {
				calls, redirects := 0, 0
				body := &toolBody{Reader: strings.NewReader("synthetic-private-detail")}
				status := http.StatusOK
				switch scenario {
				case "redirect":
					status = http.StatusFound
				case "oversize":
					body.Reader = strings.NewReader(strings.Repeat("x", deepSeekToolResponseLimit+1))
				case "broken":
					body.Reader = brokenToolBody{}
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if scenario == "cancelled" {
					cancel()
				}
				client := &http.Client{Timeout: time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { redirects++; return nil }}
				client.Transport = linkTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					deadline, ok := r.Context().Deadline()
					if !ok || time.Until(deadline) > feature.timeout || time.Until(deadline) > time.Minute {
						t.Fatal("caller deadline/timeout lost")
					}
					if scenario == "cancelled" {
						if r.Context().Err() == nil {
							t.Fatal("parent cancellation lost")
						}
						return nil, r.Context().Err()
					}
					return &http.Response{StatusCode: status, Header: http.Header{"Location": {"https://other.example"}}, Body: body}, nil
				})
				out, err := feature.run(ctx, client)
				if err == nil || out != "" || calls != 1 || redirects != 0 || strings.Contains(err.Error(), "synthetic-private-detail") {
					t.Fatalf("out=%q err=%v calls=%d redirects=%d", out, err, calls, redirects)
				}
				if scenario != "cancelled" && !body.closed {
					t.Fatal("response not closed")
				}
				client.CheckRedirect(nil, nil)
				if redirects != 1 || client.Timeout != time.Minute {
					t.Fatal("caller client mutated")
				}
			})
		}
	}
}

func deepSeekToolEvent() *zero.Event {
	return &zero.Event{SelfID: 456, GroupID: 10, UserID: 123, NativeMessage: []byte(`[{"type":"at","data":{"qq":"456"}},{"type":"reply","data":{"id":"-12"}},{"type":"text","data":{"text":"ASR"}}]`)}
}
