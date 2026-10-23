package bilibiliparse

import (
	"fmt"
	"strings"
	"testing"
)

func TestSummaryResponses(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		fail             bool
	}{
		{"valid", `{"code":0,"data":{"model_result":{"summary":"fixture summary"}}}`, "fixture summary", false},
		{"empty", `{"code":0,"data":{"model_result":{"summary":""}}}`, "暂无可用", false},
		{"api-error", `{"code":-101}`, "", true},
		{"business-error", `{"code":0,"data":{"code":1}}`, "", true},
		{"missing-code", `{}`, "", true},
		{"empty-body", ``, "", true},
		{"truncated", `{"code":0`, "", true},
		{"missing-data", `{"code":0}`, "", true},
		{"null-data", `{"code":0,"data":null}`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := decodeVideoSummary([]byte(tc.body))
			if (err != nil) != tc.fail {
				t.Fatalf("err=%v", err)
			}
			if !tc.fail && !strings.Contains(fmt.Sprint(msg), tc.want) {
				t.Fatalf("msg=%v", msg)
			}
		})
	}
}
