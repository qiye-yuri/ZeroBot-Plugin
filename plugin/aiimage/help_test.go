package aiimage

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestImageHelpRoutesBeforeGeneration(t *testing.T) {
	for _, query := range []string{"", "帮助", "帮助 尺寸", "帮助尺寸", "--help", "help", "--help 质量", "参数", "帮助 未知主题", "帮助 --size=4k"} {
		pages, ok := imageHelp(query)
		if !ok || len(pages) == 0 || !strings.Contains(strings.Join(pages, "\n"), "AI画图") {
			t.Fatal(query, pages, ok)
		}
	}
	for _, query := range []string{"海边日落", "--size=4k 海边", "帮我画一只猫"} {
		if _, ok := imageHelp(query); ok {
			t.Fatal("prompt mistaken for help", query)
		}
	}
	pages, ok := imageHelp("帮助 全部")
	if !ok || len(pages) != 10 {
		t.Fatal(len(pages), ok)
	}
	for _, page := range pages {
		if utf8.RuneCountInString(page) > 1200 {
			t.Fatal("help page too long")
		}
	}
}
