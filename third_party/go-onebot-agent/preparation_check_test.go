package goba

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
)

type preparationMemory struct{ load func() }

func (*preparationMemory) Save(int64, string) error { return nil }
func (m *preparationMemory) Load(int64) []string {
	if m.load != nil {
		m.load()
	}
	return nil
}

type preparationHistory struct {
	modelize func(model.Protocol, int64, func(*fmt.Stringer))
	appends  atomic.Int32
}

func (h *preparationHistory) Append(int64, string, fmt.Stringer) error { h.appends.Add(1); return nil }
func (h *preparationHistory) Modelize(p model.Protocol, grp int64, system string, _ bool, pre func(*fmt.Stringer)) (deepinfra.Model, error) {
	p.System(system)
	if h.modelize != nil {
		h.modelize(p, grp, pre)
	}
	return p, nil
}
func preparationEvent(sources ...string) *Event {
	segments := []any{}
	for _, source := range sources {
		segments = append(segments, map[string]any{"type": "image", "data": map[string]string{"file": source}})
	}
	raw, _ := json.Marshal(segments)
	return &Event{PostType: "message", Message: raw}
}
func preparationApply(p model.Protocol, pre func(*fmt.Stringer), ev *Event) {
	var value fmt.Stringer = ev
	if pre != nil {
		pre(&value)
	}
	p.User(model.NewContentText(value.String()))
}

func TestCheckedPreparationMemoryHistoryAndLegacyBoundaries(t *testing.T) {
	stopped := errors.New("synthetic task stopped")
	for _, phase := range []string{"initial", "memory", "history"} {
		t.Run(phase, func(t *testing.T) {
			var stop atomic.Bool
			var loads, models atomic.Int32
			mem := &preparationMemory{load: func() {
				loads.Add(1)
				if phase == "memory" {
					stop.Store(true)
				}
			}}
			h := &preparationHistory{modelize: func(p model.Protocol, _ int64, pre func(*fmt.Stringer)) {
				models.Add(1)
				if phase == "history" {
					stop.Store(true)
				}
				preparationApply(p, pre, preparationEvent())
				preparationApply(p, pre, preparationEvent())
			}}
			ag := NewAgent(&Config{}, 16, 8, time.Hour, "", mem, true, true)
			ag.SetHistoryStore(h)
			if phase == "initial" {
				stop.Store(true)
			}
			reqs, err := ag.GetActionChecked(deepinfra.NewAPI("https://synthetic.invalid", ""), model.NewOpenAI("synthetic", "", 0, 1, 128), 10, PermRoleUser, 0, false, func() error {
				if stop.Swap(false) {
					return stopped
				}
				return nil
			})
			if err != stopped || len(reqs) != 0 || h.appends.Load() != 0 {
				t.Fatal("cancelled preparation returned actions or wrote records", err)
			}
			if (phase == "initial" && loads.Load() != 0) || (phase != "history" && models.Load() != 0) {
				t.Fatal("stopped preparation continued", loads.Load(), models.Load())
			}
		})
	}
	ag := NewAgent(&Config{}, 16, 8, time.Hour, "", &preparationMemory{}, true, true)
	if _, err := ag.GetActionChecked(deepinfra.API{}, model.NewOpenAI("synthetic", "", 0, 1, 128), 10, PermRoleUser, 0, false, func() error { return nil }); err != ErrCheckedHistoryRequired {
		t.Fatal("legacy callback silently bypassed checks", err)
	}
}

func TestCheckedPreparationStopsAfterDownloadOrVision(t *testing.T) {
	for _, phase := range []string{"download", "vision"} {
		t.Run(phase, func(t *testing.T) {
			data := imageInputPNG(t)
			started, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			var imageCalls, visionCalls, agentCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					imageCalls.Add(1)
					if phase == "download" && r.URL.Path == "/first" {
						close(started)
						<-release
					}
					w.Write(data)
					return
				}
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				reply := `{"action":"end_action","params":{}}`
				if body["model"] == "vision" {
					visionCalls.Add(1)
					if phase == "vision" {
						close(started)
						<-release
					}
					reply = "synthetic image description"
				} else {
					agentCalls.Add(1)
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": reply}}}})
			}))
			defer server.Close()
			// Release any blocked handler before httptest.Server.Close during a failed assertion.
			defer once.Do(func() { close(release) })
			h := &preparationHistory{modelize: func(p model.Protocol, _ int64, pre func(*fmt.Stringer)) {
				preparationApply(p, pre, preparationEvent(server.URL+"/first", server.URL+"/second"))
				preparationApply(p, pre, preparationEvent(server.URL+"/third"))
			}}
			ag := NewAgent(&Config{}, 16, 8, time.Hour, "", &preparationMemory{}, true, true)
			ag.SetHistoryStore(h)
			api := deepinfra.NewAPI(server.URL, "")
			api.SetHTTPClient(server.Client())
			ag.SetViewImageAPI(api, model.NewOpenAI("vision", "", 0, 1, 128))
			var stop atomic.Bool
			stopped := errors.New("synthetic stopped")
			done := make(chan error, 1)
			go func() {
				reqs, err := ag.GetActionChecked(api, model.NewOpenAI("agent", "", 0, 1, 128), 10, PermRoleUser, 0, false, func() error {
					if stop.Load() {
						return stopped
					}
					return nil
				})
				if len(reqs) != 0 {
					done <- errors.New("stopped preparation returned actions")
					return
				}
				done <- err
			}()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("in-flight request missing")
			}
			stop.Store(true)
			once.Do(func() { close(release) })
			select {
			case err := <-done:
				if err != stopped {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("preparation did not return")
			}
			wantVision := int32(0)
			if phase == "vision" {
				wantVision = 1
			}
			if imageCalls.Load() != 1 || visionCalls.Load() != wantVision || agentCalls.Load() != 0 || h.appends.Load() != 0 {
				t.Fatal("work continued after stop", imageCalls.Load(), visionCalls.Load(), agentCalls.Load(), h.appends.Load())
			}
			cached := 0
			_ = ag.imgpcache.Range(func(_ uint64, _ imageDescription) error { cached++; return nil })
			if cached != 0 {
				t.Fatal("stopped vision response was cached")
			}
		})
	}
}

func TestCheckedPreparationResponseAndConcurrentScopeIsolation(t *testing.T) {
	var stopped atomic.Bool
	stopErr := errors.New("synthetic settings changed")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["model"] == "cancel-after-response" {
			stopped.Store(true)
		}
		reply := `{"action":"end_action","params":{}}`
		if body["model"] == "scope-vision" {
			reply = "synthetic image description"
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": reply}}}})
	}))
	defer server.Close()
	h := &preparationHistory{}
	ag := NewAgent(&Config{}, 16, 8, time.Hour, "", &preparationMemory{}, true, true)
	ag.SetHistoryStore(h)
	api := deepinfra.NewAPI(server.URL, "")
	api.SetHTTPClient(server.Client())
	if _, err := ag.GetActionChecked(api, model.NewOpenAI("cancel-after-response", "", 0, 1, 128), 10, PermRoleUser, 0, false, func() error {
		if stopped.Load() {
			return stopErr
		}
		return nil
	}); err != stopErr || h.appends.Load() != 0 {
		t.Fatal("cancelled result was parsed or recorded", err)
	}
	source := "base64://" + base64.StdEncoding.EncodeToString(imageInputPNG(t))
	h.modelize = func(p model.Protocol, _ int64, pre func(*fmt.Stringer)) {
		preparationApply(p, pre, preparationEvent(source))
	}
	ag.SetViewImageAPI(api, model.NewOpenAI("scope-vision", "", 0, 1, 128))
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			grp := int64(i + 20)
			_, err := ag.GetActionChecked(api, model.NewOpenAI("agent", "", 0, 1, 128), grp, PermRoleUser, 0, false, func() error {
				if i%2 == 0 {
					return stopErr
				}
				return nil
			})
			if (i%2 == 0 && err != stopErr) || (i%2 != 0 && !errors.Is(err, io.EOF)) {
				t.Error("check leaked across concurrent conversations", i, err)
			}
		}(i)
	}
	wg.Wait()
	if h.appends.Load() != 6 {
		t.Fatal("unexpected terminus writes", h.appends.Load())
	}
	// Original API and nil-check behavior still work without persistent history.
	legacy := NewAgent(&Config{}, 16, 8, time.Hour, "", &preparationMemory{}, true, true)
	if _, err := legacy.GetAction(api, model.NewOpenAI("agent", "", 0, 1, 128), 10, PermRoleUser, 0, false); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
}
