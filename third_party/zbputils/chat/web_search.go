package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

const deepSeekSearchEndpoint = "https://api.deepseek.com/v1/tools/search"
const webSearchLimit = 2

type WebSearchResult struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type WebSearchPage struct {
	Status  string            `json:"status"`
	Results []WebSearchResult `json:"results"`
	Message string            `json:"message,omitempty"`
}

// WebSearch is scoped to one task. It has its own quota, independent of local
// history/memory queries, and never receives conversation text or QQ IDs.
type WebSearch struct {
	allowed      bool
	key          ModelKey
	conversation int64
	task         *AgentTask
	usage        TaskWebSearch
	client       *http.Client
	sources      []WebSearchResult
	outcome      string
	lastMessage  string
}

func NewTaskWebSearch(c AgentConfig, bot, conversation int64, role goba.PermRole, task *AgentTask) *WebSearch {
	key := ModelKey("")
	if c.WebSearch != nil {
		key = c.WebSearch.Key
	}
	return &WebSearch{allowed: c.SearchAllowed(bot, conversation, role), key: key, conversation: conversation, task: task, usage: TaskWebSearch{Available: true, Limit: webSearchLimit}, client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (s *WebSearch) Allowed() bool { return s.allowed }

func (s *WebSearch) Outcome() string {
	if s.outcome == "ok" || s.outcome == "empty" {
		return "ok"
	}
	return "local_error"
}

// ReplyIfUnsent covers an Agent that ends after searching without sending an
// answer. Keep the actual results instead of falling back to unrelated chat.
func (s *WebSearch) ReplyIfUnsent() string {
	if s.outcome == "" {
		return ""
	}
	if len(s.sources) == 0 {
		return "本次未取得搜索资料：" + s.lastMessage
	}
	var reply strings.Builder
	reply.WriteString("搜索结果（来源摘要，未抓取网页全文）：")
	for _, source := range s.sources {
		fmt.Fprintf(&reply, "\n\n%d. %s\n%s\n%s", source.ID, source.Title, source.Snippet, source.URL)
	}
	return reply.String()
}

// WithSources appends the verified URLs when the model's answer omitted them.
// It retains the original target and message segments and never fetches a URL.
func (s *WebSearch) WithSources(req zero.APIRequest) zero.APIRequest {
	if len(s.sources) == 0 || (req.Action != "send_private_msg" && req.Action != "send_group_msg") {
		return req
	}
	raw, err := json.Marshal(req.Params["message"])
	if err != nil {
		return req
	}
	segments := message.ParseMessage(raw)
	text := segments.ExtractPlainText()
	var footer strings.Builder
	for _, source := range s.sources {
		if !strings.Contains(text, source.URL) {
			if footer.Len() == 0 {
				footer.WriteString("\n\n搜索来源：")
			}
			fmt.Fprintf(&footer, "\n%d. %s %s", source.ID, source.Title, source.URL)
		}
	}
	if footer.Len() > 0 {
		segments = append(segments, message.Text(footer.String()))
		req.Params["message"] = segments
	}
	return req
}

func (s *WebSearch) Execute(ag *goba.Agent, req zero.APIRequest, check func() error) (err error) {
	s.outcome = "error"
	if req.Action != goba.SearchWeb || !s.allowed {
		return goba.ErrPermissionDenied
	}
	if err = check(); err != nil {
		return err
	}
	if err = ag.AddRequest(s.conversation, &req); err != nil {
		return err
	}
	page := WebSearchPage{Status: "error", Results: []WebSearchResult{}}
	query, valid := req.Params["query"].(string)
	valid = valid && len(req.Params) == 1 && strings.TrimSpace(query) != "" && utf8.ValidString(query) && utf8.RuneCountInString(query) <= 128
	if !valid {
		s.usage.Denied++
		page.Message = "搜索参数仅允许1至128字符的query"
	} else if s.usage.Used >= webSearchLimit {
		s.usage.Denied++
		page.Message = "本任务已达两次搜索上限"
	} else {
		s.usage.Used++
		if err = s.record(); err != nil {
			return err
		}
		page = s.search(query)
		if page.Status == "error" {
			s.usage.Errors++
		} else {
			s.usage.Successful++
			if page.Status == "empty" {
				s.usage.Empty++
			}
		}
	}
	s.outcome = page.Status
	// An admitted request can finish after stop/reload; preserve counts, then
	// discard its content instead of starting another model round or send.
	if err = s.record(); err != nil {
		return err
	}
	if err = check(); err != nil {
		return err
	}
	s.lastMessage = page.Message
	if page.Status == "ok" {
		for _, source := range page.Results {
			exists := false
			for _, saved := range s.sources {
				if saved.URL == source.URL {
					exists = true
					break
				}
			}
			if !exists {
				source.ID = len(s.sources) + 1
				s.sources = append(s.sources, source)
			}
		}
	}
	data, err := json.Marshal(page)
	if err != nil {
		return err
	}
	status := "ok"
	if page.Status == "error" {
		status = "error"
	}
	return ag.AddResponse(s.conversation, &goba.APIResponse{Status: status, Data: data, Message: page.Message})
}

func (s *WebSearch) record() error {
	if s.task == nil {
		return nil
	}
	return s.task.captureWebSearch(s.usage)
}

// WebSearchDiagnostic contains only safe transport and parse metadata.
type WebSearchDiagnostic struct {
	Category   string
	HTTPStatus int
}

func (s *WebSearch) search(query string) WebSearchPage {
	page, _ := SearchWeb(context.Background(), s.client, s.key, query)
	return page
}

// SearchWeb performs one fixed-endpoint request without task, quota or reply effects.
// Callers supply the query and key; the deadline and source limits match Agent search.
func SearchWeb(ctx context.Context, client *http.Client, key ModelKey, query string) (WebSearchPage, WebSearchDiagnostic) {
	diagnostic := WebSearchDiagnostic{Category: "invalid_response"}
	failure := func(category, message string) (WebSearchPage, WebSearchDiagnostic) {
		diagnostic.Category = category
		return WebSearchPage{Status: "error", Results: []WebSearchResult{}, Message: message}, diagnostic
	}
	body, _ := json.Marshal(map[string]any{"query": query, "count": 5, "need_content": false})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deepSeekSearchEndpoint, bytes.NewReader(body))
	if err != nil {
		return failure("invalid_response", "搜索请求创建失败")
	}
	req.Header.Set("Authorization", "Bearer "+string(key))
	req.Header.Set("Content-Type", "application/json")
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := copyClient.Do(req)
	if err != nil {
		return failure("network_error", "搜索连接失败或超时，本轮未取得资料")
	}
	defer resp.Body.Close()
	diagnostic.HTTPStatus = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		return failure("http_error", "搜索服务拒绝请求或暂不可用，请检查凭据、余额或额度")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024+1))
	if err != nil {
		return failure("network_error", "搜索响应读取失败或超过256 KiB")
	}
	if len(raw) > 256*1024 {
		return failure("response_limit", "搜索响应读取失败或超过256 KiB")
	}
	var response struct {
		Results *[]struct{ Title, URL, Snippet, Summary string } `json:"results"`
	}
	if json.Unmarshal(raw, &response) != nil || response.Results == nil {
		return failure("invalid_response", "搜索响应格式无效，不能判断为没有结果")
	}
	page := WebSearchPage{Status: "ok", Results: []WebSearchResult{}}
	for _, item := range *response.Results {
		if len(page.Results) == 5 {
			break
		}
		u, e := url.Parse(item.URL)
		if e != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || len(item.URL) > 2048 {
			continue
		}
		snippet := item.Snippet
		if snippet == "" {
			snippet = item.Summary
		}
		page.Results = append(page.Results, WebSearchResult{ID: len(page.Results) + 1, Title: clipSearchText(item.Title, 256), URL: item.URL, Snippet: clipSearchText(snippet, 1600)})
	}
	if len(page.Results) == 0 {
		if len(*response.Results) > 0 {
			return failure("invalid_response", "搜索结果没有可用的来源链接")
		}
		page.Status = "empty"
		page.Message = "本次没有找到匹配结果，不能据此断言资料不存在"
	}
	diagnostic.Category = "success"
	if page.Status == "empty" {
		diagnostic.Category = "empty"
	}
	return page, diagnostic
}

func clipSearchText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	text = text[:limit]
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return text
}

var errWebSearchUsage = errors.New("搜索任务使用记录无效")
