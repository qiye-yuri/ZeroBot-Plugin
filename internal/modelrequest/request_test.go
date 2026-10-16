package modelrequest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/FloatTech/ZeroBot-Plugin/internal/modelstats"
	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
)

func TestChunkedChatResponsesCompleteAfterDecode(t *testing.T) {
	for _, kind := range []modelstats.Kind{modelstats.GPT, modelstats.Chat} {
		for _, size := range []int{32, 4096} {
			t.Run(fmt.Sprintf("kind-%d/bytes-%d", kind, size), func(t *testing.T) {
				text := strings.Repeat("a", size)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(200)
					w.(http.Flusher).Flush()
					fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, text)
				}))
				defer server.Close()
				stats := modelstats.New()
				api := deepinfra.NewAPI(server.URL, "synthetic-key")
				api.SetHTTPClient(stats.Client(&http.Client{Timeout: time.Second}, kind))
				reply, err := Request(api, model.NewOpenAI("synthetic", "", 0, 1, 128, ""))
				if err != nil || reply != text {
					t.Fatal("reply changed", err)
				}
				if s := stats.Read(); s.Started != 1 || s.Completed != 1 || s.Incomplete != 0 || s.Failed != 0 || s.Active != 0 {
					t.Fatal("complete JSON transfer counted incorrectly", s.Counts)
				}
			})
		}
	}
}

func TestDecodedReplyWaitsForTransferEnd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"early reply"}}]}`)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	stats := modelstats.New()
	api := deepinfra.NewAPI(server.URL, "")
	api.SetHTTPClient(stats.Client(&http.Client{Timeout: 100 * time.Millisecond}, modelstats.Chat))
	reply, err := Request(api, model.NewOpenAI("synthetic", "", 0, 1, 128, ""))
	if !errors.Is(err, context.DeadlineExceeded) || reply != "" {
		t.Fatal("unfinished transfer returned an early reply", err)
	}
	if s := stats.Read(); s.Failed != 1 || s.Completed != 0 || s.Incomplete != 0 || s.Active != 0 {
		t.Fatal("transfer timeout counted incorrectly", s.Counts)
	}
}

type tailFailureBody struct {
	text   *strings.Reader
	closes int
}

func (b *tailFailureBody) Read(p []byte) (int, error) {
	if b.text.Len() == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	return b.text.Read(p)
}
func (b *tailFailureBody) Close() error { b.closes++; return nil }

type tailFailureTransport struct{ body io.ReadCloser }

func (t tailFailureTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, ContentLength: -1, Header: make(http.Header), Body: t.body}, nil
}

func TestReplyTailReadFailureIsNotSuccess(t *testing.T) {
	body := &tailFailureBody{text: strings.NewReader(`{"choices":[{"message":{"content":"early reply"}}]}`)}
	stats := modelstats.New()
	api := deepinfra.NewAPI("https://synthetic.invalid/model", "")
	api.SetHTTPClient(stats.Client(&http.Client{Transport: tailFailureTransport{body}}, modelstats.GPT))
	reply, err := Request(api, model.NewOpenAI("synthetic", "", 0, 1, 128, ""))
	if !errors.Is(err, io.ErrUnexpectedEOF) || reply != "" || body.closes != 1 {
		t.Fatal("tail failure lost", err, body.closes)
	}
	if s := stats.Read(); s.Failed != 1 || s.Completed != 0 || s.Incomplete != 0 || s.Active != 0 {
		t.Fatal("tail failure counted incorrectly", s.Counts)
	}
}

func TestParseFailureDoesNotDrainReply(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":]`)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	stats := modelstats.New()
	api := deepinfra.NewAPI(server.URL, "")
	api.SetHTTPClient(stats.Client(&http.Client{Timeout: time.Second}, modelstats.GPT))
	_, err := Request(api, model.NewOpenAI("synthetic", "", 0, 1, 128, ""))
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("parse error lost or unnecessarily waited for EOF", err)
	}
	if s := stats.Read(); s.Incomplete != 1 || s.Completed != 0 || s.Failed != 0 || s.Active != 0 {
		t.Fatal("early parse failure counted incorrectly", s.Counts)
	}
}
