package aichat

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/FloatTech/ZeroBot-Plugin/internal/modelstats"
)

func TestAgentResponseLimitIsIndependentOfVision(t *testing.T) {
	for _, tc := range []struct {
		name             string
		size             int
		declared, failed bool
	}{{"above-vision-limit", visionResponseBytes + 1024, false, false}, {"exact-agent-limit", agentResponseBytes, false, false}, {"declared-agent-overflow", agentResponseBytes + 1, true, true}, {"streamed-agent-overflow", agentResponseBytes + 1, false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			body := &visionTestBody{Reader: strings.NewReader(strings.Repeat("x", tc.size))}
			length := int64(-1)
			if tc.declared {
				length = int64(tc.size)
			}
			transport := &visionTestTransport{body: body, length: length, status: 200}
			collector := modelstats.New()
			client := collector.Client(boundedAgentClient(&http.Client{Transport: transport}), modelstats.Agent)
			rsp, err := client.Get("https://synthetic.invalid/model")
			if tc.failed {
				if !errors.Is(err, errAgentResponseLimit) || body.read > agentResponseBytes+1 || (tc.declared && body.read != 0) {
					t.Fatal("excess not bounded", err, body.read)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				data, e := io.ReadAll(rsp.Body)
				rsp.Body.Close()
				if e != nil || len(data) != tc.size {
					t.Fatal("valid Agent response rejected", e, len(data))
				}
			}
			stats := collector.Read()
			if stats.Active != 0 || stats.Incomplete != 0 || (tc.failed && (stats.Failed != 1 || stats.Completed != 0)) || (!tc.failed && (stats.Completed != 1 || stats.Failed != 0)) {
				t.Fatal("incorrect Agent statistics", stats.Counts)
			}
			if body.closes != 1 {
				t.Fatal("body leaked")
			}
		})
	}
}
