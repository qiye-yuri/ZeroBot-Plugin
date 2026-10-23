// Package modelstats counts model HTTP exchanges without retaining their contents.
package modelstats

import (
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FloatTech/ZeroBot-Plugin/internal/processidentity"
)

type Kind uint8

const (
	GPT Kind = iota // /gpt and group summary
	Chat
	Agent
	Vision
	kindCount
)

type Counts struct {
	Started             uint64  `json:"started"`
	Active              uint64  `json:"active"`
	Completed           uint64  `json:"completed"`
	Failed              uint64  `json:"failed"`
	Incomplete          uint64  `json:"incomplete"`
	AverageMilliseconds float64 `json:"averageMilliseconds"`
	MaxMilliseconds     float64 `json:"maxMilliseconds"`
}

type Category struct {
	Kind string `json:"kind"`
	Counts
}

type Snapshot struct {
	Process   *processidentity.Identity `json:"process,omitempty"`
	StartedAt time.Time                 `json:"startedAt"`
	CheckedAt time.Time                 `json:"checkedAt"`
	Counts
	Categories []Category `json:"categories"`
	Recent     []Request  `json:"recent"`
}

type aggregate struct {
	Counts
	totalMilliseconds float64
}
type Collector struct {
	mu      sync.Mutex
	started time.Time
	total   aggregate
	rows    [kindCount]aggregate
	recent  []Request
	serial  uint64
}

func New() *Collector { return &Collector{started: time.Now().UTC()} }

var Default = New()

// Read returns aggregate counters and a bounded copy of recent metadata.
func (c *Collector) Read() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := Snapshot{StartedAt: c.started, CheckedAt: time.Now().UTC(), Counts: c.total.Counts, Categories: make([]Category, kindCount)}
	for i, name := range [...]string{"gpt", "chat", "agent", "vision"} {
		s.Categories[i] = Category{Kind: name, Counts: c.rows[i].Counts}
	}
	s.Recent = append([]Request(nil), c.recent...)
	for i := range s.Recent {
		s.Recent[i].Usage = cloneUsage(s.Recent[i].Usage)
	}
	return s
}

func (c *Collector) begin(kind Kind, record Request, capture *usageCapture) func(outcome uint8) {
	c.mu.Lock()
	c.serial++
	record.ID = c.serial
	record.StartedAt = time.Now().UTC()
	c.total.Started++
	c.total.Active++
	c.rows[kind].Started++
	c.rows[kind].Active++
	c.mu.Unlock()
	start := time.Now()
	var once sync.Once
	return func(outcome uint8) {
		once.Do(func() {
			data := capture.finish()
			if outcome == 0 {
				record.Usage = parseUsage(data)
			}
			record.Outcome = [...]string{"completed", "failed", "incomplete"}[outcome]
			ms := float64(time.Since(start)) / float64(time.Millisecond)
			c.mu.Lock()
			defer c.mu.Unlock()
			record.Milliseconds = ms
			if len(c.recent) == 256 {
				copy(c.recent, c.recent[1:])
				c.recent = c.recent[:255]
			}
			c.recent = append(c.recent, record)
			for _, a := range []*aggregate{&c.total, &c.rows[kind]} {
				a.Active--
				switch outcome {
				case 0:
					a.Completed++
				case 1:
					a.Failed++
				default:
					a.Incomplete++
				}
				a.totalMilliseconds += ms
				finished := a.Completed + a.Failed + a.Incomplete
				a.AverageMilliseconds = a.totalMilliseconds / float64(finished)
				if ms > a.MaxMilliseconds {
					a.MaxMilliseconds = ms
				}
			}
		})
	}
}

// Client copies the client and preserves its timeout, jar, redirect policy and
// original transport. Exchanges include redirects; it neither retries nor reads
// request payloads. It inspects a bounded copy of consumed response bytes for usage. Invalid categories simply leave it unchanged.
func (c *Collector) Client(client *http.Client, kind Kind, labels ...Labels) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	if kind >= kindCount {
		return client
	}
	copy := *client
	base := copy.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	var label Labels
	if len(labels) > 0 {
		label = labels[0]
	}
	copy.Transport = &transport{base: base, collector: c, kind: kind, labels: label}
	return &copy
}

type transport struct {
	base      http.RoundTripper
	collector *Collector
	kind      Kind
	labels    Labels
	sequence  atomic.Uint64
}

func (t *transport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}
func (t *transport) RoundTrip(r *http.Request) (response *http.Response, err error) {
	capture := &usageCapture{}
	record := Request{Sequence: t.sequence.Add(1), Kind: [...]string{"gpt", "chat", "agent", "vision"}[t.kind], Model: t.labels.Model, Session: anonymous(t.labels.Session), Task: anonymous(t.labels.Task)}
	if len(record.Model) > 128 {
		record.Model = record.Model[:128]
	}
	if t.labels.Model != "" {
		record.Provider = r.URL.Hostname()
	}
	finish := t.collector.begin(t.kind, record, capture)
	returned := false
	defer func() {
		if !returned {
			finish(1)
		}
	}() // Preserve a transport panic.
	response, err = t.base.RoundTrip(r)
	returned = true
	if err != nil || response == nil {
		finish(1)
		return
	}
	ok := response.StatusCode >= 200 && response.StatusCode < 300
	if response.Body == nil || response.Body == http.NoBody {
		if ok {
			finish(0)
		} else {
			finish(1)
		}
		return
	}
	response.Body = &body{ReadCloser: response.Body, finish: finish, ok: ok, remaining: response.ContentLength, capture: capture}
	return
}

// Completion means 2xx plus EOF or the full declared Content-Length. An early close
// is incomplete, not successful. Failures never retain error messages.
type body struct {
	io.ReadCloser
	finish    func(uint8)
	ok        bool
	remaining int64
	capture   *usageCapture
}

func (b *body) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.capture.add(p[:n])
	if b.remaining >= 0 {
		b.remaining -= int64(n)
	}
	if err != nil {
		if err == io.EOF && b.ok && b.remaining <= 0 {
			b.finish(0)
		} else {
			b.finish(1)
		}
	} else if b.remaining == 0 {
		if b.ok {
			b.finish(0)
		} else {
			b.finish(1)
		}
	}
	return n, err
}
func (b *body) Close() error {
	err := b.ReadCloser.Close()
	if err != nil || !b.ok {
		b.finish(1)
	} else {
		b.finish(2)
	}
	return err
}
