package modelstats

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// Labels are supplied by the host, never extracted from request contents.
type Labels struct{ Model, Session, Task string }
type Usage struct {
	Input  *int64 `json:"input"`
	Output *int64 `json:"output"`
	Hit    *int64 `json:"hit"`
	Miss   *int64 `json:"miss"`
}
type Request struct {
	Sequence     uint64    `json:"sequence"`
	ID           uint64    `json:"id"`
	Kind         string    `json:"kind"`
	Provider     string    `json:"provider,omitempty"`
	Model        string    `json:"model,omitempty"`
	Session      string    `json:"session,omitempty"`
	Task         string    `json:"task,omitempty"`
	StartedAt    time.Time `json:"startedAt"`
	Milliseconds float64   `json:"milliseconds"`
	Outcome      string    `json:"outcome"`
	Usage        Usage     `json:"usage"`
}

var identitySalt = func() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}()

func anonymous(s string) string {
	if s == "" {
		return ""
	}
	h := hmac.New(sha256.New, identitySalt)
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil)[:12])
}

type usageCapture struct {
	mu   sync.Mutex
	data []byte
	done bool
}

func (c *usageCapture) add(p []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		return
	}
	if len(c.data)+len(p) > 2<<20 {
		c.data = nil
		c.done = true
		return
	}
	c.data = append(c.data, p...)
}
func token(v json.RawMessage) *int64 {
	var n int64
	if len(v) == 0 || string(v) == "null" || json.Unmarshal(v, &n) != nil || n < 0 {
		return nil
	}
	return &n
}
func parseUsage(data []byte) Usage {
	var response struct {
		Usage map[string]json.RawMessage `json:"usage"`
	}
	if json.Unmarshal(data, &response) != nil {
		return Usage{}
	}
	u := response.Usage
	result := Usage{Input: token(u["prompt_tokens"]), Output: token(u["completion_tokens"]), Hit: token(u["prompt_cache_hit_tokens"]), Miss: token(u["prompt_cache_miss_tokens"])}
	if result.Input == nil {
		result.Input = token(u["input_tokens"])
	}
	if result.Output == nil {
		result.Output = token(u["output_tokens"])
	}
	if result.Hit == nil {
		var details map[string]json.RawMessage
		if json.Unmarshal(u["prompt_tokens_details"], &details) == nil {
			result.Hit = token(details["cached_tokens"])
		}
	}
	return result
}
func cloneUsage(u Usage) Usage {
	cp := func(v *int64) *int64 {
		if v == nil {
			return nil
		}
		n := *v
		return &n
	}
	return Usage{cp(u.Input), cp(u.Output), cp(u.Hit), cp(u.Miss)}
}

func (c *usageCapture) finish() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	data := c.data
	c.data = nil
	c.done = true
	return data
}
