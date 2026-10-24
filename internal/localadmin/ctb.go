package localadmin

import (
	"github.com/FloatTech/ZeroBot-Plugin/internal/ctb"
	"net/http"
)

func (s *Server) ctbSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Revision string      `json:"revision"`
		Config   *ctb.Config `json:"config"`
	}
	if r.Method == http.MethodPost {
		if !readJSONLimit(w, r, &req, 16384) {
			return
		}
		if !validRevision(req.Revision) || req.Config == nil {
			apiError(w, 400, "请先读取配置")
			return
		}
	}
	v, e := ctb.Default.Settings(req.Revision, req.Config)
	if e != nil {
		apiError(w, 409, e.Error())
		return
	}
	jsonReply(w, 200, v)
}
