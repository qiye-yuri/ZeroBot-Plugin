package query

import (
	"errors"
	bz "github.com/FloatTech/AnimeAPI/bilibili"
	"strings"
	"testing"
)

func TestCompositionMissingProfile(t *testing.T) {
	got := strings.Join(compositionStats(bz.MemberCard{}, 0, true), " ")
	for _, want := range []string{"仅展示粉丝牌", "比例：未知", "注册日期：未知"} {
		if !strings.Contains(got, want) {
			t.Fatal(got)
		}
	}
	for _, bad := range []string{"NaN", "1970", "粉丝：0", "0.00%"} {
		if strings.Contains(got, bad) {
			t.Fatal(got)
		}
	}
}
func TestCompositionEmptyAttention(t *testing.T) {
	got := strings.Join(compositionStats(bz.MemberCard{}, 0, false), " ")
	if !strings.Contains(got, "未取得关注列表") || strings.Contains(got, "NaN") {
		t.Fatal(got)
	}
}

func TestCompositionIndependentSources(t *testing.T) {
	for _, profileFails := range []bool{false, true} {
		for _, medalFails := range []bool{false, true} {
			calls := 0
			_, m, pe, me := compositionSources("123", func(string) (bz.MemberCard, error) {
				if profileFails {
					return bz.MemberCard{}, errors.New("timeout")
				}
				return bz.MemberCard{Mid: "123"}, nil
			}, func(string) ([]bz.Medal, error) {
				calls++
				if medalFails {
					return nil, errors.New("timeout")
				}
				return []bz.Medal{{}}, nil
			})
			if calls != 1 || (pe != nil) != profileFails || (me != nil) != medalFails || (!medalFails && len(m) != 1) {
				t.Fatalf("independent source failure: %v %v", pe, me)
			}
		}
	}
}
