package aichat

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FloatTech/ZeroBot-Plugin/internal/modelstats"
	"github.com/FloatTech/zbputils/chat"
)

// Explicit opt-in: read provider configuration only, send synthetic evidence,
// and retain the provider's real usage fields without guessing absent values.
func TestRealRollingSummary(t *testing.T) {
	path := os.Getenv("LAYERED_REAL_CONFIG_DB")
	out := os.Getenv("LAYERED_REAL_RESULT")
	if path == "" || out == "" {
		t.Skip("explicit real-provider test not requested")
	}
	now := time.Now().In(time.FixedZone("Asia/Shanghai", 8*3600))
	minute := now.Hour()*60 + now.Minute()
	if now.Weekday() != time.Saturday && now.Weekday() != time.Sunday && ((minute >= 540 && minute < 720) || (minute >= 840 && minute < 1080)) {
		t.Fatal("real model test is outside the allowed off-peak window")
	}
	db, e := sql.Open("sqlite3", "file:/"+filepath.ToSlash(path)+"?mode=ro")
	if e != nil {
		t.Fatal("read-only configuration unavailable")
	}
	defer db.Close()
	rows, e := db.Query(`SELECT ext FROM __resp WHERE ext IS NOT NULL AND ext NOT IN ('','-')`)
	if e != nil {
		t.Fatal("configuration lookup failed")
	}
	var cfg chat.AgentConfig
	found := false
	for rows.Next() {
		var raw string
		if rows.Scan(&raw) != nil {
			rows.Close()
			t.Fatal("configuration scan failed")
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal([]byte(raw), &fields) != nil || (fields["ModelName"] == nil && fields["ModelRoutes"] == nil) {
			continue
		}
		if found {
			rows.Close()
			t.Fatal("ambiguous provider configuration")
		}
		if json.Unmarshal([]byte(raw), &cfg) != nil {
			rows.Close()
			t.Fatal("configuration decode failed")
		}
		found = true
	}
	e = rows.Err()
	rows.Close()
	if e != nil || !found {
		t.Fatal("provider not found")
	}
	cfg, e = cfg.ResolveModels()
	if e != nil {
		t.Fatal("provider resolution failed")
	}
	input := chat.SummaryRequest{Previous: "截至档案#10：项目代号SUM_LM_1029，用户希望被称为小叶，原计划周五演示，部署尚未确认。", Records: "[#11 event] 用户明确将演示改到周六，周五计划作废。\n[#12 request] 尝试部署。\n[#13 response] 部署失败，服务未启动。\n[#14 event] 用户仍希望被称为小叶；部署需要重新检查，不能声称成功。"}
	reply, e := summaryModel(cfg, 87654321, -23456000)(input)
	passed := e == nil && strings.Contains(reply, "SUM_LM_1029") && strings.Contains(reply, "周六") && strings.Contains(reply, "失败") && strings.Contains(reply, "小叶")
	result := map[string]any{"passed": passed, "realProvider": true, "realQQMessagesSent": false, "realChatDataRead": false, "reply": reply, "modelRequests": modelstats.Default.Read(), "checkedAt": time.Now().Format(time.RFC3339)}
	b, _ := json.MarshalIndent(result, "", "  ")
	if os.WriteFile(out, b, 0600) != nil {
		t.Fatal("cannot save test evidence")
	}
	if !passed {
		t.Fatal("real summary did not preserve required synthetic facts; inspect local evidence")
	}
}
