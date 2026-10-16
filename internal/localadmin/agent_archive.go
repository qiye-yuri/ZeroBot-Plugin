package localadmin

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/FloatTech/zbputils/chat"
)

type archiveRequest struct {
	BotID     string          `json:"botId"`
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Cursor    string          `json:"cursor"`
	Keyword   string          `json:"keyword"`
	Archived  bool            `json:"archived"`
	Seq       string          `json:"seq"`
	Before    string          `json:"before"`
	From      string          `json:"from"`
	Until     string          `json:"until"`
	Sender    json.RawMessage `json:"sender"`
	EventType string          `json:"eventType"`
}
type archiveCursor struct {
	Bot, Type, ID           string
	Archived                bool
	Snapshot, Next, Expires int64
}

func archiveID(s string) (int64, bool) {
	n, e := strconv.ParseInt(s, 10, 64)
	return n, e == nil && n > 0 && strconv.FormatInt(n, 10) == s
}

func (s *Server) archive(w http.ResponseWriter, r *http.Request) {
	var req archiveRequest
	search := r.Method == http.MethodPost
	list := r.URL.Path == "/api/agent/conversations"
	radius := 0
	hasContext := false
	if search {
		if !readJSON(w, r, &req) {
			return
		}
	} else {
		q := r.URL.Query()
		hasContext = q.Has("context")
		if hasContext {
			value := q.Get("context")
			if len(q["context"]) != 1 || len(value) != 1 || value[0] < '0' || value[0] > '3' || len(q["seq"]) != 1 || list || q.Get("cursor") != "" || q.Get("archived") == "true" {
				apiError(w, 400, "前后事件须指定单条来源，范围为 0 至 3，且不能包含重置前记录或分页")
				return
			}
			radius = int(value[0] - '0')
		}
		if q.Has("before") || q.Has("from") || q.Has("until") || q.Has("sender") || q.Has("eventType") {
			apiError(w, 400, "before/from/until/sender/eventType 仅用于文本搜索")
			return
		}
		req = archiveRequest{BotID: q.Get("botId"), Type: q.Get("type"), ID: q.Get("id"), Cursor: q.Get("cursor"), Archived: q.Get("archived") == "true"}
		req.Seq = q.Get("seq")
		if q.Has("seq") && req.Seq == "" {
			apiError(w, 400, "来源序号无效")
			return
		}
		if a := q.Get("archived"); a != "" && a != "true" && a != "false" {
			apiError(w, 400, "查询参数无效")
			return
		}
	}
	bot, ok := archiveID(req.BotID)
	if !ok {
		apiError(w, 400, "请选择有效机器人账号")
		return
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
	q := chat.ArchiveQuery{Search: search, Keyword: strings.TrimSpace(req.Keyword), IncludeResetHistory: req.Archived}
	if !chat.ValidArchiveEventType(req.EventType) {
		apiError(w, 400, "事件类型无效，仅支持 message/notice/request 或留空不限")
		return
	}
	q.SearchEventType = req.EventType
	if len(req.Sender) > 0 {
		var text string
		if string(req.Sender) == "null" || json.Unmarshal(req.Sender, &text) != nil {
			apiError(w, 400, "发言者须为 QQ 号字符串，或留空不限")
			return
		}
		if text != "" {
			sender, valid := archiveID(text)
			if !search || !valid {
				apiError(w, 400, "发言者须为正整数 QQ 号字符串，仅用于文本搜索")
				return
			}
			q.SearchSender = sender
		}
	}
	var rangeErr error
	q.SearchFrom, q.SearchUntil, rangeErr = chat.ParseArchiveSearchRange(req.From, req.Until)
	if rangeErr != nil {
		apiError(w, 400, "搜索时间须为带时区的 RFC3339 时间，开始须早于结束")
		return
	}
	if req.Before != "" {
		before, valid := archiveID(req.Before)
		if !search || !valid {
			apiError(w, 400, "搜索边界序号无效")
			return
		}
		q.Before = before
	}
	if req.Seq != "" {
		seq, valid := archiveID(req.Seq)
		if !valid || list || search || req.Cursor != "" {
			apiError(w, 400, "单条档案须提供有效序号，不能与列表、搜索或分页混用")
			return
		}
		q.Sequence = seq
	}
	if hasContext {
		q.Context = radius
		q.EventsOnly = true
	}
	if !list {
		id, ok := archiveID(req.ID)
		if !ok || (req.Type != "group" && req.Type != "private") {
			apiError(w, 400, "会话范围无效")
			return
		}
		q.Conversation = id
		if req.Type == "private" {
			q.Conversation = -id
		}
	} else if req.Type != "" || req.ID != "" || req.Archived {
		apiError(w, 400, "列表范围无效")
		return
	}
	if search && ((q.Keyword == "" && q.SearchSender == 0 && q.SearchFrom.IsZero() && q.SearchUntil.IsZero() && q.SearchEventType == "") || utf8.RuneCountInString(q.Keyword) > 128 || req.Cursor != "") {
		apiError(w, 400, "请填写关键词、发言者、时间或事件类型，关键词最多 128 个字符，搜索不使用分页游标")
		return
	}
	// Bind cursors to the authenticated session as well as bot/scope/reset mode.
	s.mu.Lock()
	_, session, valid := s.currentSession(r)
	s.mu.Unlock()
	if !valid {
		apiError(w, 401, "请重新登录")
		return
	}
	sign := func(raw []byte) []byte {
		m := hmac.New(sha256.New, s.archiveKey[:])
		m.Write([]byte(session.CSRF))
		m.Write(raw)
		return m.Sum(nil)
	}
	if req.Cursor != "" {
		parts := strings.Split(req.Cursor, ".")
		var c archiveCursor
		if len(parts) != 2 || len(req.Cursor) > 2048 {
			apiError(w, 400, "分页游标无效")
			return
		}
		data, e := base64.RawURLEncoding.DecodeString(parts[0])
		mac, e2 := base64.RawURLEncoding.DecodeString(parts[1])
		if e != nil || e2 != nil || !hmac.Equal(mac, sign(data)) || json.Unmarshal(data, &c) != nil || c.Bot != req.BotID || c.Type != req.Type || c.ID != req.ID || c.Archived != req.Archived || c.Expires <= s.now().Unix() || c.Snapshot <= 0 {
			apiError(w, 409, "分页已失效，请重新读取")
			return
		}
		q.Snapshot = c.Snapshot
		if list {
			q.HasAfter = true
			q.AfterConversation = c.Next
		} else {
			q.Before = c.Next
		}
	}
	s.mu.Lock()
	if s.archiveActive >= 2 || (search && s.now().Sub(s.archiveLastSearch) < 250*time.Millisecond) {
		s.mu.Unlock()
		apiError(w, 429, "档案查询繁忙，请稍后重试")
		return
	}
	s.archiveActive++
	if search {
		s.archiveLastSearch = s.now()
	}
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.archiveActive--; s.mu.Unlock() }()
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	reader := s.cfg.AgentArchive
	if reader == nil {
		reader = chat.ReadAgentArchive
	}
	page, err := reader(ctx, bot, q)
	if err != nil {
		apiError(w, 503, "会话档案暂不可读取，请稍后重试")
		return
	}
	next := ""
	if page.More && !search && q.Sequence == 0 {
		data, _ := json.Marshal(archiveCursor{req.BotID, req.Type, req.ID, req.Archived, page.Snapshot, page.Next, s.now().Add(15 * time.Minute).Unix()})
		next = base64.RawURLEncoding.EncodeToString(data) + "." + base64.RawURLEncoding.EncodeToString(sign(data))
	}
	anchor := ""
	appliedSender := ""
	if q.SearchSender > 0 {
		appliedSender = strconv.FormatInt(q.SearchSender, 10)
	}
	if radius > 0 {
		anchor = req.Seq
	}
	jsonReply(w, 200, struct {
		chat.ArchivePage
		NextCursor string `json:"nextCursor"`
		BotID      string `json:"botId"`
		Type       string `json:"type"`
		ID         string `json:"id"`
		AnchorSeq  string `json:"anchorSeq,omitempty"`
		Context    int    `json:"context,omitempty"`
		Sender     string `json:"sender,omitempty"`
		EventType  string `json:"eventType,omitempty"`
	}{page, next, req.BotID, req.Type, req.ID, anchor, radius, appliedSender, q.SearchEventType})
}
