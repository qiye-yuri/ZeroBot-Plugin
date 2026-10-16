package goba

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
)

type modelFailureTransport func(*http.Request) (*http.Response, error)

func (f modelFailureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestModelRequestFailureClassificationAndSafeText(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		body string
	}{
		{"unauthorized", 401, "synthetic-provider-secret"},
		{"unavailable", 503, "synthetic-provider-secret"},
		{"invalid-protocol", 200, "synthetic-provider-secret"},
		{"empty-protocol", 200, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(tc.code)
				io.WriteString(w, tc.body)
			}))
			defer server.Close()
			history := &preparationHistory{}
			memory := &actionLimitMemory{}
			ag := NewAgent(&Config{}, 16, 8, time.Hour, "", memory, false, false)
			ag.SetHistoryStore(history)
			api := deepinfra.NewAPI(server.URL, "synthetic-key")
			api.SetHTTPClient(server.Client())
			reqs, err := ag.GetAction(api, model.NewOpenAI("synthetic", "", 0, 1, 128), 10, PermRoleOwner, 0, false)
			if !errors.Is(err, ErrModelRequestFailed) || err.Error() != ErrModelRequestFailed.Error() || errors.Unwrap(err) == nil || reqs != nil || calls.Load() != 1 || history.appends.Load() != 0 || memory.saved.Load() != 0 {
				t.Fatal("incorrect model failure result", err, len(reqs), calls.Load())
			}
			if tc.name == "empty-protocol" && !errors.Is(err, io.EOF) {
				t.Fatal("model failure lost original EOF cause", err)
			}
		})
	}
}

func TestModelFailureCauseAndStopPriority(t *testing.T) {
	for _, stopped := range []bool{false, true} {
		history := &preparationHistory{}
		ag := NewAgent(&Config{}, 16, 8, time.Hour, "", &actionLimitMemory{}, true, true)
		ag.SetHistoryStore(history)
		cancelled := errors.New("synthetic task stopped")
		var called bool
		api := deepinfra.NewAPI("https://synthetic.invalid/synthetic-private-path", "")
		api.SetHTTPClient(&http.Client{Transport: modelFailureTransport(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, context.DeadlineExceeded
		})})
		reqs, err := ag.GetActionChecked(api, model.NewOpenAI("synthetic", "", 0, 1, 128), 10, PermRoleOwner, 0, false, func() error {
			if called && stopped {
				return cancelled
			}
			return nil
		})
		if reqs != nil || history.appends.Load() != 0 || !called {
			t.Fatal("failed call wrote requests", err)
		}
		if stopped {
			if err != cancelled || errors.Is(err, ErrModelRequestFailed) {
				t.Fatal("model failure masked stop", err)
			}
		} else {
			var networkError *url.Error
			if !errors.Is(err, ErrModelRequestFailed) || !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "synthetic-private-path") || !errors.As(err, &networkError) {
				t.Fatal("failure lost cause or leaked details", err)
			}
		}
	}
}
