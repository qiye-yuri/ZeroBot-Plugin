package bilibiliparse

import (
	"regexp"
	"testing"
)

// Keep link parsing coverage independent of the removed video download feature.
func TestLinkPatterns(t *testing.T) {
	for _, tc := range []struct {
		name, input, pattern string
	}{
		{"dynamic", "https://t.bilibili.com/12345", searchDynamic},
		{"mobile-dynamic", "https://m.bilibili.com/dynamic/12345", searchDynamic},
		{"article", "https://www.bilibili.com/read/cv12345", searchArticle},
		{"mobile-article", "https://www.bilibili.com/read/mobile/12345", searchArticle},
		{"live", "https://live.bilibili.com/12345", searchLiveRoom},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !regexp.MustCompile(tc.pattern).MatchString(tc.input) {
				t.Fatal("valid link did not match", tc.input)
			}
		})
	}
}
