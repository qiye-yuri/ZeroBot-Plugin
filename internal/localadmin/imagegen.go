package localadmin

import (
	"context"
	"fmt"
	core "github.com/FloatTech/ZeroBot-Plugin/internal/imagegen"
	sql "github.com/FloatTech/sqlite"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func (s *Server) imageSettings(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ImageGen == nil {
		apiError(w, 409, "当前程序未接入画图配置")
		return
	}
	var next *core.Update
	if r.Method == http.MethodPost {
		next = &core.Update{}
		if !readJSONLimit(w, r, next, 16384) {
			return
		}
		if !validRevision(next.Revision) {
			apiError(w, 400, "请先读取配置后再保存")
			return
		}
	}
	out, err := s.cfg.ImageGen(next, func(db *sql.Sqlite) error {
		dir := filepath.Join(s.cfg.DataDir, "imagegen-backups")
		if e := os.MkdirAll(dir, 0700); e != nil {
			return e
		}
		if e := protectPath(dir, true); e != nil {
			return e
		}
		path := filepath.Join(dir, time.Now().UTC().Format("20060102T150405.000000000")+".sqlite")
		if e := writeNewPrivateFile(path, nil); e != nil {
			return e
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if _, e := db.ExecContext(ctx, "VACUUM INTO ?", path); e != nil {
			return fmt.Errorf("画图快照失败")
		}
		return nil
	})
	if err != nil {
		apiError(w, 409, err.Error())
		return
	}
	jsonReply(w, 200, out)
}
