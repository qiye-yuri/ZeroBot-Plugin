package localadmin

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/FloatTech/zbputils/chat"
)

type AgentMemoryStore interface {
	View(context.Context, int64, int64, int64) (chat.MemoryView, error)
	ViewFiltered(context.Context, int64, int64, int64, string) (chat.MemoryView, error)
	ViewVersionsNewest(context.Context, int64, int64, int64) (chat.MemoryView, error)
	ViewRevision(context.Context, int64, int64, int64) (chat.MemoryView, error)
	SearchEnabled(context.Context, int64, string, int) (chat.MemorySearchPage, error)
	SearchEnabledBefore(context.Context, int64, string, int, int64) (chat.MemorySearchPage, error)
	SearchEnabledFromSource(context.Context, int64, string, int, int64, string) (chat.MemorySearchPage, error)
	SearchCurrent(context.Context, int64, string, int, int64, string, string) (chat.MemorySearchPage, error)
	SearchCurrentInRange(context.Context, int64, string, int, int64, string, string, string, string) (chat.MemorySearchPage, error)
	ReadEnabled(context.Context, int64, int64, int64) (chat.MemorySearchPage, error)
	ReadCurrent(context.Context, int64, int64) (chat.MemoryView, error)
	CreateManual(context.Context, int64, string) (chat.MemoryItem, bool, error)
	Update(context.Context, int64, int64, int64, string, string) error
	Revert(context.Context, int64, int64, int64, int64) error
	Preview(context.Context, int64) (chat.MemoryPreview, error)
	Migrate(context.Context, int64, string) error
}
type memoryRequest struct {
	BotID          string `json:"botId"`
	Type           string `json:"type"`
	ID             string `json:"id"`
	MemoryID       string `json:"memoryId"`
	Revision       string `json:"revision"`
	TargetRevision string `json:"targetRevision"`
	Action         string `json:"action"`
	Text           string `json:"text"`
	Ticket         string `json:"ticket"`
	Keyword        string `json:"keyword"`
	Before         string `json:"before"`
	Source         string `json:"source"`
	State          string `json:"state"`
	From           string `json:"from"`
	Until          string `json:"until"`
}
type memoryTicket struct {
	Bot, Type, ID, Digest string
	Expires               int64
}

func (s *Server) memory(w http.ResponseWriter, r *http.Request) {
	var req memoryRequest
	if r.Method == "POST" {
		if !readJSONLimit(w, r, &req, 8192) {
			return
		}
	} else {
		q := r.URL.Query()
		if q.Has("before") || q.Has("source") || q.Has("from") || q.Has("until") {
			apiError(w, 400, "记忆编号边界、来源与时间条件仅用于记忆搜索")
			return
		}
		req = memoryRequest{BotID: q.Get("botId"), Type: q.Get("type"), ID: q.Get("id"), MemoryID: q.Get("memoryId"), Revision: q.Get("revision")}
	}
	order := r.URL.Query().Get("order")
	if r.URL.Query().Has("order") && (r.Method != "GET" || r.URL.Path != "/api/agent/memories" || req.MemoryID == "" || (order != "" && order != "asc" && order != "desc")) {
		apiError(w, 400, "版本排序仅用于指定记忆的历史查询，支持 asc/desc")
		return
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
	search := r.URL.Path == "/api/agent/memories/search"
	if !search && (req.From != "" || req.Until != "") {
		apiError(w, 400, "修订时间条件仅用于记忆搜索")
		return
	}
	create := r.URL.Path == "/api/agent/memories/create"
	current := r.URL.Path == "/api/agent/memories/current"
	currentID := int64(0)
	if current {
		var valid bool
		currentID, valid = archiveID(req.MemoryID)
		if !valid || r.URL.Query().Has("after") || r.URL.Query().Has("state") || r.URL.Query().Has("revision") {
			apiError(w, 400, "定位当前记忆须提供记忆编号，不能与分页、状态或修订号混用")
			return
		}
	}
	if (!search && req.Source != "") || !chat.ValidMemorySearchSource(req.Source) {
		apiError(w, 400, "记忆搜索来源无效，仅支持 agent/admin/legacy 或留空不限")
		return
	}
	if !search && req.Before != "" {
		apiError(w, 400, "记忆编号边界不能与修订或迁移操作混用")
		return
	}
	if !search && req.State != "" {
		apiError(w, 400, "记忆搜索状态不能与修订或迁移操作混用")
		return
	}
	if create && (req.MemoryID != "" || req.Revision != "" || req.TargetRevision != "" || req.Action != "" || req.Ticket != "" || req.Keyword != "") {
		apiError(w, 400, "新增记忆仅接受正文与会话范围")
		return
	}
	before := int64(0)
	if search {
		start, end, rangeErr := chat.ParseArchiveSearchRange(req.From, req.Until)
		if rangeErr != nil {
			apiError(w, 400, "修订时间须带时区，开始须早于结束")
			return
		}
		if req.State == "" {
			req.State = "active"
		}
		if !chat.ValidMemorySearchState(req.State) {
			apiError(w, 400, "记忆搜索状态无效，仅支持 active/disabled/all")
			return
		}
		req.Keyword = strings.TrimSpace(req.Keyword)
		if (req.Keyword == "" && req.Source == "" && start.IsZero() && end.IsZero()) || utf8.RuneCountInString(req.Keyword) > 128 || req.MemoryID != "" || req.Revision != "" || req.Action != "" || req.TargetRevision != "" || req.Text != "" || req.Ticket != "" {
			apiError(w, 400, "须提供记忆搜索词、来源或修订时间，关键词最多 128 个字符，不能与修订操作混用")
			return
		}
		if req.Before != "" {
			var valid bool
			before, valid = archiveID(req.Before)
			if !valid {
				apiError(w, 400, "记忆编号边界须为正整数字符串")
				return
			}
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	s.mu.Lock()
	if s.archiveActive >= 2 || (search && s.now().Sub(s.archiveLastSearch) < 250*time.Millisecond) {
		s.mu.Unlock()
		apiError(w, 429, "档案与记忆操作繁忙，请稍后重试")
		return
	}
	s.archiveActive++
	if search {
		s.archiveLastSearch = s.now()
	}
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.archiveActive--; s.mu.Unlock() }()
	var store AgentMemoryStore
	var err error
	if s.cfg.AgentMemory != nil {
		store, err = s.cfg.AgentMemory(bot)
	} else {
		store, err = chat.AgentMemories(bot)
	}
	if err != nil {
		memoryError(w, err)
		return
	}
	if create {
		item, created, e := store.CreateManual(ctx, grp, req.Text)
		if e != nil {
			memoryError(w, e)
			return
		}
		jsonReply(w, 200, map[string]any{"saved": true, "created": created, "item": item})
		return
	}
	if current {
		value, e := store.ReadCurrent(ctx, grp, currentID)
		if e != nil {
			memoryError(w, e)
			return
		}
		jsonReply(w, 200, value)
		return
	}
	if search {
		v, e := store.SearchCurrentInRange(ctx, grp, req.Keyword, 20, before, req.Source, req.State, req.From, req.Until)
		if e != nil {
			memoryError(w, e)
			return
		}
		jsonReply(w, 200, v)
		return
	}
	if r.URL.Path == "/api/agent/memories/read" || r.URL.Path == "/api/agent/memories/revision" {
		id, ok := archiveID(req.MemoryID)
		rev, rok := archiveID(req.Revision)
		if !ok || !rok || r.URL.Query().Has("after") || r.URL.Query().Has("state") {
			apiError(w, 400, "精确读取须提供记忆编号和修订号，不能与分页或筛选混用")
			return
		}
		var v any
		var e error
		if r.URL.Path == "/api/agent/memories/revision" {
			v, e = store.ViewRevision(ctx, grp, id, rev)
		} else {
			v, e = store.ReadEnabled(ctx, grp, id, rev)
		}
		if e != nil {
			memoryError(w, e)
			return
		}
		jsonReply(w, 200, v)
		return
	}
	if r.Method == "GET" {
		after := int64(0)
		if text := r.URL.Query().Get("after"); text != "" {
			var ok bool
			after, ok = archiveID(text)
			if !ok {
				apiError(w, 400, "分页参数无效")
				return
			}
		}
		id := int64(0)
		if req.MemoryID != "" {
			var ok bool
			id, ok = archiveID(req.MemoryID)
			if !ok {
				apiError(w, 400, "记忆编号无效")
				return
			}
		}
		state := r.URL.Query().Get("state")
		if state == "" {
			state = "all"
		}
		if (state != "all" && state != "active" && state != "disabled") || (id != 0 && state != "all") {
			apiError(w, 400, "记忆筛选无效，历史版本不支持启停筛选")
			return
		}
		var v chat.MemoryView
		var e error
		if order == "desc" {
			v, e = store.ViewVersionsNewest(ctx, grp, id, after)
		} else {
			v, e = store.ViewFiltered(ctx, grp, after, id, state)
		}
		if e != nil {
			memoryError(w, e)
			return
		}
		jsonReply(w, 200, v)
		return
	}
	if r.URL.Path == "/api/agent/memories/update" {
		id, ok := archiveID(req.MemoryID)
		rev, rok := archiveID(req.Revision)
		if !ok || !rok {
			apiError(w, 400, "记忆编号或版本无效")
			return
		}
		var e error
		if req.Action == "revert" {
			target, valid := archiveID(req.TargetRevision)
			if !valid || target >= rev || req.Text != "" {
				apiError(w, 400, "历史版本参数无效")
				return
			}
			e = store.Revert(ctx, grp, id, rev, target)
		} else {
			if req.TargetRevision != "" {
				apiError(w, 400, "当前操作不接受历史版本参数")
				return
			}
			e = store.Update(ctx, grp, id, rev, req.Action, req.Text)
		}
		if e != nil {
			memoryError(w, e)
			return
		}
		jsonReply(w, 200, map[string]bool{"saved": true})
		return
	}
	s.mu.Lock()
	_, session, valid := s.currentSession(r)
	s.mu.Unlock()
	if !valid {
		apiError(w, 401, "请重新登录")
		return
	}
	sign := func(raw []byte) []byte {
		h := hmac.New(sha256.New, s.archiveKey[:])
		h.Write([]byte(session.CSRF))
		h.Write([]byte("memory-migration:"))
		h.Write(raw)
		return h.Sum(nil)
	}
	if r.URL.Path == "/api/agent/memories/migration-preview" {
		p, e := store.Preview(ctx, grp)
		if e != nil {
			memoryError(w, e)
			return
		}
		expires := s.now().Add(5 * time.Minute).Unix()
		data, _ := json.Marshal(memoryTicket{req.BotID, req.Type, req.ID, p.Digest, expires})
		ticket := base64.RawURLEncoding.EncodeToString(data) + "." + base64.RawURLEncoding.EncodeToString(sign(data))
		jsonReply(w, 200, map[string]any{"count": p.Count, "bytes": p.Bytes, "mode": p.Mode, "ticket": ticket, "expiresAt": time.Unix(expires, 0).UTC().Format(time.RFC3339)})
		return
	}
	parts := strings.Split(req.Ticket, ".")
	if len(parts) != 2 || len(req.Ticket) > 2048 {
		apiError(w, 400, "迁移预览无效")
		return
	}
	data, e := base64.RawURLEncoding.DecodeString(parts[0])
	mac, e2 := base64.RawURLEncoding.DecodeString(parts[1])
	var ticket memoryTicket
	if e != nil || e2 != nil || !hmac.Equal(mac, sign(data)) || json.Unmarshal(data, &ticket) != nil || ticket.Bot != req.BotID || ticket.Type != req.Type || ticket.ID != req.ID || ticket.Expires <= s.now().Unix() {
		apiError(w, 409, "迁移预览已失效，请重新预览")
		return
	}
	if e = store.Migrate(ctx, grp, ticket.Digest); e != nil {
		memoryError(w, e)
		return
	}
	jsonReply(w, 200, map[string]bool{"migrated": true})
}
func memoryError(w http.ResponseWriter, err error) {
	status := 503
	text := "记忆操作未完成，请重新读取后核对"
	if errors.Is(err, chat.ErrMemoryConflict) {
		status = 409
		text = chat.ErrMemoryConflict.Error()
	}
	if errors.Is(err, chat.ErrMemoryInput) {
		status = 400
		text = chat.ErrMemoryInput.Error()
	}
	apiError(w, status, text)
}
