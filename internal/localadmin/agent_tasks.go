package localadmin

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/FloatTech/zbputils/chat"
)

type AgentTaskStore interface {
	Page(context.Context, int64, int64) (chat.AgentTaskPage, error)
	PageFiltered(context.Context, int64, int64, string) (chat.AgentTaskPage, error)
	PageWithReason(context.Context, int64, int64, string, string) (chat.AgentTaskPage, error)
	PageWithFilters(context.Context, int64, int64, string, string, string) (chat.AgentTaskPage, error)
	PageInRange(context.Context, int64, int64, string, string, string, string, string) (chat.AgentTaskPage, error)
	PageMatching(context.Context, int64, int64, chat.TaskPageFilter) (chat.AgentTaskPage, error)
	Read(context.Context, int64, string) (chat.AgentTaskInfo, error)
	Cancel(context.Context, int64, string, string) (chat.AgentTaskInfo, error)
}

// Observation time belongs to this HTTP response, not the persisted task.
type agentTaskSnapshot struct {
	chat.AgentTaskInfo
	ObservedAt string `json:"observedAt"`
}

type agentTaskPageSnapshot struct {
	chat.AgentTaskPage
	ObservedAt string `json:"observedAt"`
	From       string `json:"from,omitempty"`
	Until      string `json:"until,omitempty"`
}

var agentTaskIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func (s *Server) agentTasks(w http.ResponseWriter, r *http.Request) {
	readMethod := r.Method == http.MethodGet || r.Method == http.MethodHead
	var req struct {
		BotID   string `json:"botId"`
		Type    string `json:"type"`
		ID      string `json:"id"`
		TaskID  string `json:"taskId"`
		Version string `json:"version"`
	}
	if r.Method == "POST" {
		if !readJSONLimit(w, r, &req, 2048) {
			return
		}
	} else {
		q := r.URL.Query()
		req.BotID = q.Get("botId")
		req.Type = q.Get("type")
		req.ID = q.Get("id")
		req.TaskID = q.Get("taskId")
	}
	read := r.URL.Path == "/api/agent/tasks/read"
	if readMethod {
		q := r.URL.Query()
		if (read && (!agentTaskIDPattern.MatchString(req.TaskID) || q.Has("before") || q.Has("state") || q.Has("reason") || q.Has("phase") || q.Has("outcome") || q.Has("from") || q.Has("until") || q.Has("version"))) || (!read && q.Has("taskId")) {
			apiError(w, 400, "单条任务读取须提供任务编号，不能与分页、列表筛选或版本混用")
			return
		}
	}
	bot, bok := archiveID(req.BotID)
	grp, gok := archiveID(req.ID)
	if !bok || !gok || (req.Type != "group" && req.Type != "private") {
		apiError(w, 400, "机器人或会话范围无效")
		return
	}
	if req.Type == "private" {
		grp = -grp
	}
	known := false
	for _, id := range s.cfg.Overview().BotAccounts {
		if id == req.BotID {
			known = true
			break
		}
	}
	if !known {
		apiError(w, 404, "机器人当前未连接")
		return
	}
	if r.Method == "POST" {
		_, ok := archiveID(req.Version)
		if !ok || !agentTaskIDPattern.MatchString(req.TaskID) {
			apiError(w, 400, "任务编号或版本无效")
			return
		}
	}
	before := int64(0)
	state := "all"
	reason := "all"
	phase := "all"
	outcome := "all"
	if values, exists := r.URL.Query()["outcome"]; exists {
		if !readMethod || len(values) != 1 || !chat.ValidTaskOutcomeFilter(values[0]) {
			apiError(w, 400, "最近动作结果筛选无效，仅用于任务列表")
			return
		}
		outcome = values[0]
	}
	from, until := "", ""
	for _, bound := range []struct {
		name  string
		value *string
	}{{"from", &from}, {"until", &until}} {
		if values, exists := r.URL.Query()[bound.name]; exists {
			if !readMethod || len(values) != 1 || values[0] == "" {
				apiError(w, 400, "任务开始时间范围无效，仅用于任务列表")
				return
			}
			*bound.value = values[0]
		}
	}
	start, end, rangeErr := chat.ParseArchiveSearchRange(from, until)
	if rangeErr != nil {
		apiError(w, 400, "任务开始时间须为绝对时间，开始须早于结束")
		return
	}
	if !start.IsZero() {
		from = start.Format(time.RFC3339Nano)
	}
	if !end.IsZero() {
		until = end.Format(time.RFC3339Nano)
	}
	if values, exists := r.URL.Query()["phase"]; exists {
		if !readMethod || len(values) != 1 || !chat.ValidTaskPhaseFilter(values[0]) {
			apiError(w, 400, "任务阶段筛选无效，仅用于任务列表")
			return
		}
		phase = values[0]
	}
	if r.URL.Query().Has("reason") {
		values := r.URL.Query()["reason"]
		if !readMethod || len(values) != 1 || !chat.ValidTaskReasonFilter(values[0]) {
			apiError(w, 400, "任务错误类别筛选无效，仅用于任务列表")
			return
		}
		reason = values[0]
	}
	if readMethod {
		if value := r.URL.Query().Get("state"); value != "" {
			state = value
		}
		if !chat.ValidTaskFilter(state) {
			apiError(w, 400, "任务状态筛选无效")
			return
		}
	}
	if readMethod && r.URL.Query().Has("before") {
		var ok bool
		before, ok = archiveID(r.URL.Query().Get("before"))
		if !ok {
			apiError(w, 400, "任务分页参数无效")
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	s.mu.Lock()
	if s.archiveActive >= 2 {
		s.mu.Unlock()
		apiError(w, 429, "Agent 操作繁忙，请稍后重试")
		return
	}
	s.archiveActive++
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.archiveActive--; s.mu.Unlock() }()
	var store AgentTaskStore
	var err error
	if s.cfg.AgentTasks != nil {
		store, err = s.cfg.AgentTasks(bot)
	} else {
		store, err = chat.AgentTasks(bot)
	}
	if err != nil {
		agentTaskError(w, err)
		return
	}
	if readMethod {
		if read {
			value, e := store.Read(ctx, grp, req.TaskID)
			if e != nil {
				agentTaskError(w, e)
				return
			}
			jsonReply(w, 200, agentTaskSnapshot{value, s.now().UTC().Format(time.RFC3339Nano)})
			return
		}
		page, e := store.PageMatching(ctx, grp, before, chat.TaskPageFilter{State: state, Reason: reason, Phase: phase, Outcome: outcome, From: from, Until: until})
		if e != nil {
			agentTaskError(w, e)
			return
		}
		jsonReply(w, 200, agentTaskPageSnapshot{page, s.now().UTC().Format(time.RFC3339Nano), from, until})
		return
	}
	value, e := store.Cancel(ctx, grp, req.TaskID, req.Version)
	if e != nil {
		agentTaskError(w, e)
		return
	}
	jsonReply(w, 200, agentTaskSnapshot{value, s.now().UTC().Format(time.RFC3339Nano)})
}
func agentTaskError(w http.ResponseWriter, e error) {
	status := 503
	message := "任务状态未确认，请重新读取；不会自动重试停止请求"
	if errors.Is(e, chat.ErrAgentTaskMissing) {
		status = 404
		message = "任务不存在"
	}
	if errors.Is(e, chat.ErrAgentTaskConflict) {
		status = 409
		message = "任务版本已变化，请重新读取"
	}
	apiError(w, status, message)
}
