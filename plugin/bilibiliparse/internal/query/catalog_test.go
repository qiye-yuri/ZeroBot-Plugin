package query

import (
	"context"
	"github.com/jinzhu/gorm"
	_ "modernc.org/sqlite"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCatalogUpdate(t *testing.T) {
	t.Run("first-use-fallback-upsert", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "vup.db")
		good := `[{"mid":123,"uname":"original","roomid":4}]`
		calls := map[string]int{}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls[r.URL.Path]++
			if r.URL.Path == "/bad" {
				w.WriteHeader(503)
				return
			}
			w.Write([]byte(good))
		}))
		defer srv.Close()
		client := newDanmakuClient()
		defer client.CloseIdleConnections()
		if err := updateVupFrom(context.Background(), client, path, []string{srv.URL + "/bad", srv.URL + "/good", srv.URL + "/unused"}); err != nil {
			t.Fatal(err)
		}
		if calls["/bad"] != 1 || calls["/good"] != 1 || calls["/unused"] != 0 {
			t.Fatalf("fallback=%v", calls)
		}
		good = `[{"mid":123,"uname":"renamed","roomid":5}]`
		if err := updateVupFrom(context.Background(), client, path, []string{srv.URL + "/good"}); err != nil {
			t.Fatal(err)
		}
		db, err := initializeVup(path)
		if err != nil {
			t.Fatal(err)
		}
		defer (*gorm.DB)(db).Close()
		rows, err := db.filterVup([]int64{123})
		if err != nil || len(rows) != 1 || rows[0].Uname != "renamed" || rows[0].Roomid != 5 {
			t.Fatalf("rows=%v err=%v", rows, err)
		}
	})
	for _, body := range []string{"", `{}`, `[]`, `[{"mid":0,"uname":"invalid"}]`, `[{"mid":1}]`, `[{"mid":1,"uname":"ok"},`} {
		t.Run("reject-"+body, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "vup.db")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
			defer srv.Close()
			if err := updateVupFrom(context.Background(), srv.Client(), path, []string{srv.URL}); err == nil {
				t.Fatal("invalid catalog accepted")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("created database on error: %v", err)
			}
		})
	}
	t.Run("transaction-rollback", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "vup.db")
		db, err := initializeVup(path)
		if err != nil {
			t.Fatal(err)
		}
		g := (*gorm.DB)(db)
		if err := g.Create(&vup{Mid: 1, Uname: "keep", Roomid: 1}).Error; err != nil {
			t.Fatal(err)
		}
		if err := g.Exec("CREATE TRIGGER reject_two BEFORE INSERT ON vup WHEN NEW.mid=2 BEGIN SELECT RAISE(ABORT, 'fixture'); END").Error; err != nil {
			t.Fatal(err)
		}
		g.Close()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`[{"mid":1,"uname":"changed","roomid":9},{"mid":2,"uname":"rejected","roomid":2}]`))
		}))
		defer srv.Close()
		if err := updateVupFrom(context.Background(), srv.Client(), path, []string{srv.URL}); err == nil {
			t.Fatal("expected database rejection")
		}
		db, err = initializeVup(path)
		if err != nil {
			t.Fatal(err)
		}
		defer (*gorm.DB)(db).Close()
		rows, err := db.filterVup([]int64{1, 2})
		if err != nil || len(rows) != 1 || rows[0].Uname != "keep" || rows[0].Roomid != 1 {
			t.Fatalf("partial update: %v %v", rows, err)
		}
	})
}
