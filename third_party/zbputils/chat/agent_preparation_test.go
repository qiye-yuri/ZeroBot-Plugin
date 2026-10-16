package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
	goba "github.com/fumiama/go-onebot-agent"
)

type preparationTestMemory struct{}

func (preparationTestMemory) Load(int64) []string      { return nil }
func (preparationTestMemory) Save(int64, string) error { return nil }

func TestAgentPreparationTaskStopAndReload(t *testing.T) {
	for _, phase := range []string{"download", "vision", "settings"} {
		t.Run(phase, func(t *testing.T) {
			h := taskFixture(t)
			s := h.tasks
			started, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			var images, visions, agents atomic.Int32
			var data bytes.Buffer
			if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 32, 32))); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					images.Add(1)
					if phase == "download" {
						close(started)
						<-release
					}
					w.Write(data.Bytes())
					return
				}
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				if body["model"] == "vision" {
					visions.Add(1)
					if phase != "download" {
						close(started)
						<-release
					}
				} else {
					agents.Add(1)
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": "synthetic image description"}}}})
			}))
			defer server.Close()
			defer once.Do(func() { close(release) })
			raw, _ := json.Marshal([]any{map[string]any{"type": "image", "data": map[string]string{"file": server.URL + "/first"}}, map[string]any{"type": "image", "data": map[string]string{"file": server.URL + "/second"}}})
			if err := h.Append(10, "event", &goba.Event{PostType: "message", Message: raw}); err != nil {
				t.Fatal(err)
			}
			r, err := s.Begin(10)
			if err != nil {
				t.Fatal(err)
			}
			if err = r.Model(); err != nil {
				t.Fatal(err)
			}
			ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", preparationTestMemory{}, true, true)
			ag.SetHistoryStore(h)
			api := deepinfra.NewAPI(server.URL, "")
			api.SetHTTPClient(server.Client())
			ag.SetViewImageAPI(api, model.NewOpenAI("vision", "", 0, 1, 128, ""))
			var changed atomic.Bool
			done := make(chan error, 1)
			go func() {
				reqs, ended, err := NextAgentActionsChecked(&ag, false, 0, api, model.NewOpenAI("agent", "", 0, 1, 128, ""), 10, goba.PermRoleUser, func() error {
					if changed.Load() {
						return ErrAgentReloaded
					}
					return r.Check()
				})
				if len(reqs) != 0 || ended {
					done <- ErrAgentTaskConflict
					return
				}
				done <- err
			}()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("request did not start")
			}
			if phase == "settings" {
				changed.Store(true)
			} else {
				v, err := s.Read(context.Background(), 10, r.info.ID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = s.Cancel(context.Background(), 10, v.ID, v.Version); err != nil {
					t.Fatal(err)
				}
			}
			once.Do(func() { close(release) })
			var runErr error
			select {
			case runErr = <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("task did not stop")
			}
			want := ErrAgentCancelled
			if phase == "settings" {
				want = ErrAgentReloaded
			}
			if runErr != want {
				t.Fatal(runErr)
			}
			if err = r.Finish(runErr); err != nil {
				t.Fatal(err)
			}
			v, err := s.Read(context.Background(), 10, r.info.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !v.Persisted || v.Activity.ModelPreparations != 1 || v.Activity.FallbackPreparations != 0 || v.Activity.LocalActions != 0 || v.Activity.PlatformActions != 0 || v.Results != (TaskResults{Available: true}) || v.Queries.Used != 0 {
				t.Fatal("preparation misreported as executed actions", v)
			}
			if phase == "settings" {
				if v.Status != "failed" || v.Error != "settings_changed" {
					t.Fatal(v)
				}
			} else if v.Status != "cancelled" || v.CancelRequestedAt == "" {
				t.Fatal(v)
			}
			wantVision := int32(1)
			if phase == "download" {
				wantVision = 0
			}
			if images.Load() != 1 || visions.Load() != wantVision || agents.Load() != 0 {
				t.Fatal("work continued", images.Load(), visions.Load(), agents.Load())
			}
		})
	}
}
