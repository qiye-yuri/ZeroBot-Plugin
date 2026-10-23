package goba

import (
	"fmt"
	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type cacheHistory struct{ writes []string }

func (h *cacheHistory) Append(_ int64, _ string, s fmt.Stringer) error {
	h.writes = append(h.writes, s.String())
	return nil
}
func (*cacheHistory) Modelize(p model.Protocol, _ int64, system string, userSystem bool, _ func(*fmt.Stringer)) (deepinfra.Model, error) {
	if userSystem {
		p.User(model.NewContentText(system))
	} else {
		p.System(system)
	}
	p.User(model.NewContentText("synthetic-history-first"))
	p.Assistant(model.NewContentText("synthetic-history-reply"))
	p.User(model.NewContentText("synthetic-history-latest"))
	return p, nil
}
func TestCacheRuntimeFollowsHistoryAcrossProtocols(t *testing.T) {
	for name, p := range map[string]model.Protocol{"openai": model.NewOpenAI("synthetic", "", 0, 1, 128), "ollama": model.NewOLLaMA("synthetic", "", 0, 1, 128), "genai": model.NewGenAI("synthetic", 0, 1, 128)} {
		for _, userSystem := range []bool{false, true} {
			t.Run(fmt.Sprint(name, userSystem), func(t *testing.T) {
				h := &cacheHistory{}
				ag := NewAgent(&Config{Nickname: "synthetic-bot"}, 16, 8, time.Hour, "", &preparationMemory{}, true, true)
				ag.SetHistoryStore(h)
				api := deepinfra.NewAPI("https://synthetic.invalid", "")
				var bodies []string
				api.SetHTTPClient(&http.Client{Transport: modelFailureTransport(func(r *http.Request) (*http.Response, error) {
					b, _ := io.ReadAll(r.Body)
					bodies = append(bodies, string(b))
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
				})})
				for iter := 0; iter < 2; iter++ {
					ag.GetAction(api, p, -10, PermRoleOwner, iter, userSystem)
				}
				if len(bodies) != 2 {
					t.Fatal("missing requests")
				}
				var prefix string
				for iter, b := range bodies {
					pos := strings.Index(b, "宿主提供的本轮上下文")
					if pos < 0 || strings.Count(b, "宿主提供的本轮上下文") != 1 || strings.Index(b, "synthetic-history-latest") > pos || strings.Contains(b[:pos], "当前时间：") || !strings.Contains(b[pos:], fmt.Sprintf("迭代轮次：%d", iter)) {
						t.Fatal("runtime order or iteration incorrect")
					}
					if iter == 0 {
						prefix = b[:pos]
					} else if prefix != b[:pos] {
						t.Fatal("stable prefix changed")
					}
				}
				for _, s := range h.writes {
					if strings.Contains(s, "本轮上下文") {
						t.Fatal("runtime archived")
					}
				}
			})
		}
	}
}

type cacheMemory struct{ text string }

func (m *cacheMemory) Load(int64) []string    { return []string{m.text} }
func (*cacheMemory) Save(int64, string) error { return nil }
func TestChangingMemoryDoesNotInvalidateRulePrefix(t *testing.T) {
	memory := &cacheMemory{text: "synthetic-memory-first"}
	ag := NewAgent(&Config{}, 16, 8, time.Hour, "", memory, true, true)
	first, tail, err := ag.promptParts(PermRoleOwner, 1, -10)
	if err != nil || strings.Contains(first, memory.text) || !strings.Contains(tail, memory.text) {
		t.Fatal("memory misplaced", err)
	}
	memory.text = "synthetic-memory-second\n### 记忆\nsynthetic-marker"
	second, tail, err := ag.promptParts(PermRoleOwner, 2, -10)
	if err != nil || first != second || !strings.Contains(tail, memory.text) || strings.Contains(tail, "synthetic-memory-first") {
		t.Fatal("memory stale or prefix changed", err)
	}
}
