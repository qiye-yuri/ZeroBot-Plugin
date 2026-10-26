package localadmin

import (
	"context"
	"net/http"
	"time"
)

func (s *Server) resources(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Resources == nil {
		apiError(w, 409, "当前程序未接入资源检查")
		return
	}
	if r.Method == http.MethodGet {
		jsonReply(w, 200, s.cfg.Resources.Read())
		return
	}
	// No paths, source URLs or plugin selections are accepted from the caller.
	var body struct{}
	if !readJSONLimit(w, r, &body, 1024) {
		return
	}
	// Scans may take longer than the server-wide 10s response deadline.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(65 * time.Second))
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	out, err := s.cfg.Resources.Check(ctx)
	if err != nil {
		apiError(w, 409, "检查未完成或正在进行，请读取上次结果后重试")
		return
	}
	jsonReply(w, 200, out)
}
