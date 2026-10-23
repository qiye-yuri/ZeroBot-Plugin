package query

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/jinzhu/gorm"
)

var (
	vtbURLs = [...]string{"https://api.vtbs.moe/v1/short", "https://api.tokyo.vtbs.moe/v1/short", "https://vtbs.musedash.moe/v1/short"}
	vdb     *vupdb
)

// vupdb 分数数据库
type vupdb gorm.DB

type vup struct {
	Mid    int64  `gorm:"column:mid;primary_key"`
	Uname  string `gorm:"column:uname"`
	Roomid int64  `gorm:"column:roomid"`
}

func (vup) TableName() string {
	return "vup"
}

// initializeVup 初始化vup数据库
func initializeVup(dbpath string) (*vupdb, error) {
	if _, err := os.Stat(dbpath); err != nil || os.IsNotExist(err) {
		// 生成文件
		f, err := os.Create(dbpath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
	}
	gdb, err := gorm.Open("sqlite3", dbpath)
	if err != nil {
		return nil, err
	}
	gdb.AutoMigrate(&vup{})
	return (*vupdb)(gdb), nil
}

func (vdb *vupdb) insertVupByMid(mid int64, uname string, roomid int64) (err error) {
	db := (*gorm.DB)(vdb)
	v := vup{
		Mid:    mid,
		Uname:  uname,
		Roomid: roomid,
	}
	if err = db.Model(&vup{}).First(&v, "mid = ? ", mid).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			err = db.Model(&vup{}).Create(&v).Error
		}
	}
	return
}

// filterVup 筛选vup
func (vdb *vupdb) filterVup(ids []int64) (vups []vup, err error) {
	db := (*gorm.DB)(vdb)
	if err = db.Model(&vup{}).Find(&vups, "mid in (?)", ids).Error; err != nil {
		return vups, err
	}
	return
}

// A catalog update opens its own database; it does not depend on a previous
// 查成分 command having initialized the query connection.
func updateVup(dbpath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	client := newDanmakuClient()
	defer client.CloseIdleConnections()
	return updateVupFrom(ctx, client, dbpath, vtbURLs[:])
}

func updateVupFrom(ctx context.Context, client *http.Client, dbpath string, urls []string) error {
	var catalog []vup
	for _, url := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0")
		res, err := client.Do(req)
		if err != nil {
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(res.Body, (8<<20)+1))
		res.Body.Close()
		if readErr != nil || res.StatusCode != http.StatusOK || len(data) > 8<<20 {
			continue
		}
		var rows []vup
		if json.Unmarshal(data, &rows) != nil || len(rows) == 0 {
			continue
		}
		valid := true
		for _, row := range rows {
			if row.Mid <= 0 || row.Uname == "" {
				valid = false
				break
			}
		}
		if valid {
			catalog = rows
			break
		}
	}
	if len(catalog) == 0 {
		return fmt.Errorf("VUP资料源暂不可用或格式无效，原资料库未修改")
	}
	db, err := initializeVup(dbpath)
	if err != nil {
		return err
	}
	gdb := (*gorm.DB)(db)
	defer gdb.Close()
	tx := gdb.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()
	for _, row := range catalog {
		current := vup{Mid: row.Mid}
		if err := tx.Where("mid = ?", row.Mid).Assign(map[string]any{"uname": row.Uname, "roomid": row.Roomid}).FirstOrCreate(&current).Error; err != nil {
			return err
		}
	}
	return tx.Commit().Error
}
