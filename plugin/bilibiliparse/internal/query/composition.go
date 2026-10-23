package query

import (
	"fmt"
	bz "github.com/FloatTech/AnimeAPI/bilibili"
	"time"
)

func compositionStats(u bz.MemberCard, count int, partial bool) []string {
	if partial {
		return []string{"用户资料暂不可用，仅展示粉丝牌", "粉丝/关注/成分比例：未知", "注册日期：未知"}
	}
	ratio := "成分比例：未知（未取得关注列表）"
	if len(u.Attentions) > 0 {
		ratio = fmt.Sprintf("管人痴成分：%.2f%%（%d/%d）", float64(count)/float64(len(u.Attentions))*100, count, len(u.Attentions))
	}
	registered := "注册日期：未知"
	if u.Regtime > 0 {
		registered = "注册日期：" + time.Unix(u.Regtime, 0).Format("2006-01-02 15:04:05")
	}
	return []string{fmt.Sprintf("粉丝：%d  可读取关注：%d", u.Fans, len(u.Attentions)), ratio, registered}
}

// Always try the independent medal source even when the profile source fails.
func compositionSources(id string, profile func(string) (bz.MemberCard, error), medals func(string) ([]bz.Medal, error)) (bz.MemberCard, []bz.Medal, error, error) {
	u, profileErr := profile(id)
	m, medalErr := medals(id)
	return u, m, profileErr, medalErr
}
