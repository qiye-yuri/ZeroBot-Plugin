package aichat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/FloatTech/ZeroBot-Plugin/internal/modelstats"
)

func TestBoundedModelChunkedJSONCompletion(t *testing.T) {
	for _, tc := range []struct {
		name string
		wrap func(*http.Client) *http.Client
		kind modelstats.Kind
	}{{"agent", boundedAgentClient, modelstats.Agent}, {"vision", boundedVisionClient, modelstats.Vision}} {
		t.Run(tc.name, func(t *testing.T) {
			for _, size := range []int{512, 1024, 65536} {
				payload := `{"text":"` + strings.Repeat("a", size) + `"}`
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush() // Force a real unknown-length transfer.
					io.WriteString(w, payload)
				}))
				collector := modelstats.New()
				client := collector.Client(tc.wrap(&http.Client{Timeout: time.Second}), tc.kind)
				rsp, err := client.Get(server.URL)
				if err != nil {
					server.Close()
					t.Fatal(err)
				}
				var result struct{ Text string }
				// Match the provider decoder: one JSON Decode, with no EOF read.
				err = json.NewDecoder(rsp.Body).Decode(&result)
				rsp.Body.Close()
				server.Close()
				if err != nil || result.Text != strings.Repeat("a", size) || rsp.ContentLength != int64(len(payload)) {
					t.Fatal("chunked response changed", err, rsp.ContentLength)
				}
				if s := collector.Read(); s.Started != 1 || s.Completed != 1 || s.Active != 0 || s.Incomplete != 0 || s.Failed != 0 {
					t.Fatal("decoded transfer not completed", s.Counts)
				}
			}
		})
	}
}

func TestBoundedModelChunkedWaitsForEOFWithinTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"text":"premature"}`)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	collector := modelstats.New()
	client := collector.Client(boundedAgentClient(&http.Client{Timeout: 100 * time.Millisecond}), modelstats.Agent)
	rsp, err := client.Get(server.URL)
	if !errors.Is(err, context.DeadlineExceeded) || rsp != nil {
		t.Fatal("unfinished transfer accepted", err)
	}
	if s := collector.Read(); s.Started != 1 || s.Failed != 1 || s.Active != 0 || s.Completed != 0 || s.Incomplete != 0 {
		t.Fatal("timeout not counted as failure", s.Counts)
	}
}

type modelCompletionFailureBody struct {
	readErr, closeErr error
	closes            int
}

func (b *modelCompletionFailureBody) Read([]byte) (int, error) { return 0, b.readErr }
func (b *modelCompletionFailureBody) Close() error             { b.closes++; return b.closeErr }

type modelCompletionTransport struct{ body io.ReadCloser }

func (t modelCompletionTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, ContentLength: -1, Header: make(http.Header), Body: t.body}, nil
}

func TestBoundedModelUnknownLengthFailuresCloseOnce(t *testing.T) {
	for _, tc := range []struct {
		name              string
		readErr, closeErr error
	}{{"truncated", io.ErrUnexpectedEOF, nil}, {"close-error", io.EOF, io.ErrClosedPipe}, {"read-error-first", io.ErrUnexpectedEOF, io.ErrClosedPipe}} {
		t.Run(tc.name, func(t *testing.T) {
			body := &modelCompletionFailureBody{readErr: tc.readErr, closeErr: tc.closeErr}
			collector := modelstats.New()
			client := collector.Client(boundedAgentClient(&http.Client{Transport: modelCompletionTransport{body}}), modelstats.Agent)
			_, err := client.Get("https://synthetic.invalid/model")
			want := tc.readErr
			if want == io.EOF {
				want = tc.closeErr
			}
			if !errors.Is(err, want) || body.closes != 1 {
				t.Fatal("failure lost or body leaked", err, body.closes)
			}
			if s := collector.Read(); s.Failed != 1 || s.Active != 0 || s.Completed != 0 || s.Incomplete != 0 {
				t.Fatal("failed transfer completed", s.Counts)
			}
		})
	}
}

func TestBoundedModelKnownLengthStillRejectsStreamOverflow(t *testing.T) {
	body := &visionTestBody{Reader: strings.NewReader(strings.Repeat("x", visionResponseBytes+2))}
	client := boundedVisionClient(&http.Client{Transport: &visionTestTransport{body: body, length: 1, status: 200}})
	rsp, err := client.Get("https://synthetic.invalid/model")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(rsp.Body)
	if !errors.Is(err, errVisionResponseLimit) || len(data) > visionResponseBytes || body.read > visionResponseBytes+1 {
		t.Fatal("declared short body bypassed bound", err)
	}
	if n, err := rsp.Body.Read(make([]byte, 1)); n != 0 || !errors.Is(err, errVisionResponseLimit) {
		t.Fatal("overflow did not remain failed", n, err)
	}
	rsp.Body.Close()
	if body.closes != 1 {
		t.Fatal("body leaked")
	}
}
