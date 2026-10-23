package updatewatch

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSplitFindingsMergeWithoutWeakeningEvidence(t *testing.T) {
	changes := []Change{{Current: fixture("1.4.1")}, {Current: fixture("1.4.2")}, {Current: fixture("1.4.3")}}
	findings := []Finding{
		{Indices: []int{0}, Summary: "新增功能", Impact: "新增用途", Evidence: []string{changes[0].Current.Text}},
		{Indices: []int{1}, Summary: "修复问题", Impact: "减少崩溃", Evidence: []string{changes[1].Current.Text}},
		{Indices: []int{0, 1}, Summary: "相关修复", Impact: "避免丢失", Evidence: []string{changes[0].Current.Text}},
		{Indices: []int{2}, Summary: "独立事件", Impact: "独立影响", Evidence: []string{changes[2].Current.Text}},
	}
	b, _ := json.Marshal(Decision{Findings: findings})
	got, err := ParseDecision(string(b), changes)
	if err != nil || len(got) != 2 || len(got[0].Keys) != 2 || !strings.Contains(got[0].Text, "新增功能") || !strings.Contains(got[0].Text, "相关修复") {
		t.Fatalf("split/bridge merge failed: %v %v", got, err)
	}
	findings[2].Evidence = []string{"unsupported evidence must still fail"}
	b, _ = json.Marshal(Decision{Findings: findings})
	if _, err = ParseDecision(string(b), changes); err == nil {
		t.Fatal("overlapping finding bypassed evidence check")
	}
}
