package antiabuse

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	sqlite "github.com/FloatTech/sqlite"
)

type antidb struct {
	sync.RWMutex
	sqlite.Sqlite
}

type banWord struct {
	Word string `db:"word"`
}

type banTime struct {
	ID   int64 `db:"id"`
	Time int64 `db:"time"`
}

var (
	nilban = &banWord{}
	nilbt  = &banTime{}
)

func newantidb(path string) (*antidb, error) {
	db := &antidb{Sqlite: sqlite.New(path)}
	err := db.Open(bandur)
	if err != nil {
		return nil, err
	}
	_ = db.FindFor("__bantime__", nilbt, "", func() error {
		t := time.Unix(nilbt.Time, 0)
		ttl := time.Until(t.Add(bandur))
		if ttl <= 0 {
			_ = managers.DoUnblock(nilbt.ID)
			return nil
		}
		cache.Set(nilbt.ID, struct{}{})
		cache.Touch(nilbt.ID, -time.Since(t))
		return nil
	})
	_ = db.Del("__bantime__", "WHERE time <= ?", strconv.FormatInt(time.Now().Add(-bandur).Unix(), 10))
	return db, nil
}

func (db *antidb) isInAntiList(gid int64, msg string) bool {
	grp := strconv.FormatInt(gid, 36)
	db.RLock()
	defer db.RUnlock()
	return db.CanFind(grp, "WHERE word <> '' AND instr(?, word)>0", msg)
}

func (db *antidb) insertWord(gid int64, word string) error {
	word = strings.TrimSpace(word)
	if word == "" {
		return errors.New("违禁词不能为空")
	}
	grp := strconv.FormatInt(gid, 36)
	db.Lock()
	defer db.Unlock()
	err := db.Create(grp, nilban)
	if err != nil {
		return err
	}
	if db.CanFind(grp, "WHERE word = ?", word) {
		return nil
	}
	return db.Insert(grp, &banWord{Word: word})
}

func (db *antidb) deleteWord(gid int64, word string) error {
	word = strings.TrimSpace(word)
	if word == "" {
		return errors.New("违禁词不能为空")
	}
	grp := strconv.FormatInt(gid, 36)
	db.Lock()
	defer db.Unlock()
	if n, _ := db.Count(grp); n == 0 {
		return errors.New("本群还没有违禁词~")
	}
	return db.Del(grp, "WHERE word = ?", word)
}

func (db *antidb) listWords(gid int64) string {
	grp := strconv.FormatInt(gid, 36)
	word := &banWord{}
	sb := strings.Builder{}
	sb.WriteByte('[')
	i := 0
	db.RLock()
	defer db.RUnlock()
	_ = db.FindFor(grp, word, "WHERE word <> ''", func() error {
		if i > 0 {
			sb.WriteString(" | ")
		}
		sb.WriteString(word.Word)
		i++
		return nil
	})
	if i == 0 {
		return "[]"
	}
	sb.WriteByte(']')
	return sb.String()
}
