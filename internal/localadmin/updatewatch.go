package localadmin

import (
	core "github.com/FloatTech/ZeroBot-Plugin/internal/updatewatch"
	"net/http"
)

func (s *Server) updateWatch(w http.ResponseWriter, r *http.Request) {
	if s.cfg.UpdateWatch == nil {
		apiError(w, 409, "当前程序未接入更新监控")
		return
	}
	var req struct {
		Revision string       `json:"revision"`
		Config   *core.Config `json:"config"`
	}
	if r.Method == http.MethodPost {
		if !readJSONLimit(w, r, &req, 16384) {
			return
		}
		if !validRevision(req.Revision) || req.Config == nil {
			apiError(w, 400, "请先读取配置后再保存")
			return
		}
	}
	out, err := s.cfg.UpdateWatch(req.Revision, req.Config)
	if err != nil {
		apiError(w, 409, err.Error())
		return
	}
	jsonReply(w, 200, out)
}
