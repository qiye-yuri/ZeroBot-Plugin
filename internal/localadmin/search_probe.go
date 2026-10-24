package localadmin

import (
	"net/http"
	"time"

	"github.com/FloatTech/zbputils/chat"
)

const searchProbeQuery = "Go 编程语言 官方文档"

type searchProbeResult struct {
	Revision     string `json:"revision"`
	CheckedAt    string `json:"checkedAt"`
	Milliseconds int64  `json:"milliseconds"`
	Category     string `json:"category"`
	HTTPStatus   int    `json:"httpStatus,omitempty"`
	ResultCount  int    `json:"resultCount"`
	Message      string `json:"message"`
}

func (s *Server) testAISearch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Revision string `json:"revision"`
	}
	if !readJSONLimit(w, r, &req, 2048) {
		return
	}
	if !validRevision(req.Revision) {
		apiError(w, 400, "请重新读取有效的搜索设置版本")
		return
	}
	if s.cfg.AI == nil || s.cfg.AI.repository == nil {
		apiError(w, 409, "当前程序未接入 AI 设置")
		return
	}
	cfg, revision, err := s.cfg.AI.repository.Read()
	if err != nil {
		apiError(w, 500, "搜索设置读取失败")
		return
	}
	if revision != req.Revision {
		apiError(w, 409, "搜索配置已变化，请重新读取后测试")
		return
	}
	if cfg.WebSearch == nil || cfg.WebSearch.Key == "" {
		apiError(w, 400, "尚未保存搜索密钥，请先保存后测试")
		return
	}
	if !s.beginAIProbe(w) {
		return
	}
	defer s.endAIProbe()
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(15 * time.Second))
	start := time.Now()
	page, diagnostic := chat.SearchWeb(r.Context(), &http.Client{Timeout: 10 * time.Second}, cfg.WebSearch.Key, searchProbeQuery)
	result := searchProbeResult{Revision: revision, CheckedAt: start.UTC().Format(time.RFC3339Nano), Milliseconds: time.Since(start).Milliseconds(), Category: diagnostic.Category, HTTPStatus: diagnostic.HTTPStatus, ResultCount: len(page.Results)}
	result.Message = map[string]string{
		"success":          "本次搜索基础调用通过；不代表已启用会话搜索",
		"empty":            "本次调用完成，没有匹配结果；不代表认证失败或资料不存在",
		"http_error":       "搜索服务拒绝请求或暂不可用，请检查凭据、余额或额度",
		"network_error":    "搜索网络连接、读取失败或超时，本次未取得可验证资料",
		"invalid_response": "搜索响应格式或来源链接无效，不能判断为没有结果",
		"response_limit":   "搜索响应超过256 KiB，本次测试失败",
	}[diagnostic.Category]
	jsonReply(w, 200, result)
}
